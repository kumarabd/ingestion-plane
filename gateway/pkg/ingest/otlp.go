package ingest

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

// Config contains configuration for OTLP ingestion
type Config struct {
	MaxLogSize     int           `json:"max_log_size" yaml:"max_log_size" default:"1048576"`                // 1MB
	MaxBatchSize   int           `json:"max_batch_size" yaml:"max_batch_size" default:"1000"`               // 1000 logs per batch
	MaxLabels      int           `json:"max_labels" yaml:"max_labels" default:"100"`                        // 100 labels per log
	MaxFields      int           `json:"max_fields" yaml:"max_fields" default:"200"`                        // 200 fields per log
	RequestTimeout time.Duration `json:"request_timeout" yaml:"request_timeout" default:"30s"`              // 30s timeout
	ValidateUTF8   bool          `json:"validate_utf8" yaml:"validate_utf8" default:"true"`                 // Validate UTF-8
	AllowedSchemas []string      `json:"allowed_schemas" yaml:"allowed_schemas" default:"JSON,LOGFMT,TEXT"` // Allowed schema types
}

// Handler handles OTLP log ingestion
type Handler struct {
	config  *Config
	log     *logger.Handler
	metric  *metrics.Handler
	emitter NormalizedLogEmitter
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

// NewOTLPHandler creates a new OTLP ingestion handler
func NewOTLPHandler(config *Config, log *logger.Handler, metric *metrics.Handler, emitter NormalizedLogEmitter) *Handler {
	return &Handler{
		config:  config,
		log:     log,
		metric:  metric,
		emitter: emitter,
	}
}

// HandleOTLPHTTP handles OTLP HTTP log ingestion
func (h *Handler) HandleOTLPHTTP(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.config.RequestTimeout)
	defer cancel()

	// Read and validate request body
	body, err := c.GetRawData()
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to read request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Validate request size
	if len(body) > h.config.MaxLogSize {
		h.log.Warn().Int("size", len(body)).Int("max_size", h.config.MaxLogSize).Msg("Request body too large")
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body too large"})
		return
	}

	// Parse OTLP request
	req := plogotlp.NewExportRequest()
	if err := req.UnmarshalProto(body); err != nil {
		h.log.Error().Err(err).Msg("Failed to unmarshal OTLP request")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid OTLP request format"})
		return
	}

	// Process logs
	processedCount, err := h.processLogs(ctx, req.Logs())
	if err != nil {
		h.log.Error().Err(err).Msg("Failed to process logs")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process logs"})
		return
	}

	// Record metrics
	h.metric.IncrementCounter("otlp_logs_processed_total", map[string]string{
		"status": "success",
		"format": "http",
	})
	h.metric.AddHistogram("otlp_logs_batch_size", float64(processedCount), map[string]string{
		"format": "http",
	})

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"processed_logs": processedCount,
		"timestamp":      time.Now().UTC(),
	})
}

// processLogs processes a batch of OTLP logs
func (h *Handler) processLogs(ctx context.Context, logs plog.Logs) (int, error) {
	processedCount := 0
	resourceLogs := logs.ResourceLogs()

	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)
		scopeLogs := resourceLog.ScopeLogs()

		for j := 0; j < scopeLogs.Len(); j++ {
			scopeLog := scopeLogs.At(j)
			logRecords := scopeLog.LogRecords()

			for k := 0; k < logRecords.Len(); k++ {
				logRecord := logRecords.At(k)

				// Validate and process individual log record
				normalizedLog, err := h.normalizeLogRecord(resourceLog, scopeLog, logRecord)
				if err != nil {
					h.log.Warn().Err(err).Msg("Failed to normalize log record, skipping")
					continue
				}

				// Emit normalized log
				if err := h.emitter.EmitNormalizedLog(ctx, normalizedLog); err != nil {
					h.log.Error().Err(err).Msg("Failed to emit normalized log")
					return processedCount, err
				}

				processedCount++
			}
		}
	}

	return processedCount, nil
}

