package web

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/DanielHermesGT/jungle-gaming-test/internal/observability"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// withRequestLog adds correlationId, logs each request, and records HTTP latency.
func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corr := strings.TrimSpace(r.Header.Get("X-Correlation-Id"))
		if corr == "" {
			corr = observability.NewCorrelationID()
		}
		w.Header().Set("X-Correlation-Id", corr)
		ctx := observability.WithCorrelationID(r.Context(), corr)

		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r.WithContext(ctx))
		latency := time.Since(start)

		path := r.URL.Path
		if !isQuietPath(path) {
			slog.Info("http request",
				"correlationId", corr,
				"method", r.Method,
				"path", path,
				"status", rec.status,
				"latencyMs", latency.Milliseconds(),
			)
		}
	})
}

func isQuietPath(path string) bool {
	return path == "/health/live" || path == "/health/ready" || path == "/metrics"
}
