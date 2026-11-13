// Package observability exposes Prometheus metrics for the API.
//
// Only three HTTP dimensions are recorded (method, chi route pattern,
// status). Raw paths, user ids and query strings never become labels, so
// the series count stays bounded by the number of routes.
package observability

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const namespace = "minigame"

// Metrics owns a private registry so tests can build several instances
// without "duplicate metrics collector registration" panics.
type Metrics struct {
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// New builds a registry with Go runtime, process and build-info collectors.
func New(version string) *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		registry: reg,
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_total",
			Help: "HTTP requests by method, route pattern and status code.",
		}, []string{"method", "route", "status"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace, Subsystem: "http", Name: "request_duration_seconds",
			Help: "HTTP request latency by method and route pattern.",
			// 1 ms .. ~10 s; item use and reward claim should sit well under 50 ms.
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace, Subsystem: "http", Name: "requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
	}
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace, Name: "build_info",
		Help: "Build information; the value is always 1.",
	}, []string{"version"})
	buildInfo.WithLabelValues(version).Set(1)

	reg.MustRegister(
		m.requests, m.duration, m.inFlight, buildInfo,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	return m
}

// RegisterDB exports the connection-pool statistics of db (open / in-use /
// idle connections, wait count and duration) under go_sql_*.
func (m *Metrics) RegisterDB(db *sql.DB, name string) {
	if db == nil {
		return
	}
	m.registry.MustRegister(collectors.NewDBStatsCollector(db, name))
}

// Handler serves the registry in the Prometheus text format.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		// A failing collector must not take the scrape down with it.
		ErrorHandling: promhttp.ContinueOnError,
	})
}

// Middleware records every request. It must be mounted on the root router
// so chi has resolved the full route pattern by the time the handler
// returns; requests that matched no route are bucketed as "unmatched".
func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		ww := chimw.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		status := ww.Status()
		if status == 0 {
			status = http.StatusOK
		}
		route := RoutePattern(r)
		m.requests.WithLabelValues(r.Method, route, strconv.Itoa(status)).Inc()
		m.duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
	})
}

// RoutePattern returns the chi pattern that served r ("/api/v1/items/use"),
// or "unmatched" for 404 / 405 responses, which otherwise would turn every
// probing scanner path into a new series.
func RoutePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return "unmatched"
	}
	pattern := rctx.RoutePattern()
	// chi reports a 404 inside a mounted group as the mount's wildcard
	// ("/api/v1/*"); the API has no wildcard routes of its own, so any
	// trailing /* means "nothing matched".
	if pattern == "" || strings.HasSuffix(pattern, "/*") {
		return "unmatched"
	}
	return pattern
}
