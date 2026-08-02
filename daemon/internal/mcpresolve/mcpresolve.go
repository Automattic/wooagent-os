package mcpresolve

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

// reconcileEvery is how often the daemon re-checks which store it should
// be talking to. A minute is well inside the time it takes an operator to
// finish pairing and click Run, and the check is one indexed SELECT plus a
// keychain read when nothing has changed.
const reconcileEvery = time.Minute

// Source records where an MCP connection came from, for logging.
type Source string

const (
	SourcePairedStore Source = "paired store"
	SourceEnv         Source = "environment"
)

// Target is a resolved MCP connection plus the provenance needed to
// explain it to an operator.
type Target struct {
	Config mcp.Config
	Source Source
	// Label is the human-facing store identity — the host, or the endpoint
	// when no store URL is known.
	Label string
}

// Mismatch describes an environment-vs-paired-store disagreement.
//
// This exists as a value rather than only a log line because a log line
// is invisible to anyone running the desktop app or the embedded UI. The
// operator most likely to be in this state — env vars exported months
// ago, then re-paired through the UI — is exactly the operator who never
// looks at daemon stdout. Serving it over HTTP lets the UI say the same
// thing the terminal says.
type Mismatch struct {
	// EnvHost is the host WOOAGENT_MCP_URL names, and is being ignored.
	EnvHost string `json:"env_host"`
	// PairedHost is the host actually in use.
	PairedHost string `json:"paired_host"`
}

// Resolve decides which store the daemon's MCP client should talk
// to (DSGWOO-1470), reporting any divergence to out.
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
// Returns ok=false when neither source is configured; the caller treats
// that as "MCP not wired", which is a supported v0.1 state.
func Resolve(ctx context.Context, db *sql.DB, sec secrets.Store, out io.Writer) (Target, bool) {
	target, mismatch, ok := Describe(ctx, db, sec, out)
	if mismatch != nil {
		fmt.Fprintf(out,
			"→ mcp: WOOAGENT_MCP_URL (%s) does not match the paired store (%s); using the paired store. Unset the env vars to silence this.\n",
			mismatch.EnvHost, mismatch.PairedHost)
	}
	return target, ok
}

// Describe resolves the target and reports any env/paired divergence
// without writing the mismatch line itself, so a caller that isn't a
// terminal (the HTTP API) can present it its own way.
//
// out still receives the paired-store diagnostics from pairedTarget —
// unreadable token, missing endpoint — since those explain a fallback the
// caller didn't ask for. Pass io.Discard to silence them.
func Describe(ctx context.Context, db *sql.DB, sec secrets.Store, out io.Writer) (Target, *Mismatch, bool) {
	paired, hasPaired := pairedTarget(ctx, db, sec, out)
	env, hasEnv := envTarget()

	switch {
	case hasPaired && hasEnv:
		if !sameHost(paired.Config.Endpoint, env.Config.Endpoint) {
			return paired, &Mismatch{
				EnvHost:    HostOf(env.Config.Endpoint),
				PairedHost: paired.Label,
			}, true
		}
		return paired, nil, true
	case hasPaired:
		return paired, nil, true
	case hasEnv:
		return env, nil, true
	default:
		return Target{}, nil, false
	}
}

// OpenClient resolves the store the daemon would talk to and returns a
// client for it. For the debug one-shots in cmd/ — persona-marketing,
// persona-pricing and friends.
//
// Those binaries used to build a client straight from WOOAGENT_MCP_URL,
// which made them the sharpest edge in the repo: they write proposals
// into the same SQLite the daemon serves, so running one with a stale env
// var produced proposals describing products from a store the daemon
// isn't paired to. Approving those then applies one store's copy or
// prices to a different store. Sharing Resolve with the daemon means a
// tool can't target a store the daemon wouldn't.
//
// The secret store is opened here rather than passed in so callers don't
// each repeat the keychain dance. If it can't be opened — no keychain in
// this environment — resolution degrades to env vars rather than failing,
// which keeps these tools usable in CI.
//
// Returns ok=false when neither a paired store nor a complete set of env
// vars is available; the caller should exit with its own message, since
// what to do about it differs per tool.
func OpenClient(ctx context.Context, db *sql.DB, out io.Writer) (*mcp.Client, Target, bool) {
	// A keychain failure is expected in some environments and isn't worth
	// a warning — Resolve falls through to env vars, and the caller
	// reports which store it landed on either way.
	sec, err := secrets.Open()
	if err != nil {
		sec = nil
	}
	target, ok := Resolve(ctx, db, sec, out)
	if !ok {
		return nil, Target{}, false
	}
	return mcp.NewClient(target.Config), target, true
}

// pairedTarget builds a target from the most recently paired store.
// Returns false when there is no paired store, or when its token can't be
// read — a paired row whose keychain entry is missing can't authenticate,
// so falling through to env is better than returning a client that 401s.
func pairedTarget(ctx context.Context, db *sql.DB, sec secrets.Store, out io.Writer) (Target, bool) {
	if db == nil || sec == nil {
		return Target{}, false
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
		return Target{}, false
	}
	if endpoint == "" || tokenRef == "" {
		fmt.Fprintf(out, "→ mcp: paired store %s has no endpoint or token; falling back to environment\n", HostOf(storeURL))
		return Target{}, false
	}
	token, err := sec.Get(ctx, tokenRef)
	if err != nil || token == "" {
		fmt.Fprintf(out, "→ mcp: paired store %s token unreadable (%v); falling back to environment\n", HostOf(storeURL), err)
		return Target{}, false
	}
	return Target{
		Config: mcp.Config{Endpoint: endpoint, BearerToken: token},
		Source: SourcePairedStore,
		Label:  HostOf(storeURL),
	}, true
}

// envTarget builds a target from WOOAGENT_MCP_URL/_USER/_APP_PASSWORD. All
// three must be set.
func envTarget() (Target, bool) {
	endpoint := os.Getenv("WOOAGENT_MCP_URL")
	user := os.Getenv("WOOAGENT_MCP_USER")
	pass := os.Getenv("WOOAGENT_MCP_APP_PASSWORD")
	if endpoint == "" || user == "" || pass == "" {
		return Target{}, false
	}
	return Target{
		Config: mcp.Config{
			Endpoint: endpoint,
			Username: user,
			// WP shows app passwords with spaces for readability; strip them
			// so pasted values from the admin UI work without preprocessing.
			Password: strings.ReplaceAll(pass, " ", ""),
		},
		Source: SourceEnv,
		Label:  HostOf(endpoint),
	}, true
}

// StartReconciler keeps the shared client pointed at the current paired
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
func StartReconciler(ctx context.Context, c *mcp.Client, db *sql.DB, sec secrets.Store, out io.Writer) {
	if c == nil {
		return
	}
	t := time.NewTicker(reconcileEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			ReconcileOnce(ctx, c, db, sec, out)
		}
	}
}

// ReconcileOnce re-resolves the target and retargets the client if the
// store changed. Split out from the loop so it's directly testable.
func ReconcileOnce(ctx context.Context, c *mcp.Client, db *sql.DB, sec secrets.Store, out io.Writer) {
	target, ok := Resolve(ctx, db, sec, io.Discard)
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

// HostOf reduces a URL to its host for display. Falls back to the raw
// string when it doesn't parse, so logging never hides the real value.
func HostOf(raw string) string {
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
	ha, hb := HostOf(a), HostOf(b)
	return ha != "" && ha == hb
}
