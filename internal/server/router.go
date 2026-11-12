// Package server assembles the chi router: global middleware, probe
// endpoints and the versioned API groups registered by each domain package.
package server

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/internal/health"
	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/middleware"
)

// Deps are the shared dependencies handed to every domain package.
type Deps struct {
	Config  config.Config
	Logger  *slog.Logger
	DB      *sql.DB
	Version string
}

// NewRouter builds the HTTP handler. Domain routes are mounted under /api/v1
// as they are implemented (Day 2 onwards).
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()

	// Order matters: request id first so every later log line carries it;
	// recoverer before timeout so a panic inside a timed-out request still
	// returns JSON.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger(d.Logger))
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(chimw.Timeout(d.Config.RequestTimeout))
	r.Use(chimw.NoCache)

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(r.Context(), w, httpx.ErrNotFound)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteError(r.Context(), w,
			httpx.NewError(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))
	})

	h := health.NewHandler(d.DB, d.Version)
	r.Get("/health", h.Live)
	r.Get("/ready", h.Ready)

	r.Route("/api/v1", func(api chi.Router) {
		// Day 2: auth, player
		// Day 3: items, rewards, ranking
		// Day 4: admin
		_ = api
	})

	return r
}
