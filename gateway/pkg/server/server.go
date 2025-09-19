package server

import (
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
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
func New(l *logger.Handler, m *metrics.Handler, config *Config, ingest *ingest.Handler) (*Handler, error) {
	// Create HTTP server if configured
	var httpServer *HTTP
	if config.HTTP != nil {
		httpServer = NewHTTP(config.HTTP, ingest, l, m)
	}

	// Create gRPC server if configured
	var grpcServer *GRPC
	if config.GRPC != nil {
		grpcServer = NewGRPC(config.GRPC, ingest, l, m)
	}

	return &Handler{
		HTTP:   httpServer,
		GRPC:   grpcServer,
		config: config,
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
