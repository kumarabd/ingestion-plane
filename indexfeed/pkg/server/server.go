package server

import (
	"fmt"
	"log"
	"net"

	indexfeedv1 "github.com/kumarabd/ingestion-plane/contracts/indexfeed/v1"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Server represents the gRPC server
type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	port       string
}

// New creates a new gRPC server
func New(port string, svc *service.Service) (*Server, error) {
	// Create gRPC server with options
	grpcServer := grpc.NewServer(
		grpc.MaxRecvMsgSize(32*1024*1024),
		grpc.MaxSendMsgSize(32*1024*1024),
	)

	// Register the IndexFeed service
	indexfeedv1.RegisterCandidateIngestServer(grpcServer, svc)
	log.Printf("DEBUG: gRPC service registered")

	// Register health service
	hs := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, hs)
	log.Printf("DEBUG: Health service registered")

	// Create listener
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on port %s: %w", port, err)
	}

	return &Server{
		grpcServer: grpcServer,
		listener:   lis,
		port:       port,
	}, nil
}

// Start starts the gRPC server
func (s *Server) Start() error {
	log.Printf("INFO: IndexFeed CandidateIngest listening on :%s", s.port)
	return s.grpcServer.Serve(s.listener)
}

// Stop gracefully stops the gRPC server
func (s *Server) Stop() {
	log.Printf("INFO: Stopping gRPC server gracefully")
	s.grpcServer.GracefulStop()
}
