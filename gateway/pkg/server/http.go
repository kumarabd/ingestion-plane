package server

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/gokit/logger"
	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/indexfeed"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/miner"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/sampler"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/sink/loki"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/types"
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

	// Miner components
	minerConfig   *miner.Config
	minerClient   miner.Client
	minerBatcher  *miner.Batcher
	minerInputCh  chan logtypes.NormalizedLog
	minerOutputCh chan types.MinedRecord

	// Sampler components
	samplerConfig             *sampler.SamplerConfig
	enforcementConfig         *sampler.EnforcementConfig
	samplerClient             sampler.SamplerClient
	samplerBatcher            *sampler.SamplerBatcher
	samplerInputCh            chan *sampler.PipelineRecord
	samplerOutputKeptCh       chan *sampler.PipelineRecord
	samplerOutputSuppressedCh chan *sampler.PipelineRecord

	// Loki sink components
	lokiConfig *loki.LokiConfig
	lokiSink   *loki.LokiSink

	// Index-Feed producer components
	indexFeedConfig *indexfeed.Config
	indexFeedClient indexfeed.Client
}

// NewHTTP creates a new HTTP server instance
func NewHTTP(config *HTTPConfig, minerConfig *miner.Config, samplerConfig *sampler.SamplerConfig, enforcementConfig *sampler.EnforcementConfig, lokiConfig *loki.LokiConfig, indexFeedConfig *indexfeed.Config, in *ingest.Handler, l *logger.Handler, m *metrics.Handler) *HTTP {
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
		handler:                   gin.New(),
		ingest:                    in,
		log:                       l,
		metric:                    m,
		config:                    config,
		minerConfig:               minerConfig,
		samplerConfig:             samplerConfig,
		enforcementConfig:         enforcementConfig,
		lokiConfig:                lokiConfig,
		indexFeedConfig:           indexFeedConfig,
		rawQueue:                  make(chan queuedItem, config.Bounds.MaxBatch*2), // Buffer for 2x max batch size
		workerCtx:                 workerCtx,
		workerCancel:              workerCancel,
		minerInputCh:              make(chan logtypes.NormalizedLog, 4096),  // Bounded channel for miner input
		minerOutputCh:             make(chan types.MinedRecord, 4096),       // Bounded channel for miner output
		samplerInputCh:            make(chan *sampler.PipelineRecord, 4096), // Bounded channel for sampler input
		samplerOutputKeptCh:       make(chan *sampler.PipelineRecord, 4096), // Bounded channel for kept records
		samplerOutputSuppressedCh: make(chan *sampler.PipelineRecord, 4096), // Bounded channel for suppressed records
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

	// Initialize miner components
	if err := s.initMiner(); err != nil {
		return fmt.Errorf("failed to initialize miner: %w", err)
	}

	// Initialize sampler components
	if err := s.initSampler(); err != nil {
		return fmt.Errorf("failed to initialize sampler: %w", err)
	}

	// Initialize Loki sink
	if err := s.initLoki(); err != nil {
		return fmt.Errorf("failed to initialize Loki sink: %w", err)
	}

	// Initialize Index-Feed producer
	if err := s.initIndexFeed(); err != nil {
		return fmt.Errorf("failed to initialize Index-Feed producer: %w", err)
	}

	// Start the raw worker
	s.startRawWorker()

	// Start the miner batcher
	s.startMinerBatcher()

	// Start the sampler batcher
	s.startSamplerBatcher()

	// Start the Loki sink
	s.startLokiSink()

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

	// Stop the Index-Feed processor first
	if s.indexFeedClient != nil {
		if err := s.indexFeedClient.Close(ctx); err != nil {
			s.log.Error().Err(err).Msg("Error closing Index-Feed processor")
		} else {
			s.log.Info().Msg("Index-Feed processor stopped")
		}
	}

	// Stop the Loki sink
	if s.lokiSink != nil {
		s.lokiSink.Stop()
		s.log.Info().Msg("Loki sink stopped")
	}

	// Stop the sampler batcher
	if s.samplerBatcher != nil {
		s.samplerBatcher.Stop()
		s.log.Info().Msg("Sampler batcher stopped")
	}

	// Stop the miner batcher
	if s.minerBatcher != nil {
		s.minerBatcher.Stop()
		s.log.Info().Msg("Miner batcher stopped")
	}

	// Stop the raw worker
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

	s.workerWg.Add(1)
	go s.runMinerToSamplerBridge()

	s.workerWg.Add(1)
	go s.runSamplerToLokiBridge()

	s.log.Info().Msg("Raw worker started")
}

// stopRawWorker gracefully stops the raw worker
func (s *HTTP) stopRawWorker() {
	s.log.Info().Msg("Stopping raw worker...")
	s.workerCancel()
	s.workerWg.Wait()
	s.log.Info().Msg("Raw worker stopped")
}

