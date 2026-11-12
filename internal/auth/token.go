package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "mini-game-backend"

// jwtClaims is the on-the-wire shape. UserID rides in the standard "sub".
type jwtClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Token errors returned by Parse.
var (
	ErrTokenInvalid = errors.New("token invalid")
	ErrTokenExpired = errors.New("token expired")
)

// TokenIssuer signs and verifies HS256 access tokens.
type TokenIssuer struct {
	secret []byte
	expiry time.Duration
	now    func() time.Time
}

// NewTokenIssuer builds an issuer. now is injectable for tests; nil means time.Now.
func NewTokenIssuer(secret string, expiry time.Duration, now func() time.Time) *TokenIssuer {
	if now == nil {
		now = time.Now
	}
	return &TokenIssuer{secret: []byte(secret), expiry: expiry, now: now}
}

// Issue creates a signed token for the user and returns it with its claims.
func (t *TokenIssuer) Issue(userID int64, role string) (string, Claims, error) {
	jti, err := newJTI()
	if err != nil {
		return "", Claims{}, err
	}
	now := t.now()
	exp := now.Add(t.expiry)
	c := jwtClaims{
		Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   fmt.Sprint(userID),
			ID:        jti,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(t.secret)
	if err != nil {
		return "", Claims{}, fmt.Errorf("sign token: %w", err)
	}
	return signed, Claims{UserID: userID, Role: role, JTI: jti, ExpiresAt: exp}, nil
}

// Parse verifies signature, algorithm, issuer and expiry, then returns the claims.
func (t *TokenIssuer) Parse(tokenString string) (Claims, error) {
	var c jwtClaims
	_, err := jwt.ParseWithClaims(tokenString, &c,
		func(*jwt.Token) (any, error) { return t.secret, nil },
		// Pinning the algorithm defeats "alg: none" / RS->HS confusion attacks.
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(t.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrTokenExpired
		}
		return Claims{}, ErrTokenInvalid
	}
	var userID int64
	if _, err := fmt.Sscan(c.Subject, &userID); err != nil || userID <= 0 {
		return Claims{}, ErrTokenInvalid
	}
	if c.Role != RolePlayer && c.Role != RoleAdmin {
		return Claims{}, ErrTokenInvalid
	}
	if len(c.ID) != 32 {
		return Claims{}, ErrTokenInvalid
	}
	return Claims{UserID: userID, Role: c.Role, JTI: c.ID, ExpiresAt: c.ExpiresAt.Time}, nil
}

// newJTI returns 128 random bits as 32 hex chars (matches revoked_tokens.jti CHAR(32)).
func newJTI() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate jti: %w", err)
	}
	return hex.EncodeToString(b), nil
}
