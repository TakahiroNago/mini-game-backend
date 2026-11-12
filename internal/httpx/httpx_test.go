package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteError_APIError(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, NewError(http.StatusConflict, "ALREADY_CLAIMED", "already claimed"))

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	var body struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "ALREADY_CLAIMED" || body.Error.Message != "already claimed" {
		t.Fatalf("unexpected body: %+v", body)
	}
}

func TestWriteError_HidesInternalDetails(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteError(context.Background(), rec, errors.New("sql: connection refused to 10.0.0.5"))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Fatalf("internal detail leaked: %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"INTERNAL_ERROR"`) {
		t.Fatalf("expected generic code, got %s", rec.Body.String())
	}
}

func TestWriteError_WrappedAPIError(t *testing.T) {
	rec := httptest.NewRecorder()
	wrapped := errors.Join(errors.New("ctx"), ErrNotFound)
	WriteError(context.Background(), rec, wrapped)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDecode(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name     string
		body     string
		wantCode string // "" means success
	}{
		{"valid", `{"name":"a"}`, ""},
		{"empty", ``, "BAD_REQUEST"},
		{"malformed", `{"name":`, "BAD_REQUEST"},
		{"unknown field", `{"name":"a","x":1}`, "BAD_REQUEST"},
		{"trailing data", `{"name":"a"}{}`, "BAD_REQUEST"},
		{"too large", `{"name":"` + strings.Repeat("a", MaxBodyBytes) + `"}`, "PAYLOAD_TOO_LARGE"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			var p payload
			err := Decode(httptest.NewRecorder(), req, &p)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.Code != tt.wantCode {
				t.Fatalf("err = %v, want code %s", err, tt.wantCode)
			}
		})
	}
}
