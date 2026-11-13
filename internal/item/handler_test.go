package item

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/identity"
	"github.com/takahironago/mini-game-backend/internal/player"
)

type useCall struct {
	userID, itemID int64
	quantity       int
	key            string
}

type stubService struct {
	useErr error
	calls  []useCall
}

func (s *stubService) List(context.Context) ([]Item, error) {
	return []Item{{ID: 1, Name: "Potion", ItemType: TypeExperience, EffectValue: 50}}, nil
}

func (s *stubService) Use(_ context.Context, userID, itemID int64, quantity int, key string) (UseResult, error) {
	s.calls = append(s.calls, useCall{userID, itemID, quantity, key})
	if s.useErr != nil {
		return UseResult{}, s.useErr
	}
	return UseResult{TransactionID: 42, ItemID: itemID, Quantity: quantity,
		Progress: player.Progress{Level: 2, Experience: 0, Coins: 0}}, nil
}

func newRouter(svc *stubService) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.WithClaims(r.Context(),
				identity.Claims{UserID: 7, Role: identity.RolePlayer, JTI: "x"})))
		})
	})
	NewHandler(svc).Routes(r)
	return r
}

func postUse(h http.Handler, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/use", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeErrCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body.Error.Code
}

func TestUse_RejectsBadIdempotencyKey(t *testing.T) {
	for name, key := range map[string]string{
		"missing":   "",
		"too long":  strings.Repeat("k", 65),
		"has space": "key with space",
		"has slash": "a/b",
	} {
		t.Run(name, func(t *testing.T) {
			svc := &stubService{}
			rec := postUse(newRouter(svc), key, `{"item_id":1,"quantity":1}`)
			if rec.Code != http.StatusBadRequest || decodeErrCode(t, rec) != "IDEMPOTENCY_KEY_REQUIRED" {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if len(svc.calls) != 0 {
				t.Fatal("service must not be called without a valid key")
			}
		})
	}
}

func TestUse_Validation(t *testing.T) {
	for name, body := range map[string]string{
		"missing item_id":   `{"quantity":1}`,
		"negative quantity": `{"item_id":1,"quantity":-1}`,
		"quantity too high": `{"item_id":1,"quantity":100}`,
		"unknown field":     `{"item_id":1,"player_id":99}`,
		"malformed":         `{"item_id":`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &stubService{}
			rec := postUse(newRouter(svc), "550e8400-e29b-41d4-a716-446655440000", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if len(svc.calls) != 0 {
				t.Fatal("service must not be called on validation failure")
			}
		})
	}
}

func TestUse_DefaultsQuantityToOne(t *testing.T) {
	svc := &stubService{}
	rec := postUse(newRouter(svc), "key-1", `{"item_id":3}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(svc.calls) != 1 || svc.calls[0] != (useCall{userID: 7, itemID: 3, quantity: 1, key: "key-1"}) {
		t.Fatalf("unexpected service call: %+v", svc.calls)
	}
	var body struct {
		TransactionID int64 `json:"transaction_id"`
		Replayed      bool  `json:"replayed"`
		Player        struct {
			Level int `json:"level"`
		} `json:"player"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.TransactionID != 42 || body.Replayed || body.Player.Level != 2 {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestUse_ErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrItemNotFound, http.StatusNotFound, "ITEM_NOT_FOUND"},
		{ErrInsufficientItem, http.StatusConflict, "INSUFFICIENT_ITEM"},
		{ErrKeyMismatch, http.StatusUnprocessableEntity, "IDEMPOTENCY_KEY_MISMATCH"},
		{player.ErrNoPlayer, http.StatusNotFound, "PLAYER_NOT_FOUND"},
	} {
		rec := postUse(newRouter(&stubService{useErr: tt.err}), "key-2", `{"item_id":1}`)
		if rec.Code != tt.status || decodeErrCode(t, rec) != tt.code {
			t.Fatalf("%v: status = %d body = %s", tt.err, rec.Code, rec.Body.String())
		}
	}
}

func TestList(t *testing.T) {
	rec := httptest.NewRecorder()
	newRouter(&stubService{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"name":"Potion"`) {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}
