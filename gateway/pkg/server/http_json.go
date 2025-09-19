package server

import (
	"encoding/json"
	"net/http"
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

	var batch ingest.RawLogBatch
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

	// Convert JSON batch to standardized RawLogBatch
	rawLogBatch := ingest.RawLogBatch{
		Records: batch.Records,
	}

	// Enqueue for background processing
	if err := s.enqueueRawBatch(ctx, rawLogBatch); err != nil {
		if s.metric != nil {
			s.metric.IncIngestRejectedTotal("enqueue_timeout")
		}
		c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "service busy, please retry"})
		return
	}

	// Record enqueue metrics
	latency := time.Since(start).Seconds()
	if s.metric != nil {
		s.metric.AddHistogram("json_enqueue_latency_seconds", latency, map[string]string{
			"method": "POST",
		})
		s.metric.IncrementCounter("json_logs_enqueued_total", map[string]string{
			"method": "POST",
			"status": "success",
		})
	}

	// Return success response immediately
	c.JSON(http.StatusOK, gin.H{
		"message":   "success",
		"enqueued":  true,
		"timestamp": time.Now().UTC(),
	})
}
