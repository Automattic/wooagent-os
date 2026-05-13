package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/httpapi"
	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/registry"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"

	// Side-effect imports register the persona implementations. Adding a
	// new persona is a one-line change here plus a new package under
	// internal/personas/<slug>/.
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/marketing"
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/pricing"
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/sales-support"
)

func newRunCmd() *cobra.Command {
	var bind string
	var skipPersonas bool
	c := &cobra.Command{
		Use:   "run",
		Short: "Start the WooAgent OS daemon (headless REST API)",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			ctx, cancel := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			paths, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			cfg, err := config.LoadOrDefault(paths.ConfigFile)
			if err != nil {
				return err
			}
			if bind != "" {
				cfg.BindAddr = bind
			}

			st, err := store.Open(ctx, paths.DBFile)
			if err != nil {
				return fmt.Errorf("open store (did you run `wooagent init`?): %w", err)
			}
			defer st.Close()

			am := auth.New(st.DB)
			has, err := am.AnyTokenExists(ctx)
			if err != nil {
				return err
			}

			// The keychain is the durable home for device tokens (paired
			// stores) and provider API keys. v0.1 refuses to start without
			// it — see internal/secrets/secrets.go for the probe rationale.
			secretStore, err := secrets.Open()
			if err != nil {
				return fmt.Errorf("open secret store: %w", err)
			}
			if !has {
				return fmt.Errorf("no auth tokens present — run `wooagent init` or `wooagent auth token create` first")
			}

			// Per-run UI session token. Templated into index.html by the
			// uiassets handler so the embedded UI auto-connects without
			// the operator pasting a bearer token. Operator-issued
			// long-lived tokens (from `wooagent init` / `wooagent auth
			// token create`) are unaffected — they still validate via
			// the same Manager.Validate path.
			uiSessionToken, err := am.MintUISession(ctx)
			if err != nil {
				return fmt.Errorf("mint UI session token: %w", err)
			}

			// MCP wiring is optional. The approve endpoint requires it; everything
			// else (kanban, issue detail, list/create) runs without.
			//
			// TODO(plan #4 — Companion Plugin): cut MCP over to paired stores.
			// Once the device-pair handshake lands and `stores` rows can reach
			// status='paired', resolve mcp.Client config from the row +
			// keychain (token_ref) here, with env-var fallback. The auth
			// shape (bearer vs Basic vs WP App Password issued at pair time)
			// is decided by the plugin work, so we stay env-var-only here
			// until then to avoid locking in an assumption.
			mcpClient := loadMCPClient(cmd.OutOrStdout())

			// Build the PEP. The manifest is the trust allowlist; the PEP wraps
			// the MCP client so callers can never reach MCP directly. When the
			// MCP client is nil (UI-only daemon run), we still build the PEP so
			// audit rows are written for denials and so the no-shortcut rule
			// doesn't quietly turn off.
			defaultManifest, err := manifest.Default()
			if err != nil {
				return fmt.Errorf("load default manifest: %w", err)
			}
			lookup, err := manifest.NewLookup(defaultManifest)
			if err != nil {
				return fmt.Errorf("index manifest: %w", err)
			}
			var pepInstance *pep.PEP
			if mcpClient != nil {
				pepInstance = pep.New(lookup, mcpClient, st.DB)
			} else {
				pepInstance = pep.New(lookup, nil, st.DB)
			}

			srv := httpapi.New(st, am, pepInstance, secretStore, uiSessionToken)

			// Sweep paired stores once at startup, then keep them fresh on a
			// periodic ticker. Both fire-and-forget — the HTTP server starts
			// immediately so the UI can connect before the first sweep
			// finishes (a slow store shouldn't bench the daemon).
			go srv.Abilities().RunAll(ctx)
			srv.Abilities().SchedulePeriodic(ctx)

			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "→ Daemon running on http://%s (headless)\n", cfg.BindAddr)
			if mcpClient != nil {
				fmt.Fprintln(out, "→ MCP store wired — Approve will write through to the connected store.")
			} else {
				fmt.Fprintln(out, "→ MCP not configured — Approve will return 503 (set WOOAGENT_MCP_URL/USER/APP_PASSWORD to enable).")
			}
			fmt.Fprintln(out, "→ Point a UI at this daemon:")
			fmt.Fprintln(out, "    • hosted:  https://ui.wooagent.dev (when available)")
			fmt.Fprintln(out, "    • local:   run `wooagent ui` in another terminal")
			fmt.Fprintln(out, "→ Press Ctrl+C to stop.")

			// Spawn persona seed runs in a background goroutine. Each persona
			// checks agents.enabled and an "already-has-open-issues" guard, so
			// restarts of the daemon don't duplicate-seed the kanban. Failures
			// in one persona never block another or the HTTP server. See
			// internal/personas/personas.go for the contract.
			if !skipPersonas {
				go runPersonas(ctx, st, secretStore, mcpClient, out)
			}

			return httpapi.Run(ctx, cfg.BindAddr, srv.Handler())
		},
	}
	c.Flags().StringVar(&bind, "bind", "", "address to bind (default: value of config.bind_addr or localhost:7777)")
	c.Flags().BoolVar(&skipPersonas, "skip-personas", false, "skip the on-boot persona seed run (useful for HTTP-only debugging)")
	return c
}

