package config

import (
	"fmt"
	"time"

	config_pkg "github.com/kumarabd/gokit/config"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/forwarder"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/server"
)

var (
	ApplicationName    = "default"
	ApplicationVersion = "dev"
)

type Config struct {
	Server  *server.Config        `json:"server,omitempty" yaml:"server,omitempty"`
	OTLP    *ingest.Config        `json:"otlp" yaml:"otlp"`
	Emitter *ingest.EmitterConfig `json:"emitter" yaml:"emitter"`
	Metrics *metrics.Options      `json:"metrics,omitempty" yaml:"metrics,omitempty"`
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
			MaxLogSize:     1048576, // 1MB
			MaxBatchSize:   1000,    // 1000 logs per batch
			MaxLabels:      100,     // 100 labels per log
			MaxFields:      200,     // 200 fields per log
			RequestTimeout: 30,      // 30s timeout
			ValidateUTF8:   true,    // Validate UTF-8
			AllowedSchemas: []string{"JSON", "LOGFMT", "TEXT"},
		},
		Emitter: &ingest.EmitterConfig{
			OutputType:    "forwarder", // stdout, kafka, grpc, forwarder
			BatchSize:     100,         // Batch size for output
			FlushInterval: 5,           // Flush interval in seconds
			Forwarder: &forwarder.Config{
				ShadowMode:    true,  // Enable shadow mode (no drops)
				MaxRetries:    3,     // Max retry attempts
				RetryDelay:    1,     // 1s delay between retries
				Timeout:       10,    // 10s request timeout
				BatchSize:     100,   // Batch size for forwarding
				FlushInterval: 5,     // 5s flush interval
				MaxQueueSize:  10000, // Max queue size
				Miner: forwarder.MinerConfig{
					Enabled:  true,
					Endpoint: "http://localhost:8001/api/v1/logs",
					Timeout:  5, // 5s timeout
				},
				Loki: forwarder.LokiConfig{
					Enabled:  true,
					Endpoint: "http://localhost:3100/loki/api/v1/push",
					Timeout:  5, // 5s timeout
					TenantID: "",
				},
			},
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
