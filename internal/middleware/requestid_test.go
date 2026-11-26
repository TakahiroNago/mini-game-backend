package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	chimw "github.com/go-chi/chi/v5/middleware"
)

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRequestIDIsRandomHexWithoutHostname(t *testing.T) {
	var ids []string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ids = append(ids, chimw.GetReqID(r.Context()))
	}))
	for range 2 {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}

	host, _ := os.Hostname()
	for _, id := range ids {
		if !requestIDPattern.MatchString(id) {
			t.Fatalf("request id %q is not 32 hex characters", id)
		}
		if host != "" && strings.Contains(id, host) {
			t.Fatalf("request id %q contains the hostname", id)
		}
	}
	if ids[0] == ids[1] {
		t.Fatalf("two requests got the same id %q", ids[0])
	}
}

func TestRequestIDIgnoresClientSuppliedValue(t *testing.T) {
	var got string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = chimw.GetReqID(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-Id", "forged-by-client")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if got == "forged-by-client" || !requestIDPattern.MatchString(got) {
		t.Fatalf("client-supplied request id was adopted: %q", got)
	}
}
