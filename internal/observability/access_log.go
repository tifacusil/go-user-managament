package observability

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// skipAccessLog holds paths that are too noisy to log on every hit (health
// probes and the metrics scrape run on a tight interval).
var skipAccessLog = map[string]bool{
	"/metrics": true,
}

// AccessLogger emits one structured line per completed HTTP request:
// method, path, status, response size, and latency. It uses the per-request
// logger from the context so trace_id is attached automatically.
//
// Place it after RequestLogger in the middleware chain so the context logger
// exists. Liveness/readiness probes and /metrics are skipped to avoid noise;
// HealthPath is passed in because it is configurable.
func AccessLogger(healthPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipAccessLog[r.URL.Path] || r.URL.Path == healthPath {
				next.ServeHTTP(w, r)
				return
			}

			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}

			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			}

			l := FromContext(r.Context())
			l.LogAttrs(r.Context(), level, "http request handled",
				slog.String("event", "http_access"),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", status),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
				slog.String("remote_addr", r.RemoteAddr),
			)
		})
	}
}
