package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kumarabd/ingestion-plane/indexfeed/internal/config"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/database"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/server"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/service"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/vector"
)

func main() {
	log.Printf("INFO: Starting %s service (version: %s)", config.ApplicationName, config.ApplicationVersion)

	// Load configuration
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("INFO: Configuration loaded:")
	log.Printf("  GRPC Port: %s", cfg.Server.GRPCPort)
	log.Printf("  Postgres: %s", cfg.Database.GetConnectionString())
	log.Printf("  Qdrant: %s:%d", cfg.Vector.Host, cfg.Vector.Port)
	log.Printf("  Collection: %s", cfg.Vector.Collection)
	log.Printf("  Vector Dim: %d", cfg.Vector.VectorDim)

	// Initialize database client
	log.Printf("DEBUG: Connecting to Postgres...")
	dbClient, err := database.New(cfg.Database.GetConnectionString())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbClient.Close()
	log.Printf("INFO: Postgres connection established")

	// Initialize database schema
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	log.Printf("DEBUG: Ensuring database schema...")
	if err := dbClient.InitSchema(ctx); err != nil {
		log.Fatalf("failed to initialize schema: %v", err)
	}
	log.Printf("INFO: Database schema ensured")

	// Initialize Qdrant vector upserter
	log.Printf("DEBUG: Connecting to Qdrant...")
	vecUpserter, err := vector.NewUpserter(context.Background(), cfg.Vector.Host, cfg.Vector.Port, cfg.Vector.Collection, cfg.Vector.VectorDim)
	if err != nil {
		log.Fatalf("failed to connect to Qdrant: %v", err)
	}
	log.Printf("INFO: Qdrant connection established")

	// Initialize service
	svc := service.New(cfg.Service, cfg.Vector.VectorDim, dbClient, vecUpserter)
	log.Printf("DEBUG: Service initialized")

	// Create and start gRPC server
	srv, err := server.New(cfg.Server.GRPCPort, svc)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	// Create and start health server
	log.Printf("DEBUG: Creating health server on port 8080...")
	healthSrv := server.NewHealthServer("8080")

	// Channel to receive health server startup errors
	healthErrCh := make(chan error, 1)

	// Start health server
	go func() {
		log.Printf("INFO: Starting health server on :8080...")
		err := healthSrv.Start()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("ERROR: Health server failed: %v", err)
			healthErrCh <- err
		}
	}()

	// Wait a moment and check if health server started successfully
	time.Sleep(200 * time.Millisecond)
	select {
	case err := <-healthErrCh:
		log.Fatalf("FATAL: Health server failed to start: %v", err)
	default:
		log.Printf("INFO: Health server started successfully on :8080")
	}

	// Start gRPC server
	go func() {
		log.Printf("INFO: Starting gRPC server on :%s...", cfg.Server.GRPCPort)
		if err := srv.Start(); err != nil {
			log.Printf("ERROR: gRPC server failed: %v", err)
			log.Fatalf("gRPC server failed: %v", err)
		}
	}()

	// Mark as ready after successful initialization
	log.Printf("DEBUG: Marking service as ready...")
	healthSrv.SetReady(true)
	log.Printf("INFO: Service is ready - health endpoints: http://:8080/healthz, http://:8080/readyz")

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	log.Printf("INFO: Received signal: %v - initiating graceful shutdown...", sig)
	log.Printf("DEBUG: Setting ready state to false...")
	healthSrv.SetReady(false)

	log.Printf("DEBUG: Stopping gRPC server...")
	srv.Stop()

	log.Printf("INFO: Server stopped successfully")
}
