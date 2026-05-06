// Package uiassets serves the WooAgent OS React UI as static files baked
// into the daemon binary. The contract: ui/ is built with `npm run build`,
// the resulting ui/dist/ is copied into daemon/internal/uiassets/dist/
// (via scripts/build-ui-into-daemon.sh) before `go build`, and the embed
// below picks up whatever's there.
//
// Why embed instead of host-the-UI-separately:
//   - Single-binary install promise (CLAUDE.md). `wooagent run` is the
//     only process the operator needs; no separate Vite dev server, no
//     hosted CDN dependency.
//   - Same architecture for local-laptop installs and remote-VM
//     deployments — the daemon answers HTTP from `:7777` either way.
//   - SPA + API on the same origin = no CORS for the embedded path
//     (CORS middleware stays for the Vite dev workflow on `:5173`).
//
// The dist/ directory always contains at least a placeholder index.html
// so the embed compiles on a fresh clone. Production builds overwrite
// it via the copy script above.
package uiassets

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"strings"
)

//go:embed dist
var embedded embed.FS

// Handler returns an http.Handler that serves the embedded UI:
//   - exact-file matches (assets/index-abc123.js, etc.) come from the
//     embed verbatim with proper MIME types from http.FileServer
//   - any other path returns index.html so React Router resolves it
//     client-side (operators can deep-link to /onboard/store, /settings,
//     etc. without the daemon needing to know about routes)
//   - /v1/* is explicitly rejected as a defense-in-depth measure
//
// `uiSessionToken` is the bearer token the embedded UI uses to call
// /v1/*. When non-empty, it's injected into index.html as
// `window.__WOOAGENT_TOKEN__` so the React app auto-connects without
// the operator pasting credentials. Injection happens ONLY for
// loopback requests (Host = localhost / 127.0.0.1 / ::1) — if the
// daemon is bound to a non-loopback address (e.g. `--bind 0.0.0.0`),
// remote requests get the unmodified placeholder/build and fall back
// to the manual paste flow. Empty token disables injection entirely.
//
// Caller mounts this last on the chi router (typically as the NotFound
// handler) so registered API routes win.
func Handler(uiSessionToken string) http.Handler {
	subFS, err := fs.Sub(embedded, "dist")
	if err != nil {
		panic("uiassets: dist/ not embedded — did the build copy ui/dist into the embed dir?")
	}
	indexHTML, err := fs.ReadFile(subFS, "index.html")
	if err != nil {
		panic("uiassets: dist/index.html missing — placeholder or built UI required")
	}
	fileServer := http.FileServer(http.FS(subFS))
	indexWithToken := injectUISessionToken(indexHTML, uiSessionToken)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}

		// Pick the index variant: same-machine (loopback Host) gets the
		// auto-token build; remote requests get the unmodified one.
		body := indexHTML
		if uiSessionToken != "" && isLoopbackHost(r.Host) {
			body = indexWithToken
		}

		if r.URL.Path == "/" || r.URL.Path == "" {
			serveIndex(w, body)
			return
		}

		clean := strings.TrimPrefix(r.URL.Path, "/")
		info, err := fs.Stat(subFS, clean)
		if err != nil || info.IsDir() {
			serveIndex(w, body)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

func serveIndex(w http.ResponseWriter, indexHTML []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// SPA shell — no caching so a deploy with a new index.html (which
	// references new hashed asset filenames) is picked up immediately.
	// The hashed assets themselves are cache-friendly via fingerprint.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(indexHTML)
}

// injectUISessionToken returns a copy of indexHTML with a small
// bootstrap script inserted before </head>. The script sets
// `window.__WOOAGENT_TOKEN__` so the React app auto-connects on load
// (loadConnection() in ui/src/api/client.ts checks that global before
// falling back to localStorage). When token is empty, returns the
// input unchanged so a fresh-clone placeholder still renders cleanly.
//
// Token is JSON-encoded for safe HTML embedding — escapes quotes,
// backslashes, and any future surprises.
func injectUISessionToken(indexHTML []byte, token string) []byte {
	if token == "" {
		return indexHTML
	}
	headClose := []byte("</head>")
	if !bytes.Contains(indexHTML, headClose) {
		// No </head> means this is the placeholder or a stripped build;
		// don't inject (operator gets the manual flow). Better than
		// silently appending a script tag where it might not parse.
		return indexHTML
	}
	encoded, _ := json.Marshal(token)
	bootstrap := []byte("<script>window.__WOOAGENT_TOKEN__=" + string(encoded) + ";</script>")
	return bytes.Replace(indexHTML, headClose, append(bootstrap, headClose...), 1)
}

// isLoopbackHost reports whether the Host header (host:port or host)
// resolves to a loopback address. Used to gate auto-token injection so
// a daemon bound to 0.0.0.0 doesn't hand its session token to remote
// browsers; loopback is the implied "same machine = trusted" boundary.
func isLoopbackHost(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}
	hostname = strings.Trim(hostname, "[]")
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
