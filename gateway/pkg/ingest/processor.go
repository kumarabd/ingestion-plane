package ingest

import (
	"context"
	"fmt"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

// Processor handles all protocol ingestion through a common interface
type Processor struct {
	handler *Handler
	log     *logger.Handler
	metric  *metrics.Handler
}

// NewProcessor creates a new unified processor
func NewProcessor(handler *Handler, log *logger.Handler, metric *metrics.Handler) *Processor {
	return &Processor{
		handler: handler,
		log:     log,
		metric:  metric,
	}
}

// ProcessRequest processes any protocol request through the unified interface
func (up *Processor) ProcessRequest(ctx context.Context, req *CommonIngestRequest) (*BatchProcessResult, error) {
	if req == nil {
		return &BatchProcessResult{
			ProcessedCount: 0,
			TotalReceived:  0,
			Errors:         []string{"request cannot be nil"},
		}, fmt.Errorf("request cannot be nil")
	}

	if len(req.Records) == 0 {
		return &BatchProcessResult{
			ProcessedCount: 0,
			TotalReceived:  0,
			Errors:         []string{"empty batch"},
		}, fmt.Errorf("empty batch")
	}

	// Validate batch size against limits
	if len(req.Records) > up.handler.GetConfig().MaxBatchSize {
		if up.metric != nil {
			up.metric.IncIngestRejectedTotal("batch_too_large")
		}
		return &BatchProcessResult{
			ProcessedCount: 0,
			TotalReceived:  len(req.Records),
			Errors:         []string{"batch size exceeds maximum"},
		}, fmt.Errorf("batch size exceeds maximum")
	}

	processedCount := 0
	var errors []string

	// Process each record in the batch
	for i, record := range req.Records {
		// Convert common record to normalized log
		normalizedLog, err := up.convertToNormalizedLog(record)
		if err != nil {
			up.log.Warn().Err(err).Int("record_index", i).Str("protocol", req.Protocol).Msg("Failed to convert record, skipping")
			errors = append(errors, fmt.Sprintf("record %d: %v", i, err))
			continue
		}

		// Emit the normalized log
		if err := up.handler.GetEmitter().EmitNormalizedLog(ctx, normalizedLog); err != nil {
			up.log.Error().Err(err).Int("record_index", i).Str("protocol", req.Protocol).Msg("Failed to emit normalized log")
			errors = append(errors, fmt.Sprintf("record %d emit failed: %v", i, err))
			// In shadow mode, we continue processing other records
			continue
		}

		processedCount++
	}

	// Record metrics
	if up.metric != nil {
		up.metric.IncIngestBatchesTotal(req.Protocol)
		for i := 0; i < processedCount; i++ {
			up.metric.IncIngestRecordsTotal(req.Protocol, "normalized")
		}
	}

	return &BatchProcessResult{
		ProcessedCount: processedCount,
		TotalReceived:  len(req.Records),
		Errors:         errors,
	}, nil
}

// convertToNormalizedLog converts a common ingest record to normalized log format
func (up *Processor) convertToNormalizedLog(record CommonIngestRecord) (*NormalizedLog, error) {
	// Determine schema based on message content
	schema := DetermineSchema(record.Message, record.Fields)

	// Create normalized log
	normalizedLog := &NormalizedLog{
		Timestamp: record.Timestamp,
		Labels:    record.Labels,
		Message:   record.Message,
		Fields:    record.Fields,
		Schema:    schema,
		Sanitized: false, // Will be determined by the pipeline
		OrigLen:   uint32(len(record.Message)),
	}

	return normalizedLog, nil
}
