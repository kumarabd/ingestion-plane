package logtypes

import (
	"time"
)

// RedactionReport represents PII redaction information
type RedactionReport struct {
	Applied bool     `json:"applied"`
	Rules   []string `json:"rules"`
	Count   int      `json:"count"`
}

// NormalizedLog represents a normalized log entry
type NormalizedLog struct {
	Timestamp time.Time         `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Message   string            `json:"message"`
	Schema    string            `json:"schema"`
	Sanitized bool              `json:"sanitized"`
	Truncated bool              `json:"truncated"`
	Redaction RedactionReport   `json:"redaction"`
}

// NormalizedLogBatch is a batch of normalized logs
type NormalizedLogBatch struct {
	Records []NormalizedLog `json:"records"` // required; must not be empty
}
