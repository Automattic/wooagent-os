package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/wooagent-os/wooagent-os/daemon/internal/abilities"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pairing"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
)

// Store is the v0.1 store-connection wire shape. Field names + status enum
// match ui/src/api/client.ts (commit fb98b45 shipped the UI before the
// daemon side, so the UI is the contract).
//
// Optional fields are pointer-or-omitempty so the wire reads "the daemon
// hasn't filled this yet" rather than zero values:
//   - PairingCode/PairURL/ExpiresAt: present only while status='pairing'
//   - PairedAt: set when status flips to paired
//   - DeviceName/AbilityCount/LastDiscoveredAt: populated post-pairing
type Store struct {
	ID               string `json:"id"`
	URL              string `json:"url"`
	MCPEndpoint      string `json:"mcp_endpoint,omitempty"`
	DeviceName       string `json:"device_name,omitempty"`
	Status           string `json:"status"`
	PairingCode      string `json:"pairing_code,omitempty"`
	PairURL          string `json:"pair_url,omitempty"`
	ExpiresAt        string `json:"expires_at,omitempty"`
	PairedAt         string `json:"paired_at,omitempty"`
	AbilityCount     *int   `json:"ability_count,omitempty"`
	LastDiscoveredAt string `json:"last_discovered_at,omitempty"`
	// CapabilityGaps lists what the daemon needs that this store can't do —
	// an ability it doesn't register, or one whose input schema rejects a
	// parameter a persona sends. Almost always means the WooAgent Companion
	// Plugin is out of date. Empty/omitted when the store satisfies the
	// daemon. Advisory: a store with gaps still works for the personas whose
	// abilities are present (DSGWOO-1471).
	CapabilityGaps []abilities.Gap `json:"capability_gaps,omitempty"`
}

// composeDeviceName returns the human-readable device label sent to the
// Companion Plugin's pair/request endpoint and rendered in the wp-admin
// "Currently paired" list. Format: `username@hostname`. Distinguishes
// pairings from multiple OS user accounts on the same machine — bare
// hostname collapses them all to the same name (e.g. "Mac.lan"). Both
// failures fall back to a static label so pairing still works in
// sandboxed environments where os/user.Current() can return an error.
func composeDeviceName() string {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown-host"
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username + "@" + host
	}
	return host
}

// pairingTTL is the window during which an operator can confirm a pending
// pairing in wp-admin. Tuned for "type a code from the screen on a device
// you're already logged into" — long enough to walk to a different tab,
// short enough that an unattended code doesn't sit forever.
const pairingTTL = 10 * time.Minute

// pairingCodeAlphabet is Crockford-style base32 minus visually ambiguous
// glyphs (0/O, 1/I/L, U). The operator types this code in wp-admin; we
// optimize for low transcription error over alphabet size.
const pairingCodeAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"

// verifyThrottle bounds the staleness-probe rate per paired row. Every
// GET/LIST of a paired row triggers verifyPaired, but inside this window
// the probe is a no-op — the previous verification is treated as still
// authoritative. Tuned for "the operator clicks Remove in wp-admin and
// gets back to the WooAgent kanban within ~30s" — long enough that a
// chatty UI doesn't fan out one HTTP call per render.
const verifyThrottle = 30 * time.Second

// generatePairingCode returns "WOOA-XXXX-XXXX" — 8 random chars from a
// 30-char ambiguity-free alphabet (~4×10¹¹ possibilities). Sufficient for a
// 10-minute window; the Companion Plugin should still rate-limit failed
// attempts.
func generatePairingCode() (string, error) {
	const n = 8
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i, b := range buf {
		out[i] = pairingCodeAlphabet[int(b)%len(pairingCodeAlphabet)]
	}
	return "WOOA-" + string(out[:4]) + "-" + string(out[4:]), nil
}

// normalizeStoreURL validates and canonicalizes the URL the operator typed.
// Requirements: https scheme, a host, no path/query/fragment, no embedded
// userinfo. The trailing slash is stripped so "https://x" and "https://x/"
// don't produce two rows.
func normalizeStoreURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" {
		return "", errors.New("url must use https://")
	}
	if u.Host == "" {
		return "", errors.New("url must include a host")
	}
	if u.User != nil {
		return "", errors.New("url must not include userinfo")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("url must not include query or fragment")
	}
	if u.Path != "" && u.Path != "/" {
		return "", errors.New("url must not include a path")
	}
	return "https://" + u.Host, nil
}

