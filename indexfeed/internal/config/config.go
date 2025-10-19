package config

import (
	"fmt"

	config_pkg "github.com/kumarabd/gokit/config"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/database"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/server"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/service"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/vector"
)

var (
	ApplicationName    = "indexfeed"
	ApplicationVersion = "dev"
)

// Config holds all configuration for the IndexFeed service
type Config struct {
	Server   *server.Config   `json:"server" yaml:"server"`
	Database *database.Config `json:"database" yaml:"database"`
	Vector   *vector.Config   `json:"vector" yaml:"vector"`
	Service  *service.Config  `json:"service" yaml:"service"`
}

// New creates a new config instance
func New() (*Config, error) {
	// Create default config object
	configObject := &Config{
		Server: &server.Config{
			GRPCPort: "50070",
		},
		Database: &database.Config{
			Host:     "localhost",
			Port:     5432,
			User:     "postgres",
			Password: "postgres",
			Database: "ingestion_plane",
			SSLMode:  "disable",
		},
		Vector: &vector.Config{
			Host:       "localhost",
			Port:       6334,
			Collection: "templates",
			VectorDim:  384,
		},
		Service: &service.Config{
			LabelAllowlist: []string{
				"service",
				"env",
				"severity",
				"namespace",
			},
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
