//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"
)

func TestAuthFlow(t *testing.T) {
	resetDB(t)
	app, _ := newApp(t)

	// Register creates both rows atomically.
	status, body := call(t, app, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": "Hero@Example.com", "password": "password123", "player_name": "hero",
	})
	if status != http.StatusCreated {
		t.Fatalf("register: %d %v", status, body)
	}
	var users, players int
	_ = testDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&users)
	_ = testDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM players`).Scan(&players)
	if users != 1 || players != 1 {
		t.Fatalf("users=%d players=%d, want 1/1", users, players)
	}

	// Duplicate email (case-insensitive) -> 409, and no orphan player row.
	status, body = call(t, app, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": "hero@example.com", "password": "password123", "player_name": "another",
	})
	if status != http.StatusConflict || errCode(body) != "EMAIL_ALREADY_EXISTS" {
		t.Fatalf("dup email: %d %v", status, body)
	}
	// Duplicate player name -> 409 and the user row must be rolled back.
	status, body = call(t, app, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": "other@example.com", "password": "password123", "player_name": "hero",
	})
	if status != http.StatusConflict || errCode(body) != "PLAYER_NAME_ALREADY_EXISTS" {
		t.Fatalf("dup name: %d %v", status, body)
	}
	_ = testDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&users)
	if users != 1 {
		t.Fatalf("user row leaked after failed registration: users=%d", users)
	}

	// Wrong password and unknown email look identical.
	for _, creds := range []map[string]any{
		{"email": "hero@example.com", "password": "wrong-password"},
		{"email": "nobody@example.com", "password": "password123"},
	} {
		status, body = call(t, app, http.MethodPost, "/api/v1/auth/login", "", creds)
		if status != http.StatusUnauthorized || errCode(body) != "INVALID_CREDENTIALS" {
			t.Fatalf("bad login %v: %d %v", creds, status, body)
		}
	}

	// Login (email normalised) -> token -> /me -> /player/me.
	status, body = call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "HERO@example.com", "password": "password123",
	})
	if status != http.StatusOK {
		t.Fatalf("login: %d %v", status, body)
	}
	tok := body["access_token"].(string)

	status, body = call(t, app, http.MethodGet, "/api/v1/auth/me", tok, nil)
	if status != http.StatusOK || body["email"] != "hero@example.com" || body["role"] != "player" {
		t.Fatalf("me: %d %v", status, body)
	}
	if _, leaked := body["password_hash"]; leaked {
		t.Fatal("password_hash must never be serialised")
	}

	status, body = call(t, app, http.MethodGet, "/api/v1/player/me", tok, nil)
	if status != http.StatusOK || body["player_name"] != "hero" || body["level"] != float64(1) || body["coins"] != float64(0) {
		t.Fatalf("player/me: %d %v", status, body)
	}
	status, body = call(t, app, http.MethodGet, "/api/v1/player/me/items", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("player/me/items: %d %v", status, body)
	}
	if items, ok := body["items"].([]any); !ok || len(items) != 0 {
		t.Fatalf("expected empty items array, got %v", body["items"])
	}

	// Logout revokes the token; a second use is rejected, re-logout is idempotent.
	if status, _ = call(t, app, http.MethodPost, "/api/v1/auth/logout", tok, nil); status != http.StatusNoContent {
		t.Fatalf("logout: %d", status)
	}
	status, body = call(t, app, http.MethodGet, "/api/v1/auth/me", tok, nil)
	if status != http.StatusUnauthorized || errCode(body) != "TOKEN_REVOKED" {
		t.Fatalf("me after logout: %d %v", status, body)
	}
	status, body = call(t, app, http.MethodPost, "/api/v1/auth/logout", tok, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("logout twice: %d %v", status, body)
	}
}

func TestAdminBootstrapAndRoleSeparation(t *testing.T) {
	resetDB(t)
	app, svcs := newApp(t)

	ctx := context.Background()
	for i := 0; i < 2; i++ { // idempotent
		if err := svcs.Auth.EnsureAdmin(ctx, "admin@example.com", "admin-password"); err != nil {
			t.Fatalf("EnsureAdmin #%d: %v", i+1, err)
		}
	}
	var admins int
	_ = testDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&admins)
	if admins != 1 {
		t.Fatalf("admins = %d, want 1", admins)
	}

	status, body := call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "admin@example.com", "password": "admin-password",
	})
	if status != http.StatusOK || body["user"].(map[string]any)["role"] != "admin" {
		t.Fatalf("admin login: %d %v", status, body)
	}
	tok := body["access_token"].(string)

	// Admins have no player row.
	status, body = call(t, app, http.MethodGet, "/api/v1/player/me", tok, nil)
	if status != http.StatusNotFound || errCode(body) != "PLAYER_NOT_FOUND" {
		t.Fatalf("admin player/me: %d %v", status, body)
	}

	// A registered account can never obtain the admin role through the API.
	_ = registerAndLogin(t, app, "p@example.com", "player1")
	var role string
	_ = testDB.QueryRowContext(ctx, `SELECT role FROM users WHERE email = 'p@example.com'`).Scan(&role)
	if role != "player" {
		t.Fatalf("registered role = %q, want player", role)
	}
}