// scanStore reads a stores row into the wire shape. Used by the list and
// get-by-id paths so the column order stays in lockstep across both.
func scanStore(scanner interface {
	Scan(...any) error
}) (Store, error) {
	var s Store
	var deviceName, pairingCode, expiresAt, pairedAt, lastDiscovered sql.NullString
	var abilityCount sql.NullInt64
	if err := scanner.Scan(
		&s.ID, &s.URL, &s.MCPEndpoint,
		&deviceName, &s.Status,
		&pairingCode, &expiresAt, &pairedAt,
		&lastDiscovered, &abilityCount,
	); err != nil {
		return Store{}, err
	}
	s.DeviceName = deviceName.String
	s.PairingCode = pairingCode.String
	s.ExpiresAt = expiresAt.String
	s.PairedAt = pairedAt.String
	s.LastDiscoveredAt = lastDiscovered.String
	if abilityCount.Valid {
		v := int(abilityCount.Int64)
		s.AbilityCount = &v
	}
	if s.Status == "pairing" && s.URL != "" {
		// `?page=wooagent` matches the slug declared in the Companion
		// Plugin's add_menu_page() — `?page=wooagent-pair` would 404 in
		// wp-admin. The code= query string prefills the input on the
		// Pair device screen so the operator confirms with one click.
		s.PairURL = s.URL + "/wp-admin/admin.php?page=wooagent&code=" + s.PairingCode
	}
	return s, nil
}

const storeSelectCols = `id, url, mcp_endpoint, device_name, status, pairing_code, expires_at, paired_at, last_discovered_at, ability_count`

