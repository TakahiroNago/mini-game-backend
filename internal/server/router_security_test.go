package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/takahironago/mini-game-backend/internal/config"
)

// newHardenedRouter enables every Day 5 control with tight limits so the
// tests below can hit them without a database.
func newHardenedRouter(t *testing.T) http.Handler {
	t.Helper()
	d := Deps{
		Config: config.Config{
			RequestTimeout: 5 * time.Second,
			MaxBodyBytes:   2048,
			CORSOrigins:    []string{"https://game.example.com"},
			JWT:            config.JWTConfig{Secret: "unit-test-secret", Expiry: time.Hour},
			RateLimit:      config.RateLimitConfig{PerMinute: 20, AuthPerMinute: 3},
		},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		Version: "test",
	}
	return NewRouter(d, NewServices(d))
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	h := newHardenedRouter(t)
	for _, path := range []string{"/health", "/nope", "/api/v1/player/me"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Header().Get("X-Content-Type-Options") != "nosniff" || rec.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s: missing security headers: %v", path, rec.Header())
		}
		if rec.Header().Get("Cache-Control") == "" {
			t.Fatalf("%s: missing Cache-Control", path)
		}
	}
}

func TestOversizedBodyIsRejectedBeforeHandlers(t *testing.T) {
	h := newHardenedRouter(t)
	body := `{"email":"a@example.com","password":"` + strings.Repeat("p", 4096) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "PAYLOAD_TOO_LARGE") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestCORSPreflightThroughRouter(t *testing.T) {
	h := newHardenedRouter(t)
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/items/use", nil)
	req.Header.Set("Origin", "https://game.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "authorization, content-type, idempotency-key")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	// The preflight must be answered by the CORS layer, not by the 401 of
	// the Authenticate middleware behind it.
	if rec.Code != http.StatusOK && rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://game.example.com" {
		t.Fatalf("headers = %v", rec.Header())
	}

	// A foreign origin is answered without any grant.
	req.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("foreign origin granted: %v", rec.Header())
	}
}

func TestAuthRoutesHaveTheirOwnRateLimit(t *testing.T) {
	h := newHardenedRouter(t)
	post := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = "203.0.113.7:4321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	// AuthPerMinute = 3: the first three reach validation (400), the fourth is 429.
	for i := 0; i < 3; i++ {
		if rec := post("/api/v1/auth/login"); rec.Code != http.StatusBadRequest {
			t.Fatalf("attempt %d: status = %d body %s", i+1, rec.Code, rec.Body.String())
		}
	}
	rec := post("/api/v1/auth/register") // register shares the budget with login
	if rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "RATE_LIMITED") {
		t.Fatalf("4th auth attempt: status = %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}

	// The global limiter (20) still has room, so other routes keep working.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
	req.RemoteAddr = "203.0.113.7:4321"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("items after auth limit: status = %d", rec.Code)
	}
	// Probes never see a 429.
	for i := 0; i < 30; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		req.RemoteAddr = "203.0.113.7:4321"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("/health #%d: status = %d", i+1, rec.Code)
		}
	}
}

func TestGlobalRateLimitCoversTheAPI(t *testing.T) {
	h := newHardenedRouter(t)
	var last *httptest.ResponseRecorder
	for i := 0; i < 21; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/ranking", nil)
		req.RemoteAddr = "198.51.100.9:1"
		last = httptest.NewRecorder()
		h.ServeHTTP(last, req)
		if i < 20 && last.Code != http.StatusUnauthorized {
			t.Fatalf("request %d: status = %d", i+1, last.Code)
		}
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("21st request: status = %d body %s", last.Code, last.Body.String())
	}
}
