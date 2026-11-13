// Package config loads application configuration from environment variables.
//
// Every value has a development-friendly default so the API can start with
// nothing but a running MySQL, while production deployments override them
// through the container environment (never through files baked into images).
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the fully resolved application configuration.
type Config struct {
	Env             string
	HTTPAddr        string
	LogLevel        string
	AutoMigrate     bool
	CORSOrigins     []string
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
	// MaxBodyBytes caps every request body before a handler reads it.
	MaxBodyBytes int64

	DB        DBConfig
	JWT       JWTConfig
	Admin     AdminConfig
	RateLimit RateLimitConfig
}

// DBConfig holds MySQL connection settings.
type DBConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Name            string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// JWTConfig holds token signing settings.
type JWTConfig struct {
	Secret string
	Expiry time.Duration
}

// AdminConfig bootstraps the first admin account on start-up (optional).
type AdminConfig struct {
	Email    string
	Password string
}

// RateLimitConfig bounds requests per client IP and minute. Zero disables
// the corresponding limiter (useful for load tests), never negative.
type RateLimitConfig struct {
	// PerMinute applies to every /api/v1 route.
	PerMinute int
	// AuthPerMinute is the stricter budget for register / login, which are
	// unauthenticated and therefore the brute-force surface.
	AuthPerMinute int
}

// IsProduction reports whether the app runs with production hardening.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads the environment and validates the result.
func Load() (Config, error) {
	cfg := Config{
		Env:             getenv("APP_ENV", "development"),
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		LogLevel:        getenv("LOG_LEVEL", "info"),
		AutoMigrate:     getenvBool("AUTO_MIGRATE", true),
		CORSOrigins:     splitCSV(getenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173")),
		RequestTimeout:  getenvDuration("REQUEST_TIMEOUT", 10*time.Second),
		ShutdownTimeout: getenvDuration("SHUTDOWN_TIMEOUT", 15*time.Second),
		MaxBodyBytes:    getenvInt64("MAX_BODY_BYTES", 1<<20),
		DB: DBConfig{
			Host:            getenv("DB_HOST", "127.0.0.1"),
			Port:            getenvInt("DB_PORT", 3306),
			User:            getenv("DB_USER", "minigame"),
			Password:        getenv("DB_PASSWORD", ""),
			Name:            getenv("DB_NAME", "minigame"),
			MaxOpenConns:    getenvInt("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    getenvInt("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: getenvDuration("DB_CONN_MAX_LIFETIME", 5*time.Minute),
		},
		JWT: JWTConfig{
			Secret: getenv("JWT_SECRET", ""),
			Expiry: getenvDuration("JWT_EXPIRY", time.Hour),
		},
		Admin: AdminConfig{
			Email:    getenv("ADMIN_EMAIL", ""),
			Password: getenv("ADMIN_PASSWORD", ""),
		},
		RateLimit: RateLimitConfig{
			PerMinute:     getenvInt("RATE_LIMIT_PER_MINUTE", 300),
			AuthPerMinute: getenvInt("AUTH_RATE_LIMIT_PER_MINUTE", 10),
		},
	}

	if cfg.DB.Password == "" {
		return cfg, fmt.Errorf("config: DB_PASSWORD is required")
	}
	if cfg.JWT.Secret == "" {
		return cfg, fmt.Errorf("config: JWT_SECRET is required")
	}
	if (cfg.Admin.Email == "") != (cfg.Admin.Password == "") {
		return cfg, fmt.Errorf("config: ADMIN_EMAIL and ADMIN_PASSWORD must be set together")
	}
	if cfg.MaxBodyBytes < 1024 {
		return cfg, fmt.Errorf("config: MAX_BODY_BYTES must be at least 1024")
	}
	if cfg.RateLimit.PerMinute < 0 || cfg.RateLimit.AuthPerMinute < 0 {
		return cfg, fmt.Errorf("config: RATE_LIMIT_PER_MINUTE and AUTH_RATE_LIMIT_PER_MINUTE must be >= 0")
	}
	if cfg.IsProduction() {
		if cfg.AutoMigrate {
			return cfg, fmt.Errorf("config: AUTO_MIGRATE must be false in production")
		}
		if len(cfg.JWT.Secret) < 32 {
			return cfg, fmt.Errorf("config: JWT_SECRET must be at least 32 characters in production")
		}
		if cfg.RateLimit.PerMinute == 0 || cfg.RateLimit.AuthPerMinute == 0 {
			return cfg, fmt.Errorf("config: rate limits cannot be disabled in production")
		}
		for _, o := range cfg.CORSOrigins {
			if o == "*" {
				return cfg, fmt.Errorf("config: CORS_ALLOWED_ORIGINS must not be * in production")
			}
		}
	}
	return cfg, nil
}

func getenv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getenvInt64(key string, def int64) int64 {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func getenvBool(key string, def bool) bool {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getenvDuration(key string, def time.Duration) time.Duration {
	v := getenv(key, "")
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
