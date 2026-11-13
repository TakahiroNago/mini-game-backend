package httpx

import (
	"fmt"
	"net/http"
	"strconv"
)

// Pagination reads ?limit= and ?offset= from the query string. Missing
// values fall back to defaultLimit / 0; out-of-range values are rejected
// with a 400 (VALIDATION_ERROR) rather than silently clamped so clients
// notice their bug.
func Pagination(r *http.Request, defaultLimit, maxLimit int) (limit, offset int, err error) {
	limit, offset = defaultLimit, 0
	details := map[string]string{}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, convErr := strconv.Atoi(v)
		switch {
		case convErr != nil:
			details["limit"] = "must be an integer"
		case n < 1 || n > maxLimit:
			details["limit"] = fmt.Sprintf("must be between 1 and %d", maxLimit)
		default:
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		n, convErr := strconv.Atoi(v)
		switch {
		case convErr != nil:
			details["offset"] = "must be an integer"
		case n < 0:
			details["offset"] = "must be >= 0"
		default:
			offset = n
		}
	}
	if len(details) > 0 {
		return 0, 0, &Error{
			Status: http.StatusBadRequest, Code: "VALIDATION_ERROR",
			Message: "request validation failed", Details: details,
		}
	}
	return limit, offset, nil
}

// PathID parses a positive integer URL parameter, answering 400 otherwise.
func PathID(raw, name string) (int64, error) {
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, &Error{
			Status: http.StatusBadRequest, Code: "VALIDATION_ERROR",
			Message: "request validation failed", Details: map[string]string{name: "must be a positive integer"},
		}
	}
	return id, nil
}
