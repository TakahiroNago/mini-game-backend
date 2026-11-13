package middleware

import (
	"net/http"

	"github.com/go-chi/cors"

	"github.com/takahironago/mini-game-backend/internal/item"
)

// CORS allows browser clients served from another origin (a game client on
// its own domain, the Vite dev server when it is not proxying) to call the
// API. The admin UI is same-origin behind nginx and never triggers it.
//
// Credentials are deliberately off: authentication is a Bearer token in a
// header, never a cookie, so there is nothing for a cross-site request to
// ride on. An empty origin list allows no cross-origin caller at all.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	// go-chi/cors treats an empty list as "*"; we want the opposite default.
	var denyAll func(*http.Request, string) bool
	if len(allowedOrigins) == 0 {
		denyAll = func(*http.Request, string) bool { return false }
	}
	return cors.Handler(cors.Options{
		AllowedOrigins:  allowedOrigins,
		AllowOriginFunc: denyAll,
		AllowedMethods:  []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:  []string{"Authorization", "Content-Type", item.IdempotencyKeyHeader},
		ExposedHeaders:  []string{"X-Request-Id", "X-RateLimit-Limit", "X-RateLimit-Remaining", "X-RateLimit-Reset", "Retry-After"},
		// Preflight results may be cached for 5 minutes.
		MaxAge:           300,
		AllowCredentials: false,
	})
}
