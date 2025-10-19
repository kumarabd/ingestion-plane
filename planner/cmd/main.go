package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kumarabd/ingestion-plane/planner/internal/config"
	"github.com/kumarabd/ingestion-plane/planner/pkg/database"
	"github.com/kumarabd/ingestion-plane/planner/pkg/server"
	"github.com/kumarabd/ingestion-plane/planner/pkg/service"
	"github.com/kumarabd/ingestion-plane/planner/pkg/vector"
)

func main() {
	log.Printf("INFO: Starting %s service (version: %s)", config.ApplicationName, config.ApplicationVersion)

	// Load configuration
	cfg, err := config.New()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	log.Printf("INFO: Configuration loaded:")
	log.Printf("  HTTP Addr: %s", cfg.Server.HTTPAddr)
	log.Printf("  Postgres: %s", cfg.Database.GetConnectionString())
	log.Printf("  Qdrant: %s:%d", cfg.Vector.Host, cfg.Vector.Port)
	log.Printf("  Collection: %s", cfg.Vector.Collection)
	log.Printf("  Vector Dim: %d", cfg.Vector.VectorDim)
	log.Printf("  Tenant: %s", cfg.Service.Tenant)

	// Initialize database client
	log.Printf("DEBUG: Connecting to Postgres...")
	dbClient, err := database.New(cfg.Database.GetConnectionString())
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbClient.Close()
	log.Printf("INFO: Postgres connection established")

	// Initialize Qdrant vector client
	log.Printf("DEBUG: Connecting to Qdrant...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	vectorCli, err := vector.New(ctx, cfg.Vector.Host, cfg.Vector.Port, cfg.Vector.Collection, cfg.Vector.VectorDim)
	if err != nil {
		log.Fatalf("failed to connect to Qdrant: %v", err)
	}
	log.Printf("INFO: Qdrant connection established")

	// Initialize service
	svc := service.New(cfg.Service, cfg.Vector.VectorDim, dbClient, vectorCli)
	log.Printf("DEBUG: Service initialized")

	// Create HTTP router
	mux := http.NewServeMux()
	svc.Register(mux)

	// Wrap with middleware
	handler := server.WithJSON(server.WithCORS(mux))

	// Create and start HTTP server
	srv := server.New(cfg.Server, handler)

	// Handle graceful shutdown
	go func() {
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server failed: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Printf("INFO: Shutting down server...")
	log.Printf("INFO: Server stopped successfully")
}
