// Package pairing speaks to the WooAgent Companion Plugin's /pair/* REST
// endpoints. It exists to keep the HTTP shape of the pairing handshake out
// of the /v1/stores handlers (which already do enough) and to give tests
// a clean seam (httptest.NewServer can stand in for a real WP install).
//
// Why these endpoints are REST and not MCP: the pair handshake has to
// succeed BEFORE the daemon has any auth credential to use against MCP.
// Once paired, the device token minted here becomes the credential the
// daemon will use for every subsequent MCP call (v0.2 enforcement).
package pairing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PluginNotInstalled is returned by Request when the store URL responds
// 404 to /wp-json/wooagent/v1/pair/request — typically because the
// Companion Plugin isn't installed (or isn't activated). The /v1/stores
// handler maps this to row.failure_reason='companion_plugin_missing' so
// the UI surfaces a helpful message rather than a generic transient
// error.
var PluginNotInstalled = errors.New("pairing: Companion Plugin endpoints not registered")

// ErrTokenRevoked is returned by VerifyDevice when the plugin responds
// 401 to /wp-json/wooagent/v1/devices/me — the daemon's stored bearer is
// no longer in wooagent_devices (operator clicked Remove, option reset,
// etc.). The /v1/stores handler maps this to status='unpaired'.
var ErrTokenRevoked = errors.New("pairing: device token no longer recognized by the Companion Plugin")

// Status is the high-level state returned by Poll. The plugin itself uses
// strings; we surface them as a typed enum so callers don't typo.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// PollResult is what the daemon needs after a poll: the status, plus the
// device materials when the operator has approved. DeviceToken is only
// populated on the first poll after approval — the plugin clears it from
// the transient after delivery so a re-poll doesn't redeliver the secret.
type PollResult struct {
	Status      Status
	DeviceID    string
	DeviceName  string
	DeviceToken string
}

// Client is the daemon-side wrapper around the plugin's REST routes.
// One Client is safe to reuse across requests (and across goroutines —
// http.Client is concurrent-safe). Test code passes a Client whose
// HTTPClient targets an httptest.Server URL.
type Client struct {
	HTTPClient *http.Client
}

// NewClient returns a Client with a sensible HTTP timeout. Tight enough
// that a slow store doesn't leave the UI hung on a "creating pairing"
// spinner; long enough that genuinely-slow Pressable installs answer
// before we give up.
func NewClient() *Client {
	return &Client{
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Request asks the plugin to register `code` as a pending pairing for
// `deviceName`. Returns PluginNotInstalled if the route 404s; any other
// non-2xx is returned as an error with the response body inlined.
func (c *Client) Request(ctx context.Context, storeURL, code, deviceName string) error {
	u, err := pairURL(storeURL, "request", "")
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{
		"code":        code,
		"device_name": deviceName,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("pair/request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return PluginNotInstalled
	}
	if res.StatusCode/100 != 2 {
		return statusError("pair/request", res)
	}
	return nil
}

// Poll asks the plugin whether the operator has approved (or rejected)
// the pairing for `code`. A 404 here means the transient expired or was
// never registered; we surface it as PluginNotInstalled-or-expired —
// callers handle it the same way (treat the row as expired).
func (c *Client) Poll(ctx context.Context, storeURL, code string) (PollResult, error) {
	u, err := pairURL(storeURL, "poll", code)
	if err != nil {
		return PollResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return PollResult{}, err
	}
	// Defensive: WPCom/Pressable's Batcache caches GET responses for
	// 5 minutes by default, which would leave the daemon stuck on a
	// stale "pending" response after wp-admin approves. The plugin's
	// poll callback also calls nocache_headers() server-side; this
	// request header is the belt-and-suspenders complement.
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return PollResult{}, fmt.Errorf("pair/poll: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return PollResult{}, PluginNotInstalled
	}
	if res.StatusCode/100 != 2 {
		return PollResult{}, statusError("pair/poll", res)
	}
	var parsed struct {
		Status      string `json:"status"`
		DeviceID    string `json:"device_id"`
		DeviceName  string `json:"device_name"`
		DeviceToken string `json:"device_token"`
	}
	if err := json.NewDecoder(res.Body).Decode(&parsed); err != nil {
		return PollResult{}, fmt.Errorf("pair/poll decode: %w", err)
	}
	return PollResult{
		Status:      Status(parsed.Status),
		DeviceID:    parsed.DeviceID,
		DeviceName:  parsed.DeviceName,
		DeviceToken: parsed.DeviceToken,
	}, nil
}

// VerifyDevice asks the plugin whether the bearer it presents is still a
// registered device. 200 → still paired (the daemon refreshes
// last_verified_at and is done). 401 → ErrTokenRevoked, the daemon flips
// its local row to 'unpaired' and drops the keychain entry. 404 →
// PluginNotInstalled (the plugin was uninstalled or downgraded after
// pairing — treat as transient: don't mutate the row, the next probe
// will retry).
//
// Cache-Control no-cache for the same Batcache reason as Poll: the
// "device removed" transition has to surface within seconds, not after
// the upstream cache TTL expires.
func (c *Client) VerifyDevice(ctx context.Context, storeURL, deviceToken string) error {
	base := strings.TrimRight(storeURL, "/")
	u, err := url.Parse(base + "/wp-json/wooagent/v1/devices/me")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("devices/me: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusUnauthorized, res.StatusCode == http.StatusForbidden:
		// WordPress returns 403 when an alternate auth source (cookies)
		// resolves a user without admin caps — extremely unlikely for the
		// daemon, but coalescing the two means a misconfigured plugin
		// can't strand a paired row in a "neither 200 nor 401" limbo.
		return ErrTokenRevoked
	case res.StatusCode == http.StatusNotFound:
		return PluginNotInstalled
	case res.StatusCode/100 != 2:
		return statusError("devices/me", res)
	}
	return nil
}

// Revoke tells the plugin to drop the device whose token we present.
// Bearer-as-identity: no device_id in the body, the plugin uses the
// hashed token to find the row. Best-effort — the daemon row + keychain
// entry are deleted regardless of the plugin's response.
func (c *Client) Revoke(ctx context.Context, storeURL, deviceToken string) error {
	u, err := pairURL(storeURL, "revoke", "")
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader([]byte("{}")))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+deviceToken)
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("pair/revoke: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		return statusError("pair/revoke", res)
	}
	return nil
}

// pairURL composes the REST endpoint URL. Centralized so a query-string
// vs path-segment decision (poll uses ?code=) doesn't drift across calls.
func pairURL(storeURL, action, code string) (string, error) {
	base := strings.TrimRight(storeURL, "/")
	u, err := url.Parse(base + "/wp-json/wooagent/v1/pair/" + action)
	if err != nil {
		return "", err
	}
	if code != "" {
		q := u.Query()
		q.Set("code", code)
		u.RawQuery = q.Encode()
	}
	return u.String(), nil
}

func statusError(prefix string, res *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(res.Body, 512))
	return fmt.Errorf("%s: %d %s: %s", prefix, res.StatusCode, http.StatusText(res.StatusCode), strings.TrimSpace(string(body)))
}
