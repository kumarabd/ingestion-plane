package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
)

// jsonHandler handles JSON protocol log ingestion
func (s *HTTP) jsonHandler(c *gin.Context, start time.Time) {
	ctx := c.Request.Context()

	// Handle optional gzip compression
	reader, err := getBodyReader(c.Request)
	if err != nil {
		if s.metric != nil {
			s.metric.IncIngestRejectedTotal("bad_body")
		}
		_ = c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	defer reader.Close()

	var batch RawLogBatch
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&batch); err != nil {
		if s.metric != nil {
			s.metric.IncIngestRejectedTotal("bad_json")
		}
		_ = c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid json batch"})
		return
	}

	// Convert JSON batch to common ingest request
	commonReq := s.convertJSONToCommonRequest(&batch)

	// Process using unified processor
	processor := s.ingest.GetProcessor()
	result, err := processor.ProcessRequest(ctx, commonReq)

	if err != nil {
		// Handle validation errors
		if err.Error() == "empty batch" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "empty batch"})
			return
		}
		if err.Error() == "batch size exceeds maximum" {
			c.AbortWithStatusJSON(http.StatusRequestEntityTooLarge, gin.H{"error": "batch too large"})
			return
		}
		// Other errors
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "processing failed"})
		return
	}

	// Record metrics
	latency := time.Since(start).Seconds()
	if s.metric != nil {
		s.metric.AddHistogram("http_push_latency_seconds", latency, map[string]string{
			"method": "POST",
		})
		s.metric.AddHistogram("http_batch_size", float64(result.TotalReceived), map[string]string{
			"method": "POST",
		})
		s.metric.IncrementCounter("http_logs_processed_total", map[string]string{
			"method": "POST",
			"status": "success",
		})
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"message":         "success",
		"processed_count": result.ProcessedCount,
		"total_received":  result.TotalReceived,
		"processing_time": latency,
		"errors":          result.Errors,
	})
}

// convertJSONToCommonRequest converts JSON batch request to common ingest request
func (s *HTTP) convertJSONToCommonRequest(jsonBatch *RawLogBatch) *ingest.CommonIngestRequest {
	var records []ingest.CommonIngestRecord

	for _, rawLog := range jsonBatch.Records {
		// Determine schema based on format hint or payload content
		schema := s.determineSchema(rawLog.Payload, rawLog.FormatHint)

		// Extract fields if it's JSON or LOGFMT
		fields := ingest.ExtractFields(rawLog.Payload, schema)

		record := ingest.CommonIngestRecord{
			Timestamp: rawLog.Timestamp.UnixNano(),
			Labels:    rawLog.Labels,
			Message:   rawLog.Payload,
			Fields:    fields,
		}

		records = append(records, record)
	}

	return &ingest.CommonIngestRequest{
		Protocol: "json",
		Records:  records,
	}
}

// determineSchema determines the schema based on format hint or payload content
func (s *HTTP) determineSchema(payload, formatHint string) string {
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
	return ingest.DetermineSchema(payload, nil)
}
