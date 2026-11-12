package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/takahironago/mini-game-backend/internal/httpx"
)

// Recoverer converts panics into a JSON 500 and logs the stack trace with the
// request id, instead of chi's default plain-text response.
func Recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}
				if err, ok := rec.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(rec) // let net/http handle connection aborts
				}
				log.ErrorContext(r.Context(), "panic recovered",
					"request_id", chimw.GetReqID(r.Context()),
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				httpx.JSON(w, http.StatusInternalServerError, map[string]any{
					"error": httpx.ErrInternal,
				})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
