package cli

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/wooagent-os/wooagent-os/daemon/internal/abilities"
	"github.com/wooagent-os/wooagent-os/daemon/internal/ask"
	"github.com/wooagent-os/wooagent-os/daemon/internal/ask/agents"
	asktools "github.com/wooagent-os/wooagent-os/daemon/internal/ask/tools"
	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/config"
	"github.com/wooagent-os/wooagent-os/daemon/internal/httpapi"
	"github.com/wooagent-os/wooagent-os/daemon/internal/llm/anthropic"
	"github.com/wooagent-os/wooagent-os/daemon/internal/manifest"
	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/personas"
	"github.com/wooagent-os/wooagent-os/daemon/internal/registry"
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/wooagent-os/wooagent-os/daemon/internal/sweeper"
	"github.com/wooagent-os/wooagent-os/daemon/internal/telemetry"

	// Side-effect imports register the persona implementations. Adding a
	// new persona is a one-line change here plus a new package under
	// internal/personas/<slug>/.
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/marketing"
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/pricing"
	_ "github.com/wooagent-os/wooagent-os/daemon/internal/personas/reporting"
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

			// Refuse to start if another process is already bound to this
			// address. Done BEFORE store.Open / EnsureUISession / any other
			// side-effect: F3's EnsureUISession is idempotent, but a doomed
			// startup still does work that's pointless on a port collision.
			// We could also detect "is the process specifically wooagent?"
			// but pointing the operator at lsof/pgrep is portable and the
			// information they need either way.
			if err := probeBindAddr(cfg.BindAddr); err != nil {
				return err
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
			thresholds := pep.DefaultThresholds()
			overlayPath := filepath.Join(paths.Root, "budgets.json")
			if loaded, err := pep.LoadBudgetOverlay(overlayPath, thresholds, slog.Default()); err != nil {
				slog.Default().Warn("budget overlay failed to load; using defaults", "path", overlayPath, "err", err)
			} else {
				thresholds = loaded
			}
			budgetGate := pep.NewBudgetGate(st.DB, thresholds)
			var pepInstance *pep.PEP
			if mcpClient != nil {
				pepInstance = pep.New(lookup, mcpClient, st.DB, budgetGate)
			} else {
				pepInstance = pep.New(lookup, nil, st.DB, budgetGate)
			}

			srv := httpapi.New(st, am, pepInstance, secretStore, uiSessionToken)

			// Sweep paired stores once at startup, then keep them fresh on a
			// periodic ticker. Both fire-and-forget — the HTTP server starts
			// immediately so the UI can connect before the first sweep
			// finishes (a slow store shouldn't bench the daemon).
			go srv.Abilities().RunAll(ctx)
			srv.Abilities().SchedulePeriodic(ctx)

			// 30-day TTL purge for dismissed issues (DSGWOO-1277). Runs
			// once at startup then every 24h until shutdown. Override the
			// window via WOOAGENT_DISMISS_TTL_DAYS (e.g., 0 for immediate
			// during demos).
			ttlDays := dismissTTLDays(cmd.OutOrStdout())
			sw := &sweeper.Sweeper{
				DB:    st.DB,
				TTL:   time.Duration(ttlDays) * 24 * time.Hour,
				Every: 24 * time.Hour,
				Out:   cmd.OutOrStdout(),
			}
			go sw.Start(ctx)
			fmt.Fprintf(cmd.OutOrStdout(), "→ sweeper: dismiss-TTL active (%d days; override via WOOAGENT_DISMISS_TTL_DAYS)\n", ttlDays)

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
						Store:     st,
						MCP:       mcpClient,
						Skills:    skills,
						Env:       env,
						Recorder:  telemetry.NewSQLiteRecorder(st.DB, budgetGate),
						Abilities: abilities.NewChecker(st.DB, lookup),
					},
					Out:    out,
					Budget: budgetGate,
				}
				if err := sch.Start(ctx); err != nil {
					return fmt.Errorf("start scheduler: %w", err)
				}
				srv.SetScheduler(sch)

				// Ask Agent drawer (DSGWOO-1348). Wires the /v1/ask
				// handler if the Anthropic API key is configured;
				// otherwise the route returns 503 and the chat is
				// disabled (the cadence-mode personas keep working
				// against their own API-key access path).
				if env.AnthropicAPIKey != "" {
					srv.SetAsk(buildAskConfig(env, st, sch, mcpClient))
				} else {
					fmt.Fprintln(out, "→ ask: ANTHROPIC_API_KEY not set; /v1/ask disabled")
				}
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

// probeBindAddr attempts to bind addr (closing the listener immediately)
// so a port collision surfaces before `wooagent run` does any setup
// side-effects (DB writes, token mints). Returns an error suitable for
// surfacing directly to the operator — names the port and points at
// concrete commands for locating the conflicting process.
func probeBindAddr(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf(
			"cannot bind %s: %w — is another daemon already running? "+
				"check with: lsof -i :%s  or  pgrep -fl wooagent",
			addr, err, portFromAddr(addr),
		)
	}
	_ = ln.Close()
	return nil
}

// dismissTTLDays reads the override env var and returns a positive
// integer day count. Falls back to the package default and logs a
// warning when the var is set but doesn't parse. Zero is allowed (the
// demo / dogfood escape hatch).
func dismissTTLDays(out io.Writer) int {
	const defaultDays = 30
	raw := os.Getenv("WOOAGENT_DISMISS_TTL_DAYS")
	if raw == "" {
		return defaultDays
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		fmt.Fprintf(out, "→ sweeper: ignoring invalid WOOAGENT_DISMISS_TTL_DAYS=%q; using default %d\n", raw, defaultDays)
		return defaultDays
	}
	return n
}

