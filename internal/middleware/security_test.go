package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/takahironago/mini-game-backend/internal/httpx"
)

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(okHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	for k, want := range map[string]string{
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
		"Content-Security-Policy": "default-src 'none'; frame-ancestors 'none'",
	} {
		if got := rec.Header().Get(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

// decodeHandler mimics a real handler: it decodes JSON and maps errors.
func decodeHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v map[string]any
		if err := httpx.Decode(w, r, &v); err != nil {
			httpx.WriteError(r.Context(), w, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("non-JSON body %q", rec.Body.String())
	}
	return body.Error.Code
}

func TestBodyLimit(t *testing.T) {
	const limit = 64
	h := BodyLimit(limit)(decodeHandler())

	t.Run("under the limit passes", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("declared Content-Length over the limit is rejected unread", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", limit+1)))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "PAYLOAD_TOO_LARGE" {
			t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("streamed body over the limit is cut off at read time", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":"`+strings.Repeat("x", limit)+`"}`))
		req.ContentLength = -1 // chunked: length unknown up front
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "PAYLOAD_TOO_LARGE" {
			t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("zero falls back to the default", func(t *testing.T) {
		rec := httptest.NewRecorder()
		BodyLimit(0)(decodeHandler()).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"a":1}`)))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}

func TestCORS(t *testing.T) {
	h := CORS([]string{"https://game.example.com"})(okHandler())

	t.Run("preflight from an allowed origin", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/items/use", nil)
		req.Header.Set("Origin", "https://game.example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type, Idempotency-Key")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent && rec.Code != http.StatusOK {
			t.Fatalf("status = %d", rec.Code)
		}
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://game.example.com" {
			t.Fatalf("Allow-Origin = %q", got)
		}
		if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(got, "Idempotency-Key") {
			t.Fatalf("Allow-Headers = %q, want Idempotency-Key", got)
		}
		if rec.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Fatal("credentials must not be allowed")
		}
	})

	t.Run("preflight from a foreign origin gets no CORS grant", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/api/v1/items/use", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("foreign origin must not be allowed")
		}
	})

	t.Run("actual request exposes the request id and rate-limit headers", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/items", nil)
		req.Header.Set("Origin", "https://game.example.com")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "https://game.example.com" {
			t.Fatalf("status = %d headers %v", rec.Code, rec.Header())
		}
		if got := rec.Header().Get("Access-Control-Expose-Headers"); !strings.Contains(got, "X-Request-Id") || !strings.Contains(got, "Retry-After") {
			t.Fatalf("Expose-Headers = %q", got)
		}
	})

	t.Run("same-origin request is untouched", func(t *testing.T) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/items", nil))
		if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("status = %d headers %v", rec.Code, rec.Header())
		}
	})

	t.Run("empty origin list allows nobody", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "https://game.example.com")
		rec := httptest.NewRecorder()
		CORS(nil)(okHandler()).ServeHTTP(rec, req)
		if rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("expected no CORS grant")
		}
	})
}

func TestRateLimit(t *testing.T) {
	h := RateLimit(2)(okHandler())
	get := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = ip
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < 2; i++ {
		if rec := get("10.0.0.1:1000"); rec.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d", i+1, rec.Code)
		}
	}
	rec := get("10.0.0.1:2000") // same client, different source port
	if rec.Code != http.StatusTooManyRequests || errCode(t, rec) != "RATE_LIMITED" {
		t.Fatalf("3rd request: status = %d body %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Retry-After") == "" || rec.Header().Get("X-RateLimit-Limit") != "2" {
		t.Fatalf("missing rate-limit headers: %v", rec.Header())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("429 must be JSON, got %q", ct)
	}

	// Another client has its own budget.
	if rec := get("10.0.0.2:1000"); rec.Code != http.StatusOK {
		t.Fatalf("other ip: status = %d", rec.Code)
	}
	// IPv6 clients share a /64.
	if rec := get("[2001:db8::1]:1"); rec.Code != http.StatusOK {
		t.Fatalf("ipv6 #1: %d", rec.Code)
	}
	if rec := get("[2001:db8::2]:1"); rec.Code != http.StatusOK {
		t.Fatalf("ipv6 #2: %d", rec.Code)
	}
	if rec := get("[2001:db8::3]:1"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("ipv6 #3 should share the /64 bucket: %d", rec.Code)
	}

	t.Run("zero disables", func(t *testing.T) {
		off := RateLimit(0)(okHandler())
		for i := 0; i < 50; i++ {
			rec := httptest.NewRecorder()
			off.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("request %d: %d", i, rec.Code)
			}
		}
	})
}
