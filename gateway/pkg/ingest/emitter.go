package ingest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/forwarder"
)

// EmitterConfig contains configuration for the log emitter
type EmitterConfig struct {
	OutputType    string            `json:"output_type" yaml:"output_type" default:"stdout"`  // stdout, kafka, grpc, forwarder
	BatchSize     int               `json:"batch_size" yaml:"batch_size" default:"100"`       // Batch size for output
	FlushInterval int               `json:"flush_interval" yaml:"flush_interval" default:"5"` // Flush interval in seconds
	Forwarder     *forwarder.Config `json:"forwarder" yaml:"forwarder"`                       // Forwarder configuration
}

// Emitter handles emission of normalized logs
type Emitter struct {
	config    *EmitterConfig
	log       *logger.Handler
	metric    *metrics.Handler
	forwarder *forwarder.Forwarder
}

// NewEmitter creates a new log emitter
func NewEmitter(config *EmitterConfig, log *logger.Handler, metric *metrics.Handler) (*Emitter, error) {
	emitter := &Emitter{
		config: config,
		log:    log,
		metric: metric,
	}

	// Initialize forwarder if configured
	if config.OutputType == "forwarder" && config.Forwarder != nil {
		forwarder := forwarder.NewForwarder(config.Forwarder, log, metric)
		emitter.forwarder = forwarder
	}

	return emitter, nil
}

// EmitNormalizedLog emits a normalized log
func (e *Emitter) EmitNormalizedLog(ctx context.Context, normalizedLog *NormalizedLog) error {
	switch e.config.OutputType {
	case "stdout":
		return e.emitToStdout(ctx, normalizedLog)
	case "kafka":
		return e.emitToKafka(ctx, normalizedLog)
	case "grpc":
		return e.emitToGRPC(ctx, normalizedLog)
	case "forwarder":
		return e.emitToForwarder(ctx, normalizedLog)
	default:
		return fmt.Errorf("unsupported output type: %s", e.config.OutputType)
	}
}

// emitToStdout emits normalized log to stdout (for development/testing)
func (e *Emitter) emitToStdout(ctx context.Context, normalizedLog *NormalizedLog) error {
	// Convert to JSON for output
	jsonData, err := json.Marshal(normalizedLog)
	if err != nil {
		e.log.Error().Err(err).Msg("Failed to marshal normalized log to JSON")
		return err
	}

	// Log the normalized log
	e.log.Info().
		Str("schema", normalizedLog.Schema).
		Int64("timestamp", normalizedLog.Timestamp).
		Uint32("orig_len", normalizedLog.OrigLen).
		Bool("sanitized", normalizedLog.Sanitized).
		RawJSON("normalized_log", jsonData).
		Msg("Emitted normalized log")

	// Record metrics
	e.metric.IncrementCounter("normalized_logs_emitted_total", map[string]string{
		"output_type": "stdout",
		"schema":      normalizedLog.Schema,
	})

	return nil
}

// emitToKafka emits normalized log to Kafka (placeholder for future implementation)
func (e *Emitter) emitToKafka(ctx context.Context, normalizedLog *NormalizedLog) error {
	// TODO: Implement Kafka emission
	e.log.Info().Msg("Kafka emission not yet implemented, falling back to stdout")
	return e.emitToStdout(ctx, normalizedLog)
}

// emitToGRPC emits normalized log via gRPC (placeholder for future implementation)
func (e *Emitter) emitToGRPC(ctx context.Context, normalizedLog *NormalizedLog) error {
	// TODO: Implement gRPC emission
	e.log.Info().Msg("gRPC emission not yet implemented, falling back to stdout")
	return e.emitToStdout(ctx, normalizedLog)
}

// emitToForwarder emits normalized log via forwarder
func (e *Emitter) emitToForwarder(ctx context.Context, normalizedLog *NormalizedLog) error {
	if e.forwarder == nil {
		return fmt.Errorf("forwarder not initialized")
	}

	// Convert to forwarder log entry format
	logEntry := &forwarder.LogEntry{
		Timestamp: normalizedLog.Timestamp,
		Labels:    normalizedLog.Labels,
		Message:   normalizedLog.Message,
		Fields:    normalizedLog.Fields,
		Schema:    normalizedLog.Schema,
		Sanitized: normalizedLog.Sanitized,
		OrigLen:   normalizedLog.OrigLen,
	}

	// Forward the log
	return e.forwarder.Forward(ctx, logEntry)
}

// Start starts the emitter (required for forwarder)
func (e *Emitter) Start() error {
	if e.forwarder != nil {
		return e.forwarder.Start()
	}
	return nil
}

// Stop stops the emitter (required for forwarder)
func (e *Emitter) Stop() error {
	if e.forwarder != nil {
		return e.forwarder.Stop()
	}
	return nil
}

// ValidateConfig validates the emitter configuration
func (e *Emitter) ValidateConfig() error {
	validOutputTypes := []string{"stdout", "kafka", "grpc", "forwarder"}
	for _, validType := range validOutputTypes {
		if e.config.OutputType == validType {
			return nil
		}
	}
	return fmt.Errorf("invalid output_type: %s, must be one of %v", e.config.OutputType, validOutputTypes)
}
