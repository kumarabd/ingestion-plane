package ingest

import (
	"context"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	grpcCodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCService implements the IngestService gRPC interface
type GRPCService struct {
	ingestv1.UnimplementedIngestServiceServer
	handler *Handler
	log     *logger.Handler
	metric  *metrics.Handler
	tracer  trace.Tracer
}

// NewGRPCService creates a new gRPC service instance
func NewGRPCService(handler *Handler, log *logger.Handler, metric *metrics.Handler) *GRPCService {
	return &GRPCService{
		handler: handler,
		log:     log,
		metric:  metric,
		tracer:  otel.Tracer("gateway/grpc"),
	}
}

// Push implements the IngestService.Push gRPC method
func (s *GRPCService) Push(ctx context.Context, req *ingestv1.NormalizedLogBatch) (*ingestv1.Ack, error) {
	ctx, span := s.tracer.Start(ctx, "IngestService.Push")
	defer span.End()

	start := time.Now()
	processedCount := 0

	// Validate request
	if req == nil {
		span.RecordError(status.Error(grpcCodes.InvalidArgument, "request cannot be nil"))
		span.SetStatus(codes.Error, "invalid request")
		return &ingestv1.Ack{
			Success:        false,
			ProcessedCount: 0,
			ErrorMessage:   "request cannot be nil",
			ProcessedAt:    time.Now().UnixNano(),
		}, status.Error(grpcCodes.InvalidArgument, "request cannot be nil")
	}

	if len(req.Logs) == 0 {
		span.RecordError(status.Error(grpcCodes.InvalidArgument, "batch cannot be empty"))
		span.SetStatus(codes.Error, "empty batch")
		return &ingestv1.Ack{
			Success:        false,
			ProcessedCount: 0,
			ErrorMessage:   "batch cannot be empty",
			ProcessedAt:    time.Now().UnixNano(),
		}, status.Error(grpcCodes.InvalidArgument, "batch cannot be empty")
	}

	// Validate batch size
	if len(req.Logs) > s.handler.config.MaxBatchSize {
		span.RecordError(status.Error(grpcCodes.InvalidArgument, "batch size exceeds maximum"))
		span.SetStatus(codes.Error, "batch too large")
		return &ingestv1.Ack{
			Success:        false,
			ProcessedCount: 0,
			ErrorMessage:   "batch size exceeds maximum",
			ProcessedAt:    time.Now().UnixNano(),
		}, status.Error(grpcCodes.InvalidArgument, "batch size exceeds maximum")
	}

	// Process each log in the batch
	for _, protoLog := range req.Logs {
		// Convert protobuf log to internal format
		normalizedLog, err := s.convertProtoToNormalizedLog(protoLog)
		if err != nil {
			s.log.Warn().Err(err).Msg("Failed to convert protobuf log, skipping")
			continue
		}

		// Emit the normalized log
		if err := s.handler.emitter.EmitNormalizedLog(ctx, normalizedLog); err != nil {
			s.log.Error().Err(err).Msg("Failed to emit normalized log")
			// In shadow mode, we continue processing other logs
			continue
		}

		processedCount++
	}

	// Record metrics
	latency := time.Since(start).Seconds()
	s.metric.AddHistogram("grpc_push_latency_seconds", latency, map[string]string{
		"method": "Push",
	})
	s.metric.AddHistogram("grpc_batch_size", float64(len(req.Logs)), map[string]string{
		"method": "Push",
	})
	s.metric.IncrementCounter("grpc_logs_processed_total", map[string]string{
		"method": "Push",
		"status": "success",
	})

	// Set span attributes
	span.SetAttributes(
		attribute.Int("batch.size", len(req.Logs)),
		attribute.Int("processed.count", processedCount),
		attribute.Float64("latency.seconds", latency),
	)

	// Return success response
	return &ingestv1.Ack{
		Success:        true,
		ProcessedCount: uint32(processedCount),
		ErrorMessage:   "",
		ProcessedAt:    time.Now().UnixNano(),
	}, nil
}

// convertProtoToNormalizedLog converts a protobuf NormalizedLog to internal format
func (s *GRPCService) convertProtoToNormalizedLog(protoLog *ingestv1.NormalizedLog) (*NormalizedLog, error) {
	// Convert schema type
	var schema string
	switch protoLog.Schema {
	case ingestv1.SchemaType_SCHEMA_TYPE_JSON:
		schema = "JSON"
	case ingestv1.SchemaType_SCHEMA_TYPE_LOGFMT:
		schema = "LOGFMT"
	case ingestv1.SchemaType_SCHEMA_TYPE_TEXT:
		schema = "TEXT"
	default:
		schema = "TEXT"
	}

	// Create internal normalized log
	normalizedLog := &NormalizedLog{
		Timestamp: protoLog.Timestamp,
		Labels:    protoLog.Labels,
		Message:   protoLog.Message,
		Fields:    protoLog.Fields,
		Schema:    schema,
		Sanitized: protoLog.Sanitized,
		OrigLen:   protoLog.OrigLen,
	}

	return normalizedLog, nil
}
