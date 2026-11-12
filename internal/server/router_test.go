package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/takahironago/mini-game-backend/internal/config"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	return NewRouter(Deps{
		Config:  config.Config{RequestTimeout: 5 * time.Second},
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      nil, // /ready would nil-deref; only /health and 404s are exercised here
		Version: "test",
	})
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
