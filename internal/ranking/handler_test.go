package ranking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/takahironago/mini-game-backend/internal/httpx"
	"github.com/takahironago/mini-game-backend/internal/identity"
)

func TestParsePagination(t *testing.T) {
	tests := []struct {
		query      string
		wantLimit  int
		wantOffset int
		wantErr    string // offending field, "" for success
	}{
		{"", DefaultLimit, 0, ""},
		{"limit=5&offset=10", 5, 10, ""},
		{"limit=100", 100, 0, ""},
		{"limit=0", 0, 0, "limit"},
		{"limit=101", 0, 0, "limit"},
		{"limit=abc", 0, 0, "limit"},
		{"offset=-1", 0, 0, "offset"},
		{"offset=1.5", 0, 0, "offset"},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/?"+tt.query, nil)
			limit, offset, err := parsePagination(r)
			if tt.wantErr == "" {
				if err != nil || limit != tt.wantLimit || offset != tt.wantOffset {
					t.Fatalf("got (%d, %d, %v), want (%d, %d, nil)", limit, offset, err, tt.wantLimit, tt.wantOffset)
				}
				return
			}
			var apiErr *httpx.Error
			if !errors.As(err, &apiErr) || apiErr.Status != http.StatusBadRequest {
				t.Fatalf("expected 400 error, got %v", err)
			}
			if _, ok := apiErr.Details[tt.wantErr]; !ok {
				t.Fatalf("expected details for %q, got %v", tt.wantErr, apiErr.Details)
			}
		})
	}
}

type stubService struct{ lastLimit, lastOffset int }

func (s *stubService) Page(_ context.Context, limit, offset int) (Page, error) {
	s.lastLimit, s.lastOffset = limit, offset
	return Page{Rankings: []Entry{{Rank: int64(offset) + 1, PlayerID: 1, PlayerName: "hero", Level: 3}},
		Limit: limit, Offset: offset, Total: 1}, nil
}

func (s *stubService) Me(context.Context, int64) (MyRank, error) {
	return MyRank{Rank: 1, PlayerID: 1, PlayerName: "hero", Level: 3, Total: 1}, nil
}

func TestHandlerRoutes(t *testing.T) {
	svc := &stubService{}
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(identity.WithClaims(r.Context(), identity.Claims{UserID: 1})))
		})
	})
	NewHandler(svc).Routes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?limit=2&offset=4", nil))
	if rec.Code != http.StatusOK || svc.lastLimit != 2 || svc.lastOffset != 4 {
		t.Fatalf("page: status = %d limit = %d offset = %d body = %s", rec.Code, svc.lastLimit, svc.lastOffset, rec.Body.String())
	}
	var page Page
	_ = json.NewDecoder(rec.Body).Decode(&page)
	if page.Total != 1 || len(page.Rankings) != 1 || page.Rankings[0].Rank != 5 {
		t.Fatalf("unexpected page: %+v", page)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?limit=0", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad limit: status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/me", nil))
	var me MyRank
	_ = json.NewDecoder(rec.Body).Decode(&me)
	if rec.Code != http.StatusOK || me.Rank != 1 || me.PlayerName != "hero" {
		t.Fatalf("me: status = %d body = %+v", rec.Code, me)
	}
}
