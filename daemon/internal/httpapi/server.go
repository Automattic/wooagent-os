package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
	"github.com/wooagent-os/wooagent-os/daemon/internal/store"
	"github.com/wooagent-os/wooagent-os/daemon/internal/version"
)

// Server wraps a chi router configured with the v1 API, CORS, and bearer-token
// auth. Callers pass it to http.Server.
type Server struct {
	router chi.Router
	store  *store.Store
	auth   *auth.Manager
}

func New(st *store.Store, am *auth.Manager) *Server {
	s := &Server{store: st, auth: am}
	s.router = s.buildRouter()
	return s
}

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
		r.Get("/v1/abilities", s.handleListAbilities)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no route matches")
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
