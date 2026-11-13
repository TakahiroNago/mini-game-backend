package config

import "testing"

func TestLoad_SecurityDefaults(t *testing.T) {
	t.Setenv("DB_PASSWORD", "x")
	t.Setenv("JWT_SECRET", "s")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MaxBodyBytes != 1<<20 || cfg.RateLimit.PerMinute != 300 || cfg.RateLimit.AuthPerMinute != 10 {
		t.Fatalf("unexpected defaults: body=%d rate=%+v", cfg.MaxBodyBytes, cfg.RateLimit)
	}

	t.Setenv("MAX_BODY_BYTES", "4096")
	t.Setenv("RATE_LIMIT_PER_MINUTE", "0")
	t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", "5")
	cfg, err = Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.MaxBodyBytes != 4096 || cfg.RateLimit.PerMinute != 0 || cfg.RateLimit.AuthPerMinute != 5 {
		t.Fatalf("overrides not applied: body=%d rate=%+v", cfg.MaxBodyBytes, cfg.RateLimit)
	}
}

func TestLoad_SecurityValidation(t *testing.T) {
	t.Setenv("DB_PASSWORD", "x")
	t.Setenv("JWT_SECRET", "s")

	t.Setenv("MAX_BODY_BYTES", "10")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for tiny MAX_BODY_BYTES")
	}
	t.Setenv("MAX_BODY_BYTES", "")

	t.Setenv("RATE_LIMIT_PER_MINUTE", "-1")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for negative rate limit")
	}
	t.Setenv("RATE_LIMIT_PER_MINUTE", "")

	// Production refuses disabled limiters and wildcard CORS.
	t.Setenv("APP_ENV", "production")
	t.Setenv("AUTO_MIGRATE", "false")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef")
	t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for disabled auth rate limit in production")
	}
	t.Setenv("AUTH_RATE_LIMIT_PER_MINUTE", "")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://game.example.com, *")
	if _, err := Load(); err == nil {
		t.Fatal("expected error for wildcard CORS in production")
	}
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://game.example.com")
	if _, err := Load(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
