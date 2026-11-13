package middleware

import (
	"net/http"

	"github.com/takahironago/mini-game-backend/internal/httpx"
)

// SecurityHeaders adds the browser-hardening headers that make sense for a
// JSON API. The admin UI's nginx serves HTML itself; these only need to stop
// an API response from being interpreted as anything but data.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// JSON is never a document: forbid every active content type so a
		// reflected payload cannot run even if a client renders the body.
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// BodyLimit caps every request body at maxBytes. Reading past the limit
// fails with *http.MaxBytesError, which httpx.Decode maps to 413, and the
// server closes the connection instead of draining a multi-gigabyte upload.
// Values <= 0 fall back to httpx.MaxBodyBytes.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	if maxBytes <= 0 {
		maxBytes = httpx.MaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Reject obviously oversized uploads from the declared length
			// without reading a byte.
			if r.ContentLength > maxBytes {
				httpx.WriteError(r.Context(), w, httpx.ErrPayloadTooLarge)
				return
			}
			if r.Body != nil && r.Body != http.NoBody {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}