// initMiner initializes the miner client and batcher
func (s *HTTP) initMiner() error {
	if s.minerConfig == nil {
		return fmt.Errorf("miner config is nil")
	}

	// Create miner client
	client, err := miner.NewClient(s.minerConfig)
	if err != nil {
		return fmt.Errorf("failed to create miner client: %w", err)
	}
	s.minerClient = client

	// Create miner batcher
	s.minerBatcher = miner.NewBatcher(
		s.minerConfig.MaxBatch,
		s.minerConfig.MaxBatchWait,
		s.minerInputCh,
		s.minerOutputCh,
		s.minerClient,
		miner.NewMetrics(s.metric),
		s.log,
	)

	return nil
}

// startMinerBatcher starts the miner batcher
func (s *HTTP) startMinerBatcher() {
	if s.minerBatcher != nil {
		s.minerBatcher.Start()
		s.log.Info().Msg("Miner batcher started")
	}
}

// initSampler initializes the sampler client and batcher
func (s *HTTP) initSampler() error {
	if s.samplerConfig == nil {
		return fmt.Errorf("sampler config is nil")
	}

	// Create sampler client
	client, err := sampler.NewSamplerClient(s.samplerConfig)
	if err != nil {
		return fmt.Errorf("failed to create sampler client: %w", err)
	}
	s.samplerClient = client

	// Create sampler batcher
	s.samplerBatcher = sampler.NewSamplerBatcher(
		s.samplerConfig,
		s.enforcementConfig,
		s.samplerClient,
		sampler.NewSamplerMetrics(s.metric),
		s.log,
		s.samplerInputCh,
		s.samplerOutputKeptCh,
		s.samplerOutputSuppressedCh,
	)

	return nil
}

// startSamplerBatcher starts the sampler batcher
func (s *HTTP) startSamplerBatcher() {
	if s.samplerBatcher != nil {
		s.samplerBatcher.Start()
		s.log.Info().Msg("Sampler batcher started")
	}
}

// initLoki initializes the Loki sink
func (s *HTTP) initLoki() error {
	if s.lokiConfig == nil {
		return fmt.Errorf("loki config is nil")
	}

	// Create Loki sink
	s.lokiSink = loki.NewLokiSink(
		s.lokiConfig,
		loki.NewLokiMetrics(s.metric),
		s.log,
	)

	return nil
}

// startLokiSink starts the Loki sink
func (s *HTTP) startLokiSink() {
	if s.lokiSink != nil {
		s.lokiSink.Start()
		s.log.Info().Msg("Loki sink started")
	}
}

