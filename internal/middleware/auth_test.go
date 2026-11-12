package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/takahironago/mini-game-backend/internal/auth"
)

type revokedSet map[string]bool

func (r revokedSet) IsRevoked(_ context.Context, jti string) (bool, error) { return r[jti], nil }

type failingParser struct{ err error }

func (p failingParser) Parse(string) (auth.Claims, error) { return auth.Claims{}, p.err }

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, _ := auth.ClaimsFromContext(r.Context())
		w.Header().Set("X-User", c.Role)
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthenticate(t *testing.T) {
	iss := auth.NewTokenIssuer("secret", time.Hour, nil)
	playerTok, playerClaims, _ := iss.Issue(1, auth.RolePlayer)
	adminTok, _, _ := iss.Issue(2, auth.RoleAdmin)
	revoked := revokedSet{playerClaims.JTI: true}

	mw := Authenticate(iss, revokedSet{})
	tests := []struct {
		name   string
		header string
		mw     func(http.Handler) http.Handler
		want   int
		role   string
	}{
		{"no header", "", mw, http.StatusUnauthorized, ""},
		{"wrong scheme", "Basic xyz", mw, http.StatusUnauthorized, ""},
		{"garbage token", "Bearer abc.def.ghi", mw, http.StatusUnauthorized, ""},
		{"valid player", "Bearer " + playerTok, mw, http.StatusOK, auth.RolePlayer},
		{"valid admin, case-insensitive scheme", "bearer " + adminTok, mw, http.StatusOK, auth.RoleAdmin},
		{"revoked", "Bearer " + playerTok, Authenticate(iss, revoked), http.StatusUnauthorized, ""},
		{"expired", "Bearer " + playerTok, Authenticate(failingParser{auth.ErrTokenExpired}, revokedSet{}), http.StatusUnauthorized, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set("Authorization", tt.header)
			}
			rec := httptest.NewRecorder()
			tt.mw(okHandler()).ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, tt.want, rec.Body.String())
			}
			if tt.role != "" && rec.Header().Get("X-User") != tt.role {
				t.Fatalf("role = %q, want %q", rec.Header().Get("X-User"), tt.role)
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	iss := auth.NewTokenIssuer("secret", time.Hour, nil)
	playerTok, _, _ := iss.Issue(1, auth.RolePlayer)
	adminTok, _, _ := iss.Issue(2, auth.RoleAdmin)
	chain := Authenticate(iss, revokedSet{})(RequireRole(auth.RoleAdmin)(okHandler()))

	for _, tt := range []struct {
		name  string
		token string
		want  int
	}{
		{"player is forbidden", playerTok, http.StatusForbidden},
		{"admin is allowed", adminTok, http.StatusOK},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			rec := httptest.NewRecorder()
			chain.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}

	t.Run("without Authenticate it is 401", func(t *testing.T) {
		rec := httptest.NewRecorder()
		RequireRole(auth.RoleAdmin)(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
	})
}