// normalizeLogRecord converts an OTLP log record to a normalized log
func (h *Handler) normalizeLogRecord(resourceLog plog.ResourceLog, scopeLog plog.ScopeLog, logRecord plog.LogRecord) (*NormalizedLog, error) {
	// Extract timestamp
	timestamp := logRecord.Timestamp().AsTime().UnixNano()
	if timestamp == 0 {
		timestamp = time.Now().UnixNano()
	}

	// Extract message
	message := logRecord.Body().AsString()
	if !h.config.ValidateUTF8 || !utf8.ValidString(message) {
		if h.config.ValidateUTF8 {
			return nil, fmt.Errorf("invalid UTF-8 in log message")
		}
		// If UTF-8 validation is disabled, replace invalid sequences
		message = strings.ToValidUTF8(message, "")
	}

	// Validate message size
	if len(message) > h.config.MaxLogSize {
		return nil, fmt.Errorf("log message too large: %d bytes", len(message))
	}

	// Extract labels from resource attributes
	labels := make(map[string]string)
	resourceAttrs := resourceLog.Resource().Attributes()
	resourceAttrs.Range(func(k string, v any) bool {
		if len(labels) >= h.config.MaxLabels {
			return false // Stop if we hit the limit
		}
		if str, ok := v.(string); ok {
			labels[k] = str
		}
		return true
	})

	// Add scope information to labels
	if scopeLog.Scope().Name() != "" {
		labels["scope.name"] = scopeLog.Scope().Name()
	}
	if scopeLog.Scope().Version() != "" {
		labels["scope.version"] = scopeLog.Scope().Version()
	}

	// Extract fields from log record attributes
	fields := make(map[string]string)
	logAttrs := logRecord.Attributes()
	logAttrs.Range(func(k string, v any) bool {
		if len(fields) >= h.config.MaxFields {
			return false // Stop if we hit the limit
		}
		if str, ok := v.(string); ok {
			fields[k] = str
		}
		return true
	})

	// Add standard OTLP fields
	if logRecord.SeverityNumber() != 0 {
		fields["severity_number"] = fmt.Sprintf("%d", logRecord.SeverityNumber())
	}
	if logRecord.SeverityText() != "" {
		fields["severity_text"] = logRecord.SeverityText()
	}
	if logRecord.TraceID().IsEmpty() == false {
		fields["trace_id"] = logRecord.TraceID().HexString()
	}
	if logRecord.SpanID().IsEmpty() == false {
		fields["span_id"] = logRecord.SpanID().HexString()
	}

	// Determine schema type
	schema := h.determineSchema(message, fields)

	// Create normalized log
	normalizedLog := &NormalizedLog{
		Timestamp: timestamp,
		Labels:    labels,
		Message:   message,
		Fields:    fields,
		Schema:    schema,
		Sanitized: false, // No redaction/masking yet
		OrigLen:   uint32(len(message)),
	}

	return normalizedLog, nil
}

// determineSchema determines the schema type based on message content
func (h *Handler) determineSchema(message string, fields map[string]string) string {
	// Check if it's JSON
	if strings.HasPrefix(strings.TrimSpace(message), "{") && strings.HasSuffix(strings.TrimSpace(message), "}") {
		return "JSON"
	}

	// Check if it's logfmt (key=value pairs)
	if strings.Contains(message, "=") && !strings.Contains(message, "{") {
		return "LOGFMT"
	}

	// Default to TEXT
	return "TEXT"
}

// ValidateConfig validates the OTLP handler configuration
func (h *Handler) ValidateConfig() error {
	if h.config.MaxLogSize <= 0 {
		return fmt.Errorf("max_log_size must be positive")
	}
	if h.config.MaxBatchSize <= 0 {
		return fmt.Errorf("max_batch_size must be positive")
	}
	if h.config.MaxLabels <= 0 {
		return fmt.Errorf("max_labels must be positive")
	}
	if h.config.MaxFields <= 0 {
		return fmt.Errorf("max_fields must be positive")
	}
	if h.config.RequestTimeout <= 0 {
		return fmt.Errorf("request_timeout must be positive")
	}
	return nil
}
