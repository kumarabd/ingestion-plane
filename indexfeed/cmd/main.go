package main

import (
	"context"
	"log"
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

	// Handle graceful shutdown
	go func() {
		if err := srv.Start(); err != nil {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Printf("INFO: Shutting down server...")
	srv.Stop()
	log.Printf("INFO: Server stopped successfully")
}
