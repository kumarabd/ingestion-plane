package main

import (
	"fmt"
	"os"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/config"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/server"
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

	// Initialize a new ingest handler with configuration and emitter
	ingestHandler, err := ingest.NewHandler(configHandler.OTLP, configHandler.Emitter, log, metricsHandler)
	if err != nil {
		log.Error().Err(err).Msg("ingest initialization failed")
		os.Exit(1)
	}

	// Start the ingest handler (required for forwarder)
	if err := ingestHandler.Start(); err != nil {
		log.Error().Err(err).Msg("ingest handler start failed")
		os.Exit(1)
	}
	log.Info().Msg("ingest initialized")

	// Create server instance
	srv, err := server.New(log, metricsHandler, configHandler.Server, ingestHandler)
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

	// Stop the ingest handler gracefully
	if err := ingestHandler.Stop(); err != nil {
		log.Error().Err(err).Msg("ingest handler stop failed")
	}
	log.Info().Msg("ingest handler stopped")
}