// loadMCPClient assembles an MCP client from env vars if all three are set;
// otherwise returns nil and writes a hint to out. The caller decides whether
// nil is fatal (it isn't, for v0.1).
func loadMCPClient(out interface{ Write([]byte) (int, error) }) *mcp.Client {
	url := os.Getenv("WOOAGENT_MCP_URL")
	user := os.Getenv("WOOAGENT_MCP_USER")
	pass := os.Getenv("WOOAGENT_MCP_APP_PASSWORD")
	if url == "" || user == "" || pass == "" {
		return nil
	}
	return mcp.NewClient(mcp.Config{
		Endpoint: url,
		Username: user,
		// WP shows app passwords with spaces for readability; strip them so
		// pasted values from the admin UI work without preprocessing.
		Password: strings.ReplaceAll(pass, " ", ""),
	})
}

// runPersonas fans out registered personas in goroutines and logs their
// outcomes. Personas may be skipped (env not set, no work to do, etc.) or
// land an issue. Either way, the daemon keeps serving HTTP. Errors are
// logged but never propagated — a single broken persona must not bench the
// rest of the fleet.
func runPersonas(ctx context.Context, st *store.Store, sec secrets.Store, mcpClient *mcp.Client, out io.Writer) {
	skills, err := loadSkillsForPersonas(out)
	if err != nil {
		fmt.Fprintf(out, "→ persona init: skill registry unavailable: %v\n", err)
		// Continue anyway; personas that need a skill will skip themselves
		// with a clear reason. Personas that don't need one (Marketing) keep
		// working.
		skills = map[string]registry.Skill{}
	}

	env := resolvePersonaEnv(ctx, st.DB, sec, envFromOS(), out)

	deps := personas.Deps{
		Store:    st,
		MCP:      mcpClient,
		Skills:   skills,
		Env:      env,
		Recorder: telemetry.NewSQLiteRecorder(st.DB),
	}

	all := personas.All()
	if len(all) == 0 {
		fmt.Fprintln(out, "→ persona init: no personas registered")
		return
	}
	fmt.Fprintf(out, "→ persona init: running %d personas sequentially\n", len(all))

	// Serial, not parallel. The MCP client has a single Mcp-Session-Id
	// field; concurrent Initialize calls from goroutines race on that
	// field and the loser sees "invalid or expired session" on its next
	// CallTool. Sequential runs are also kinder to the WP store
	// (one inbound request at a time) and to LLM rate limits. Phase 2
	// personas run a few seconds each — the lost parallelism is cheap.
	for _, p := range all {
		// Honor cancellation between personas — Ctrl+C should exit
		// promptly without waiting for the rest of the queue.
		select {
		case <-ctx.Done():
			fmt.Fprintln(out, "→ persona init: context cancelled before completion")
			return
		default:
		}
		res, err := personas.RunAndPersist(ctx, p, deps)
		if err != nil {
			fmt.Fprintf(out, "  · %s: error · %v\n", p.Slug(), err)
			continue
		}
		if res.Skipped {
			fmt.Fprintf(out, "  · %s: skipped · %s\n", p.Slug(), res.SkipReason)
			continue
		}
		fmt.Fprintf(out, "  · %s: issue %s landed in_review\n", p.Slug(), res.IssueID)
	}
	fmt.Fprintln(out, "→ persona init: done")
}

// loadSkillsForPersonas resolves the skills directory relative to the
// daemon's working directory. Personas that need a specific skill look it
// up by name in this map; personas that don't need any aren't affected by
// a missing directory.
func loadSkillsForPersonas(out io.Writer) (map[string]registry.Skill, error) {
	// Prefer the override env (testing); fall back to the well-known repo
	// layout. Production deployments will likely set the env explicitly so
	// the daemon doesn't depend on cwd at all.
	dir := os.Getenv("WOOAGENT_SKILLS_DIR")
	if dir == "" {
		dir = "skills"
	}
	skills, err := registry.LoadSkills(dir)
	if err != nil {
		return nil, err
	}
	if len(skills) == 0 {
		fmt.Fprintf(out, "→ persona init: no skills found in %q (set WOOAGENT_SKILLS_DIR if running from outside daemon/)\n", dir)
	}
	return skills, nil
}
