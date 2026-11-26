package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// RequestID gives every request a random 128-bit id, stored under chi's
// context key so chimw.GetReqID keeps working for the logger and error
// writer.
//
// It replaces chi's own RequestID for two reasons: chi's ids start with the
// machine's hostname, which the X-Request-Id response header would publish,
// and chi adopts whatever X-Request-Id the client sends, which lets a caller
// write arbitrary text into the logs. The id here is always generated.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), chimw.RequestIDKey, newRequestID())
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newRequestID() string {
	var b [16]byte
	// crypto/rand.Read never fails on supported platforms (it aborts the
	// process instead), so there is no error path to handle.
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
