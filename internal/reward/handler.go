package reward

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/identity"
)

type service interface {
	Claim(ctx context.Context, userID int64) (ClaimResult, error)
	Status(ctx context.Context, userID int64) (Status, error)
}

// Handler serves /api/v1/rewards (authenticated).
type Handler struct {
	svc service
}

// NewHandler returns a Handler.
func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

// Routes mounts the reward endpoints; Authenticate must wrap them.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/claim", h.claim)
	r.Get("/status", h.status)
}

type claimResponse struct {
	ClaimID    int64  `json:"claim_id"`
	RewardDate string `json:"reward_date"`
	Coins      int    `json:"coins"`
	Experience int    `json:"experience"`
	Player     any    `json:"player"`
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	res, err := h.svc.Claim(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, claimResponse{
		ClaimID: res.ClaimID, RewardDate: res.RewardDate,
		Coins: res.Coins, Experience: res.Experience, Player: res.Progress,
	})
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	st, err := h.svc.Status(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, st)
}