// handleCreateStore initiates pairing for a store URL.
//
// Idempotent on url:
//   - existing row in {pairing,expired,failed}: rotate code + extend expiry
//     in place and return the row. The UI's auto-regen-once-on-expiry path
//     depends on this.
//   - existing row in 'paired': 409 store_already_exists. Operator must
//     DELETE first if they want to re-pair.
//
// Companion Plugin integration is stubbed for v0.1 — the row sits in
// 'pairing' until expires_at, at which point GET /v1/stores/:id transitions
// it to 'expired'. The actual pair-request → operator-approve handshake
// lands when the plugin's wooagent-device-pair/* tools ship.
func (s *Server) handleCreateStore(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	if !decodeJSONBody(w, r, &req, "invalid_json") {
		return
	}
	canonURL, err := normalizeStoreURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_url", err.Error())
		return
	}

	code, err := generatePairingCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "rand_error", err.Error())
		return
	}
	now := time.Now().UTC()
	expires := now.Add(pairingTTL).Format(time.RFC3339)
	nowStr := now.Format(time.RFC3339)
	// WP MCP Adapter exposes its default server at this canonical path
	// (Streamable HTTP, session-bound). The earlier `/wp-json/mcp/v1`
	// placeholder was an incorrect guess that 404'd on every initialize
	// call. See progress.md Pre-Phase 1 entry + cmd/spike-mcp main.go for
	// the canonical URL.
	mcpEndpoint := canonURL + "/wp-json/mcp/mcp-adapter-default-server"

	ctx := r.Context()

	// Look up an existing row by url. The UNIQUE(url) index makes this
	// cheap; we resolve before insert so we can decide between idempotent
	// rotation, 409, and fresh insert.
	var existingID, existingStatus string
	err = s.store.DB.QueryRowContext(ctx,
		`SELECT id, status FROM stores WHERE url = ?`, canonURL,
	).Scan(&existingID, &existingStatus)
	switch {
	case err == nil:
		if existingStatus == "paired" {
			writeError(w, http.StatusConflict, "store_already_exists",
				"a paired store with that url exists; DELETE it first to re-pair")
			return
		}
		// Rotate the code in place. Status returns to 'pairing' regardless
		// of whether it was 'expired' or 'failed' — the operator is asking
		// to start over.
		if _, err := s.store.DB.ExecContext(ctx,
			`UPDATE stores SET status='pairing', pairing_code=?, expires_at=?, paired_at=NULL,
			                  failure_reason=NULL, updated_at=? WHERE id=?`,
			code, expires, nowStr, existingID,
		); err != nil {
			writeError(w, http.StatusInternalServerError, "db_error", err.Error())
			return
		}
		s.kickoffPairing(ctx, existingID, canonURL, code)
		s.respondStoreByID(w, r, existingID, http.StatusOK)
		return
	case errors.Is(err, sql.ErrNoRows):
		// fall through to insert
	default:
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	id := "store_" + uuid.NewString()
	if _, err := s.store.DB.ExecContext(ctx,
		`INSERT INTO stores(id, url, mcp_endpoint, status, pairing_code, expires_at, created_at, updated_at)
		 VALUES(?, ?, ?, 'pairing', ?, ?, ?, ?)`,
		id, canonURL, mcpEndpoint, code, expires, nowStr, nowStr,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	s.kickoffPairing(ctx, id, canonURL, code)
	s.respondStoreByID(w, r, id, http.StatusCreated)
}

// kickoffPairing tells the Companion Plugin to expect `code`. PluginNotInstalled
// flips the row to failed (so the UI surfaces a clear "install the plugin"
// message); any other error is logged but doesn't abort — the operator
// can retry via the rotate-on-resubmit path.
func (s *Server) kickoffPairing(ctx context.Context, id, storeURL, code string) {
	err := s.pairing.Request(ctx, storeURL, code, composeDeviceName())
	if err == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if errors.Is(err, pairing.PluginNotInstalled) {
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE stores SET status='failed', pairing_code=NULL,
			                  failure_reason='companion_plugin_missing', updated_at=?
			 WHERE id=? AND status='pairing'`,
			now, id,
		)
	}
}

// handleListStores returns every stores row. No pagination — the realistic
// upper bound is single digits in v0.1.
//
// Before composing the response, runs verifyPaired for each row in
// status='paired' so the response reflects any in-flight wp-admin
// "Remove device" events. The App.tsx initial gate keys off the LIST
// endpoint (App.tsx:116), so probing here is what makes a paired-but-
// revoked store route the operator back to onboarding on the next page
// load, not just on Step2 polling during a re-pair.
func (s *Server) handleListStores(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	s.verifyAllPaired(ctx)

	rows, err := s.store.DB.QueryContext(ctx,
		`SELECT `+storeSelectCols+` FROM stores ORDER BY created_at DESC`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	defer rows.Close()

	stores := []Store{}
	for rows.Next() {
		row, err := scanStore(rows)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
			return
		}
		row.CapabilityGaps = s.capabilityGaps(ctx, row)
		stores = append(stores, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"stores": stores})
}

// capabilityGaps attaches the compatibility report for a paired store.
//
// Read straight from the abilities already cached by discovery, so this
// costs one indexed SELECT and never touches the network. Unpaired stores
// have nothing discovered yet, so there is nothing meaningful to report.
//
// A failure here is not worth failing the request over — the store list is
// how an operator reaches the rest of the product, and a missing advisory
// is better than a 500.
func (s *Server) capabilityGaps(ctx context.Context, row Store) []abilities.Gap {
	if row.Status != "paired" {
		return nil
	}
	gaps, err := abilities.CheckStore(ctx, s.store.DB, row.ID)
	if err != nil {
		return nil
	}
	return gaps
}

// verifyAllPaired runs the staleness probe over every status='paired'
// row. Reads ids + the verify inputs in a single SELECT (so we don't
// hold an open rows cursor while issuing UPDATEs against the same DB),
// then loops sequentially — v0.1 caps stores at single digits, so
// parallelizing isn't worth the goroutine bookkeeping.
func (s *Server) verifyAllPaired(ctx context.Context) {
	rows, err := s.store.DB.QueryContext(ctx,
		`SELECT id, url, COALESCE(token_ref, ''), COALESCE(last_verified_at, '')
		 FROM stores WHERE status='paired'`)
	if err != nil {
		return
	}
	type pairedRow struct{ id, url, tokenRef, lastVerifiedAt string }
	var all []pairedRow
	for rows.Next() {
		var p pairedRow
		if err := rows.Scan(&p.id, &p.url, &p.tokenRef, &p.lastVerifiedAt); err != nil {
			rows.Close()
			return
		}
		all = append(all, p)
	}
	rows.Close()
	for _, p := range all {
		s.verifyPaired(ctx, p.id, p.url, p.tokenRef, p.lastVerifiedAt)
	}
}

// verifyPaired is the per-row staleness probe. Skips when there's no
// keychain ref (test fixtures that promote rows directly to 'paired'
// have no bearer to present) or when the throttle window hasn't elapsed.
// Three outcomes from VerifyDevice, all silent at the HTTP layer — this
// is a side-band check on a read-path handler:
//   - nil:                     bump last_verified_at, keep paired
//   - pairing.ErrTokenRevoked: drop the secret, flip row to 'unpaired',
//     clear token_ref so a future re-pair
//     starts fresh
//   - any other error:         no-op (network blip, plugin uninstalled,
//     etc.) — the next probe outside the
//     throttle window will retry
func (s *Server) verifyPaired(ctx context.Context, id, storeURL, tokenRef, lastVerifiedAt string) {
	if tokenRef == "" {
		return
	}
	if lastVerifiedAt != "" {
		if t, err := time.Parse(time.RFC3339, lastVerifiedAt); err == nil && time.Since(t) < verifyThrottle {
			return
		}
	}
	token, err := s.secrets.Get(ctx, tokenRef)
	if err != nil {
		return
	}
	err = s.pairing.VerifyDevice(ctx, storeURL, token)
	now := time.Now().UTC().Format(time.RFC3339)
	if err == nil {
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE stores SET last_verified_at=?, updated_at=? WHERE id=?`,
			now, now, id,
		)
		return
	}
	if errors.Is(err, pairing.ErrTokenRevoked) {
		// Order: wipe the secret first, then the row pointer. If the
		// row UPDATE failed we'd leave a dangling token_ref pointing at
		// a missing keychain entry, which the next read would treat as
		// "still paired but secret gone" — acceptable, the probe would
		// retry and likely 401 again.
		_ = s.secrets.Delete(ctx, tokenRef)
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE stores SET status='unpaired', token_ref=NULL,
			                  last_verified_at=?, updated_at=?
			 WHERE id=? AND status='paired'`,
			now, now, id,
		)
	}
}

// handleGetStore reads one store. Lazy-transitions a 'pairing' row to
// 'expired' when its window has closed — the UI polls this during Step 2
// of onboarding, so the read path is also where time-based state changes
// land. (Avoids a daemon-internal goroutine per pending pair.)
func (s *Server) handleGetStore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.respondStoreByID(w, r, id, http.StatusOK)
}

// respondStoreByID is the shared read path: load row, lazy-expire if past
// expires_at, write the wire shape. Used by GET /v1/stores/:id and by the
// POST handler so the create response and the read response are always
// produced from the same SQL.
//
// On status='paired' rows, fires a throttled verifyPaired before the
// final scan so a revoked-device wp-admin event surfaces as 'unpaired'
// on the very next poll (DSGWOO-1275). The 30s throttle is per row, so
// rapid Step2/popover reads share one HTTP call to /devices/me.
func (s *Server) respondStoreByID(w http.ResponseWriter, r *http.Request, id string, successStatus int) {
	ctx := r.Context()

	var status, expiresAt, storeURL, pairingCode string
	var tokenRef, lastVerifiedAt sql.NullString
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status, COALESCE(expires_at, ''), url, COALESCE(pairing_code, ''),
		         token_ref, last_verified_at
		   FROM stores WHERE id = ?`, id,
	).Scan(&status, &expiresAt, &storeURL, &pairingCode, &tokenRef, &lastVerifiedAt)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "store_not_found", "no store with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if status == "pairing" && expiresAt != "" {
		if t, err := time.Parse(time.RFC3339, expiresAt); err == nil && time.Now().UTC().After(t) {
			now := time.Now().UTC().Format(time.RFC3339)
			if _, err := s.store.DB.ExecContext(ctx,
				`UPDATE stores SET status='expired', pairing_code=NULL, updated_at=? WHERE id=? AND status='pairing'`,
				now, id,
			); err != nil {
				writeError(w, http.StatusInternalServerError, "db_error", err.Error())
				return
			}
			status = "expired"
		} else if pairingCode != "" {
			s.pollPairing(ctx, id, storeURL, pairingCode)
		}
	} else if status == "paired" {
		s.verifyPaired(ctx, id, storeURL, tokenRef.String, lastVerifiedAt.String)
	}

	row := s.store.DB.QueryRowContext(ctx,
		`SELECT `+storeSelectCols+` FROM stores WHERE id = ?`, id)
	storeRow, err := scanStore(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_scan", err.Error())
		return
	}
	writeJSON(w, successStatus, storeRow)
}

