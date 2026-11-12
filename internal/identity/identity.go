// Package identity defines the authenticated principal (verified token
// claims) and how it travels through a request context. It has no
// dependencies so any domain package can read "who is calling" without
// importing the auth package (which itself depends on domain packages).
package identity

import (
	"context"
	"time"
)

// Role values stored in users.role and in the JWT.
const (
	RolePlayer = "player"
	RoleAdmin  = "admin"
)

// Claims are the verified contents of an access token.
type Claims struct {
	UserID    int64
	Role      string
	JTI       string
	ExpiresAt time.Time
}

type ctxKey struct{}

// WithClaims stores verified claims in ctx (set by the auth middleware).
func WithClaims(ctx context.Context, c Claims) context.Context {
	return context.WithValue(ctx, ctxKey{}, c)
}

// ClaimsFromContext returns the claims set by the auth middleware.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	c, ok := ctx.Value(ctxKey{}).(Claims)
	return c, ok
}
