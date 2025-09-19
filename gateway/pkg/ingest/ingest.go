package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

// Config contains configuration for log ingestion
type Config struct {
	MaxLogSize      int           `json:"max_log_size" yaml:"max_log_size" default:"1048576"`                // 1MB
	MaxMessageBytes int           `json:"max_message_bytes" yaml:"max_message_bytes" default:"1048576"`      // 1MB max message size
	MaxBatchSize    int           `json:"max_batch_size" yaml:"max_batch_size" default:"1000"`               // 1000 logs per batch
	MaxLabels       int           `json:"max_labels" yaml:"max_labels" default:"100"`                        // 100 labels per log
	MaxFields       int           `json:"max_fields" yaml:"max_fields" default:"200"`                        // 200 fields per log
	RequestTimeout  time.Duration `json:"request_timeout" yaml:"request_timeout" default:"30s"`              // 30s timeout
	ValidateUTF8    bool          `json:"validate_utf8" yaml:"validate_utf8" default:"true"`                 // Validate UTF-8
	AllowedSchemas  []string      `json:"allowed_schemas" yaml:"allowed_schemas" default:"JSON,LOGFMT,TEXT"` // Allowed schema types
}

// RedactionReport contains information about PII redaction applied to a log
type RedactionReport struct {
	Applied bool     `json:"applied"`
	Rules   []string `json:"rules"`
	Count   int      `json:"count"`
}

// NormalizedLogEmitter interface for emitting normalized logs
type NormalizedLogEmitter interface {
	EmitNormalizedLogBatch(ctx context.Context, batch *NormalizedLogBatch) error
}

// NormalizedLog represents the normalized log structure
type NormalizedLog struct {
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Message   string            `json:"message"`
	Schema    string            `json:"schema"`
	Sanitized bool              `json:"sanitized"`
	Truncated bool              `json:"truncated"`
	Redaction RedactionReport   `json:"redaction"`
}

// RawLog represents a single raw log line as received from clients.
type RawLog struct {
	Timestamp  time.Time         `json:"timestamp"`             // optional: client-provided timestamp
	Labels     map[string]string `json:"labels,omitempty"`      // optional structured labels (service, env, etc.)
	Payload    string            `json:"payload"`               // the raw log message, unnormalized
	FormatHint string            `json:"format_hint,omitempty"` // optional: "json"|"logfmt"|"text"
}

// RawLogBatch is a batch of raw logs posted to /v1/ingest in JSON mode.
type RawLogBatch struct {
	Records []RawLog `json:"records"` // required; must not be empty
}

// NormalizedLogBatch is a batch of normalized logs
type NormalizedLogBatch struct {
	Records []NormalizedLog `json:"records"` // required; must not be empty
}

// Handler provides access to configuration and emitter
type Handler struct {
	config  *Config
	emitter *Emitter
}

// NewHandler creates a new handler with configuration and initializes emitter
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

// Start starts the handler's emitter (required for forwarder)
func (h *Handler) Start() error {
	return h.emitter.Start()
}

// Stop stops the handler's emitter (required for forwarder)
func (h *Handler) Stop() error {
	return h.emitter.Stop()
}

// NormalizeAndRedactBatch processes a raw log batch by normalizing and redacting it
func (h *Handler) NormalizeAndRedactBatch(ctx context.Context, batch *RawLogBatch, log *logger.Handler, metric *metrics.Handler) (*NormalizedLogBatch, error) {
	start := time.Now()
	success := true

	if batch == nil || len(batch.Records) == 0 {
		return &NormalizedLogBatch{Records: []NormalizedLog{}}, nil
	}

	var normalizedRecords []NormalizedLog

	// Process each RawLog in the batch
	for _, rawLog := range batch.Records {
		// Normalize the raw log
		normalizedLog := NormalizeRawLog(rawLog, h.config.MaxMessageBytes)
		normalizedRecords = append(normalizedRecords, normalizedLog)

		// Track metrics for each record
		if metric != nil {
			// Increment records total for JSON normalized records
			metric.IncIngestRecordsTotal("json", "normalized")

			// Track PII redactions for each rule applied
			if normalizedLog.Redaction.Applied {
				for _, rule := range normalizedLog.Redaction.Rules {
					metric.IncPIIRedactionsTotal(rule)
				}
			}
		}
	}

	// Record processing metrics
	latency := time.Since(start)
	if metric != nil {
		metric.ObserveNormalizeLatency(latency, success)
		metric.AddHistogram("raw_worker_processing_latency_seconds", latency.Seconds(), map[string]string{})
		metric.IncrementCounter("raw_worker_items_processed_total", map[string]string{})
	}

	if log != nil {
		log.Debug().
			Int("processed_count", len(normalizedRecords)).
			Int("total_received", len(batch.Records)).
			Msg("Processed raw batch")
	}

	return &NormalizedLogBatch{Records: normalizedRecords}, nil
}

// determineSchema determines the schema based on format hint or payload content
func (h *Handler) determineSchema(payload, formatHint string) string {
	// Use format hint if provided
	if formatHint != "" {
		switch strings.ToUpper(formatHint) {
		case "JSON":
			return "JSON"
		case "LOGFMT":
			return "LOGFMT"
		case "TEXT":
			return "TEXT"
		}
	}

	// Auto-detect using shared utility function
	return DetermineSchema(payload, nil)
}
