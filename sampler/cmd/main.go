package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	log.Printf("DEBUG: Closing Redis connection...")
	store.Close()

	log.Printf("INFO: Server stopped successfully")
}
