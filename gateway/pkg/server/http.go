package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/service"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// HTTPConfig contains configuration for the HTTP server
type HTTPConfig struct {
	Host         string        `json:"host" yaml:"host" default:"0.0.0.0"`
	Port         string        `json:"port" yaml:"port" default:"8080"`
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout" default:"30s"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout" default:"30s"`
	IdleTimeout  time.Duration `json:"idle_timeout" yaml:"idle_timeout" default:"60s"`
}

// HTTP implements the Server interface for HTTP
type HTTP struct {
	handler   *gin.Engine
	service   *service.Handler
	log       *logger.Handler
	metric    *metrics.Handler
	config    *HTTPConfig
	server    *http.Server
	isRunning bool
	mu        sync.RWMutex
}

// NewHTTP creates a new HTTP server instance
func NewHTTP(config *HTTPConfig, service *service.Handler, l *logger.Handler, m *metrics.Handler) *HTTP {
	gin.SetMode(gin.ReleaseMode)

	server := &HTTP{
		handler: gin.New(),
		service: service,
		log:     l,
		metric:  m,
		config:  config,
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

// Start starts the HTTP server
func (s *HTTP) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isRunning {
		return fmt.Errorf("HTTP server is already running")
	}

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

// Stop gracefully shuts down the HTTP server
func (s *HTTP) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunning || s.server == nil {
		return nil
	}

	s.log.Info().Msg("Shutting down HTTP server...")

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
	// Minimal endpoints as specified
	// /v1/ingest (batch)
	s.handler.POST("/v1/ingest", s.ingestHandler)

	// /healthz
	s.handler.GET("/healthz", s.healthHandler)

	// /metrics
	s.handler.GET("/metrics", s.metricsHandler)
}

// ingestHandler handles batch log ingestion
func (s *HTTP) ingestHandler(c *gin.Context) {
	otlpHandler := s.service.GetOTLPHandler()
	otlpHandler.HandleOTLPHTTP(c)
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
