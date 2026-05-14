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
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
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
	var skipScheduler bool
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

			// Persistent UI session token. Templated into index.html by
			// the uiassets handler so the embedded UI auto-connects
			// without the operator pasting a bearer token. Plaintext
			// lives in paths.UISessionFile (mode 0600) so restarts
			// reuse the same token and don't break already-open
			// browser sessions. Operator-issued long-lived tokens
			// (`wooagent init` / `auth token create`) are unaffected.
			uiSessionToken, err := am.EnsureUISession(ctx, paths.UISessionFile)
			if err != nil {
				return fmt.Errorf("ensure UI session token: %w", err)
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

			// Build and start the scheduler. Replaces the old boot-time runPersonas
			// fan-out — every persona attempt now lands as a row in runs, with a
			// cadence-driven tick loop, classified retries, and operator-visible
			// reasons.
			if !skipScheduler {
				skills, err := loadSkillsForPersonas(out)
				if err != nil {
					fmt.Fprintf(out, "→ scheduler: skill registry unavailable: %v\n", err)
					skills = map[string]registry.Skill{}
				}
				env := resolvePersonaEnv(ctx, st.DB, secretStore, envFromOS(), out)
				sch := &scheduler.Scheduler{
					Store:    st,
					Personas: personas.All(),
					Deps: personas.Deps{
						Store:    st,
						MCP:      mcpClient,
						Skills:   skills,
						Env:      env,
						Recorder: telemetry.NewSQLiteRecorder(st.DB),
					},
					Out: out,
				}
				if err := sch.Start(ctx); err != nil {
					return fmt.Errorf("start scheduler: %w", err)
				}
				srv.SetScheduler(sch)
			}

			return httpapi.Run(ctx, cfg.BindAddr, srv.Handler())
		},
	}
	c.Flags().StringVar(&bind, "bind", "", "address to bind (default: value of config.bind_addr or localhost:7777)")
	c.Flags().BoolVar(&skipScheduler, "skip-scheduler", false, "skip starting the scheduler (useful for HTTP-only debugging)")
	c.Flags().BoolVar(&skipScheduler, "skip-personas", false, "DEPRECATED: alias for --skip-scheduler")
	_ = c.Flags().MarkDeprecated("skip-personas", "use --skip-scheduler")
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

// loadSkillsForPersonas returns the embedded skill registry by default,
// or whatever WOOAGENT_SKILLS_DIR points at if the env var is set.
// Personas that need a specific skill look it up by name in this map;
// personas that don't need any aren't affected by a missing directory.
func loadSkillsForPersonas(out io.Writer) (map[string]registry.Skill, error) {
	skills, err := registry.Skills()
	if err != nil {
		return nil, err
	}
	source := "embedded"
	if dir := os.Getenv("WOOAGENT_SKILLS_DIR"); dir != "" {
		source = fmt.Sprintf("disk override: %q", dir)
	}
	if len(skills) == 0 {
		fmt.Fprintf(out, "→ persona init: no skills found (%s)\n", source)
	}
	return skills, nil
}
