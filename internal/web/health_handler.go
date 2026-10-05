package web

import (
	"context"
	"net/http"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/database"
)

// QueueReadyChecker verifies SQS readiness (optional in unit tests).
type QueueReadyChecker interface {
	Ready(ctx context.Context) error
}

// HealthHandler serves liveness and readiness probes.
type HealthHandler struct {
	db  *database.DB
	sqs QueueReadyChecker
}

func NewHealthHandler(db *database.DB, sqs QueueReadyChecker) *HealthHandler {
	return &HealthHandler{db: db, sqs: sqs}
}

func (h *HealthHandler) Live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Ready checks PostgreSQL and SQS.
func (h *HealthHandler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := h.db.Pool.Ping(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "not_ready", "postgres unavailable")
		return
	}
	if h.sqs != nil {
		if err := h.sqs.Ready(ctx); err != nil {
			writeError(w, http.StatusServiceUnavailable, "not_ready", "sqs unavailable")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}
