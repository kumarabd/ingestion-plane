package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPConfig contains configuration for the HTTP server
type HTTPConfig struct {
	Host         string          `json:"host" yaml:"host" default:"0.0.0.0"`
	Port         string          `json:"port" yaml:"port" default:"8080"`
	ReadTimeout  time.Duration   `json:"read_timeout" yaml:"read_timeout" default:"30s"`
	WriteTimeout time.Duration   `json:"write_timeout" yaml:"write_timeout" default:"30s"`
	IdleTimeout  time.Duration   `json:"idle_timeout" yaml:"idle_timeout" default:"60s"`
	Bounds       *BoundsConfig   `json:"bounds" yaml:"bounds"`
	Pipeline     *PipelineConfig `json:"pipeline" yaml:"pipeline"`
}

// BoundsConfig contains bounds configuration for ingestion
type BoundsConfig struct {
	MaxBatch        int `json:"max_batch" yaml:"max_batch" default:"1000"`
	MaxMessageBytes int `json:"max_message_bytes" yaml:"max_message_bytes" default:"65536"`
}

// PipelineConfig contains pipeline configuration
type PipelineConfig struct {
	EnqueueTimeout time.Duration `json:"enqueue_timeout" yaml:"enqueue_timeout" default:"5s"`
}

// RawLogBatch is now defined in the ingest package and used for all protocols

// queuedItem represents an item queued for pipeline processing
type queuedItem struct {
	Ctx   context.Context
	Batch ingest.RawLogBatch
}

// HTTP implements the Server interface for HTTP
type HTTP struct {
	handler   *gin.Engine
	ingest    *ingest.Handler
	log       *logger.Handler
	metric    *metrics.Handler
	config    *HTTPConfig
	server    *http.Server
	isRunning bool
	mu        sync.RWMutex

	// Raw worker components
	rawQueue     chan queuedItem
	workerCtx    context.Context
	workerCancel context.CancelFunc
	workerWg     sync.WaitGroup
}

// NewHTTP creates a new HTTP server instance
func NewHTTP(config *HTTPConfig, in *ingest.Handler, l *logger.Handler, m *metrics.Handler) *HTTP {
	gin.SetMode(gin.ReleaseMode)

	// Set up default configuration if not provided
	if config.Bounds == nil {
		config.Bounds = &BoundsConfig{
			MaxBatch:        1000,
			MaxMessageBytes: 65536,
		}
	}
	if config.Pipeline == nil {
		config.Pipeline = &PipelineConfig{
			EnqueueTimeout: 5 * time.Second,
		}
	}

	// Initialize worker context
	workerCtx, workerCancel := context.WithCancel(context.Background())

	server := &HTTP{
		handler:      gin.New(),
		ingest:       in,
		log:          l,
		metric:       m,
		config:       config,
		rawQueue:     make(chan queuedItem, config.Bounds.MaxBatch*2), // Buffer for 2x max batch size
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}

	// Add global middleware
	server.handler.Use(gin.Recovery())
	server.handler.Use(server.loggingMiddleware())
	server.handler.Use(server.corsMiddleware())

	// Routes will be set up in setupRoutes()
	// Add HTTP-specific routes
	server.setupRoutes()

	return server
}

// Start starts the HTTP server and raw worker
func (s *HTTP) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("HTTP server is already running")
	}

	// Start the raw worker
	s.startRawWorker()

	addr := fmt.Sprintf("%s:%s", s.config.Host, s.config.Port)

	s.server = &http.Server{
		Addr:         addr,
		Handler:      s.handler,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
		IdleTimeout:  s.config.IdleTimeout,
	}

	s.isRunning = true
	s.log.Info().Msgf("Starting HTTP server on %s", addr)

	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the HTTP server and raw worker
func (s *HTTP) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning || s.server == nil {
		return nil
	}

	s.log.Info().Msg("Shutting down HTTP server...")

	// Stop the raw worker first
	s.stopRawWorker()

	if err := s.server.Shutdown(ctx); err != nil {
		s.log.Error().Err(err).Msg("Error during HTTP server shutdown")
		return err
	}

	s.isRunning = false
	s.log.Info().Msg("HTTP server stopped")
	return nil
}

// IsRunning returns true if the HTTP server is currently running
func (s *HTTP) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isRunning
}

