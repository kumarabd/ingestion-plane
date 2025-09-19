package server

import (
	"context"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"google.golang.org/grpc"
)

func grpcLoggingInterceptor(log *logger.Handler) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		log.Info().
			Str("method", info.FullMethod).
			Msg("gRPC Request")

		resp, err := handler(ctx, req)

		if err != nil {
			log.Error().
				Err(err).
				Str("method", info.FullMethod).
				Msg("gRPC Request failed")
		} else {
			log.Info().
				Str("method", info.FullMethod).
				Msg("gRPC Request completed")
		}

		return resp, err
	}
}

func grpcMetricsInterceptor(metric *metrics.Handler) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		start := time.Now()

		resp, err := handler(ctx, req)

		// Record metrics
		status := "success"
		if err != nil {
			status = "error"
		}

		if metric != nil {
			metric.AddHistogram("grpc_request_latency_seconds", time.Since(start).Seconds(), map[string]string{
				"method": info.FullMethod,
				"status": status,
			})
			metric.IncrementCounter("grpc_requests_total", map[string]string{
				"method": info.FullMethod,
				"status": status,
			})
		}

		return resp, err
	}
}

func grpcErrorInterceptor(log *logger.Handler) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		resp, err := handler(ctx, req)

		if err != nil {
			// Log error details
			log.Error().
				Err(err).
				Str("method", info.FullMethod).
				Msg("gRPC Error")
		}

		return resp, err
	}
}
