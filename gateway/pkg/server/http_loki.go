package server

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/grafana/loki/pkg/logproto"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
)

// lokiHandler handles Grafana Loki protocol log ingestion
func (s *HTTP) lokiHandler(c *gin.Context, start time.Time) {
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

	// Parse Loki push request using official logproto library
	var lokiReq logproto.PushRequest
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&lokiReq); err != nil {
		if s.metric != nil {
			s.metric.IncIngestRejectedTotal("bad_loki_json")
		}
		_ = c.Error(err)
		c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "invalid loki request format"})
		return
	}

	// Convert Loki request to common ingest request
	commonReq := s.convertLokiToCommonRequest(&lokiReq)

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
		s.metric.AddHistogram("loki_push_latency_seconds", latency, map[string]string{
			"method": "POST",
		})
		s.metric.AddHistogram("loki_batch_size", float64(result.TotalReceived), map[string]string{
			"method": "POST",
		})
		s.metric.IncrementCounter("loki_logs_processed_total", map[string]string{
			"method": "POST",
			"status": "success",
		})
	}

	// Return Loki-compatible response
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
}

// convertLokiToCommonRequest converts Loki push request to common ingest request
func (s *HTTP) convertLokiToCommonRequest(lokiReq *logproto.PushRequest) *ingest.CommonIngestRequest {
	var records []ingest.CommonIngestRecord

	for _, stream := range lokiReq.Streams {
		for _, entry := range stream.Entries {
			// Use the official logproto timestamp format
			timestampNs := entry.Timestamp.UnixNano()

			// Parse Loki labels string format: "{key1=\"value1\",key2=\"value2\"}"
			labels := parseLokiLabels(stream.Labels)

			record := ingest.CommonIngestRecord{
				Timestamp: timestampNs,
				Labels:    labels,
				Message:   entry.Line,
				Fields:    make(map[string]string),
			}

			records = append(records, record)
		}
	}

	return &ingest.CommonIngestRequest{
		Protocol: "loki",
		Records:  records,
	}
}

// parseLokiLabels parses Loki labels string format: "{key1=\"value1\",key2=\"value2\"}"
func parseLokiLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)

	// Remove outer braces
	labelsStr = strings.Trim(labelsStr, "{}")
	if labelsStr == "" {
		return labels
	}

	// Split by comma and parse each key=value pair
	pairs := strings.Split(labelsStr, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		// Find the first = sign
		eqIndex := strings.Index(pair, "=")
		if eqIndex == -1 {
			continue
		}

		key := strings.TrimSpace(pair[:eqIndex])
		value := strings.TrimSpace(pair[eqIndex+1:])

		// Remove quotes from value
		value = strings.Trim(value, "\"")

		labels[key] = value
	}

	return labels
}
