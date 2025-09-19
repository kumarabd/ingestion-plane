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

	// Convert OTLP request to standardized RawLogBatch
	rawLogBatch := s.convertOTLPToRawLogBatch(&req)

	// Enqueue for background processing
	if err := s.enqueueRawBatch(ctx, *rawLogBatch); err != nil {
		if s.metric != nil {
			s.metric.IncIngestRejectedTotal("enqueue_timeout")
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "service busy, please retry"})
		return
	}

	// Record enqueue metrics
	latency := time.Since(start).Seconds()
	if s.metric != nil {
		s.metric.AddHistogram("otlp_enqueue_latency_seconds", latency, map[string]string{
			"method": "POST",
		})
		s.metric.IncrementCounter("otlp_logs_enqueued_total", map[string]string{
			"method": "POST",
			"status": "success",
		})
	}

	// Return success response immediately
	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"timestamp": time.Now().UTC(),
	})
}

// convertOTLPToRawLogBatch converts OTLP request to raw log batch (same pattern as other handlers)
func (s *HTTP) convertOTLPToRawLogBatch(otlpReq *plogotlp.ExportRequest) *ingest.RawLogBatch {
	var records []ingest.RawLog
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

				// Add standard OTLP fields to labels
				if logRecord.SeverityNumber() != 0 {
					labels["severity_number"] = fmt.Sprintf("%d", logRecord.SeverityNumber())
				}
				if logRecord.SeverityText() != "" {
					labels["severity_text"] = logRecord.SeverityText()
				}
				if !logRecord.TraceID().IsEmpty() {
					labels["trace_id"] = logRecord.TraceID().String()
				}
				if !logRecord.SpanID().IsEmpty() {
					labels["span_id"] = logRecord.SpanID().String()
				}

				record := ingest.RawLog{
					Timestamp: time.Unix(0, timestamp),
					Labels:    labels,
					Payload:   message,
				}

				records = append(records, record)
			}
		}
	}

	return &ingest.RawLogBatch{
		Records: records,
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
