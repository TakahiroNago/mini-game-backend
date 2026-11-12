package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DB_PASSWORD", "x")
	t.Setenv("JWT_SECRET", "s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.DB.Port != 3306 || cfg.RequestTimeout != 10*time.Second || !cfg.AutoMigrate {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "http://localhost:5173" {
		t.Fatalf("CORS = %v", cfg.CORSOrigins)
	}
}

func TestLoad_RequiredValues(t *testing.T) {
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("JWT_SECRET", "s")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for empty DB_PASSWORD")
	}

	t.Setenv("DB_PASSWORD", "x")
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for empty JWT_SECRET")
	}

	t.Setenv("JWT_SECRET", "s")
	t.Setenv("ADMIN_EMAIL", "admin@example.com")
	t.Setenv("ADMIN_PASSWORD", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when only ADMIN_EMAIL is set")
	}
}

func TestLoad_ProductionHardening(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("DB_PASSWORD", "x")
	t.Setenv("AUTO_MIGRATE", "false")

	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for short JWT_SECRET in production")
	}

	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	if _, err := Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	t.Setenv("AUTO_MIGRATE", "true")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for AUTO_MIGRATE=true in production")
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	content := "# comment\nDOTENV_A=hello\nDOTENV_B=\"quoted value\"\nDOTENV_EXISTING=from-file\ninvalid line\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOTENV_EXISTING", "from-env")
	t.Setenv("DOTENV_A", "")
	t.Setenv("DOTENV_B", "")
	os.Unsetenv("DOTENV_A")
	os.Unsetenv("DOTENV_B")

	if err := LoadDotEnv(path); err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	if got := os.Getenv("DOTENV_A"); got != "hello" {
		t.Fatalf("DOTENV_A = %q", got)
	}
	if got := os.Getenv("DOTENV_B"); got != "quoted value" {
		t.Fatalf("DOTENV_B = %q", got)
	}
	if got := os.Getenv("DOTENV_EXISTING"); got != "from-env" {
		t.Fatalf("existing env must not be overwritten, got %q", got)
	}
	if err := LoadDotEnv(filepath.Join(dir, "missing")); err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
}
