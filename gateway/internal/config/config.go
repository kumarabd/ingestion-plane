package config

import (
	"fmt"
	"time"

	config_pkg "github.com/kumarabd/gokit/config"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/indexfeed"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/miner"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/sampler"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/server"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/sink/loki"
)

var (
	ApplicationName    = "default"
	ApplicationVersion = "dev"
)

type Config struct {
	Server      *server.Config             `json:"server,omitempty" yaml:"server,omitempty"`
	OTLP        *ingest.Config             `json:"otlp" yaml:"otlp"`
	Emitter     *ingest.EmitterConfig      `json:"emitter" yaml:"emitter"`
	Miner       *miner.Config              `json:"miner" yaml:"miner"`
	Sampler     *sampler.SamplerConfig     `json:"sampler" yaml:"sampler"`
	Enforcement *sampler.EnforcementConfig `json:"enforcement" yaml:"enforcement"`
	Loki        *loki.LokiConfig           `json:"loki" yaml:"loki"`
	IndexFeed   *indexfeed.Config          `json:"indexfeed" yaml:"indexfeed"`
	Metrics     *metrics.Options           `json:"metrics,omitempty" yaml:"metrics,omitempty"`
	//Traces  *traces.Options  `json:"traces,omitempty" yaml:"traces,omitempty"`
}

// New creates a new config instance
func New() (*Config, error) {
	// Create default config object
	configObject := &Config{
		Server: &server.Config{
			HTTP: &server.HTTPConfig{
				Bounds: &server.BoundsConfig{
					MaxBatch:        1000,
					MaxMessageBytes: 65536,
				},
				Pipeline: &server.PipelineConfig{
					EnqueueTimeout: 5 * time.Second,
				},
			},
		},
		OTLP: &ingest.Config{
			MaxLogSize:      1048576, // 1MB
			MaxMessageBytes: 1048576, // 1MB max message size
			MaxBatchSize:    1000,    // 1000 logs per batch
			MaxLabels:       100,     // 100 labels per log
			MaxFields:       200,     // 200 fields per log
			RequestTimeout:  30,      // 30s timeout
			ValidateUTF8:    true,    // Validate UTF-8
			AllowedSchemas:  []string{"JSON", "LOGFMT", "TEXT"},
		},
		Emitter: &ingest.EmitterConfig{
			OutputType:    "stdout", // stdout, kafka, grpc
			BatchSize:     100,      // Batch size for output
			FlushInterval: 5,        // Flush interval in seconds
		},
		Miner: &miner.Config{
			Addr:           "dns:///miner:9000", // Miner gRPC endpoint
			Timeout:        200 * time.Millisecond,
			MaxBatch:       500,
			MaxBatchWait:   25 * time.Millisecond,
			MaxRetries:     2,
			RetryBaseDelay: 20 * time.Millisecond,
			ShadowOnly:     true, // Shadow mode - don't drop logs yet
		},
		Sampler: &sampler.SamplerConfig{
			Addr:         "dns:///sampler:9000", // Sampler gRPC endpoint
			Timeout:      100 * time.Millisecond,
			MaxBatch:     1000,
			MaxBatchWait: 25 * time.Millisecond,
		},
		Enforcement: &sampler.EnforcementConfig{
			Debug: true,
			Info:  false,
			Warn:  false,
			Error: false,
			ByNamespace: map[string]bool{
				"staging": true,
			},
		},
		Loki: &loki.LokiConfig{
			Addr:             "http://loki:3100",
			FlushInterval:    400 * time.Millisecond,
			MaxBatchBytes:    1000000, // 1MB
			MaxBatchEntries:  5000,
			MaxBufferBytes:   268435456, // 256MB
			MaxBufferEntries: 1000000,
			RequestTimeout:   5 * time.Second,
			Retry: loki.RetryConfig{
				Enabled:        true,
				InitialBackoff: 200 * time.Millisecond,
				MaxBackoff:     5 * time.Second,
			},
			DropPolicy: loki.DropPolicy{
				DebugFirst:  true,
				ProtectInfo: true,
			},
			Labels: loki.LabelConfig{
				Static: map[string]string{
					"gateway": "true",
				},
			},
		},
		IndexFeed: &indexfeed.Config{
			Addr:           "dns:///indexfeed:9000",
			Timeout:        200 * time.Millisecond,
			MaxRetries:     2,
			RetryBaseDelay: 20 * time.Millisecond,
		},
		Metrics: &metrics.Options{},
	}

	// Load config using gokit config package
	finalConfig, err := config_pkg.New(configObject)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Safe type assertion
	if finalConfig == nil {
		return nil, fmt.Errorf("config is nil")
	}

	cfg, ok := finalConfig.(*Config)
	if !ok {
		return nil, fmt.Errorf("config type assertion failed: expected *Config, got %T", finalConfig)
	}

	return cfg, nil
}
