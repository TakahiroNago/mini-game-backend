// Package httpx contains the small HTTP helpers shared by every handler:
// JSON encoding, the unified error envelope and bounded request decoding.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// MaxBodyBytes bounds every JSON request body (1 MiB is generous for this API).
const MaxBodyBytes = 1 << 20

// Error is an API error that maps to an HTTP status and a stable machine code.
// Handlers return these; anything else becomes a generic 500 so internals
// never leak to clients.
type Error struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
	// Err is the underlying cause, logged but never serialized.
	Err error `json:"-"`
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Code, e.Err)
	}
	return e.Code + ": " + e.Message
}

// Unwrap exposes the cause to errors.Is / errors.As.
func (e *Error) Unwrap() error { return e.Err }

// NewError constructs an API error.
func NewError(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

// Common errors reused across packages.
var (
	ErrBadRequest   = NewError(http.StatusBadRequest, "BAD_REQUEST", "invalid request")
	ErrUnauthorized = NewError(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	ErrForbidden    = NewError(http.StatusForbidden, "FORBIDDEN", "insufficient permissions")
	ErrNotFound     = NewError(http.StatusNotFound, "NOT_FOUND", "resource not found")
	ErrInternal     = NewError(http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
)

type errorEnvelope struct {
	Error *Error `json:"error"`
}

// JSON writes v as a JSON response with the given status.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Headers are already sent; the best we can do is log it.
		slog.Error("encode response", "error", err)
	}
}

// WriteError maps err to the unified error envelope. Unknown errors are
// logged with the request id and returned as a generic 500.
func WriteError(ctx context.Context, w http.ResponseWriter, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = ErrInternal
	}
	if apiErr.Status >= 500 {
		slog.ErrorContext(ctx, "request failed",
			"request_id", middleware.GetReqID(ctx),
			"error", err,
		)
	}
	JSON(w, apiErr.Status, errorEnvelope{Error: &Error{Code: apiErr.Code, Message: apiErr.Message}})
}

// Decode reads a bounded JSON body into v, rejecting unknown fields and
// trailing data so malformed clients fail fast with a 400.
func Decode(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var maxErr *http.MaxBytesError
		switch {
		case errors.As(err, &maxErr):
			return NewError(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large")
		case errors.Is(err, io.EOF):
			return NewError(http.StatusBadRequest, "BAD_REQUEST", "request body is required")
		default:
			return NewError(http.StatusBadRequest, "BAD_REQUEST", "malformed JSON body")
		}
	}
	if dec.More() {
		return NewError(http.StatusBadRequest, "BAD_REQUEST", "unexpected data after JSON body")
	}
	return nil
}
