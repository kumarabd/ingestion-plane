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

	// Convert Loki request to standardized RawLogBatch
	rawLogBatch := s.convertLokiToRawLogBatch(&lokiReq)

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
		s.metric.AddHistogram("loki_enqueue_latency_seconds", latency, map[string]string{
			"method": "POST",
		})
		s.metric.IncrementCounter("loki_logs_enqueued_total", map[string]string{
			"method": "POST",
			"status": "success",
		})
	}

	// Return Loki-compatible response immediately
	c.JSON(http.StatusOK, gin.H{
		"status": "success",
	})
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

// convertLokiToRawLogBatch converts Loki push request to standardized RawLogBatch
func (s *HTTP) convertLokiToRawLogBatch(lokiReq *logproto.PushRequest) ingest.RawLogBatch {
	var records []ingest.RawLog

	for _, stream := range lokiReq.Streams {
		for _, entry := range stream.Entries {
			// Parse Loki labels string format: "{key1=\"value1\",key2=\"value2\"}"
			labels := parseLokiLabels(stream.Labels)

			record := ingest.RawLog{
				Timestamp:  entry.Timestamp,
				Labels:     labels,
				Payload:    entry.Line,
				FormatHint: "text", // Loki entries are typically text
			}

			records = append(records, record)
		}
	}

	return ingest.RawLogBatch{
		Records: records,
	}
}
