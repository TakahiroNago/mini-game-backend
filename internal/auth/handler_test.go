package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type stubService struct {
	registerErr error
	loginErr    error
	registered  []string
}

func (s *stubService) Register(_ context.Context, email, _, playerName string) (RegisterResult, error) {
	s.registered = append(s.registered, email+"/"+playerName)
	if s.registerErr != nil {
		return RegisterResult{}, s.registerErr
	}
	return RegisterResult{UserID: 1, PlayerID: 2}, nil
}

func (s *stubService) Login(context.Context, string, string) (LoginResult, error) {
	if s.loginErr != nil {
		return LoginResult{}, s.loginErr
	}
	return LoginResult{Token: "tok", ExpiresAt: time.Unix(0, 0).UTC(), User: User{ID: 1, Email: "a@b.c", Role: RolePlayer}}, nil
}

func (s *stubService) Logout(context.Context, Claims) error { return nil }

func (s *stubService) CurrentUser(context.Context, int64) (User, error) {
	return User{ID: 1, Email: "a@b.c", Role: RolePlayer}, nil
}

func newRouter(svc *stubService) http.Handler {
	r := chi.NewRouter()
	h := NewHandler(svc)
	r.Group(h.PublicRoutes)
	r.Group(func(pr chi.Router) {
		// Simulate the auth middleware having run.
		pr.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), Claims{UserID: 1, Role: RolePlayer, JTI: "x"})))
			})
		})
		h.ProtectedRoutes(pr)
	})
	return r
}

func post(h http.Handler, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
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

func TestRegister_Validation(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"missing fields", `{}`},
		{"bad email", `{"email":"nope","password":"password123","player_name":"hero"}`},
		{"short password", `{"email":"a@b.c","password":"short","player_name":"hero"}`},
		{"player name too short", `{"email":"a@b.c","password":"password123","player_name":"h"}`},
		{"player name with symbols", `{"email":"a@b.c","password":"password123","player_name":"he ro'; DROP TABLE users;--"}`},
		{"unknown field", `{"email":"a@b.c","password":"password123","player_name":"hero","role":"admin"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := &stubService{}
			rec := post(newRouter(svc), "/register", tt.body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body %s)", rec.Code, rec.Body.String())
			}
			if len(svc.registered) != 0 {
				t.Fatal("service must not be called on validation failure")
			}
		})
	}
}

func TestRegister_Success(t *testing.T) {
	svc := &stubService{}
	rec := post(newRouter(svc), "/register", `{"email":"a@b.c","password":"password123","player_name":"hero"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestRegister_Conflict(t *testing.T) {
	rec := post(newRouter(&stubService{registerErr: ErrEmailTaken}), "/register",
		`{"email":"a@b.c","password":"password123","player_name":"hero"}`)
	if rec.Code != http.StatusConflict || errCode(t, rec) != "EMAIL_ALREADY_EXISTS" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestLogin(t *testing.T) {
	rec := post(newRouter(&stubService{}), "/login", `{"email":"a@b.c","password":"password123"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&body)
	if body.AccessToken != "tok" || body.TokenType != "Bearer" {
		t.Fatalf("unexpected body: %+v", body)
	}

	rec = post(newRouter(&stubService{loginErr: ErrInvalidLogin}), "/login", `{"email":"a@b.c","password":"password123"}`)
	if rec.Code != http.StatusUnauthorized || errCode(t, rec) != "INVALID_CREDENTIALS" {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestMeAndLogout(t *testing.T) {
	r := newRouter(&stubService{})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"email":"a@b.c"`) {
		t.Fatalf("me: status = %d body = %s", rec.Code, rec.Body.String())
	}
	if rec := post(r, "/logout", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("logout: status = %d", rec.Code)
	}
}
