package server

import (
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/indexfeed"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/miner"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/sampler"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/sink/loki"
)

// Type represents the type of server to create
type Type string

const (
	TypeHTTP Type = "http"
	TypeGRPC Type = "grpc"
)

// Config contains configuration for all server types
type Config struct {
	HTTP *HTTPConfig `json:"http" yaml:"http"`
	GRPC *GRPCConfig `json:"grpc" yaml:"grpc"`
}

// Legacy support - keeping the old Handler for backward compatibility
type Handler struct {
	HTTP   *HTTP
	GRPC   *GRPC
	config *Config
	log    *logger.Handler
}

// New creates a new server handler
func New(l *logger.Handler, m *metrics.Handler, serverConfig *Config, minerConfig *miner.Config, samplerConfig *sampler.SamplerConfig, enforcementConfig *sampler.EnforcementConfig, lokiConfig *loki.LokiConfig, indexFeedConfig *indexfeed.Config, ingest *ingest.Handler) (*Handler, error) {
	// Create HTTP server if configured
	var httpServer *HTTP
	if serverConfig.HTTP != nil {
		httpServer = NewHTTP(serverConfig.HTTP, minerConfig, samplerConfig, enforcementConfig, lokiConfig, indexFeedConfig, ingest, l, m)
	}

	// Create gRPC server if configured
	var grpcServer *GRPC
	if serverConfig.GRPC != nil {
		grpcServer = NewGRPC(serverConfig.GRPC, ingest, l, m)
	}

	return &Handler{
		HTTP:   httpServer,
		GRPC:   grpcServer,
		config: serverConfig,
		log:    l,
	}, nil
}

// Start starts the server
func (h *Handler) Start(ch chan struct{}) {
	// Start HTTP server if available
	if h.HTTP != nil {
		go func() {
			if err := h.HTTP.Start(); err != nil {
				h.log.Error().Err(err).Msg("HTTP server failed")
			}
			ch <- struct{}{}
		}()
	}

	// Start gRPC server if available
	if h.GRPC != nil {
		go func() {
			if err := h.GRPC.Start(); err != nil {
				h.log.Error().Err(err).Msg("gRPC server failed")
			}
			ch <- struct{}{}
		}()
	}
}
