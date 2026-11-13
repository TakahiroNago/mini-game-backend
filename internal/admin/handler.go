package admin

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/identity"
	"github.com/takahironago/mini-game-backend/internal/item"
)

// Pagination bounds for the list endpoints.
const (
	DefaultLimit = 20
	MaxLimit     = 100
)

type service interface {
	Dashboard(ctx context.Context) (Dashboard, error)
	ListUsers(ctx context.Context, q string, limit, offset int) (UsersPage, error)
	GetUser(ctx context.Context, id int64) (UserDetail, error)
	ListItems(ctx context.Context, includeDeleted bool) ([]item.Item, error)
	CreateItem(ctx context.Context, actorUserID int64, in item.Input) (item.Item, error)
	UpdateItem(ctx context.Context, actorUserID, id int64, in item.Input) (item.Item, error)
	DeleteItem(ctx context.Context, actorUserID, id int64) error
	GrantItem(ctx context.Context, actorUserID, playerID, itemID int64, quantity int) (GrantResult, error)
	ListAuditLogs(ctx context.Context, limit, offset int) (AuditPage, error)
}

// Handler serves /api/v1/admin. Authenticate + RequireRole(admin) must wrap it.
type Handler struct {
	svc service
}

// NewHandler returns a Handler.
func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

// Routes mounts the admin endpoints.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/dashboard", h.dashboard)
	r.Get("/users", h.listUsers)
	r.Get("/users/{id}", h.getUser)
	r.Get("/items", h.listItems)
	r.Post("/items", h.createItem)
	r.Put("/items/{id}", h.updateItem)
	r.Delete("/items/{id}", h.deleteItem)
	r.Post("/players/{id}/items", h.grantItem)
	r.Get("/audit-logs", h.listAuditLogs)
}

func actor(w http.ResponseWriter, r *http.Request) (identity.Claims, bool) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
	}
	return claims, ok
}

func (h *Handler) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := h.svc.Dashboard(r.Context())
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := httpx.Pagination(r, DefaultLimit, MaxLimit)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) > 255 {
		httpx.WriteError(r.Context(), w, &httpx.Error{Status: http.StatusBadRequest, Code: "VALIDATION_ERROR",
			Message: "request validation failed", Details: map[string]string{"q": "must be at most 255 characters"}})
		return
	}
	page, err := h.svc.ListUsers(r.Context(), q, limit, offset)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}

func (h *Handler) getUser(w http.ResponseWriter, r *http.Request) {
	id, err := httpx.PathID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	d, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, d)
}

func (h *Handler) listItems(w http.ResponseWriter, r *http.Request) {
	includeDeleted := r.URL.Query().Get("include_deleted") == "true"
	items, err := h.svc.ListItems(r.Context(), includeDeleted)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type itemRequest struct {
	Name            string `json:"name" validate:"required,min=1,max=64"`
	Description     string `json:"description" validate:"max=255"`
	ItemType        string `json:"item_type" validate:"required,oneof=coin experience"`
	EffectValue     int    `json:"effect_value" validate:"required,gt=0,lte=1000000"`
	StarterQuantity int    `json:"starter_quantity" validate:"gte=0,lte=99"`
}

func (req itemRequest) input() item.Input {
	return item.Input{
		Name: strings.TrimSpace(req.Name), Description: strings.TrimSpace(req.Description),
		ItemType: req.ItemType, EffectValue: req.EffectValue, StarterQuantity: req.StarterQuantity,
	}
}

func decodeItem(w http.ResponseWriter, r *http.Request) (item.Input, error) {
	var req itemRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		return item.Input{}, err
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := httpx.Validate(req); err != nil {
		return item.Input{}, err
	}
	return req.input(), nil
}

func (h *Handler) createItem(w http.ResponseWriter, r *http.Request) {
	claims, ok := actor(w, r)
	if !ok {
		return
	}
	in, err := decodeItem(w, r)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	created, err := h.svc.CreateItem(r.Context(), claims.UserID, in)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, created)
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) {
	claims, ok := actor(w, r)
	if !ok {
		return
	}
	id, err := httpx.PathID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	in, err := decodeItem(w, r)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	updated, err := h.svc.UpdateItem(r.Context(), claims.UserID, id, in)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteItem(w http.ResponseWriter, r *http.Request) {
	claims, ok := actor(w, r)
	if !ok {
		return
	}
	id, err := httpx.PathID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if err := h.svc.DeleteItem(r.Context(), claims.UserID, id); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusNoContent, nil)
}

type grantRequest struct {
	ItemID   int64 `json:"item_id" validate:"required,gt=0"`
	Quantity int   `json:"quantity" validate:"required,gte=1,lte=999"`
}

func (h *Handler) grantItem(w http.ResponseWriter, r *http.Request) {
	claims, ok := actor(w, r)
	if !ok {
		return
	}
	playerID, err := httpx.PathID(chi.URLParam(r, "id"), "id")
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	var req grantRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if err := httpx.Validate(req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.GrantItem(r.Context(), claims.UserID, playerID, req.ItemID, req.Quantity)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

func (h *Handler) listAuditLogs(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := httpx.Pagination(r, DefaultLimit, MaxLimit)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	page, err := h.svc.ListAuditLogs(r.Context(), limit, offset)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, page)
}
