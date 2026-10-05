package observability

import (
	"log/slog"
	"os"
	"sync/atomic"
)

// SetupJSON configures the default slog handler as JSON on stdout.
func SetupJSON() {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
}

// Counters are process-local metrics (README §12) without a Prometheus dependency.
var (
	WagerProcessed       atomic.Int64
	WagerRejected        atomic.Int64
	WagerPendingRef      atomic.Int64
	IdempotentReplays    atomic.Int64
	Conflicts            atomic.Int64
	SQSTransientErrors   atomic.Int64
	SQSDLQ               atomic.Int64
	OutboxPublished      atomic.Int64
	OutboxRetries        atomic.Int64
	ReconcileDivergences atomic.Int64
)

// Snapshot returns current counter values for /metrics.
func Snapshot() map[string]int64 {
	return map[string]int64{
		"wager_processed":         WagerProcessed.Load(),
		"wager_rejected":          WagerRejected.Load(),
		"wager_pending_reference": WagerPendingRef.Load(),
		"idempotent_replays":      IdempotentReplays.Load(),
		"conflicts":               Conflicts.Load(),
		"sqs_transient_errors":    SQSTransientErrors.Load(),
		"sqs_dlq":                 SQSDLQ.Load(),
		"outbox_published":        OutboxPublished.Load(),
		"outbox_retries":          OutboxRetries.Load(),
		"reconcile_divergences":   ReconcileDivergences.Load(),
	}
}
