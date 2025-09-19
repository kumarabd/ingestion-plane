package ingest

import (
	"context"
	"fmt"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

// Config contains configuration for log ingestion
type Config struct {
	MaxLogSize     int           `json:"max_log_size" yaml:"max_log_size" default:"1048576"`                // 1MB
	MaxBatchSize   int           `json:"max_batch_size" yaml:"max_batch_size" default:"1000"`               // 1000 logs per batch
	MaxLabels      int           `json:"max_labels" yaml:"max_labels" default:"100"`                        // 100 labels per log
	MaxFields      int           `json:"max_fields" yaml:"max_fields" default:"200"`                        // 200 fields per log
	RequestTimeout time.Duration `json:"request_timeout" yaml:"request_timeout" default:"30s"`              // 30s timeout
	ValidateUTF8   bool          `json:"validate_utf8" yaml:"validate_utf8" default:"true"`                 // Validate UTF-8
	AllowedSchemas []string      `json:"allowed_schemas" yaml:"allowed_schemas" default:"JSON,LOGFMT,TEXT"` // Allowed schema types
}

// NormalizedLogEmitter interface for emitting normalized logs
type NormalizedLogEmitter interface {
	EmitNormalizedLog(ctx context.Context, log *NormalizedLog) error
}

// NormalizedLog represents the normalized log structure
type NormalizedLog struct {
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields"`
	Schema    string            `json:"schema"`
	Sanitized bool              `json:"sanitized"`
	OrigLen   uint32            `json:"orig_len"`
}

// RawLog represents a single raw log entry
type RawLog struct {
	Timestamp  time.Time         `json:"timestamp"`
	Labels     map[string]string `json:"labels"`
	Payload    string            `json:"payload"`     // raw line
	FormatHint string            `json:"format_hint"` // "json"|"logfmt"|"text"|"" (optional)
}

// BatchProcessResult contains the results of batch processing
type BatchProcessResult struct {
	ProcessedCount int      `json:"processed_count"`
	TotalReceived  int      `json:"total_received"`
	Errors         []string `json:"errors,omitempty"`
}

// Note: Using official Grafana Loki logproto types instead of custom types

// CommonIngestRequest represents a unified request that all protocols convert to
type CommonIngestRequest struct {
	Protocol string               `json:"protocol"` // "otlp", "json", "loki"
	Records  []CommonIngestRecord `json:"records"`
}

type CommonIngestRecord struct {
	Timestamp int64             `json:"timestamp"` // nanoseconds
	Labels    map[string]string `json:"labels"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// Handler provides access to configuration, processor, and emitter
type Handler struct {
	config    *Config
	processor *Processor
	emitter   *Emitter
}

// NewHandler creates a new handler with configuration and initializes processor and emitter
func NewHandler(config *Config, emitterConfig *EmitterConfig, log *logger.Handler, metric *metrics.Handler) (*Handler, error) {
	// Initialize emitter
	emitter, err := NewEmitter(emitterConfig, log, metric)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize emitter: %w", err)
	}

	// Create handler with embedded components
	handler := &Handler{
		config:  config,
		emitter: emitter,
	}

	// Initialize processor with the handler
	handler.processor = NewProcessor(handler, log, metric)

	return handler, nil
}

// GetConfig returns the handler configuration
func (h *Handler) GetConfig() *Config {
	return h.config
}

// GetEmitter returns the handler emitter
func (h *Handler) GetEmitter() NormalizedLogEmitter {
	return h.emitter
}

// GetProcessor returns the handler processor
func (h *Handler) GetProcessor() *Processor {
	return h.processor
}

// Start starts the handler's emitter (required for forwarder)
func (h *Handler) Start() error {
	return h.emitter.Start()
}

// Stop stops the handler's emitter (required for forwarder)
func (h *Handler) Stop() error {
	return h.emitter.Stop()
}
