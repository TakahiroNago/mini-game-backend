// Package health exposes liveness (/health) and readiness (/ready) probes.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/takahironago/mini-game-backend/internal/httpx"
)

// Pinger is satisfied by *sql.DB; abstracted so the handler is unit-testable.
type Pinger interface {
	PingContext(ctx context.Context) error
}

// Handler serves the probe endpoints.
type Handler struct {
	db      Pinger
	version string
}

// NewHandler wires the probes to their dependencies.
func NewHandler(db Pinger, version string) *Handler {
	return &Handler{db: db, version: version}
}

// Live reports that the process is up. It never touches dependencies so an
// unhealthy database does not cause the orchestrator to restart the API.
func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	httpx.JSON(w, http.StatusOK, map[string]string{
		"status":  "ok",
		"version": h.version,
	})
}

// Ready reports whether the API can serve traffic (database reachable).
func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	checks := map[string]string{"mysql": "ok"}
	status, state := http.StatusOK, "ready"
	if err := h.db.PingContext(ctx); err != nil {
		checks["mysql"] = "unavailable"
		status, state = http.StatusServiceUnavailable, "not_ready"
	}
	httpx.JSON(w, status, map[string]any{
		"status": state,
		"checks": checks,
	})
}
