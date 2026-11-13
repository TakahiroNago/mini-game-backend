// Package server assembles the chi router: global middleware, probe
// endpoints and the versioned API groups registered by each domain package.
package server

import (
	"database/sql"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/takahironago/mini-game-backend/internal/admin"
	"github.com/takahironago/mini-game-backend/internal/auth"
	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/internal/health"
	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/item"
	"github.com/takahironago/mini-game-backend/internal/middleware"
	"github.com/takahironago/mini-game-backend/internal/player"
	"github.com/takahironago/mini-game-backend/internal/ranking"
	"github.com/takahironago/mini-game-backend/internal/reward"
)

// Deps are the shared dependencies handed to every domain package.
type Deps struct {
	Config  config.Config
	Logger  *slog.Logger
	DB      *sql.DB
	Version string
}

// Services groups the domain services built once in NewServices and shared
// by the router and start-up tasks (admin bootstrap).
type Services struct {
	Tokens  *auth.TokenIssuer
	Auth    *auth.Service
	Player  *player.Service
	Item    *item.Service
	Reward  *reward.Service
	Ranking *ranking.Service
	Admin   *admin.Service
}

// NewServices constructs every domain service from the shared dependencies.
func NewServices(d Deps) *Services {
	tokens := auth.NewTokenIssuer(d.Config.JWT.Secret, d.Config.JWT.Expiry, nil)
	playerRepo := player.NewRepository()
	itemRepo := item.NewRepository()
	rankingRepo := ranking.NewRepository()
	itemSvc := item.NewService(d.DB, itemRepo, playerRepo, d.Logger)
	return &Services{
		Tokens:  tokens,
		Auth:    auth.NewService(d.DB, auth.NewRepository(), playerRepo, itemSvc, tokens, d.Logger, nil),
		Player:  player.NewService(d.DB, playerRepo),
		Item:    itemSvc,
		Reward:  reward.NewService(d.DB, reward.NewRepository(), playerRepo, d.Logger, nil),
		Ranking: ranking.NewService(d.DB, rankingRepo, playerRepo),
		Admin:   admin.NewService(d.DB, admin.NewRepository(), itemRepo, playerRepo, rankingRepo, d.Logger, nil),
	}
}

// NewRouter builds the HTTP handler.
func NewRouter(d Deps, s *Services) http.Handler {
	r := chi.NewRouter()

	// Order matters: request id first so every later log line carries it;
	// RealIP before anything keyed by client address (rate limit, logs);
	// recoverer before timeout so a panic inside a timed-out request still
	// returns JSON; CORS before the rate limiter so a 429 still carries the
	// headers a browser needs to read it.
	r.Use(chimw.RequestID)
	r.Use(chimw.RealIP)
	r.Use(middleware.Logger(d.Logger))
	r.Use(middleware.Recoverer(d.Logger))
	r.Use(chimw.Timeout(d.Config.RequestTimeout))
	r.Use(chimw.NoCache)
	r.Use(middleware.SecurityHeaders)
	r.Use(middleware.BodyLimit(d.Config.MaxBodyBytes))
	r.Use(middleware.CORS(d.Config.CORSOrigins))

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

	authn := middleware.Authenticate(s.Tokens, s.Auth)
	authH := auth.NewHandler(s.Auth)
	playerH := player.NewHandler(s.Player)
	itemH := item.NewHandler(s.Item)
	rewardH := reward.NewHandler(s.Reward)
	rankingH := ranking.NewHandler(s.Ranking)
	adminH := admin.NewHandler(s.Admin)

	r.Route("/api/v1", func(api chi.Router) {
		// Probes stay outside the limiter so orchestrators never see a 429.
		api.Use(middleware.RateLimit(d.Config.RateLimit.PerMinute))

		api.Route("/auth", func(ar chi.Router) {
			// Register / login are the brute-force surface: a much smaller
			// per-IP budget on top of the global one.
			ar.Group(func(pub chi.Router) {
				pub.Use(middleware.RateLimit(d.Config.RateLimit.AuthPerMinute))
				authH.PublicRoutes(pub)
			})
			ar.Group(func(pr chi.Router) {
				pr.Use(authn)
				authH.ProtectedRoutes(pr)
			})
		})

		api.Route("/player", func(pr chi.Router) {
			pr.Use(authn)
			playerH.Routes(pr)
		})

		api.Route("/items", func(ir chi.Router) {
			ir.Use(authn)
			itemH.Routes(ir)
		})

		api.Route("/rewards", func(rr chi.Router) {
			rr.Use(authn)
			rewardH.Routes(rr)
		})

		api.Route("/ranking", func(rr chi.Router) {
			rr.Use(authn)
			rankingH.Routes(rr)
		})

		api.Route("/admin", func(ar chi.Router) {
			ar.Use(authn)
			ar.Use(middleware.RequireRole(auth.RoleAdmin))
			adminH.Routes(ar)
		})
	})

	return r
}