// pollPairing asks the plugin whether the operator has acted on `code`
// yet. Approved → store the device token in the keychain and flip the
// row to 'paired'. Rejected → flip to 'failed' with reason
// 'operator_rejected'. PluginNotInstalled (transient gone) → flip to
// 'expired'. Any other error is silent: the UI keeps polling, and
// either the plugin recovers or the row eventually expires on its own.
func (s *Server) pollPairing(ctx context.Context, id, storeURL, code string) {
	res, err := s.pairing.Poll(ctx, storeURL, code)
	now := time.Now().UTC().Format(time.RFC3339)

	if errors.Is(err, pairing.PluginNotInstalled) {
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE stores SET status='expired', pairing_code=NULL, updated_at=? WHERE id=? AND status='pairing'`,
			now, id,
		)
		return
	}
	if err != nil {
		return
	}

	switch res.Status {
	case pairing.StatusPending:
		return

	case pairing.StatusApproved:
		// Token is delivered exactly once by the plugin. Skip the keychain
		// write on a re-poll (status='approved' but token empty) — the row
		// is already paired, this is just a redundant call.
		if res.DeviceToken == "" {
			return
		}
		tokenRef := "wooagent.stores." + id
		if err := s.secrets.Set(ctx, tokenRef, res.DeviceToken); err != nil {
			return
		}
		deviceName := res.DeviceName
		if deviceName == "" {
			deviceName = "wooagent-device"
		}
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE stores SET status='paired', pairing_code=NULL, paired_at=?,
			                  token_ref=?, device_name=?, updated_at=?
			 WHERE id=? AND status='pairing'`,
			now, tokenRef, deviceName, now, id,
		)
		// Kick off ability discovery in the background so the operator
		// arrives at the Abilities screen with a populated cache. Failure
		// here is non-fatal — pairing is already saved; the periodic
		// sweep will retry. Use a detached context so an HTTP-request
		// cancellation (the operator closing the onboarding tab) doesn't
		// abort discovery mid-flight.
		if s.abilities != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()
				if _, err := s.abilities.RunForStore(ctx, id); err != nil {
					// Best-effort. Surfacing failures here is the runner's job.
					_ = err
				}
			}()
		}

	case pairing.StatusRejected:
		_, _ = s.store.DB.ExecContext(ctx,
			`UPDATE stores SET status='failed', pairing_code=NULL,
			                  failure_reason='operator_rejected', updated_at=?
			 WHERE id=? AND status='pairing'`,
			now, id,
		)
	}
}

