package miner

import "time"

// Config contains configuration for the Miner service
type Config struct {
	Addr           string        `json:"addr" yaml:"addr" default:"dns:///miner:9000"`            // "dns:///miner:9000" or host:port
	Timeout        time.Duration `json:"timeout" yaml:"timeout" default:"200ms"`                  // e.g., 200ms
	MaxBatch       int           `json:"max_batch" yaml:"max_batch" default:"500"`                // e.g., 500
	MaxBatchWait   time.Duration `json:"max_batch_wait" yaml:"max_batch_wait" default:"25ms"`     // e.g., 25ms
	MaxRetries     int           `json:"max_retries" yaml:"max_retries" default:"2"`              // e.g., 2
	RetryBaseDelay time.Duration `json:"retry_base_delay" yaml:"retry_base_delay" default:"20ms"` // e.g., 20ms
	ShadowOnly     bool          `json:"shadow_only" yaml:"shadow_only" default:"true"`           // Shadow mode - don't drop logs yet
}
