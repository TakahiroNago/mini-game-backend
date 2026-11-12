package auth

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
)

// service is the behaviour the handler needs; an interface so handler tests
// can run without a database.
type service interface {
	Register(ctx context.Context, email, password, playerName string) (RegisterResult, error)
	Login(ctx context.Context, email, password string) (LoginResult, error)
	Logout(ctx context.Context, c Claims) error
	CurrentUser(ctx context.Context, userID int64) (User, error)
}

// Handler serves /api/v1/auth.
type Handler struct {
	svc service
}

// NewHandler returns a Handler.
func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

// PublicRoutes mounts register/login (no token required).
func (h *Handler) PublicRoutes(r chi.Router) {
	r.Post("/register", h.register)
	r.Post("/login", h.login)
}

// ProtectedRoutes mounts logout/me (Authenticate middleware must wrap these).
func (h *Handler) ProtectedRoutes(r chi.Router) {
	r.Post("/logout", h.logout)
	r.Get("/me", h.me)
}

type registerRequest struct {
	Email      string `json:"email" validate:"required,email,max=255"`
	Password   string `json:"password" validate:"required,min=8,max=72"`
	PlayerName string `json:"player_name" validate:"required,min=2,max=32,alphanumunicode"`
}

type userResponse struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
}

func toUserResponse(u User) userResponse {
	return userResponse{ID: u.ID, Email: u.Email, Role: u.Role, CreatedAt: u.CreatedAt}
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if err := httpx.Validate(req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.Register(r.Context(), req.Email, req.Password, req.PlayerName)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"user_id":   res.UserID,
		"player_id": res.PlayerID,
	})
}

type loginRequest struct {
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,max=72"`
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if err := httpx.Validate(req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"access_token": res.Token,
		"token_type":   "Bearer",
		"expires_at":   res.ExpiresAt,
		"user":         toUserResponse(res.User),
	})
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	if err := h.svc.Logout(r.Context(), claims); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	u, err := h.svc.CurrentUser(r.Context(), claims.UserID)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, toUserResponse(u))
}
