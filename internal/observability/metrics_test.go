package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func scrape(t *testing.T, m *Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d", rec.Code)
	}
	return rec.Body.String()
}

func TestMiddlewareRecordsRoutePatternNotPath(t *testing.T) {
	m := New("v-test")
	r := chi.NewRouter()
	r.Use(m.Middleware)
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/users/{id}", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
		api.Post("/items/use", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusConflict) })
	})

	for _, p := range []string{"/api/v1/users/1", "/api/v1/users/2", "/api/v1/users/3"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
	}
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/items/use", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/wp-admin/../../etc/passwd", nil))
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/users/1/../../../x", nil))

	out := scrape(t, m)
	for _, want := range []string{
		`minigame_http_requests_total{method="GET",route="/api/v1/users/{id}",status="200"} 3`,
		`minigame_http_requests_total{method="POST",route="/api/v1/items/use",status="409"} 1`,
		`minigame_http_requests_total{method="GET",route="unmatched",status="404"} 2`,
		`minigame_http_request_duration_seconds_count{method="GET",route="/api/v1/users/{id}"} 3`,
		`minigame_http_requests_in_flight 0`,
		`minigame_build_info{version="v-test"} 1`,
		`go_goroutines`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("metrics output missing %q\n%s", want, out)
		}
	}
	// Concrete ids must never leak into a label.
	if strings.Contains(out, `/api/v1/users/1`) || strings.Contains(out, "wp-admin") {
		t.Fatalf("raw path leaked into labels:\n%s", out)
	}
}

func TestSeveralInstancesCoexist(t *testing.T) {
	// A shared default registry would panic on the second New.
	a, b := New("a"), New("b")
	if !strings.Contains(scrape(t, a), `version="a"`) || !strings.Contains(scrape(t, b), `version="b"`) {
		t.Fatal("instances share state")
	}
}

func TestRegisterDBNilIsNoop(t *testing.T) {
	m := New("x")
	m.RegisterDB(nil, "db")
	if strings.Contains(scrape(t, m), "go_sql_") {
		t.Fatal("unexpected db metrics")
	}
}