// initIndexFeed initializes the Index-Feed processor
func (s *HTTP) initIndexFeed() error {
	if s.indexFeedConfig == nil {
		return fmt.Errorf("indexfeed config is nil")
	}

	// Create Index-Feed client
	client, err := indexfeed.NewClient(s.indexFeedConfig, s.log)
	if err != nil {
		return fmt.Errorf("failed to create Index-Feed client: %w", err)
	}

	// Create Index-Feed processor
	s.indexFeedClient = client

	return nil
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
				// Send normalized logs to miner for processing
				for _, nl := range batch.Records {
					select {
					case s.minerInputCh <- nl:
						// Successfully sent to miner
					default:
						// Backpressure: if miner input is full, block with context deadline
						// or drop lowest severity logs (for now, just block)
						s.minerInputCh <- nl
					}
				}

				// Also emit to the original emitter for shadow mode
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

// runMinerToSamplerBridge bridges miner output to sampler input
func (s *HTTP) runMinerToSamplerBridge() {
	defer s.workerWg.Done()

	s.log.Info().Msg("Miner to sampler bridge started")

	for {
		select {
		case minedRecord := <-s.minerOutputCh:
			// Convert MinedRecord to PipelineRecord using primary result
			pipelineRecord := &sampler.PipelineRecord{
				Normalized: minedRecord.Normalized,
				TemplateID: minedRecord.Primary.TemplateID,
				Shadow:     false,
			}

			// Send to sampler input
			select {
			case s.samplerInputCh <- pipelineRecord:
				// Successfully sent to sampler
			default:
				// Backpressure handling
				s.samplerInputCh <- pipelineRecord
			}

			// Process Index-Feed events
			s.indexFeedClient.ProcessMinedRecord(context.Background(), minedRecord)

		case <-s.workerCtx.Done():
			s.log.Info().Msg("Miner to sampler bridge shutting down")
			return
		}
	}
}

// runSamplerToLokiBridge bridges sampler kept output to Loki sink
func (s *HTTP) runSamplerToLokiBridge() {
	defer s.workerWg.Done()

	s.log.Info().Msg("Sampler to stdout bridge started")

	for {
		select {
		case keptRecord := <-s.samplerOutputKeptCh:
			// Convert PipelineRecord to LokiEntry
			lokiEntry := s.convertToLokiEntry(keptRecord)

			// Send to Loki sink
			s.lokiSink.Enqueue(context.Background(), []loki.LokiEntry{lokiEntry})

		case <-s.workerCtx.Done():
			s.log.Info().Msg("Sampler to Loki bridge shutting down")
			return
		}
	}
}

// convertToLokiEntry converts a PipelineRecord to a LokiEntry
func (s *HTTP) convertToLokiEntry(record *sampler.PipelineRecord) loki.LokiEntry {
	// Build the final log line with annotations
	line := s.buildLogLine(record)

	// Ensure required labels are present
	labels := make(map[string]string)
	for k, v := range record.Normalized.Labels {
		labels[k] = v
	}

	// Add required labels with defaults if missing
	if labels["service"] == "" {
		labels["service"] = "unknown"
	}
	if labels["env"] == "" {
		labels["env"] = "unknown"
	}
	if labels["severity"] == "" {
		labels["severity"] = "info"
	}
	if labels["namespace"] == "" {
		labels["namespace"] = "default"
	}
	if labels["pod"] == "" {
		labels["pod"] = "unknown"
	}

	return loki.LokiEntry{
		Timestamp: record.Normalized.Timestamp,
		Labels:    labels,
		Line:      line,
	}
}

// buildLogLine builds the final log line as JSON with all useful fields
func (s *HTTP) buildLogLine(record *sampler.PipelineRecord) string {
	// Create a structured log entry
	logEntry := map[string]interface{}{
		"message": record.Normalized.Message,
		"ts":      record.Normalized.Timestamp.Format(time.RFC3339Nano),
		"labels":  record.Normalized.Labels,
	}

	// Add template information
	if record.TemplateID != "" {
		logEntry["template_id"] = record.TemplateID
	}

	// Add decision information with normalized reason strings
	if record.Decision.Action.String() != "" {
		logEntry["action"] = strings.ToLower(strings.TrimPrefix(record.Decision.Action.String(), "ACTION_"))
	}

	// Normalize reason strings to human-readable format
	reasonStr := s.normalizeKeepReason(record.Decision.KeepReason)
	if reasonStr != "" {
		if record.Shadow {
			logEntry["keep_reason_shadow"] = reasonStr
			logEntry["enforced"] = false
		} else {
			logEntry["keep_reason"] = reasonStr
			logEntry["enforced"] = true
		}
	}

	// Add useful fields
	if record.Normalized.Truncated {
		logEntry["truncated"] = true
	}

	if record.Normalized.Sanitized {
		logEntry["sanitized"] = true
	}

	// Add provenance information if available
	if record.Normalized.Redaction.Applied {
		logEntry["redaction_applied"] = true
		logEntry["redaction_rules"] = record.Normalized.Redaction.Rules
		logEntry["redaction_count"] = record.Normalized.Redaction.Count
	}

	// Add sampling information
	if record.Decision.SampleRate > 0 {
		logEntry["sample_rate"] = record.Decision.SampleRate
	}

	// Add policy version
	if record.Decision.PolicyVersion != "" {
		logEntry["policy_version"] = record.Decision.PolicyVersion
	}

	// Add shadow mode indicator
	if record.Shadow {
		logEntry["shadow"] = true
	}

	// Add schema information
	if record.Normalized.Schema != "" {
		logEntry["schema"] = record.Normalized.Schema
	}

	// Convert to JSON
	jsonBytes, err := json.Marshal(logEntry)
	if err != nil {
		// Fallback to simple format if JSON marshaling fails
		return fmt.Sprintf("%s | template_id=%s action=%s",
			record.Normalized.Message,
			record.TemplateID,
			strings.ToLower(strings.TrimPrefix(record.Decision.Action.String(), "ACTION_")))
	}

	return string(jsonBytes)
}

// normalizeKeepReason converts enum values to human-readable strings
func (s *HTTP) normalizeKeepReason(reason samplerv1.KeepReason) string {
	switch reason {
	case samplerv1.KeepReason_KEEP_REASON_SEVERITY:
		return "SEVERITY"
	case samplerv1.KeepReason_KEEP_REASON_NOVEL:
		return "NOVEL"
	case samplerv1.KeepReason_KEEP_REASON_SPIKE:
		return "SPIKE"
	case samplerv1.KeepReason_KEEP_REASON_WARMUP:
		return "WARMUP"
	case samplerv1.KeepReason_KEEP_REASON_LOG2:
		return "LOG2"
	case samplerv1.KeepReason_KEEP_REASON_STEADYK:
		return "STEADYK"
	case samplerv1.KeepReason_KEEP_REASON_BUDGET:
		return "BUDGET"
	default:
		return ""
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