// GetName returns the name of the server implementation
func (s *HTTP) GetName() string {
	return "HTTP"
}

// GetHandler returns the gin engine for adding routes
func (s *HTTP) GetHandler() *gin.Engine {
	return s.handler
}

// setupRoutes adds HTTP-specific routes
func (s *HTTP) setupRoutes() {
	// Multi-protocol ingestion endpoints
	// /v1/ingest (unified endpoint - auto-detects protocol)
	s.handler.POST("/v1/ingest", s.ingestHandler)

	// Protocol-specific endpoints for explicit routing
	s.handler.POST("/loki/api/v1/push", func(c *gin.Context) {
		s.lokiHandler(c, time.Now())
	})
	s.handler.POST("/api/v1/logs", func(c *gin.Context) {
		s.jsonHandler(c, time.Now())
	})
	s.handler.POST("/v1/ingest/otlp", func(c *gin.Context) {
		s.otlpHandler(c, time.Now())
	})
	s.handler.POST("/v1/ingest/json", func(c *gin.Context) {
		s.jsonHandler(c, time.Now())
	})

	// Health and metrics endpoints
	s.handler.GET("/healthz", s.healthHandler)
	s.handler.GET("/metrics", s.metricsHandler)
}

// getBodyReader returns a reader for the request body, handling gzip decompression if needed
func getBodyReader(r *http.Request) (io.ReadCloser, error) {
	if r.Body == nil {
		return io.NopCloser(bytes.NewReader(nil)), nil
	}
	if strings.Contains(strings.ToLower(r.Header.Get("Content-Encoding")), "gzip") {
		gz, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		return gz, nil
	}
	return r.Body, nil
}

// healthHandler handles health check endpoint
func (s *HTTP) healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

// metricsHandler handles metrics endpoint
func (s *HTTP) metricsHandler(c *gin.Context) {
	promhttp.Handler().ServeHTTP(c.Writer, c.Request)
}

// loggingMiddleware adds request logging
func (s *HTTP) loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		s.log.Info().
			Str("method", param.Method).
			Str("path", param.Path).
			Int("status", param.StatusCode).
			Dur("latency", param.Latency).
			Str("client_ip", param.ClientIP).
			Str("user_agent", param.Request.UserAgent()).
			Msg("HTTP Request")
		return ""
	})
}

// corsMiddleware adds CORS headers
func (s *HTTP) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusOK)
			return
		}

		c.Next()
	}
}

// startRawWorker starts the background raw worker goroutine
func (s *HTTP) startRawWorker() {
	s.workerWg.Add(1)
	go s.runRawWorker()
	s.log.Info().Msg("Raw worker started")
}

// stopRawWorker gracefully stops the raw worker
func (s *HTTP) stopRawWorker() {
	s.log.Info().Msg("Stopping raw worker...")
	s.workerCancel()
	s.workerWg.Wait()
	s.log.Info().Msg("Raw worker stopped")
}

// runRawWorker continuously processes queued raw batches
func (s *HTTP) runRawWorker() {
	defer s.workerWg.Done()

	s.log.Info().Msg("Raw worker started processing")

	for {
		select {
		case item := <-s.rawQueue:
			batch, err := s.ingest.NormalizeAndRedactBatch(item.Ctx, &item.Batch, s.log, s.metric)
			if err != nil {
				s.log.Error().Err(err).Msg("Failed to process raw batch")
			} else if batch != nil && len(batch.Records) > 0 {
				if err := s.ingest.GetEmitter().EmitNormalizedLogBatch(item.Ctx, batch); err != nil {
					s.log.Error().Err(err).Msg("Failed to emit normalized log batch")
				}
			}
		case <-s.workerCtx.Done():
			s.log.Info().Msg("Raw worker shutting down")
			return
		}
	}
}

// enqueueRawBatch enqueues a raw batch for background processing with timeout handling
func (s *HTTP) enqueueRawBatch(ctx context.Context, batch ingest.RawLogBatch) error {
	item := queuedItem{
		Ctx:   ctx,
		Batch: batch,
	}

	select {
	case s.rawQueue <- item:
		return nil
	case <-time.After(s.config.Pipeline.EnqueueTimeout):
		return fmt.Errorf("enqueue timeout after %v", s.config.Pipeline.EnqueueTimeout)
	case <-ctx.Done():
		return ctx.Err()
	}
}
