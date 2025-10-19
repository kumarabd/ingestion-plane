package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kumarabd/ingestion-plane/sampler/internal/config"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/policy"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/redis"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/server"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/service"
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
	log.Printf("  Redis Addr: %s", cfg.Redis.Addr)
	log.Printf("  Redis DB: %d", cfg.Redis.DB)
	log.Printf("  EMA Half-life: %.2f mins", cfg.Policy.EMAHalfLifeMins)
	log.Printf("  Min Spike Per Min: %d", cfg.Policy.MinSpikePerMin)
	log.Printf("  Spike Factor: %.2f", cfg.Policy.SpikeFactor)
	log.Printf("  Warmup N: %d", cfg.Policy.WarmupN)
	log.Printf("  Log2 Enabled: %t", cfg.Policy.Log2Enabled)
	log.Printf("  Novelty Window: %d mins", cfg.Policy.NoveltyWindowMin)
	log.Printf("  Version: %s", cfg.Policy.Version)
	log.Printf("  SteadyK Debug: %d", cfg.Policy.SteadyKDebug)
	log.Printf("  SteadyK Info: %d", cfg.Policy.SteadyKInfo)
	log.Printf("  SteadyK Warn: %d", cfg.Policy.SteadyKWarn)
	log.Printf("  Budget 10m Default: %d", cfg.Policy.Budget10mDefault)

	// Initialize Redis store
	log.Printf("DEBUG: Connecting to Redis...")
	store := redis.New(cfg.Redis)

	// Ping Redis
	if err := store.Ping(context.Background()); err != nil {
		log.Fatalf("redis ping failed: %v", err)
	}
	log.Printf("INFO: Redis connection established")

	// Initialize policy
	pol := policy.New(cfg.Policy)
	log.Printf("DEBUG: Policy initialized")

	// Initialize service
	svc := service.New(pol, store)
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
	store.Close()
	log.Printf("INFO: Server stopped successfully")
}
