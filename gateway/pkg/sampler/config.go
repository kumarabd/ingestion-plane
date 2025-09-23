package sampler

import "time"

// SamplerConfig contains configuration for the Sampler service
type SamplerConfig struct {
	Addr         string        `json:"addr" yaml:"addr" default:"sampler:9000"`             // Sampler gRPC endpoint
	Timeout      time.Duration `json:"timeout" yaml:"timeout" default:"100ms"`              // RPC timeout
	MaxBatch     int           `json:"max_batch" yaml:"max_batch" default:"1000"`           // Max batch size
	MaxBatchWait time.Duration `json:"max_batch_wait" yaml:"max_batch_wait" default:"25ms"` // Max batch wait time
}

// EnforcementConfig contains configuration for enforcement rules
type EnforcementConfig struct {
	Debug       bool            `json:"debug" yaml:"debug" default:"true"`  // Enforce debug logs
	Info        bool            `json:"info" yaml:"info" default:"false"`   // Enforce info logs
	Warn        bool            `json:"warn" yaml:"warn" default:"false"`   // Enforce warn logs
	Error       bool            `json:"error" yaml:"error" default:"false"` // Enforce error logs
	ByNamespace map[string]bool `json:"by_namespace" yaml:"by_namespace"`   // Per-namespace overrides
}
