package ranking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/identity"
	"github.com/takahironago/mini-game-backend/internal/player"
)

// Pagination bounds.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Page is one leaderboard page.
type Page struct {
	Rankings []Entry `json:"rankings"`
	Limit    int     `json:"limit"`
	Offset   int     `json:"offset"`
	Total    int64   `json:"total"`
}

// MyRank is the caller's own position.
type MyRank struct {
	Rank       int64  `json:"rank"`
	PlayerID   int64  `json:"player_id"`
	PlayerName string `json:"player_name"`
	Level      int    `json:"level"`
	Experience int64  `json:"experience"`
	Total      int64  `json:"total"`
}

// Service reads the leaderboard.
type Service struct {
	db      *sql.DB
	repo    *Repository
	players *player.Repository
}

// NewService returns a Service.
func NewService(db *sql.DB, repo *Repository, players *player.Repository) *Service {
	return &Service{db: db, repo: repo, players: players}
}

// Page returns one page of the leaderboard.
func (s *Service) Page(ctx context.Context, limit, offset int) (Page, error) {
	entries, err := s.repo.List(ctx, s.db, limit, offset)
	if err != nil {
		return Page{}, fmt.Errorf("list ranking: %w", err)
	}
	total, err := s.repo.Count(ctx, s.db)
	if err != nil {
		return Page{}, fmt.Errorf("count players: %w", err)
	}
	return Page{Rankings: entries, Limit: limit, Offset: offset, Total: total}, nil
}

// Me returns the position of the player owned by userID.
func (s *Service) Me(ctx context.Context, userID int64) (MyRank, error) {
	p, err := s.players.FindByUserID(ctx, s.db, userID)
	if errors.Is(err, player.ErrPlayerNotFound) {
		return MyRank{}, player.ErrNoPlayer
	}
	if err != nil {
		return MyRank{}, fmt.Errorf("find player: %w", err)
	}
	rank, err := s.repo.RankOf(ctx, s.db, p)
	if err != nil {
		return MyRank{}, fmt.Errorf("rank of: %w", err)
	}
	total, err := s.repo.Count(ctx, s.db)
	if err != nil {
		return MyRank{}, fmt.Errorf("count players: %w", err)
	}
	return MyRank{
		Rank: rank, PlayerID: p.ID, PlayerName: p.PlayerName,
		Level: p.Level, Experience: p.Experience, Total: total,
	}, nil
}

type service interface {
	Page(ctx context.Context, limit, offset int) (Page, error)
	Me(ctx context.Context, userID int64) (MyRank, error)
}

// Handler serves /api/v1/ranking (authenticated).
type Handler struct {
	svc service
}

// NewHandler returns a Handler.
func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

// Routes mounts the ranking endpoints; Authenticate must wrap them.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.page)
	r.Get("/me", h.me)
}

func (h *Handler) page(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := parsePagination(r)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	page, err := h.svc.Page(r.Context(), limit, offset)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	me, err := h.svc.Me(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, me)
}

// parsePagination reads ?limit= and ?offset=, applying defaults and bounds.
// Out-of-range values are rejected rather than silently clamped so clients
// notice their bug.
func parsePagination(r *http.Request) (limit, offset int, err error) {
	limit, offset = DefaultLimit, 0
	details := map[string]string{}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		switch {
		case err != nil:
			details["limit"] = "must be an integer"
		case n < 1 || n > MaxLimit:
			details["limit"] = fmt.Sprintf("must be between 1 and %d", MaxLimit)
		default:
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		switch {
		case err != nil:
			details["offset"] = "must be an integer"
		case n < 0:
			details["offset"] = "must be >= 0"
		default:
			offset = n
		}
	}
	if len(details) > 0 {
		return 0, 0, &httpx.Error{
			Status: http.StatusBadRequest, Code: "VALIDATION_ERROR",
			Message: "request validation failed", Details: details,
		}
	}
	return limit, offset, nil
}
