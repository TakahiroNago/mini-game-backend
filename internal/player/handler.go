package player

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/identity"
)

// ErrNoPlayer is returned for accounts without a player row (admins).
var ErrNoPlayer = httpx.NewError(http.StatusNotFound, "PLAYER_NOT_FOUND", "no player exists for this account")

// Service exposes read models for the current player.
type Service struct {
	db   *sql.DB
	repo *Repository
}

// NewService returns a Service.
func NewService(db *sql.DB, repo *Repository) *Service { return &Service{db: db, repo: repo} }

// Me returns the player owned by userID.
func (s *Service) Me(ctx context.Context, userID int64) (Player, error) {
	p, err := s.repo.FindByUserID(ctx, s.db, userID)
	if errors.Is(err, ErrPlayerNotFound) {
		return Player{}, ErrNoPlayer
	}
	return p, err
}

// MyItems returns the inventory of the player owned by userID.
func (s *Service) MyItems(ctx context.Context, userID int64) ([]InventoryItem, error) {
	p, err := s.Me(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListInventory(ctx, s.db, p.ID)
}

type service interface {
	Me(ctx context.Context, userID int64) (Player, error)
	MyItems(ctx context.Context, userID int64) ([]InventoryItem, error)
}

// Handler serves /api/v1/player (authenticated).
type Handler struct {
	svc service
}

// NewHandler returns a Handler.
func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

// Routes mounts the player endpoints; Authenticate must wrap them.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/me", h.me)
	r.Get("/me/items", h.myItems)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	p, err := h.svc.Me(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) myItems(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	items, err := h.svc.MyItems(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}
