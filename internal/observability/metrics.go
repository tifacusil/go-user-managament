package observability

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

type Metrics struct {
	reqTotal        *prometheus.CounterVec
	reqDuration     *prometheus.HistogramVec
	grpcReqTotal    *prometheus.CounterVec
	grpcReqDuration *prometheus.HistogramVec
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
		grpcReqTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "grpc_requests_total",
			Help: "Total gRPC requests by method and status code.",
		}, []string{"method", "code"}),
		grpcReqDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "grpc_request_duration_seconds",
			Help:    "gRPC request latency (whole RPC, DB work included).",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "code"}),
	}
	prometheus.MustRegister(m.reqTotal, m.reqDuration, m.grpcReqTotal, m.grpcReqDuration)
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

// UnaryServerInterceptor records count and latency per unary gRPC call,
// labelled by full method and final status code. Mirrors the HTTP Middleware
// so HTTP and gRPC traffic land on the same /metrics.
func (m *Metrics) UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err).String()

		m.grpcReqTotal.WithLabelValues(info.FullMethod, code).Inc()
		m.grpcReqDuration.WithLabelValues(info.FullMethod, code).Observe(time.Since(start).Seconds())
		return resp, err
	}
}

func (m *Metrics) Handler() http.Handler {
	return promhttp.Handler()
}
