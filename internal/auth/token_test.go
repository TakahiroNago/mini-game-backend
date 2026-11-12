package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func fixedNow(t time.Time) func() time.Time { return func() time.Time { return t } }

func TestTokenIssueAndParse(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	iss := NewTokenIssuer("secret-for-tests", time.Hour, fixedNow(now))

	tok, issued, err := iss.Issue(42, RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	got, err := iss.Parse(tok)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got.UserID != 42 || got.Role != RoleAdmin || got.JTI != issued.JTI || len(got.JTI) != 32 {
		t.Fatalf("claims = %+v, want user 42 admin jti %s", got, issued.JTI)
	}
	if !got.ExpiresAt.Equal(now.Add(time.Hour)) {
		t.Fatalf("expires_at = %v, want %v", got.ExpiresAt, now.Add(time.Hour))
	}
}

func TestTokenParse_Rejections(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	iss := NewTokenIssuer("secret-for-tests", time.Hour, fixedNow(now))
	tok, _, _ := iss.Issue(1, RolePlayer)

	t.Run("expired", func(t *testing.T) {
		later := NewTokenIssuer("secret-for-tests", time.Hour, fixedNow(now.Add(2*time.Hour)))
		if _, err := later.Parse(tok); !errors.Is(err, ErrTokenExpired) {
			t.Fatalf("err = %v, want ErrTokenExpired", err)
		}
	})
	t.Run("wrong secret", func(t *testing.T) {
		other := NewTokenIssuer("another-secret", time.Hour, fixedNow(now))
		if _, err := other.Parse(tok); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("err = %v, want ErrTokenInvalid", err)
		}
	})
	t.Run("tampered", func(t *testing.T) {
		if _, err := iss.Parse(tok[:len(tok)-3] + "abc"); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("err = %v, want ErrTokenInvalid", err)
		}
	})
	t.Run("alg none", func(t *testing.T) {
		unsigned := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
			"sub": "1", "role": "admin", "jti": "0123456789abcdef0123456789abcdef", "iss": issuer,
			"exp": now.Add(time.Hour).Unix(),
		})
		s, err := unsigned.SignedString(jwt.UnsafeAllowNoneSignatureType)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := iss.Parse(s); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("err = %v, want ErrTokenInvalid", err)
		}
	})
	t.Run("wrong issuer", func(t *testing.T) {
		c := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "1", "role": "player", "jti": "0123456789abcdef0123456789abcdef", "iss": "someone-else",
			"exp": now.Add(time.Hour).Unix(),
		})
		s, _ := c.SignedString([]byte("secret-for-tests"))
		if _, err := iss.Parse(s); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("err = %v, want ErrTokenInvalid", err)
		}
	})
	t.Run("unknown role", func(t *testing.T) {
		c := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
			"sub": "1", "role": "superuser", "jti": "0123456789abcdef0123456789abcdef", "iss": issuer,
			"exp": now.Add(time.Hour).Unix(),
		})
		s, _ := c.SignedString([]byte("secret-for-tests"))
		if _, err := iss.Parse(s); !errors.Is(err, ErrTokenInvalid) {
			t.Fatalf("err = %v, want ErrTokenInvalid", err)
		}
	})
}

func TestPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct horse battery" {
		t.Fatal("hash must not equal the password")
	}
	if !CheckPassword(hash, "correct horse battery") {
		t.Fatal("expected password to verify")
	}
	if CheckPassword(hash, "wrong password") {
		t.Fatal("wrong password must not verify")
	}
	if _, err := HashPassword("short"); err == nil {
		t.Fatal("expected error for short password")
	}
	if _, err := HashPassword(string(make([]byte, 73))); err == nil {
		t.Fatal("expected error for password over 72 bytes")
	}
}
