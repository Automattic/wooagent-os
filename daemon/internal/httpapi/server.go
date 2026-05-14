package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/wooagent-os/wooagent-os/daemon/internal/abilities"
	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pairing"
	"github.com/wooagent-os/wooagent-os/daemon/internal/pep"
	"github.com/wooagent-os/wooagent-os/daemon/internal/scheduler"
	"github.com/wooagent-os/wooagent-os/daemon/internal/secrets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/wooagent-os/wooagent-os/daemon/internal/uiassets"
	"github.com/wooagent-os/wooagent-os/daemon/internal/version"
)

// PairingClient is the daemon-facing interface to the Companion Plugin's
// /pair/* REST endpoints. Defined here as an interface so handler tests
// can inject a fake without spinning up a real plugin install.
type PairingClient interface {
	Request(ctx context.Context, storeURL, code, deviceName string) error
	Poll(ctx context.Context, storeURL, code string) (pairing.PollResult, error)
	Revoke(ctx context.Context, storeURL, deviceToken string) error
}

// Server wraps a chi router configured with the v1 API, CORS, and bearer-token
// auth. Callers pass it to http.Server.
//
// `pep` is the gate every store-mutating call routes through (PRD §8.4.2).
// It carries the MCP client internally; the Server does not hold one
// directly because the rule "no orchestrator → MCP shortcut" is enforced by
// having pep.Invoke be the only path. pep may be nil for UI-only daemon
// runs; approve returns 503 in that case.
//
// `secrets` is the OS-keychain wrapper used by /v1/stores and
// /v1/model-providers to store device tokens and API keys without
// persisting plaintext to disk. Required: New panics if nil. Tests pass an
// in-memory backend installed via keyring.MockInit().
type Server struct {
	router         chi.Router
	store          *store.Store
	auth           *auth.Manager
	pep            *pep.PEP
	secrets        secrets.Store
	modelTester    ModelTester
	pairing        PairingClient
	abilities      *abilities.Runner
	scheduler      *scheduler.Scheduler
	uiSessionToken string
}

// New wires a Server with all required collaborators.
//
// `uiSessionToken` is the bearer token the daemon resolves at startup
// (via auth.Manager.EnsureUISession) so the embedded UI auto-connects
// without the operator pasting a token. Persists across restarts via
// paths.UISessionFile. Pass "" to disable auto-auth (the UI falls back
// to its manual URL+token form). Only ever delivered to loopback Host
// headers — see uiassets.Handler.
func New(st *store.Store, am *auth.Manager, p *pep.PEP, sec secrets.Store, uiSessionToken string) *Server {
	if sec == nil {
		panic("httpapi.New: secrets.Store is required")
	}
	s := &Server{
		store:          st,
		auth:           am,
		pep:            p,
		secrets:        sec,
		modelTester:    newRealModelTester(),
		pairing:        pairing.NewClient(),
		abilities:      abilities.New(st.DB, sec),
		uiSessionToken: uiSessionToken,
	}
	s.router = s.buildRouter()
	return s
}

// SetAbilitiesRunner overrides the auto-wired ability discovery runner.
// Tests inject a runner with a fake MCPClientFactory; the daemon main
// uses this to override PollInterval / Logger before serving traffic.
func (s *Server) SetAbilitiesRunner(r *abilities.Runner) { s.abilities = r }

// Abilities returns the runner so the daemon main can drive the startup
// sweep + periodic ticker without re-creating it.
func (s *Server) Abilities() *abilities.Runner { return s.abilities }

// SetScheduler is called by the daemon main after the scheduler is started
// so the manual-trigger handler can enqueue runs. Optional; nil scheduler
// means /v1/runs POST returns 503.
func (s *Server) SetScheduler(sch *scheduler.Scheduler) { s.scheduler = sch }

func (s *Server) Handler() http.Handler { return s.router }

func (s *Server) buildRouter() chi.Router {
	r := chi.NewRouter()

	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(30 * time.Second))
	r.Use(requestLogger)

	// The UI is served from a different origin (Vite dev server at 5173;
	// static hosts later). Permit any origin reflected back; bearer-token
	// auth is the real security boundary.
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc:  func(r *http.Request, origin string) bool { return true },
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "Accept"},
		AllowCredentials: false,
		MaxAge:           300,
	}))

	// /v1/health is unauthenticated so the UI can probe connectivity before
	// it has a token to offer. Everything else requires bearer auth.
	r.Get("/v1/health", s.handleHealth)

	r.Group(func(r chi.Router) {
		r.Use(s.bearerAuth)
		r.Get("/v1/agents", s.handleListAgents)
		r.Get("/v1/issues", s.handleListIssues)
		r.Post("/v1/issues", s.handleCreateIssue)
		r.Get("/v1/issues/{id}", s.handleGetIssue)
		r.Post("/v1/issues/{id}/approve", s.handleApproveIssue)
		r.Post("/v1/issues/{id}/reject", s.handleRejectIssue)
		r.Post("/v1/issues/{id}/dismiss", s.handleDismissIssue)
		r.Get("/v1/batches", s.handleListBatches)
		r.Post("/v1/batches", s.handleCreateBatch)
		r.Get("/v1/batches/{id}", s.handleGetBatch)
		r.Post("/v1/batches/{id}/approve-all", s.handleApproveBatch)
		r.Post("/v1/batches/{id}/reject-all", s.handleRejectBatch)
		r.Get("/v1/abilities", s.handleListAbilities)
		r.Post("/v1/abilities/{id}/trust", s.handleTrustAbility)
		r.Post("/v1/abilities/{id}/revoke", s.handleRevokeAbility)
		r.Post("/v1/abilities/{id}/restore", s.handleRestoreAbility)
		r.Get("/v1/stores", s.handleListStores)
		r.Post("/v1/stores", s.handleCreateStore)
		r.Get("/v1/stores/{id}", s.handleGetStore)
		r.Delete("/v1/stores/{id}", s.handleDeleteStore)
		r.Get("/v1/model-providers", s.handleListModelProviders)
		r.Post("/v1/model-providers", s.handleCreateModelProvider)
		r.Post("/v1/model-providers/test", s.handleTestModelProvider)
		r.Delete("/v1/model-providers/{id}", s.handleDeleteModelProvider)
		r.Get("/v1/runs", s.handleListRuns)
		r.Get("/v1/runs/{id}", s.handleGetRun)
		r.Post("/v1/runs", s.handleCreateRun)
	})

	// Catch-all handler: API paths get the JSON 404 envelope (existing
	// behavior); everything else falls through to the embedded UI so
	// `wooagent run` serves a working app at `/` without a separate UI
	// process. The Vite dev server on :5173 still works in parallel —
	// CORS above permits any origin.
	uiHandler := uiassets.Handler(s.uiSessionToken)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/v1/") {
			writeError(w, http.StatusNotFound, "not_found", "no route matches")
			return
		}
		uiHandler.ServeHTTP(w, r)
	})

	return r
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":         "ok",
		"version":        version.Version,
		"schema_version": version.SchemaVersion,
	})
}

// Run serves on the given address until ctx is cancelled. It returns the first
// error from ListenAndServe (other than http.ErrServerClosed, which is folded
// into nil) or the context error if shutdown times out.
func Run(ctx context.Context, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	errs := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			errs <- err
			return
		}
		errs <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errs:
		return err
	}
}

// ---------- shared helpers ----------

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError emits the canonical error envelope documented in
// docs/api-contract-v1.md.
func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": msg,
		},
	})
}
