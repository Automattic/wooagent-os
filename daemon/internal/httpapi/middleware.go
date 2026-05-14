package httpapi

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/wooagent-os/wooagent-os/daemon/internal/auth"
)

type contextKey string

const operatorNameKey contextKey = "wooagent.operatorName"

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
		name, err := s.auth.Validate(r.Context(), token)
		if err != nil {
			if errors.Is(err, auth.ErrInvalidToken) {
				writeError(w, http.StatusUnauthorized, "auth_invalid",
					"Bearer token does not match any active session. Restart the daemon (`wooagent run`) to mint a fresh ui-session token, or run `wooagent auth token create` for an operator token.")
				return
			}
			writeError(w, http.StatusInternalServerError, "auth_error", "auth lookup failed")
			return
		}
		ctx := context.WithValue(r.Context(), operatorNameKey, name)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// operatorName returns the bearer-token name that authorized this
// request. Empty string if not present (e.g. routes outside the
// bearer-auth middleware chain, or if called before the middleware
// has run).
func operatorName(r *http.Request) string {
	v, _ := r.Context().Value(operatorNameKey).(string)
	return v
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
