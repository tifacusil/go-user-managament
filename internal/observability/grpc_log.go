package observability

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnaryServerLogger logs one structured line per completed unary gRPC call:
// full method, response code, and latency. On a non-OK code the error message
// is included. Mirrors the HTTP AccessLogger so HTTP and gRPC traffic share a
// log shape.
func UnaryServerLogger(base *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err)

		level := slog.LevelInfo
		switch code {
		case codes.OK:
			level = slog.LevelInfo
		case codes.Internal, codes.Unknown, codes.DataLoss, codes.Unavailable:
			level = slog.LevelError
		default:
			level = slog.LevelWarn
		}

		attrs := []slog.Attr{
			slog.String("event", "grpc_call"),
			slog.String("method", info.FullMethod),
			slog.String("code", code.String()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		}
		if err != nil {
			attrs = append(attrs, slog.String("err", status.Convert(err).Message()))
		}

		base.LogAttrs(ctx, level, "grpc request handled", attrs...)
		return resp, err
	}
}
