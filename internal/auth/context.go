package auth

import "github.com/takahironago/mini-game-backend/internal/identity"

// Re-exported from identity so auth code and its callers read naturally.
type Claims = identity.Claims

const (
	RolePlayer = identity.RolePlayer
	RoleAdmin  = identity.RoleAdmin
)

// WithClaims and ClaimsFromContext are the identity helpers.
var (
	WithClaims        = identity.WithClaims
	ClaimsFromContext = identity.ClaimsFromContext
)
