package observability

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	reqTotal    *prometheus.CounterVec
	reqDuration *prometheus.HistogramVec
}

func NewMetrics() *Metrics {
	m := &Metrics{
		reqTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total HTTP requests by method, route and status.",
		}, []string{"method", "route", "status"}),
		reqDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency. Default buckets cover p50..p99.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route", "status"}),
	}
	prometheus.MustRegister(m.reqTotal, m.reqDuration)
	return m
}

func (m *Metrics) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		start := time.Now()
		next.ServeHTTP(ww, r)

		route := chi.RouteContext(r.Context()).RoutePattern()
		if route == "" {
			route = "unmatched"
		}
		status := strconv.Itoa(ww.Status())
		m.reqTotal.WithLabelValues(r.Method, route, status).Inc()
		m.reqDuration.WithLabelValues(r.Method, route, status).Observe(time.Since(start).Seconds())
	})
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}
