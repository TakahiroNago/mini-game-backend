//go:build integration

// Package integration runs the API against a real MySQL. It needs the
// TEST_DB_* environment variables (see README) and is selected with
// `go test -tags=integration ./tests/...`.
package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/internal/database"
	"github.com/takahironago/mini-game-backend/internal/server"
)

var (
	testDB  *sql.DB
	testCfg config.Config
)

// TestMain opens the database, applies migrations and wipes the tables so
// every run starts from a known state.
func TestMain(m *testing.M) {
	_ = config.LoadDotEnv("../../.env")
	// Test-specific overrides win over .env so a CI job can point at its own DB.
	for _, kv := range [][2]string{
		{"DB_HOST", "TEST_DB_HOST"}, {"DB_PORT", "TEST_DB_PORT"}, {"DB_USER", "TEST_DB_USER"},
		{"DB_PASSWORD", "TEST_DB_PASSWORD"}, {"DB_NAME", "TEST_DB_NAME"},
	} {
		if v := os.Getenv(kv[1]); v != "" {
			os.Setenv(kv[0], v)
		}
	}
	os.Setenv("JWT_SECRET", "integration-test-secret-0123456789")
	os.Setenv("APP_ENV", "test")

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, "config:", err)
		os.Exit(1)
	}
	testCfg = cfg

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	db, err := database.Open(ctx, cfg.DB)
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "database:", err)
		os.Exit(1)
	}
	if err := database.MigrateUp(context.Background(), cfg.DB); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	testDB = db

	code := m.Run()
	db.Close()
	os.Exit(code)
}

// resetDB truncates every application table (children first).
func resetDB(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := testDB.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS = 0`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"audit_logs", "revoked_tokens", "item_transactions", "reward_claims",
		"player_items", "items", "players", "users",
	} {
		if _, err := testDB.ExecContext(ctx, `TRUNCATE TABLE `+table); err != nil {
			t.Fatalf("truncate %s: %v", table, err)
		}
	}
	if _, err := testDB.ExecContext(ctx, `SET FOREIGN_KEY_CHECKS = 1`); err != nil {
		t.Fatal(err)
	}
}

// newApp builds the real router on top of the test database.
func newApp(t *testing.T) (http.Handler, *server.Services) {
	t.Helper()
	deps := server.Deps{
		Config:  testCfg,
		Logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		DB:      testDB,
		Version: "integration",
	}
	svcs := server.NewServices(deps)
	return server.NewRouter(deps, svcs), svcs
}

// call performs a JSON request and returns status + decoded body.
func call(t *testing.T, h http.Handler, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = strings.NewReader(string(b))
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	out := map[string]any{}
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("%s %s: non-JSON body %q", method, path, rec.Body.String())
		}
	}
	return rec.Code, out
}

func errCode(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		s, _ := e["code"].(string)
		return s
	}
	return ""
}

// registerAndLogin creates a player account and returns its access token.
func registerAndLogin(t *testing.T, h http.Handler, email, playerName string) string {
	t.Helper()
	status, body := call(t, h, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "password123", "player_name": playerName,
	})
	if status != http.StatusCreated {
		t.Fatalf("register: status %d body %v", status, body)
	}
	status, body = call(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": email, "password": "password123",
	})
	if status != http.StatusOK {
		t.Fatalf("login: status %d body %v", status, body)
	}
	tok, _ := body["access_token"].(string)
	if tok == "" {
		t.Fatalf("login: no access_token in %v", body)
	}
	return tok
}
