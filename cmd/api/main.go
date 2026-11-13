// Command api is the Mini Game Backend HTTP server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/takahironago/mini-game-backend/internal/config"
	"github.com/takahironago/mini-game-backend/internal/database"
	"github.com/takahironago/mini-game-backend/internal/observability"
	"github.com/takahironago/mini-game-backend/internal/server"
)

// version is injected at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// Local convenience only: real environments set variables directly.
	if err := config.LoadDotEnv(".env"); err != nil {
		return fmt.Errorf("load .env: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := newLogger(cfg.LogLevel)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer db.Close()
	logger.Info("database connected", "host", cfg.DB.Host, "name", cfg.DB.Name)

	if cfg.AutoMigrate {
		if err := database.MigrateUp(ctx, cfg.DB); err != nil {
			return err
		}
		logger.Info("migrations applied")
	}

	metrics := observability.New(version)
	metrics.RegisterDB(db, cfg.DB.Name)

	deps := server.Deps{Config: cfg, Logger: logger, DB: db, Version: version, Metrics: metrics}
	services := server.NewServices(deps)

	if err := services.Auth.EnsureAdmin(ctx, cfg.Admin.Email, cfg.Admin.Password); err != nil {
		return err
	}

	srv := &http.Server{
		Addr:    cfg.HTTPAddr,
		Handler: server.NewRouter(deps, services),
		// Slowloris / idle-connection hardening.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("http server listening", "addr", cfg.HTTPAddr, "env", cfg.Env, "version", version)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	logger.Info("shutdown signal received, draining connections", "timeout", cfg.ShutdownTimeout)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	logger.Info("server stopped")
	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
