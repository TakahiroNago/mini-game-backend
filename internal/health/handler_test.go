package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubPinger struct{ err error }

func (s stubPinger) PingContext(context.Context) error { return s.err }

func TestLive(t *testing.T) {
	h := NewHandler(stubPinger{}, "test")
	rec := httptest.NewRecorder()
	h.Live(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" || body["version"] != "test" {
		t.Fatalf("unexpected body: %v", body)
	}
}

func TestReady(t *testing.T) {
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantMySQL  string
	}{
		{"database reachable", nil, http.StatusOK, "ok"},
		{"database down", errors.New("connection refused"), http.StatusServiceUnavailable, "unavailable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHandler(stubPinger{err: tt.pingErr}, "test")
			rec := httptest.NewRecorder()
			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			var body struct {
				Checks map[string]string `json:"checks"`
			}
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Checks["mysql"] != tt.wantMySQL {
				t.Fatalf("checks.mysql = %q, want %q", body.Checks["mysql"], tt.wantMySQL)
			}
		})
	}
}
