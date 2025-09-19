package server

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ingestHandler handles batch log ingestion - routes to appropriate protocol handler
func (s *HTTP) ingestHandler(c *gin.Context) {
	start := time.Now()

	// Route to appropriate handler based on content type
	ct := c.GetHeader("Content-Type")
	if isOTLPContentType(ct) {
		s.otlpHandler(c, start)
		return
	}

	// Check for Loki protocol (specific endpoint or header)
	if strings.HasPrefix(c.Request.URL.Path, "/loki/") {
		s.lokiHandler(c, start)
		return
	}

	// Default to JSON handler
	s.jsonHandler(c, start)
}

// isOTLPContentType checks if the content type indicates OTLP protocol
func isOTLPContentType(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "application/x-protobuf") ||
		strings.Contains(ct, "application/protobuf") ||
		strings.Contains(ct, "application/octet-stream")
}