// handleDeleteStore unpairs a store: drops its keychain entry then deletes
// the row. v0.1 skips the plugin-side wooagent-device-pair/revoke call
// (plugin not yet shipped) — the keychain wipe is the part that can't leak.
// Orphaned wp-admin device entries can be revoked by the operator from the
// device list there.
func (s *Server) handleDeleteStore(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var tokenRef sql.NullString
	var storeURL string
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT token_ref, url FROM stores WHERE id = ?`, id,
	).Scan(&tokenRef, &storeURL)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "store_not_found", "no store with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	if tokenRef.Valid && tokenRef.String != "" {
		// Best-effort plugin-side revoke before we wipe the local secret.
		// Failure here doesn't block deletion: if the plugin is unreachable
		// the operator can still revoke the orphan from wp-admin's device
		// list, and our row + keychain entry are already gone.
		if token, err := s.secrets.Get(ctx, tokenRef.String); err == nil {
			_ = s.pairing.Revoke(ctx, storeURL, token)
		}
		if err := s.secrets.Delete(ctx, tokenRef.String); err != nil && !errors.Is(err, secrets.ErrNotFound) {
			writeError(w, http.StatusInternalServerError, "keychain_error", err.Error())
			return
		}
	}

	if _, err := s.store.DB.ExecContext(ctx, `DELETE FROM stores WHERE id = ?`, id); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRefreshStoreAbilities re-runs ability discovery against a paired
// store on demand. Without this, the periodic ticker (default 6h, see
// abilities.go) is the only thing that picks up abilities added by a
// plugin install on the store side. The handler is operator-driven so
// nothing happens automatically — the UI exposes it as a "Refresh"
// button on the Abilities screen.
//
// Responses:
//
//	200 — { "ability_count": N, "last_discovered_at": "..." } on success
//	404 — store_not_found
//	409 — store_not_paired (still pairing / expired / failed)
//	502 — discovery_failed (MCP roundtrip or reconcile error)
func (s *Server) handleRefreshStoreAbilities(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	ctx := r.Context()

	var status string
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status FROM stores WHERE id = ?`, id,
	).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "store_not_found", "no store with that id")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if status != "paired" {
		writeError(w, http.StatusConflict, "store_not_paired",
			"store status is "+status+"; pair the store before refreshing abilities")
		return
	}

	// 30s ceiling so a slow store doesn't pin the request indefinitely.
	// RunForStore typically completes in 1–3s against a healthy MCP host;
	// timeouts here surface to the operator as a "couldn't refresh" notice
	// rather than a UI spinner that never resolves.
	runCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	count, err := s.abilities.RunForStore(runCtx, id)
	if err != nil {
		writeError(w, http.StatusBadGateway, "discovery_failed", err.Error())
		return
	}

	var lastDiscoveredAt sql.NullString
	if err := s.store.DB.QueryRowContext(ctx,
		`SELECT last_discovered_at FROM stores WHERE id = ?`, id,
	).Scan(&lastDiscoveredAt); err != nil {
		writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ability_count":      count,
		"last_discovered_at": lastDiscoveredAt.String,
	})
}