// portFromAddr extracts the port suffix from a host:port string, falling
// back to the full string if parsing fails. Used by probeBindAddr to
// build the lsof hint.
func portFromAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return port
}

// buildAskConfig wires the Ask Agent drawer dependencies (DSGWOO-1347).
// Called from the `run` command after the scheduler has started, when
// the Anthropic API key is set in env. Registers all four live
// chat-mode agents — Chief of Staff (CoS) plus the three specialists
// (Marketing, Pricing, Sales Support). MCP-backed tools (list_products,
// get_product, list_orders, get_order) skip registration when mcpClient
// is nil; the agents still load but those reads will return "MCP not
// configured" if invoked. The picker's disabled Inventory / Accounting /
// Reporting entries have no daemon-side equivalent — they're handled
// entirely client-side via the picker's disabled state.
func buildAskConfig(env personas.Env, st *store.Store, sch *scheduler.Scheduler, mcpClient *mcp.Client) httpapi.AskConfig {
	client := anthropic.New(env.AnthropicAPIKey, askModel(env))

	// Shared read tools — same handlers across every agent. Each agent's
	// prompt narrows scope ("your past work") so the doc-strings stay
	// agent-agnostic and the model only sees the slice it should care
	// about. Cheaper than four near-duplicate handler types per persona.
	sharedReads := []anthropic.ToolHandler{
		&asktools.ListProposalsTool{DB: st.DB},
		&asktools.GetProposalTool{DB: st.DB},
		&asktools.ListRunsTool{DB: st.DB},
		&asktools.GetRunTool{DB: st.DB},
		&asktools.ListAgentsTool{DB: st.DB},
	}

	cosTools := append([]anthropic.ToolHandler{}, sharedReads...)
	cosTools = append(cosTools, &asktools.DispatchTool{
		Enqueue: sch.EnqueueOperatorAsked,
		Limiter: asktools.DefaultRateLimiter(),
	})

	marketingTools := append([]anthropic.ToolHandler{}, sharedReads...)
	marketingTools = append(marketingTools,
		&asktools.ListProductsTool{MCP: mcpClient},
		&asktools.GetProductTool{MCP: mcpClient},
		&asktools.ProduceDescriptionRewriteTool{DB: st.DB},
		&asktools.ProduceSocialPostTool{DB: st.DB},
		&asktools.ProduceLaunchCopyTool{DB: st.DB},
	)

	pricingTools := append([]anthropic.ToolHandler{}, sharedReads...)
	pricingTools = append(pricingTools,
		&asktools.ListProductsTool{MCP: mcpClient},
		&asktools.GetProductTool{MCP: mcpClient},
		&asktools.ProduceRecommendationTool{DB: st.DB},
	)

	salesSupportTools := append([]anthropic.ToolHandler{}, sharedReads...)
	salesSupportTools = append(salesSupportTools,
		&asktools.ListOrdersTool{MCP: mcpClient},
		&asktools.GetOrderTool{MCP: mcpClient},
		&asktools.ProduceReplyDraftTool{DB: st.DB},
	)

	return httpapi.AskConfig{
		Client:  client,
		Threads: ask.NewThreadStore(0), // default cap (50)
		Events:  ask.NewBroker(),
		Agents: map[ask.AgentSlug]httpapi.AskAgent{
			ask.AgentChiefOfStaff: {
				Prompt: agents.CoSPrompt,
				Tools:  cosTools,
			},
			ask.AgentMarketing: {
				Prompt: agents.MarketingPrompt,
				Tools:  marketingTools,
			},
			ask.AgentPricing: {
				Prompt: agents.PricingPrompt,
				Tools:  pricingTools,
				ServerTools: []anthropic.ToolDef{
					asktools.WebSearchToolDef,
				},
			},
			ask.AgentSalesSupport: {
				Prompt: agents.SalesSupportPrompt,
				Tools:  salesSupportTools,
			},
		},
		GetStoreName: func(ctx context.Context) string {
			return lookupPairedStoreName(ctx, st)
		},
	}
}

// askModel picks the model the /v1/ask handler will use. Defaults to
// Sonnet 4.6 (the model the CoS prompt was eval'd against). Operators
// can override via ANTHROPIC_MODEL the same way the cadence personas
// already do.
func askModel(env personas.Env) string {
	if env.AnthropicModel != "" {
		return env.AnthropicModel
	}
	return "claude-sonnet-4-6"
}

// lookupPairedStoreName returns a short display name for the currently
// paired store, used to template the {{store_name}} token in the CoS
// system prompt. The stores table doesn't store a human name today
// (DSGWOO-1348 scout finding) — derive it from the store URL host.
// Returns empty string when no paired store is configured; the prompt
// substitutes "the store" as a fallback.
func lookupPairedStoreName(ctx context.Context, st *store.Store) string {
	if st == nil || st.DB == nil {
		return ""
	}
	var rawURL string
	err := st.DB.QueryRowContext(ctx,
		`SELECT url FROM stores WHERE status = 'paired' ORDER BY paired_at DESC LIMIT 1`,
	).Scan(&rawURL)
	if err != nil || rawURL == "" {
		return ""
	}
	// Strip scheme + path; keep host. "https://store.example.com/wp-json" → "store.example.com".
	s := strings.TrimPrefix(strings.TrimPrefix(rawURL, "https://"), "http://")
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return s
}
