package item

import (
	"context"
	"net/http"
	"regexp"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/identity"
)

// IdempotencyKeyHeader must accompany every POST /items/use.
const IdempotencyKeyHeader = "Idempotency-Key"

// MaxUseQuantity bounds a single use so one request cannot loop for long in
// the level-up calculation or overflow coins.
const MaxUseQuantity = 99

var (
	// idempotencyKeyRe accepts UUIDs and similar opaque client tokens.
	idempotencyKeyRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)

	errKeyRequired = httpx.NewError(http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED",
		"Idempotency-Key header is required (1-64 chars: letters, digits, - _ . :)")
)

type service interface {
	List(ctx context.Context) ([]Item, error)
	Use(ctx context.Context, userID, itemID int64, quantity int, idempotencyKey string) (UseResult, error)
}

// Handler serves /api/v1/items (authenticated).
type Handler struct {
	svc service
}

// NewHandler returns a Handler.
func NewHandler(svc service) *Handler { return &Handler{svc: svc} }

// Routes mounts the item endpoints; Authenticate must wrap them.
func (h *Handler) Routes(r chi.Router) {
	r.Get("/", h.list)
	r.Post("/use", h.use)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	items, err := h.svc.List(r.Context())
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

type useRequest struct {
	ItemID   int64 `json:"item_id" validate:"required,gt=0"`
	Quantity int   `json:"quantity" validate:"gte=1,lte=99"`
}

type useResponse struct {
	TransactionID int64 `json:"transaction_id"`
	ItemID        int64 `json:"item_id"`
	Quantity      int   `json:"quantity"`
	Replayed      bool  `json:"replayed"`
	Player        any   `json:"player"`
}

func (h *Handler) use(w http.ResponseWriter, r *http.Request) {
	claims, ok := identity.ClaimsFromContext(r.Context())
	if !ok {
		httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
		return
	}
	key := r.Header.Get(IdempotencyKeyHeader)
	if !idempotencyKeyRe.MatchString(key) {
		httpx.WriteError(r.Context(), w, errKeyRequired)
		return
	}
	var req useRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	if req.Quantity == 0 {
		req.Quantity = 1 // omitted quantity means one unit
	}
	if err := httpx.Validate(req); err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	res, err := h.svc.Use(r.Context(), claims.UserID, req.ItemID, req.Quantity, key)
	if err != nil {
		httpx.WriteError(r.Context(), w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, useResponse{
		TransactionID: res.TransactionID,
		ItemID:        res.ItemID,
		Quantity:      res.Quantity,
		Replayed:      res.Replayed,
		Player:        res.Progress,
	})
}
