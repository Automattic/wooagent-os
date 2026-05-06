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
	"embed"
	"io/fs"
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
//   - /v1/* is explicitly rejected as a defense-in-depth measure: the
//     chi router shouldn't dispatch here for those paths, but a future
//     misconfiguration shouldn't accidentally serve UI for an API path
//
// Caller mounts this last on the chi router (typically as the NotFound
// handler) so registered API routes win.
func Handler() http.Handler {
	subFS, err := fs.Sub(embedded, "dist")
	if err != nil {
		// Build-time guarantee — dist/ exists by construction. Panicking
		// here surfaces a forgotten copy step loudly rather than serving
		// 500s for every request.
		panic("uiassets: dist/ not embedded — did the build copy ui/dist into the embed dir?")
	}
	indexHTML, err := fs.ReadFile(subFS, "index.html")
	if err != nil {
		panic("uiassets: dist/index.html missing — placeholder or built UI required")
	}
	fileServer := http.FileServer(http.FS(subFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defense in depth — API routes have their own handlers + 404
		// envelope. Anything that reaches here for /v1/* is a routing
		// bug, not a UI request.
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			http.NotFound(w, r)
			return
		}

		// Root → index.html (skip the directory-listing the file server
		// would otherwise emit).
		if r.URL.Path == "/" || r.URL.Path == "" {
			serveIndex(w, indexHTML)
			return
		}

		// Existing asset → serve it. fs.Stat fails for missing paths +
		// for directories; either way, fall through to the SPA fallback.
		clean := strings.TrimPrefix(r.URL.Path, "/")
		info, err := fs.Stat(subFS, clean)
		if err != nil || info.IsDir() {
			serveIndex(w, indexHTML)
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
