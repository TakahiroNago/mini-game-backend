package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/identity"
	"github.com/takahironago/mini-game-backend/internal/item"
)

type stubService struct {
	created []item.Input
	grants  []int
	deleted []int64
	err     error
}

func (s *stubService) Dashboard(context.Context) (Dashboard, error) {
	return Dashboard{Users: 3}, s.err
}
func (s *stubService) ListUsers(_ context.Context, q string, limit, offset int) (UsersPage, error) {
	return UsersPage{Users: []UserSummary{}, Limit: limit, Offset: offset, Total: 0}, s.err
}
func (s *stubService) GetUser(_ context.Context, id int64) (UserDetail, error) {
	return UserDetail{User: UserSummary{ID: id, Email: "a@b.c", Role: "player"}}, s.err
}
func (s *stubService) ListItems(context.Context, bool) ([]item.Item, error) {
	return []item.Item{{ID: 1, Name: "Potion"}}, s.err
}
func (s *stubService) CreateItem(_ context.Context, _ int64, in item.Input) (item.Item, error) {
	s.created = append(s.created, in)
	return item.Item{ID: 9, Name: in.Name, ItemType: in.ItemType, EffectValue: in.EffectValue}, s.err
}
func (s *stubService) UpdateItem(_ context.Context, _ int64, id int64, in item.Input) (item.Item, error) {
	return item.Item{ID: id, Name: in.Name}, s.err
}
func (s *stubService) DeleteItem(_ context.Context, _ int64, id int64) error {
	s.deleted = append(s.deleted, id)
	return s.err
}
func (s *stubService) GrantItem(_ context.Context, _ int64, _, _ int64, quantity int) (GrantResult, error) {
	s.grants = append(s.grants, quantity)
	return GrantResult{Quantity: quantity}, s.err
}
func (s *stubService) ListAuditLogs(_ context.Context, limit, offset int) (AuditPage, error) {
	return AuditPage{Logs: []AuditLog{}, Limit: limit, Offset: offset}, s.err
}

func newRouter(svc *stubService) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.WithClaims(r.Context(),
				identity.Claims{UserID: 1, Role: identity.RoleAdmin, JTI: "x"})))
		})
	})
	NewHandler(svc).Routes(r)
	return r
}

func do(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code    string            `json:"code"`
			Details map[string]string `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return body.Error.Code
}

func TestCreateItem_Validation(t *testing.T) {
	for name, body := range map[string]string{
		"missing name":         `{"item_type":"coin","effect_value":10}`,
		"bad type":             `{"name":"x","item_type":"gold","effect_value":10}`,
		"zero effect":          `{"name":"x","item_type":"coin","effect_value":0}`,
		"negative starter":     `{"name":"x","item_type":"coin","effect_value":1,"starter_quantity":-1}`,
		"starter too high":     `{"name":"x","item_type":"coin","effect_value":1,"starter_quantity":100}`,
		"name too long":        `{"name":"` + strings.Repeat("n", 65) + `","item_type":"coin","effect_value":1}`,
		"whitespace-only name": `{"name":"   ","item_type":"coin","effect_value":1}`,
		"unknown field":        `{"name":"x","item_type":"coin","effect_value":1,"deleted_at":null}`,
	} {
		t.Run(name, func(t *testing.T) {
			svc := &stubService{}
			rec := do(newRouter(svc), http.MethodPost, "/items", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if len(svc.created) != 0 {
				t.Fatal("service must not be called on validation failure")
			}
		})
	}
}

func TestCreateItem_Success(t *testing.T) {
	svc := &stubService{}
	rec := do(newRouter(svc), http.MethodPost, "/items",
		`{"name":"  Elixir ","description":"d","item_type":"experience","effect_value":300,"starter_quantity":2}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if len(svc.created) != 1 || svc.created[0] != (item.Input{Name: "Elixir", Description: "d", ItemType: "experience", EffectValue: 300, StarterQuantity: 2}) {
		t.Fatalf("unexpected input: %+v", svc.created)
	}
}

func TestPathIDValidation(t *testing.T) {
	svc := &stubService{}
	r := newRouter(svc)
	for _, p := range []struct{ method, path, body string }{
		{http.MethodGet, "/users/abc", ""},
		{http.MethodGet, "/users/0", ""},
		{http.MethodPut, "/items/-1", `{"name":"x","item_type":"coin","effect_value":1}`},
		{http.MethodDelete, "/items/x", ""},
		{http.MethodPost, "/players/nope/items", `{"item_id":1,"quantity":1}`},
	} {
		rec := do(r, p.method, p.path, p.body)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "VALIDATION_ERROR" {
			t.Fatalf("%s %s: status = %d body = %s", p.method, p.path, rec.Code, rec.Body.String())
		}
	}
	if len(svc.deleted) != 0 || len(svc.grants) != 0 {
		t.Fatal("service must not be called with an invalid id")
	}
}

func TestGrantItem(t *testing.T) {
	svc := &stubService{}
	r := newRouter(svc)
	for _, body := range []string{`{"item_id":1}`, `{"item_id":1,"quantity":0}`, `{"item_id":1,"quantity":1000}`, `{"quantity":1}`} {
		if rec := do(r, http.MethodPost, "/players/5/items", body); rec.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d", body, rec.Code)
		}
	}
	rec := do(r, http.MethodPost, "/players/5/items", `{"item_id":1,"quantity":3}`)
	if rec.Code != http.StatusOK || len(svc.grants) != 1 || svc.grants[0] != 3 {
		t.Fatalf("status = %d grants = %v body = %s", rec.Code, svc.grants, rec.Body.String())
	}
}

func TestErrorMapping(t *testing.T) {
	for _, tt := range []struct {
		err    error
		status int
		code   string
	}{
		{ErrNoUser, http.StatusNotFound, "USER_NOT_FOUND"},
		{ErrNoItem, http.StatusNotFound, "ITEM_NOT_FOUND"},
		{ErrNoPlayer, http.StatusNotFound, "PLAYER_NOT_FOUND"},
	} {
		rec := do(newRouter(&stubService{err: tt.err}), http.MethodGet, "/users/1", "")
		if rec.Code != tt.status || errCode(t, rec) != tt.code {
			t.Fatalf("%v: status = %d body = %s", tt.err, rec.Code, rec.Body.String())
		}
	}
}

func TestListsAndDelete(t *testing.T) {
	svc := &stubService{}
	r := newRouter(svc)
	if rec := do(r, http.MethodGet, "/dashboard", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"users":3`) {
		t.Fatalf("dashboard: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(r, http.MethodGet, "/users?limit=0", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("users bad limit: %d", rec.Code)
	}
	if rec := do(r, http.MethodGet, "/users?limit=5&offset=10&q=hero", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"limit":5`) {
		t.Fatalf("users: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(r, http.MethodGet, "/items?include_deleted=true", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"Potion"`) {
		t.Fatalf("items: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(r, http.MethodDelete, "/items/7", ""); rec.Code != http.StatusNoContent || len(svc.deleted) != 1 || svc.deleted[0] != 7 {
		t.Fatalf("delete: %d deleted = %v", rec.Code, svc.deleted)
	}
	if rec := do(r, http.MethodGet, "/audit-logs?limit=101", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("audit bad limit: %d", rec.Code)
	}
	if rec := do(r, http.MethodGet, "/audit-logs", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"logs":[]`) {
		t.Fatalf("audit: %d %s", rec.Code, rec.Body.String())
	}
}
