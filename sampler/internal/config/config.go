package config

import (
	"fmt"

	config_pkg "github.com/kumarabd/gokit/config"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/policy"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/redis"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/server"
)

var (
	ApplicationName    = "sampler"
	ApplicationVersion = "dev"
)

// Config holds all configuration for the Sampler service
type Config struct {
	Server *server.Config `json:"server" yaml:"server"`
	Redis  *redis.Config  `json:"redis" yaml:"redis"`
	Policy *policy.Config `json:"policy" yaml:"policy"`
}

// New creates a new config instance
func New() (*Config, error) {
	// Create default config object
	configObject := &Config{
		Server: &server.Config{
			GRPCPort: "50060",
		},
		Redis: &redis.Config{
			Addr:     "localhost:6379",
			Password: "",
			DB:       0,
		},
		Policy: &policy.Config{
			EMAHalfLifeMins:  10.0,
			MinSpikePerMin:   100,
			SpikeFactor:      3.0,
			WarmupN:          32,
			Log2Enabled:      true,
			NoveltyWindowMin: 1440, // 24h
			Version:          "sampler-redis-v1",
			SteadyKDebug:     500,
			SteadyKInfo:      100,
			SteadyKWarn:      10,
			Budget10mDefault: 0, // 0 = disabled
		},
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
