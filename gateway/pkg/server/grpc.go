package server

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/kumarabd/gokit/logger"
	ingestv1 "github.com/kumarabd/ingestion-plane/contracts/ingest/v1"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCConfig contains configuration for the gRPC server
type GRPCConfig struct {
	Host                  string `json:"host" yaml:"host" default:"0.0.0.0"`
	MaxBatch              int    `json:"max_batch" yaml:"max_batch" default:"1000"`
	Port                  string `json:"port" yaml:"port" default:"9090"`
	MaxConcurrentStreams  uint32 `json:"max_concurrent_streams" yaml:"max_concurrent_streams" default:"100"`
	MaxConnectionIdle     string `json:"max_connection_idle" yaml:"max_connection_idle" default:"30s"`
	MaxConnectionAge      string `json:"max_connection_age" yaml:"max_connection_age" default:"60s"`
	MaxConnectionAgeGrace string `json:"max_connection_age_grace" yaml:"max_connection_age_grace" default:"10s"`
	Time                  string `json:"time" yaml:"time" default:"5s"`
	Timeout               string `json:"timeout" yaml:"timeout" default:"1s"`
}

// GRPC implements the Server interface for gRPC
type GRPC struct {
	ingestv1.UnimplementedIngestServiceServer
	handler   *grpc.Server
	ingest    *ingest.Handler
	log       *logger.Handler
	metric    *metrics.Handler
	tracer    trace.Tracer
	config    *GRPCConfig
	listener  net.Listener
	isRunning bool
	mu        sync.RWMutex
}

// NewGRPC creates a new gRPC server instance
func NewGRPC(config *GRPCConfig, in *ingest.Handler, log *logger.Handler, metric *metrics.Handler) *GRPC {
	// Initiate GRPC Server object
	middlewares := middleware.ChainUnaryServer(
		grpcLoggingInterceptor(log),
		grpcErrorInterceptor(log),
		grpcMetricsInterceptor(metric),
	)

	handler := grpc.NewServer(
		grpc.UnaryInterceptor(middlewares),
		grpc.ChainStreamInterceptor(
		// grpcObj.SizeInterceptor(),
		),
	)

	server := &GRPC{
		handler: handler,
		ingest:  in,
		log:     log,
		metric:  metric,
		config:  config,
		tracer:  otel.Tracer("gateway/grpc"),
	}

	// Register the IngestService
	ingestv1.RegisterIngestServiceServer(server.handler, server)

	return server
}

// Start starts the gRPC server
func (s *GRPC) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("gRPC server is already running")
	}

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	s.isRunning = true
	s.log.Info().Msgf("Starting gRPC server on %s", addr)

	return s.handler.Serve(listener)
}

// Stop gracefully shuts down the gRPC server
func (s *GRPC) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning || s.handler == nil {
		return nil
	}

	s.log.Info().Msg("Shutting down gRPC server...")

	// Graceful stop
	s.handler.GracefulStop()

	if s.listener != nil {
		s.listener.Close()
	}

	s.isRunning = false
	s.log.Info().Msg("gRPC server stopped")
	return nil
}

// IsRunning returns true if the gRPC server is currently running
func (s *GRPC) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// Push implements the IngestService.Push gRPC method
func (s *GRPC) Push(ctx context.Context, req *ingestv1.NormalizedLogBatch) (*ingestv1.Ack, error) {
	ctx, span := s.tracer.Start(ctx, "IngestService.Push")
	defer span.End()

	start := time.Now()

	// Validate request
	if req == nil {
		span.RecordError(status.Error(grpcCodes.InvalidArgument, "request cannot be nil"))
		span.SetStatus(codes.Error, "invalid request")
		return &ingestv1.Ack{
			Message: "request cannot be nil",
		}, status.Error(grpcCodes.InvalidArgument, "request cannot be nil")
	}

	if len(req.Records) == 0 {
		span.RecordError(status.Error(grpcCodes.InvalidArgument, "batch cannot be empty"))
		span.SetStatus(codes.Error, "empty batch")
		return &ingestv1.Ack{
			Message: "batch cannot be empty",
		}, status.Error(grpcCodes.InvalidArgument, "batch cannot be empty")
	}

	// Convert gRPC request to raw log batch (same pattern as HTTP handlers)
	rawBatch := s.convertGRPCToRawLogBatch(req)

	// Validate batch size
	if len(rawBatch.Records) > s.ingest.GetConfig().MaxBatchSize {
		span.RecordError(status.Error(grpcCodes.InvalidArgument, "batch size exceeds maximum"))
		span.SetStatus(codes.Error, "batch too large")
		return &ingestv1.Ack{
			Message: "batch size exceeds maximum",
		}, status.Error(grpcCodes.InvalidArgument, "batch size exceeds maximum")
	}

	// Process using the same pattern as HTTP handlers
	batch, err := s.ingest.NormalizeAndRedactBatch(ctx, rawBatch, s.log, s.metric)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "processing failed")
		return &ingestv1.Ack{
			Message: "processing failed",
		}, status.Error(grpcCodes.Internal, "processing failed")
	}

	// Emit the batch
	if batch != nil && len(batch.Records) > 0 {
		if err := s.ingest.GetEmitter().EmitNormalizedLogBatch(ctx, batch); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "emission failed")
			return &ingestv1.Ack{
				Message: "emission failed",
			}, status.Error(grpcCodes.Internal, "emission failed")
		}
	}

	// Record metrics
	latency := time.Since(start).Seconds()
	s.metric.AddHistogram("grpc_push_latency_seconds", latency, map[string]string{
		"method": "Push",
	})
	s.metric.AddHistogram("grpc_batch_size", float64(len(rawBatch.Records)), map[string]string{
		"method": "Push",
	})
	s.metric.IncrementCounter("grpc_logs_processed_total", map[string]string{
		"method": "Push",
		"status": "success",
	})

	// Set span attributes
	span.SetAttributes(
		attribute.Int("batch.size", len(rawBatch.Records)),
		attribute.Int("processed.count", len(batch.Records)),
		attribute.Float64("latency.seconds", latency),
	)

	// Return success response
	return &ingestv1.Ack{
		Message: "success",
	}, nil
}

// convertGRPCToRawLogBatch converts gRPC request to raw log batch (same pattern as HTTP handlers)
func (s *GRPC) convertGRPCToRawLogBatch(grpcReq *ingestv1.NormalizedLogBatch) *ingest.RawLogBatch {
	var records []ingest.RawLog

	for _, protoLog := range grpcReq.Records {
		record := ingest.RawLog{
			Timestamp: protoLog.Timestamp.AsTime(),
			Labels:    protoLog.Labels,
			Payload:   protoLog.Message,
		}

		records = append(records, record)
	}

	return &ingest.RawLogBatch{
		Records: records,
	}
}
