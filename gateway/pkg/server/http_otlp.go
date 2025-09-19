package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

// otlpHandler handles OTLP protocol log ingestion
func (s *HTTP) otlpHandler(c *gin.Context, start time.Time) {
	ctx := c.Request.Context()

	// Read and validate request body
	body, err := c.GetRawData()
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to read request body")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// Validate request size
	if len(body) > s.ingest.GetConfig().MaxLogSize {
		s.log.Warn().Int("size", len(body)).Int("max_size", s.ingest.GetConfig().MaxLogSize).Msg("Request body too large")
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "Request body too large"})
		return
	}

	// Parse OTLP request
	req := plogotlp.NewExportRequest()
	if err := req.UnmarshalProto(body); err != nil {
		s.log.Error().Err(err).Msg("Failed to unmarshal OTLP request")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid OTLP request format"})
		return
	}

	// Convert OTLP request to common ingest request
	commonReq := s.convertOTLPToCommonRequest(&req)

	// Process using unified processor
	processor := s.ingest.GetProcessor()
	result, err := processor.ProcessRequest(ctx, commonReq)

	if err != nil {
		s.log.Error().Err(err).Msg("Failed to process OTLP logs")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to process logs"})
		return
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"status":         "success",
		"processed_logs": result.ProcessedCount,
		"timestamp":      time.Now().UTC(),
	})

	// Record metrics
	if s.metric != nil {
		s.metric.ObserveIngestHandlerLatency(time.Since(start), "otlp", true)
	}
}

// convertOTLPToCommonRequest converts OTLP request to common ingest request
func (s *HTTP) convertOTLPToCommonRequest(otlpReq *plogotlp.ExportRequest) *ingest.CommonIngestRequest {
	var records []ingest.CommonIngestRecord
	logs := otlpReq.Logs()

	resourceLogs := logs.ResourceLogs()
	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)
		scopeLogs := resourceLog.ScopeLogs()

		for j := 0; j < scopeLogs.Len(); j++ {
			scopeLog := scopeLogs.At(j)
			logRecords := scopeLog.LogRecords()

			for k := 0; k < logRecords.Len(); k++ {
				logRecord := logRecords.At(k)

				// Extract timestamp
				timestamp := logRecord.Timestamp().AsTime().UnixNano()
				if timestamp == 0 {
					timestamp = time.Now().UnixNano()
				}

				// Extract message
				message := logRecord.Body().AsString()
				if !s.ingest.GetConfig().ValidateUTF8 || !utf8.ValidString(message) {
					if s.ingest.GetConfig().ValidateUTF8 {
						continue // Skip invalid UTF-8
					}
					message = strings.ToValidUTF8(message, "")
				}

				// Extract labels from resource attributes
				labels := make(map[string]string)
				resourceAttrs := resourceLog.Resource().Attributes()
				resourceAttrs.Range(func(k string, v pcommon.Value) bool {
					if len(labels) >= s.ingest.GetConfig().MaxLabels {
						return false
					}
					if v.Type() == pcommon.ValueTypeStr {
						labels[k] = v.Str()
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
				logAttrs.Range(func(k string, v pcommon.Value) bool {
					if len(fields) >= s.ingest.GetConfig().MaxFields {
						return false
					}
					if v.Type() == pcommon.ValueTypeStr {
						fields[k] = v.Str()
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
					fields["trace_id"] = logRecord.TraceID().String()
				}
				if logRecord.SpanID().IsEmpty() == false {
					fields["span_id"] = logRecord.SpanID().String()
				}

				record := ingest.CommonIngestRecord{
					Timestamp: timestamp,
					Labels:    labels,
					Message:   message,
					Fields:    fields,
				}

				records = append(records, record)
			}
		}
	}

	return &ingest.CommonIngestRequest{
		Protocol: "otlp",
		Records:  records,
	}
}

// countOTLPLogRecords counts the total number of log records in OTLP logs
func (s *HTTP) countOTLPLogRecords(logs plog.Logs) int {
	count := 0
	resourceLogs := logs.ResourceLogs()

	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)
		scopeLogs := resourceLog.ScopeLogs()

		for j := 0; j < scopeLogs.Len(); j++ {
			scopeLog := scopeLogs.At(j)
			logRecords := scopeLog.LogRecords()
			count += logRecords.Len()
		}
	}

	return count
}

// findOTLPLogRecord finds the log record at the given index
func (s *HTTP) findOTLPLogRecord(logs plog.Logs, targetIndex int) (plog.ResourceLogs, plog.ScopeLogs, plog.LogRecord, error) {
	currentIndex := 0
	resourceLogs := logs.ResourceLogs()

	for i := 0; i < resourceLogs.Len(); i++ {
		resourceLog := resourceLogs.At(i)
		scopeLogs := resourceLog.ScopeLogs()

		for j := 0; j < scopeLogs.Len(); j++ {
			scopeLog := scopeLogs.At(j)
			logRecords := scopeLog.LogRecords()

			for k := 0; k < logRecords.Len(); k++ {
				if currentIndex == targetIndex {
					logRecord := logRecords.At(k)
					return resourceLog, scopeLog, logRecord, nil
				}
				currentIndex++
			}
		}
	}

	return plog.ResourceLogs{}, plog.ScopeLogs{}, plog.LogRecord{}, fmt.Errorf("log record at index %d not found", targetIndex)
}

// normalizeOTLPLogRecord converts an OTLP log record to a normalized log
func (s *HTTP) normalizeOTLPLogRecord(resourceLog plog.ResourceLogs, scopeLog plog.ScopeLogs, logRecord plog.LogRecord) (*ingest.NormalizedLog, error) {
	// Extract timestamp
	timestamp := logRecord.Timestamp().AsTime().UnixNano()
	if timestamp == 0 {
		timestamp = time.Now().UnixNano()
	}

	// Extract message
	message := logRecord.Body().AsString()
	if !s.ingest.GetConfig().ValidateUTF8 || !utf8.ValidString(message) {
		if s.ingest.GetConfig().ValidateUTF8 {
			return nil, fmt.Errorf("invalid UTF-8 in log message")
		}
		// If UTF-8 validation is disabled, replace invalid sequences
		message = strings.ToValidUTF8(message, "")
	}

	// Validate message size
	if len(message) > s.ingest.GetConfig().MaxLogSize {
		return nil, fmt.Errorf("log message too large: %d bytes", len(message))
	}

	// Extract labels from resource attributes
	labels := make(map[string]string)
	resourceAttrs := resourceLog.Resource().Attributes()
	resourceAttrs.Range(func(k string, v pcommon.Value) bool {
		if len(labels) >= s.ingest.GetConfig().MaxLabels {
			return false // Stop if we hit the limit
		}
		if v.Type() == pcommon.ValueTypeStr {
			labels[k] = v.Str()
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
	logAttrs.Range(func(k string, v pcommon.Value) bool {
		if len(fields) >= s.ingest.GetConfig().MaxFields {
			return false // Stop if we hit the limit
		}
		if v.Type() == pcommon.ValueTypeStr {
			fields[k] = v.Str()
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
		fields["trace_id"] = logRecord.TraceID().String()
	}
	if logRecord.SpanID().IsEmpty() == false {
		fields["span_id"] = logRecord.SpanID().String()
	}

	// Determine schema type
	schema := ingest.DetermineSchema(message, fields)

	// Create normalized log
	normalizedLog := &ingest.NormalizedLog{
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
