package server

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/contracts/ingest/v1"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// GRPCConfig contains configuration for the gRPC server
type GRPCConfig struct {
	Host                  string `json:"host" yaml:"host" default:"0.0.0.0"`
	Port                  string `json:"port" yaml:"port" default:"9090"`
	MaxConcurrentStreams  uint32 `json:"max_concurrent_streams" yaml:"max_concurrent_streams" default:"100"`
	MaxConnectionIdle     string `json:"max_connection_idle" yaml:"max_connection_idle" default:"30s"`
	MaxConnectionAge      string `json:"max_connection_age" yaml:"max_connection_age" default:"60s"`
	MaxConnectionAgeGrace string `json:"max_connection_age_grace" yaml:"max_connection_age_grace" default:"10s"`
	Time                  string `json:"time" yaml:"time" default:"5s"`
	Timeout               string `json:"timeout" yaml:"timeout" default:"1s"`
}

// GRPC implements the Server interface for gRPC
type GRPC struct {
	handler   *grpc.Server
	service   *service.Handler
	log       *logger.Handler
	metric    *metrics.Handler
	config    *GRPCConfig
	listener  net.Listener
	isRunning bool
	mu        sync.RWMutex
}

// NewGRPC creates a new gRPC server instance
func NewGRPC(config *GRPCConfig, service *service.Handler, log *logger.Handler, metric *metrics.Handler) *GRPC {
	// Create gRPC server with options
	opts := []grpc.ServerOption{
		grpc.MaxConcurrentStreams(config.MaxConcurrentStreams),
		// Add more gRPC options here as needed
	}

	server := &GRPC{
		handler: grpc.NewServer(opts...),
		service: service,
		log:     log,
		metric:  metric,
		config:  config,
	}

	// Register the IngestService
	otlpHandler := service.GetOTLPHandler()
	grpcService := ingest.NewGRPCService(otlpHandler, log, metric)
	ingestv1.RegisterIngestServiceServer(server.handler, grpcService)

	// Register reflection service for gRPC debugging
	reflection.Register(server.handler)

	// Register your gRPC services here
	// Example: pb.RegisterYourServiceServer(server.handler, &YourServiceServer{})

	return server
}

// Start starts the gRPC server
func (s *GRPC) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("gRPC server is already running")
	}

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	s.isRunning = true
	s.log.Info().Msgf("Starting gRPC server on %s", addr)

	return s.handler.Serve(listener)
}

// Stop gracefully shuts down the gRPC server
func (s *GRPC) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning || s.handler == nil {
		return nil
	}

	s.log.Info().Msg("Shutting down gRPC server...")

	// Graceful stop
	s.handler.GracefulStop()

	if s.listener != nil {
		s.listener.Close()
	}

	s.isRunning = false
	s.log.Info().Msg("gRPC server stopped")
	return nil
}

// IsRunning returns true if the gRPC server is currently running
func (s *GRPC) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetName returns the name of the server implementation
func (s *GRPC) GetName() string {
	return "gRPC"
}

// GetServer returns the underlying gRPC server for service registration
func (s *GRPC) GetServer() *grpc.Server {
	return s.handler
}

// RegisterService registers a gRPC service with the server
func (s *GRPC) RegisterService(desc *grpc.ServiceDesc, impl interface{}) {
	s.handler.RegisterService(desc, impl)
	s.log.Info().Msgf("Registered gRPC service: %s", desc.ServiceName)
}
