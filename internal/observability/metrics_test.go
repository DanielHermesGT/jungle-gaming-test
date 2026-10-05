package observability_test

import (
	"context"
	"testing"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
)

func TestCorrelationIDRoundTrip(t *testing.T) {
	ctx := observability.WithCorrelationID(context.Background(), "corr-1")
	if got := observability.CorrelationID(ctx); got != "corr-1" {
		t.Fatalf("got %q", got)
	}
	if observability.NewCorrelationID() == "" {
		t.Fatal("empty id")
	}
}

func TestLatencyAndLagMetrics(t *testing.T) {
	observability.ObserveProcessLatency(50 * time.Millisecond)
	observability.ObserveOutboxLag(120 * time.Millisecond)
	snap := observability.Snapshot()
	if snap["process_latency_ms_last"] < 50 {
		t.Fatalf("process last=%d", snap["process_latency_ms_last"])
	}
	if snap["outbox_lag_ms_last"] < 120 {
		t.Fatalf("lag last=%d", snap["outbox_lag_ms_last"])
	}
	if snap["process_latency_ms_count"] < 1 || snap["outbox_lag_ms_count"] < 1 {
		t.Fatalf("snap=%v", snap)
	}
}
