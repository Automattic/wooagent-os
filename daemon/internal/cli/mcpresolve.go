package cli

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/wooagent-os/wooagent-os/daemon/internal/mcp"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
)

// mcpReconcileEvery is how often the daemon re-checks which store it should
// be talking to. A minute is well inside the time it takes an operator to
// finish pairing and click Run, and the check is one indexed SELECT plus a
// keychain read when nothing has changed.
const mcpReconcileEvery = time.Minute

// mcpSource records where an MCP connection came from, for logging.
type mcpSource string

const (
	sourcePairedStore mcpSource = "paired store"
	sourceEnv         mcpSource = "environment"
)

// mcpTarget is a resolved MCP connection plus the provenance needed to
// explain it to an operator.
type mcpTarget struct {
	Config mcp.Config
	Source mcpSource
	// Label is the human-facing store identity — the host, or the endpoint
	// when no store URL is known.
	Label string
}

// resolveMCPTarget decides which store the daemon's MCP client should talk
// to (DSGWOO-1470).
//
// Precedence: the paired store wins. Environment variables are the fallback
// for headless and CI runs where nothing has been paired through the UI.
//
// The paired store has to win rather than merely fill in, because the
// failure this fixes is exactly the other order: an operator re-pairs to a
// new store, the UI reports it paired and healthy, and every run keeps
// hitting whatever stale host WOOAGENT_MCP_URL still names. Env winning
// would leave that bug in place.
//
// When both are configured and disagree, that's reported to out — silent
// divergence between what the UI shows and what the daemon calls is the
// core defect here, so it should never be quiet again.
//
// Returns ok=false when neither source is configured; the caller treats
// that as "MCP not wired", which is a supported v0.1 state.
func resolveMCPTarget(ctx context.Context, db *sql.DB, sec secrets.Store, out io.Writer) (mcpTarget, bool) {
	paired, hasPaired := pairedStoreTarget(ctx, db, sec, out)
	env, hasEnv := envTarget()

	switch {
	case hasPaired && hasEnv:
		if !sameHost(paired.Config.Endpoint, env.Config.Endpoint) {
			fmt.Fprintf(out,
				"→ mcp: WOOAGENT_MCP_URL (%s) does not match the paired store (%s); using the paired store. Unset the env vars to silence this.\n",
				hostOf(env.Config.Endpoint), paired.Label)
		}
		return paired, true
	case hasPaired:
		return paired, true
	case hasEnv:
		return env, true
	default:
		return mcpTarget{}, false
	}
}

// pairedStoreTarget builds a target from the most recently paired store.
// Returns false when there is no paired store, or when its token can't be
// read — a paired row whose keychain entry is missing can't authenticate,
// so falling through to env is better than returning a client that 401s.
func pairedStoreTarget(ctx context.Context, db *sql.DB, sec secrets.Store, out io.Writer) (mcpTarget, bool) {
	if db == nil || sec == nil {
		return mcpTarget{}, false
	}
	var storeURL, endpoint, tokenRef string
	err := db.QueryRowContext(ctx, `
		SELECT url, COALESCE(mcp_endpoint, ''), COALESCE(token_ref, '')
		  FROM stores
		 WHERE status = 'paired'
		 ORDER BY paired_at DESC
		 LIMIT 1`).Scan(&storeURL, &endpoint, &tokenRef)
	if err != nil {
		// sql.ErrNoRows is the ordinary "nothing paired yet" case.
		if err != sql.ErrNoRows {
			fmt.Fprintf(out, "→ mcp: could not read paired store (%v); falling back to environment\n", err)
		}
		return mcpTarget{}, false
	}
	if endpoint == "" || tokenRef == "" {
		fmt.Fprintf(out, "→ mcp: paired store %s has no endpoint or token; falling back to environment\n", hostOf(storeURL))
		return mcpTarget{}, false
	}
	token, err := sec.Get(ctx, tokenRef)
	if err != nil || token == "" {
		fmt.Fprintf(out, "→ mcp: paired store %s token unreadable (%v); falling back to environment\n", hostOf(storeURL), err)
		return mcpTarget{}, false
	}
	return mcpTarget{
		Config: mcp.Config{Endpoint: endpoint, BearerToken: token},
		Source: sourcePairedStore,
		Label:  hostOf(storeURL),
	}, true
}

// envTarget builds a target from WOOAGENT_MCP_URL/_USER/_APP_PASSWORD. All
// three must be set.
func envTarget() (mcpTarget, bool) {
	endpoint := os.Getenv("WOOAGENT_MCP_URL")
	user := os.Getenv("WOOAGENT_MCP_USER")
	pass := os.Getenv("WOOAGENT_MCP_APP_PASSWORD")
	if endpoint == "" || user == "" || pass == "" {
		return mcpTarget{}, false
	}
	return mcpTarget{
		Config: mcp.Config{
			Endpoint: endpoint,
			Username: user,
			// WP shows app passwords with spaces for readability; strip them
			// so pasted values from the admin UI work without preprocessing.
			Password: strings.ReplaceAll(pass, " ", ""),
		},
		Source: sourceEnv,
		Label:  hostOf(endpoint),
	}, true
}

// startMCPReconciler keeps the shared client pointed at the current paired
// store for as long as the daemon runs.
//
// The daemon builds one *mcp.Client at startup and hands it to the PEP, the
// personas and the Ask Agent tools. Before this, re-pairing to a different
// store had no effect on any of them until the process was restarted —
// which is how an operator could see "paired, 22 abilities" in the UI while
// every run failed against a decommissioned host (DSGWOO-1470).
//
// Fire-and-forget, mirroring sweeper.Start: a transient DB or keychain
// error must not take the daemon down.
func startMCPReconciler(ctx context.Context, c *mcp.Client, db *sql.DB, sec secrets.Store, out io.Writer) {
	if c == nil {
		return
	}
	t := time.NewTicker(mcpReconcileEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			reconcileMCPOnce(ctx, c, db, sec, out)
		}
	}
}

// reconcileMCPOnce re-resolves the target and retargets the client if the
// store changed. Split out from the loop so it's directly testable.
func reconcileMCPOnce(ctx context.Context, c *mcp.Client, db *sql.DB, sec secrets.Store, out io.Writer) {
	target, ok := resolveMCPTarget(ctx, db, sec, io.Discard)
	if !ok {
		// Nothing configured any more (store unpaired, env cleared). Leave
		// the existing client alone rather than pointing it at nothing —
		// personas surface a clearer skip than a client with no endpoint.
		return
	}
	if c.Retarget(target.Config) {
		fmt.Fprintf(out, "→ mcp: connected store changed; now using %s (%s)\n", target.Label, target.Source)
	}
}

// hostOf reduces a URL to its host for display. Falls back to the raw
// string when it doesn't parse, so logging never hides the real value.
func hostOf(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}

// sameHost reports whether two URLs address the same host. Comparing hosts
// rather than full URLs avoids false alarms from the endpoint path, which
// legitimately differs between a store URL and its MCP endpoint.
func sameHost(a, b string) bool {
	ha, hb := hostOf(a), hostOf(b)
	return ha != "" && ha == hb
}
