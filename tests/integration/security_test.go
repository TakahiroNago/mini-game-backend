//go:build integration

package integration

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/takahironago/mini-game-backend/internal/auth"
	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/internal/server"
)

// newAppWith builds the router with a modified copy of the test config.
func newAppWith(t *testing.T, mutate func(*config.Config)) (http.Handler, *server.Services) {
	t.Helper()
	cfg := testCfg
	mutate(&cfg)
	deps := server.Deps{
		Config:  cfg,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      testDB,
		Version: "integration",
	}
	svcs := server.NewServices(deps)
	return server.NewRouter(deps, svcs), svcs
}

// rawCall sends an arbitrary body (not necessarily JSON) and returns the recorder.
func rawCall(t *testing.T, h http.Handler, method, path, token, contentType, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// sqlInjectionStrings are the classic payloads; every one must be rejected
// by validation or treated as plain data by the placeholder-only SQL.
var sqlInjectionStrings = []string{
	`' OR '1'='1`,
	`'; DROP TABLE users; --`,
	`admin'--`,
	`" OR ""="`,
	`1; SELECT SLEEP(5)`,
	`' UNION SELECT password_hash FROM users --`,
}

func TestSecurityMalformedInput(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")

	cases := []struct {
		name, method, path, token, body string
		wantStatus                      int
		wantCode                        string
	}{
		{"empty body", http.MethodPost, "/api/v1/auth/login", "", "", 400, "BAD_REQUEST"},
		{"not json", http.MethodPost, "/api/v1/auth/login", "", "email=a&password=b", 400, "BAD_REQUEST"},
		{"truncated json", http.MethodPost, "/api/v1/auth/login", "", `{"email":"a@example.com"`, 400, "BAD_REQUEST"},
		{"trailing data", http.MethodPost, "/api/v1/auth/login", "", `{"email":"a@example.com","password":"password123"} {}`, 400, "BAD_REQUEST"},
		{"unknown field", http.MethodPost, "/api/v1/auth/login", "", `{"email":"a@example.com","password":"password123","role":"admin"}`, 400, "BAD_REQUEST"},
		{"wrong type", http.MethodPost, "/api/v1/auth/login", "", `{"email":123,"password":"password123"}`, 400, "BAD_REQUEST"},
		{"json array", http.MethodPost, "/api/v1/auth/login", "", `[]`, 400, "BAD_REQUEST"},
		{"null body", http.MethodPost, "/api/v1/auth/login", "", `null`, 400, "VALIDATION_ERROR"},
		{"missing password", http.MethodPost, "/api/v1/auth/login", "", `{"email":"a@example.com"}`, 400, "VALIDATION_ERROR"},
		{"invalid email", http.MethodPost, "/api/v1/auth/register", "", `{"email":"not-an-email","password":"password123","player_name":"abc"}`, 400, "VALIDATION_ERROR"},
		{"short password", http.MethodPost, "/api/v1/auth/register", "", `{"email":"a@example.com","password":"short","player_name":"abc"}`, 400, "VALIDATION_ERROR"},
		{"player name with symbols", http.MethodPost, "/api/v1/auth/register", "", `{"email":"a@example.com","password":"password123","player_name":"a b;c"}`, 400, "VALIDATION_ERROR"},
		{"use without key is refused before the body is read", http.MethodPost, "/api/v1/items/use", tok, `{"item_id":"1","quantity":1}`, 400, "IDEMPOTENCY_KEY_REQUIRED"},
		{"bad limit", http.MethodGet, "/api/v1/ranking?limit=abc", tok, "", 400, "VALIDATION_ERROR"},
		{"negative offset", http.MethodGet, "/api/v1/ranking?offset=-1", tok, "", 400, "VALIDATION_ERROR"},
		{"limit over max", http.MethodGet, "/api/v1/ranking?limit=1000", tok, "", 400, "VALIDATION_ERROR"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := rawCall(t, app, c.method, c.path, c.token, "application/json", c.body)
			if rec.Code != c.wantStatus {
				t.Fatalf("status = %d, want %d (body %s)", rec.Code, c.wantStatus, rec.Body.String())
			}
			if c.wantCode != "" && !strings.Contains(rec.Body.String(), `"code":"`+c.wantCode+`"`) {
				t.Fatalf("body = %s, want code %s", rec.Body.String(), c.wantCode)
			}
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("Content-Type = %q", ct)
			}
		})
	}

	// Error bodies never leak internals.
	rec := rawCall(t, app, http.MethodPost, "/api/v1/auth/login", "", "application/json", `{"email":`)
	for _, leak := range []string{"sql", "SQL", "goroutine", "panic", ".go:", "mysql"} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("error body leaks %q: %s", leak, rec.Body.String())
		}
	}
}

