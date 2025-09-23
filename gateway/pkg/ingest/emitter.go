package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
)

// EmitterConfig contains configuration for the log emitter
type EmitterConfig struct {
	OutputType    string `json:"output_type" yaml:"output_type" default:"stdout"`  // stdout, kafka, grpc
	BatchSize     int    `json:"batch_size" yaml:"batch_size" default:"100"`       // Batch size for output
	FlushInterval int    `json:"flush_interval" yaml:"flush_interval" default:"5"` // Flush interval in seconds
}

// Emitter handles emission of normalized logs
type Emitter struct {
	config *EmitterConfig
	log    *logger.Handler
	metric *metrics.Handler
}

// NewEmitter creates a new log emitter
func NewEmitter(config *EmitterConfig, log *logger.Handler, metric *metrics.Handler) (*Emitter, error) {
	emitter := &Emitter{
		config: config,
		log:    log,
		metric: metric,
	}

	return emitter, nil
}

// EmitNormalizedLogBatch emits a batch of normalized logs
func (e *Emitter) EmitNormalizedLogBatch(ctx context.Context, batch *logtypes.NormalizedLogBatch) error {
	if batch == nil || len(batch.Records) == 0 {
		return nil
	}

	switch e.config.OutputType {
	case "stdout":
		return e.emitBatchToStdout(ctx, batch)
	case "kafka":
		return e.emitBatchToKafka(ctx, batch)
	case "grpc":
		return e.emitBatchToGRPC(ctx, batch)
	default:
		return fmt.Errorf("unsupported output type: %s", e.config.OutputType)
	}
}

// emitBatchToStdout emits a batch of normalized logs to stdout (for development/testing)
func (e *Emitter) emitBatchToStdout(ctx context.Context, batch *logtypes.NormalizedLogBatch) error {
	// Convert batch to JSON for output
	jsonData, err := json.Marshal(batch)
	if err != nil {
		if e.log != nil {
			e.log.Error().Err(err).Msg("Failed to marshal normalized log batch to JSON")
		}
		return err
	}

	// Log the batch
	if e.log != nil {
		e.log.Info().
			Int("batch_size", len(batch.Records)).
			RawJSON("normalized_log_batch", jsonData).
			Msg("Emitted normalized log batch")
	}

	// Record metrics for each log in the batch
	if e.metric != nil {
		for _, normalizedLog := range batch.Records {
			e.metric.IncrementCounter("normalized_logs_emitted_total", map[string]string{
				"output_type": "stdout",
				"schema":      normalizedLog.Schema,
			})
		}
	}

	return nil
}

// emitBatchToKafka emits a batch of normalized logs to Kafka (placeholder for future implementation)
func (e *Emitter) emitBatchToKafka(ctx context.Context, batch *logtypes.NormalizedLogBatch) error {
	// TODO: Implement Kafka batch emission
	if e.log != nil {
		e.log.Info().Msg("Kafka batch emission not yet implemented, falling back to stdout")
	}
	return e.emitBatchToStdout(ctx, batch)
}

// emitBatchToGRPC emits a batch of normalized logs via gRPC (placeholder for future implementation)
func (e *Emitter) emitBatchToGRPC(ctx context.Context, batch *logtypes.NormalizedLogBatch) error {
	// TODO: Implement gRPC batch emission
	if e.log != nil {
		e.log.Info().Msg("gRPC batch emission not yet implemented, falling back to stdout")
	}
	return e.emitBatchToStdout(ctx, batch)
}

// Start starts the emitter
func (e *Emitter) Start() error {
	return nil
}

// Stop stops the emitter
func (e *Emitter) Stop() error {
	return nil
}

// ValidateConfig validates the emitter configuration
func (e *Emitter) ValidateConfig() error {
	validOutputTypes := []string{"stdout", "kafka", "grpc"}
	for _, validType := range validOutputTypes {
		if e.config.OutputType == validType {
			return nil
		}
	}
	return fmt.Errorf("invalid output_type: %s, must be one of %v", e.config.OutputType, validOutputTypes)
}
