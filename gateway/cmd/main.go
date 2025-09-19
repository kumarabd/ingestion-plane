package main

import (
	"fmt"
	"os"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/config"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/cache"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/server"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/service"
)

// main is the entry point of the application
func main() {
	// Initialize a new logger with the application name and syslog format
	log, err := logger.New(config.ApplicationName, logger.Options{
		Format: logger.SyslogLogFormat,
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Initialize a new configuration handler
	configHandler, err := config.New()
	if err != nil {
		log.Error().Err(err).Msg("")
		os.Exit(1)
	}

	// Initialize a new metrics handler with the application name and server namespace
	metricsHandler, err := metrics.New(config.ApplicationName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Initialize in-memory cache layer
	cacheHandler, err := cache.New()
	if err != nil {
		log.Error().Err(err).Msg("cache initialization failed")
		os.Exit(1)
	}

	// Initialize a new service with the logger, metrics handler, data layer, and service configuration
	serviceHandler, err := service.New(log, metricsHandler, cacheHandler, configHandler.Service)
	if err != nil {
		log.Error().Err(err).Msg("service initialization failed")
		os.Exit(1)
	}
	log.Info().Msg("service initialized")

	// Create server instance
	srv, err := server.New(log, metricsHandler, configHandler.Server, serviceHandler)
	if err != nil {
		log.Error().Err(err).Msg("server initialization failed")
		os.Exit(1)
	}
	log.Info().Msg("server initialized")

	// Run the server with graceful shutdown
	ch := make(chan struct{})
	srv.Start(ch)
	<-ch
	log.Info().Msg("server stopped")
}