func TestSecuritySQLInjectionIsInert(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	adminTok := loginAdmin(t, app, svcs.Auth.EnsureAdmin)
	playerTok := registerAndLogin(t, app, "hero@example.com", "hero")

	tableCount := func() int {
		return countRows(t, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()`)
	}
	tablesBefore := tableCount()
	usersBefore := countRows(t, `SELECT COUNT(*) FROM users`)

	for _, payload := range sqlInjectionStrings {
		// Login: the email validator rejects most payloads; the rest hit a
		// placeholder query and simply do not match.
		status, body := call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"email": payload, "password": payload,
		})
		if status != http.StatusBadRequest && status != http.StatusUnauthorized {
			t.Fatalf("login %q: %d %v", payload, status, body)
		}
		status, body = call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"email": "hero@example.com", "password": payload,
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("login with injected password %q: %d %v", payload, status, body)
		}

		// Register: player_name is alphanumunicode so quotes never reach SQL;
		// the email field goes through the email validator.
		status, body = call(t, app, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
			"email": payload + "@example.com", "password": "password123", "player_name": payload,
		})
		if status != http.StatusBadRequest {
			t.Fatalf("register %q: %d %v", payload, status, body)
		}

		// Admin search is a free-text LIKE: the payload must be data only.
		status, body = call(t, app, http.MethodGet, "/api/v1/admin/users?q="+urlEscape(payload), adminTok, nil)
		if status != http.StatusOK {
			t.Fatalf("admin search %q: %d %v", payload, status, body)
		}
		if n := len(body["users"].([]any)); n != 0 {
			t.Fatalf("admin search %q matched %d users, want 0", payload, n)
		}

		// Item names are stored verbatim (admin-only input) and come back unchanged.
		status, body = call(t, app, http.MethodPost, "/api/v1/admin/items", adminTok, map[string]any{
			"name": payload, "description": payload, "item_type": "coin", "effect_value": 1,
		})
		if status != http.StatusCreated || body["name"] != payload {
			t.Fatalf("create item %q: %d %v", payload, status, body)
		}
	}

	// Path parameters are parsed as integers; anything else is a 400, not SQL.
	for _, raw := range []string{"1%20OR%201=1", "1;DROP", "abc", "-1", "0", "1.5", "99999999999999999999"} {
		status, body := call(t, app, http.MethodGet, "/api/v1/admin/users/"+raw, adminTok, nil)
		if status != http.StatusBadRequest && status != http.StatusNotFound {
			t.Fatalf("users/%s: %d %v", raw, status, body)
		}
	}
	_ = playerTok

	if tableCount() != tablesBefore {
		t.Fatal("table count changed: injection executed DDL")
	}
	if n := countRows(t, `SELECT COUNT(*) FROM users`); n != usersBefore {
		t.Fatalf("users = %d, want %d", n, usersBefore)
	}
}

func urlEscape(s string) string {
	r := strings.NewReplacer("%", "%25", " ", "%20", "'", "%27", `"`, "%22", ";", "%3B", "#", "%23", "&", "%26", "=", "%3D", "+", "%2B")
	return r.Replace(s)
}

func TestSecurityTokens(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, _ := newApp(t)
	tok := registerAndLogin(t, app, "hero@example.com", "hero")
	status, body := call(t, app, http.MethodGet, "/api/v1/auth/me", tok, nil)
	if status != http.StatusOK {
		t.Fatalf("me: %d %v", status, body)
	}
	userID := int64(body["id"].(float64))

	expect := func(name, token string, wantCode string) {
		t.Helper()
		status, body := call(t, app, http.MethodGet, "/api/v1/player/me", token, nil)
		if status != http.StatusUnauthorized || errCode(body) != wantCode {
			t.Fatalf("%s: %d %v, want 401 %s", name, status, body, wantCode)
		}
	}

	// Expired: issued by a clock one day behind.
	past := auth.NewTokenIssuer(testCfg.JWT.Secret, time.Minute, func() time.Time { return time.Now().Add(-24 * time.Hour) })
	expired, _, _ := past.Issue(userID, auth.RolePlayer)
	expect("expired", expired, "TOKEN_EXPIRED")

	// Wrong signing key.
	forged, _, _ := auth.NewTokenIssuer("not-the-real-secret-but-long-enough", time.Hour, nil).Issue(userID, auth.RoleAdmin)
	expect("wrong secret", forged, "UNAUTHORIZED")

	// alg: none with otherwise valid claims.
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": "mini-game-backend", "sub": "1", "role": "admin", "jti": strings.Repeat("a", 32),
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	noneTok, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	expect("alg none", noneTok, "UNAUTHORIZED")

	// Tampered payload: flip the role inside a genuine token.
	parts := strings.Split(tok, ".")
	tampered := parts[0] + "." + strings.TrimRight(parts[1], "=") + "x." + parts[2]
	expect("tampered", tampered, "UNAUTHORIZED")

	// Revoked by logout, then every later use is refused.
	if status, body := call(t, app, http.MethodPost, "/api/v1/auth/logout", tok, nil); status != http.StatusNoContent {
		t.Fatalf("logout: %d %v", status, body)
	}
	expect("revoked", tok, "TOKEN_REVOKED")
	if status, _ := call(t, app, http.MethodPost, "/api/v1/auth/logout", tok, nil); status != http.StatusUnauthorized {
		t.Fatalf("double logout: %d", status)
	}
}

func TestSecurityAdminAuthorizationMatrix(t *testing.T) {
	resetDB(t)
	seedItems(t)
	app, svcs := newApp(t)
	playerTok := registerAndLogin(t, app, "p@example.com", "player1")
	adminTok := loginAdmin(t, app, svcs.Auth.EnsureAdmin)

	var playerID int64
	if err := testDB.QueryRowContext(context.Background(), `SELECT id FROM players LIMIT 1`).Scan(&playerID); err != nil {
		t.Fatal(err)
	}
	itemBody := map[string]any{"name": "X", "item_type": "coin", "effect_value": 1}
	routes := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v1/admin/dashboard", nil},
		{http.MethodGet, "/api/v1/admin/users", nil},
		{http.MethodGet, "/api/v1/admin/users/1", nil},
		{http.MethodGet, "/api/v1/admin/items", nil},
		{http.MethodPost, "/api/v1/admin/items", itemBody},
		{http.MethodPut, "/api/v1/admin/items/1", itemBody},
		{http.MethodDelete, "/api/v1/admin/items/3", nil},
		{http.MethodPost, "/api/v1/admin/players/" + itoa(playerID) + "/items", map[string]any{"item_id": 1, "quantity": 1}},
		{http.MethodGet, "/api/v1/admin/audit-logs", nil},
	}
	for _, rt := range routes {
		// Anonymous and player are refused before any handler runs.
		if status, body := call(t, app, rt.method, rt.path, "", rt.body); status != http.StatusUnauthorized || errCode(body) != "UNAUTHORIZED" {
			t.Fatalf("anonymous %s %s: %d %v", rt.method, rt.path, status, body)
		}
		if status, body := call(t, app, rt.method, rt.path, playerTok, rt.body); status != http.StatusForbidden || errCode(body) != "FORBIDDEN" {
			t.Fatalf("player %s %s: %d %v", rt.method, rt.path, status, body)
		}
		// Admin gets through the gate (the handler may still answer 2xx/4xx on content).
		if status, body := call(t, app, rt.method, rt.path, adminTok, rt.body); status == http.StatusUnauthorized || status == http.StatusForbidden {
			t.Fatalf("admin %s %s: %d %v", rt.method, rt.path, status, body)
		}
	}
	// Nothing the player tried left an audit trail.
	if n := countRows(t, `SELECT COUNT(*) FROM audit_logs WHERE actor_user_id = (SELECT id FROM users WHERE role = 'player')`); n != 0 {
		t.Fatalf("player actions audited: %d", n)
	}
}

func TestSecurityRateLimitAgainstLogin(t *testing.T) {
	resetDB(t)
	app, _ := newAppWith(t, func(c *config.Config) {
		c.RateLimit = config.RateLimitConfig{PerMinute: 1000, AuthPerMinute: 5}
	})
	_ = registerAndLogin(t, app, "hero@example.com", "hero") // 2 auth requests

	// Three more wrong passwords use up the budget of 5 ...
	for i := 0; i < 3; i++ {
		status, body := call(t, app, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
			"email": "hero@example.com", "password": "wrong-password",
		})
		if status != http.StatusUnauthorized {
			t.Fatalf("attempt %d: %d %v", i+1, status, body)
		}
	}
	// ... so the sixth attempt, even with the right password, is throttled.
	rec := rawCall(t, app, http.MethodPost, "/api/v1/auth/login", "", "application/json",
		`{"email":"hero@example.com","password":"password123"}`)
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("6th login: %d %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" || rec.Header().Get("X-RateLimit-Limit") != "5" {
		t.Fatalf("headers = %v", rec.Header())
	}
	// The limiter is keyed by client address: another client still logs in.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"hero@example.com","password":"password123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "198.51.100.42:5555"
	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("other client: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSecurityBodyLimit(t *testing.T) {
	resetDB(t)
	app, _ := newAppWith(t, func(c *config.Config) { c.MaxBodyBytes = 1024 })
	usersBefore := countRows(t, `SELECT COUNT(*) FROM users`)

	big := `{"email":"big@example.com","password":"password123","player_name":"` + strings.Repeat("a", 2048) + `"}`
	rec := rawCall(t, app, http.MethodPost, "/api/v1/auth/register", "", "application/json", big)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("oversized register: %d %s", rec.Code, rec.Body.String())
	}
	if n := countRows(t, `SELECT COUNT(*) FROM users`); n != usersBefore {
		t.Fatal("oversized request created a user")
	}
	// A normal request still fits comfortably.
	_ = registerAndLogin(t, app, "small@example.com", "small")
}
