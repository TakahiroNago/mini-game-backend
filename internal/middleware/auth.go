package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/takahironago/mini-game-backend/internal/auth"
	"github.com/takahironago/mini-game-backend/internal/httpx"
)

// TokenParser verifies an access token (satisfied by *auth.TokenIssuer).
type TokenParser interface {
	Parse(token string) (auth.Claims, error)
}

// RevocationChecker reports whether a token id was revoked by logout.
type RevocationChecker interface {
	IsRevoked(ctx context.Context, jti string) (bool, error)
}

var (
	errTokenRevoked = httpx.NewError(http.StatusUnauthorized, "TOKEN_REVOKED", "token has been revoked")
	errTokenExpired = httpx.NewError(http.StatusUnauthorized, "TOKEN_EXPIRED", "token has expired")
)

// Authenticate requires a valid "Authorization: Bearer <jwt>" header and
// stores the claims in the request context.
func Authenticate(parser TokenParser, revoked RevocationChecker) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			raw, ok := bearerToken(r.Header.Get("Authorization"))
			if !ok {
				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
				return
			}
			claims, err := parser.Parse(raw)
			if err != nil {
				if errors.Is(err, auth.ErrTokenExpired) {
					httpx.WriteError(r.Context(), w, errTokenExpired)
					return
				}
				httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
				return
			}
			isRevoked, err := revoked.IsRevoked(r.Context(), claims.JTI)
			if err != nil {
				httpx.WriteError(r.Context(), w, err)
				return
			}
			if isRevoked {
				httpx.WriteError(r.Context(), w, errTokenRevoked)
				return
			}
			next.ServeHTTP(w, r.WithContext(auth.WithClaims(r.Context(), claims)))
		})
	}
}

// RequireRole rejects authenticated requests whose role does not match.
// It must run after Authenticate.
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.ClaimsFromContext(r.Context())
			if !ok {
				httpx.WriteError(r.Context(), w, httpx.ErrUnauthorized)
				return
			}
			if claims.Role != role {
				httpx.WriteError(r.Context(), w, httpx.ErrForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(header string) (string, bool) {
	const prefix = "bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}
