package indexfeed

import "time"

// Config contains configuration for the IndexFeed service
type Config struct {
	Addr           string        `json:"addr" yaml:"addr" default:"dns:///indexfeed:9000"`        // "dns:///indexfeed:9000" or host:port
	Timeout        time.Duration `json:"timeout" yaml:"timeout" default:"200ms"`                  // e.g., 200ms
	MaxRetries     int           `json:"max_retries" yaml:"max_retries" default:"2"`              // e.g., 2
	RetryBaseDelay time.Duration `json:"retry_base_delay" yaml:"retry_base_delay" default:"20ms"` // e.g., 20ms
}
