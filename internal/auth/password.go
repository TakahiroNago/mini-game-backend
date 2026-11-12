// Package auth implements registration, login, JWT issuance/verification and
// logout (token revocation).
package auth

import (
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 12 takes roughly 250ms on a laptop core: slow enough to blunt
// offline brute force, fast enough for a login endpoint.
const bcryptCost = 12

// Password length bounds. 72 is bcrypt's hard input limit; longer input is
// silently truncated by the algorithm, so we reject it explicitly instead.
const (
	MinPasswordLen = 8
	MaxPasswordLen = 72
)

// HashPassword returns the bcrypt hash of password.
func HashPassword(password string) (string, error) {
	if l := len(password); l < MinPasswordLen || l > MaxPasswordLen {
		return "", errors.New("password length out of range")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

// CheckPassword reports whether password matches hash (constant time inside bcrypt).
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// dummyHash is compared against when the user does not exist so that login
// latency does not reveal whether an email is registered.
var dummyHash = func() string {
	h, _ := bcrypt.GenerateFromPassword([]byte("dummy-password-for-timing"), bcryptCost)
	return string(h)
}()
