package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"sync/atomic"
	"time"
)

type ctxKey int

const correlationKey ctxKey = 1

// SetupJSON configures the default slog handler as JSON on stdout.
func SetupJSON() {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	slog.SetDefault(slog.New(h))
}

// NewCorrelationID returns a random opaque id for request correlation.
func NewCorrelationID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// WithCorrelationID stores the correlation id in ctx.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationKey, id)
}

// CorrelationID returns the id from ctx, or empty.
func CorrelationID(ctx context.Context) string {
	v, _ := ctx.Value(correlationKey).(string)
	return v
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

	ProcessLatencyCount  atomic.Int64
	ProcessLatencySumMs  atomic.Int64
	ProcessLatencyLastMs atomic.Int64

	OutboxLagCount  atomic.Int64
	OutboxLagSumMs  atomic.Int64
	OutboxLagLastMs atomic.Int64
)

// ObserveProcessLatency records end-to-end processing duration.
func ObserveProcessLatency(d time.Duration) {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	ProcessLatencyCount.Add(1)
	ProcessLatencySumMs.Add(ms)
	ProcessLatencyLastMs.Store(ms)
}

// ObserveOutboxLag records delay from event occurrence to successful publish.
func ObserveOutboxLag(d time.Duration) {
	ms := d.Milliseconds()
	if ms < 0 {
		ms = 0
	}
	OutboxLagCount.Add(1)
	OutboxLagSumMs.Add(ms)
	OutboxLagLastMs.Store(ms)
}

// Snapshot returns current counter values for /metrics.
func Snapshot() map[string]int64 {
	m := map[string]int64{
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
		"process_latency_ms_last": ProcessLatencyLastMs.Load(),
		"outbox_lag_ms_last":      OutboxLagLastMs.Load(),
	}
	if c := ProcessLatencyCount.Load(); c > 0 {
		m["process_latency_ms_count"] = c
		m["process_latency_ms_avg"] = ProcessLatencySumMs.Load() / c
	}
	if c := OutboxLagCount.Load(); c > 0 {
		m["outbox_lag_ms_count"] = c
		m["outbox_lag_ms_avg"] = OutboxLagSumMs.Load() / c
	}
	return m
}
