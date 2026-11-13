package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/takahironago/mini-game-backend/internal/config"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	d := Deps{
		Config: config.Config{
			RequestTimeout: 5 * time.Second,
			JWT:            config.JWTConfig{Secret: "unit-test-secret", Expiry: time.Hour},
		},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      nil, // only routes that never touch the DB are exercised here
		Version: "test",
	}
	return NewRouter(d, NewServices(d))
}

func TestHealthRoute(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if rec.Header().Get("X-Request-Id") == "" {
		t.Fatal("expected X-Request-Id header to be set")
	}
}

func TestNotFoundIsJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("expected JSON body, got %q (%v)", rec.Body.String(), err)
	}
	if body.Error.Code != "NOT_FOUND" {
		t.Fatalf("code = %q, want NOT_FOUND", body.Error.Code)
	}
}

func TestMethodNotAllowedIsJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/health", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}

func TestMetricsRoute(t *testing.T) {
	h := newTestRouter(t)
	// Generate one request so the counter has a series to show.
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health", nil))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`minigame_build_info{version="test"} 1`,
		`minigame_http_requests_total{method="GET",route="/health",status="200"} 1`,
		"go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics missing %q", want)
		}
	}
}

// Protected routes must reject missing / malformed tokens before touching
// any dependency, so this runs without a database.
func TestProtectedRoutesRequireToken(t *testing.T) {
	paths := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/auth/me"},
		{http.MethodPost, "/api/v1/auth/logout"},
		{http.MethodGet, "/api/v1/player/me"},
		{http.MethodGet, "/api/v1/player/me/items"},
		{http.MethodGet, "/api/v1/items"},
		{http.MethodPost, "/api/v1/items/use"},
		{http.MethodPost, "/api/v1/rewards/claim"},
		{http.MethodGet, "/api/v1/rewards/status"},
		{http.MethodGet, "/api/v1/ranking"},
		{http.MethodGet, "/api/v1/ranking/me"},
		{http.MethodGet, "/api/v1/admin/dashboard"},
		{http.MethodGet, "/api/v1/admin/users"},
		{http.MethodPost, "/api/v1/admin/items"},
		{http.MethodDelete, "/api/v1/admin/items/1"},
		{http.MethodGet, "/api/v1/admin/audit-logs"},
	}
	for _, p := range paths {
		for _, header := range []string{"", "Bearer", "Bearer not-a-jwt", "Basic abc"} {
			req := httptest.NewRequest(p.method, p.path, nil)
			if header != "" {
				req.Header.Set("Authorization", header)
			}
			rec := httptest.NewRecorder()
			newTestRouter(t).ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Fatalf("%s %s with %q: status = %d, want 401", p.method, p.path, header, rec.Code)
			}
		}
	}
}
