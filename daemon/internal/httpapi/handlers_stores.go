package httpapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

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
	ID                string  `json:"id"`
	URL               string  `json:"url"`
	MCPEndpoint       string  `json:"mcp_endpoint,omitempty"`
	DeviceName        string  `json:"device_name,omitempty"`
	Status            string  `json:"status"`
	PairingCode       string  `json:"pairing_code,omitempty"`
	PairURL           string  `json:"pair_url,omitempty"`
	ExpiresAt         string  `json:"expires_at,omitempty"`
	PairedAt          string  `json:"paired_at,omitempty"`
	AbilityCount      *int    `json:"ability_count,omitempty"`
	LastDiscoveredAt  string  `json:"last_discovered_at,omitempty"`
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
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
	mcpEndpoint := canonURL + "/wp-json/mcp/v1"

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
func (s *Server) handleListStores(w http.ResponseWriter, r *http.Request) {
	rows, err := s.store.DB.QueryContext(r.Context(),
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
		stores = append(stores, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"stores": stores})
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
func (s *Server) respondStoreByID(w http.ResponseWriter, r *http.Request, id string, successStatus int) {
	ctx := r.Context()

	var status, expiresAt, storeURL, pairingCode string
	err := s.store.DB.QueryRowContext(ctx,
		`SELECT status, COALESCE(expires_at, ''), url, COALESCE(pairing_code, '') FROM stores WHERE id = ?`, id,
	).Scan(&status, &expiresAt, &storeURL, &pairingCode)
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
