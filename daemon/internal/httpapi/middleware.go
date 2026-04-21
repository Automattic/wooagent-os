package httpapi

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
)

func (s *Server) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token := ""
		if strings.HasPrefix(header, "Bearer ") {
			token = strings.TrimPrefix(header, "Bearer ")
		}
		if token == "" {
			writeError(w, http.StatusUnauthorized, "auth_missing", "Authorization: Bearer <token> required")
			return
		}
		if err := s.auth.Validate(r.Context(), token); err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				writeError(w, http.StatusUnauthorized, "auth_invalid", "token not recognized")
				return
			}
			writeError(w, http.StatusInternalServerError, "auth_error", "auth lookup failed")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requestLogger logs one line per request with method, path, status, bytes,
// duration, and the request id. Keeps things lightweight — replace with zerolog
// or slog once structured logs become useful.
func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		log.Printf("%s %s %d %dB %s req=%s",
			r.Method, r.URL.Path, ww.Status(), ww.BytesWritten(),
			time.Since(start), chimw.GetReqID(r.Context()),
		)
	})
}
