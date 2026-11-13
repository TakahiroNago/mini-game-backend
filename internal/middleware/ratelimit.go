package middleware

import (
	"net"
	"net/http"
	"time"

	"github.com/go-chi/httprate"

	"github.com/takahironago/mini-game-backend/internal/httpx"
)

// ErrRateLimited is returned (as JSON) when a client exceeds its budget.
var ErrRateLimited = httpx.NewError(http.StatusTooManyRequests, "RATE_LIMITED",
	"too many requests, retry after the number of seconds in Retry-After")

// RateLimit allows perMinute requests per client IP within a sliding one
// minute window and answers the rest with 429 plus Retry-After and the
// X-RateLimit-* headers. Zero or negative disables the limiter.
//
// The client IP is taken from r.RemoteAddr, which chi's RealIP middleware has
// already replaced with X-Forwarded-For / X-Real-IP when a trusted reverse
// proxy (the admin nginx) sits in front. Counters live in process memory:
// one API instance is the deployment target, and the limits are a
// brute-force / abuse brake rather than a precise quota, so a shared store
// is not worth its moving parts here.
func RateLimit(perMinute int) func(http.Handler) http.Handler {
	if perMinute <= 0 {
		return func(next http.Handler) http.Handler { return next }
	}
	return httprate.LimitBy(perMinute, time.Minute, keyByClientIP,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteError(r.Context(), w, ErrRateLimited)
		}),
		httprate.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
			httpx.WriteError(r.Context(), w, err)
		}),
	)
}

// keyByClientIP buckets by IP, collapsing IPv6 clients to their /64 so a
// client cannot escape its budget by rotating within its own prefix.
func keyByClientIP(r *http.Request) (string, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return httprate.CanonicalizeIP(host), nil
}
