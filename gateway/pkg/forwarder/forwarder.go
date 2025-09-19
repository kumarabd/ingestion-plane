package forwarder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Config contains configuration for the forwarder
type Config struct {
	Miner MinerConfig `json:"miner" yaml:"miner"`
	Loki  LokiConfig  `json:"loki" yaml:"loki"`

	// Shadow mode settings
	ShadowMode    bool          `json:"shadow_mode" yaml:"shadow_mode" default:"true"`        // Enable shadow mode
	MaxRetries    int           `json:"max_retries" yaml:"max_retries" default:"3"`           // Max retry attempts
	RetryDelay    time.Duration `json:"retry_delay" yaml:"retry_delay" default:"1s"`          // Delay between retries
	Timeout       time.Duration `json:"timeout" yaml:"timeout" default:"10s"`                 // Request timeout
	BatchSize     int           `json:"batch_size" yaml:"batch_size" default:"100"`           // Batch size for forwarding
	FlushInterval time.Duration `json:"flush_interval" yaml:"flush_interval" default:"5s"`    // Flush interval
	MaxQueueSize  int           `json:"max_queue_size" yaml:"max_queue_size" default:"10000"` // Max queue size
}

// MinerConfig contains configuration for Miner forwarding
type MinerConfig struct {
	Enabled  bool          `json:"enabled" yaml:"enabled" default:"true"`
	Endpoint string        `json:"endpoint" yaml:"endpoint" default:"http://localhost:8001/api/v1/logs"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout" default:"5s"`
}

// LokiConfig contains configuration for Loki forwarding
type LokiConfig struct {
	Enabled  bool          `json:"enabled" yaml:"enabled" default:"true"`
	Endpoint string        `json:"endpoint" yaml:"endpoint" default:"http://localhost:3100/loki/api/v1/push"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout" default:"5s"`
	TenantID string        `json:"tenant_id" yaml:"tenant_id" default:""`
}

// Forwarder handles forwarding logs to multiple destinations
type Forwarder struct {
	config *Config
	log    *logger.Handler
	metric *metrics.Handler
	tracer trace.Tracer

	// HTTP clients
	minerClient *http.Client
	lokiClient  *http.Client

	// Queues for batching
	minerQueue chan *LogEntry
	lokiQueue  chan *LogEntry

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Metrics
	metrics *ForwarderMetrics
}

// LogEntry represents a log entry to be forwarded
type LogEntry struct {
	Timestamp int64             `json:"timestamp"`
	Labels    map[string]string `json:"labels"`
	Message   string            `json:"message"`
	Fields    map[string]string `json:"fields"`
	Schema    string            `json:"schema"`
	Sanitized bool              `json:"sanitized"`
	OrigLen   uint32            `json:"orig_len"`

	// Forwarding metadata
	ForwardTime time.Time `json:"forward_time"`
	TraceID     string    `json:"trace_id,omitempty"`
	SpanID      string    `json:"span_id,omitempty"`
}

// ForwarderMetrics contains metrics for the forwarder
type ForwarderMetrics struct {
	// Counters
	LogsForwardedTotal *metrics.Counter
	LogsDroppedTotal   *metrics.Counter
	RetriesTotal       *metrics.Counter
	ErrorsTotal        *metrics.Counter

	// Histograms
	ForwardLatency *metrics.Histogram
	QueueSize      *metrics.Gauge
	BatchSize      *metrics.Histogram

	// Destination-specific metrics
	MinerLogsForwarded *metrics.Counter
	MinerErrors        *metrics.Counter
	MinerLatency       *metrics.Histogram

	LokiLogsForwarded *metrics.Counter
	LokiErrors        *metrics.Counter
	LokiLatency       *metrics.Histogram
}

// NewForwarder creates a new log forwarder
func NewForwarder(config *Config, log *logger.Handler, metric *metrics.Handler) *Forwarder {
	ctx, cancel := context.WithCancel(context.Background())

	forwarder := &Forwarder{
		config:     config,
		log:        log,
		metric:     metric,
		tracer:     otel.Tracer("gateway/forwarder"),
		minerQueue: make(chan *LogEntry, config.MaxQueueSize),
		lokiQueue:  make(chan *LogEntry, config.MaxQueueSize),
		ctx:        ctx,
		cancel:     cancel,
	}

	// Create HTTP clients
	forwarder.minerClient = &http.Client{
		Timeout: config.Miner.Timeout,
	}
	forwarder.lokiClient = &http.Client{
		Timeout: config.Loki.Timeout,
	}

	// Initialize metrics
	forwarder.initMetrics()

	return forwarder
}

// initMetrics initializes forwarder metrics
func (f *Forwarder) initMetrics() {
	f.metrics = &ForwarderMetrics{
		LogsForwardedTotal: f.metric.NewCounter("forwarder_logs_forwarded_total", "Total number of logs forwarded"),
		LogsDroppedTotal:   f.metric.NewCounter("forwarder_logs_dropped_total", "Total number of logs dropped"),
		RetriesTotal:       f.metric.NewCounter("forwarder_retries_total", "Total number of retry attempts"),
		ErrorsTotal:        f.metric.NewCounter("forwarder_errors_total", "Total number of forwarding errors"),

		ForwardLatency: f.metric.NewHistogram("forwarder_latency_seconds", "Forwarding latency in seconds"),
		QueueSize:      f.metric.NewGauge("forwarder_queue_size", "Current queue size"),
		BatchSize:      f.metric.NewHistogram("forwarder_batch_size", "Batch size for forwarding"),

		MinerLogsForwarded: f.metric.NewCounter("forwarder_miner_logs_forwarded_total", "Total logs forwarded to Miner"),
		MinerErrors:        f.metric.NewCounter("forwarder_miner_errors_total", "Total Miner forwarding errors"),
		MinerLatency:       f.metric.NewHistogram("forwarder_miner_latency_seconds", "Miner forwarding latency"),

		LokiLogsForwarded: f.metric.NewCounter("forwarder_loki_logs_forwarded_total", "Total logs forwarded to Loki"),
		LokiErrors:        f.metric.NewCounter("forwarder_loki_errors_total", "Total Loki forwarding errors"),
		LokiLatency:       f.metric.NewHistogram("forwarder_loki_latency_seconds", "Loki forwarding latency"),
	}
}

// Start starts the forwarder
func (f *Forwarder) Start() error {
	f.log.Info().Msg("Starting log forwarder")

	// Start Miner forwarder if enabled
	if f.config.Miner.Enabled {
		f.wg.Add(1)
		go f.forwardToMiner()
	}

	// Start Loki forwarder if enabled
	if f.config.Loki.Enabled {
		f.wg.Add(1)
		go f.forwardToLoki()
	}

	// Start metrics reporter
	f.wg.Add(1)
	go f.reportMetrics()

	f.log.Info().Msg("Log forwarder started")
	return nil
}

// Stop stops the forwarder
func (f *Forwarder) Stop() error {
	f.log.Info().Msg("Stopping log forwarder")

	f.cancel()
	f.wg.Wait()

	f.log.Info().Msg("Log forwarder stopped")
	return nil
}

// Forward forwards a log entry to all configured destinations
func (f *Forwarder) Forward(ctx context.Context, logEntry *LogEntry) error {
	ctx, span := f.tracer.Start(ctx, "forwarder.Forward")
	defer span.End()

	// Add tracing metadata
	if spanCtx := span.SpanContext(); spanCtx.IsValid() {
		logEntry.TraceID = spanCtx.TraceID().String()
		logEntry.SpanID = spanCtx.SpanID().String()
	}

	logEntry.ForwardTime = time.Now()

	// Forward to Miner if enabled
	if f.config.Miner.Enabled {
		select {
		case f.minerQueue <- logEntry:
			// Successfully queued
		default:
			// Queue full, drop log in shadow mode
			if f.config.ShadowMode {
				f.metrics.LogsDroppedTotal.Inc(map[string]string{"destination": "miner", "reason": "queue_full"})
				f.log.Warn().Msg("Miner queue full, dropping log in shadow mode")
			}
		}
	}

	// Forward to Loki if enabled
	if f.config.Loki.Enabled {
		select {
		case f.lokiQueue <- logEntry:
			// Successfully queued
		default:
			// Queue full, drop log in shadow mode
			if f.config.ShadowMode {
				f.metrics.LogsDroppedTotal.Inc(map[string]string{"destination": "loki", "reason": "queue_full"})
				f.log.Warn().Msg("Loki queue full, dropping log in shadow mode")
			}
		}
	}

	span.SetAttributes(
		attribute.String("log.schema", logEntry.Schema),
		attribute.Int("log.orig_len", int(logEntry.OrigLen)),
		attribute.Bool("log.sanitized", logEntry.Sanitized),
	)

	return nil
}

// forwardToMiner forwards logs to Miner
func (f *Forwarder) forwardToMiner() {
	defer f.wg.Done()

	batch := make([]*LogEntry, 0, f.config.BatchSize)
	ticker := time.NewTicker(f.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			// Flush remaining logs
			if len(batch) > 0 {
				f.flushMinerBatch(batch)
			}
			return

		case logEntry := <-f.minerQueue:
			batch = append(batch, logEntry)

			// Flush if batch is full
			if len(batch) >= f.config.BatchSize {
				f.flushMinerBatch(batch)
				batch = batch[:0] // Reset batch
			}

		case <-ticker.C:
			// Flush on timer
			if len(batch) > 0 {
				f.flushMinerBatch(batch)
				batch = batch[:0] // Reset batch
			}
		}
	}
}

// forwardToLoki forwards logs to Loki
func (f *Forwarder) forwardToLoki() {
	defer f.wg.Done()

	batch := make([]*LogEntry, 0, f.config.BatchSize)
	ticker := time.NewTicker(f.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			// Flush remaining logs
			if len(batch) > 0 {
				f.flushLokiBatch(batch)
			}
			return

		case logEntry := <-f.lokiQueue:
			batch = append(batch, logEntry)

			// Flush if batch is full
			if len(batch) >= f.config.BatchSize {
				f.flushLokiBatch(batch)
				batch = batch[:0] // Reset batch
			}

		case <-ticker.C:
			// Flush on timer
			if len(batch) > 0 {
				f.flushLokiBatch(batch)
				batch = batch[:0] // Reset batch
			}
		}
	}
}

// flushMinerBatch flushes a batch of logs to Miner
func (f *Forwarder) flushMinerBatch(batch []*LogEntry) {
	ctx, span := f.tracer.Start(f.ctx, "forwarder.flushMinerBatch")
	defer span.End()

	start := time.Now()

	// Convert to Miner format
	minerLogs := f.convertToMinerFormat(batch)

	// Send to Miner
	err := f.sendToMiner(ctx, minerLogs)

	latency := time.Since(start).Seconds()
	f.metrics.MinerLatency.Observe(latency)
	f.metrics.BatchSize.Observe(float64(len(batch)))

	if err != nil {
		f.metrics.MinerErrors.Inc(map[string]string{"error": err.Error()})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		f.log.Error().Err(err).Int("batch_size", len(batch)).Msg("Failed to forward batch to Miner")
	} else {
		f.metrics.MinerLogsForwarded.Add(float64(len(batch)))
		f.metrics.LogsForwardedTotal.Add(float64(len(batch)))
		span.SetAttributes(attribute.Int("batch.size", len(batch)))
	}
}

// flushLokiBatch flushes a batch of logs to Loki
func (f *Forwarder) flushLokiBatch(batch []*LogEntry) {
	ctx, span := f.tracer.Start(f.ctx, "forwarder.flushLokiBatch")
	defer span.End()

	start := time.Now()

	// Convert to Loki format
	lokiLogs := f.convertToLokiFormat(batch)

	// Send to Loki
	err := f.sendToLoki(ctx, lokiLogs)

	latency := time.Since(start).Seconds()
	f.metrics.LokiLatency.Observe(latency)

	if err != nil {
		f.metrics.LokiErrors.Inc(map[string]string{"error": err.Error()})
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		f.log.Error().Err(err).Int("batch_size", len(batch)).Msg("Failed to forward batch to Loki")
	} else {
		f.metrics.LokiLogsForwarded.Add(float64(len(batch)))
		f.metrics.LogsForwardedTotal.Add(float64(len(batch)))
		span.SetAttributes(attribute.Int("batch.size", len(batch)))
	}
}

// convertToMinerFormat converts log entries to Miner format
func (f *Forwarder) convertToMinerFormat(logs []*LogEntry) []map[string]interface{} {
	minerLogs := make([]map[string]interface{}, len(logs))

	for i, log := range logs {
		minerLogs[i] = map[string]interface{}{
			"timestamp": log.Timestamp,
			"labels":    log.Labels,
			"message":   log.Message,
			"fields":    log.Fields,
			"schema":    log.Schema,
			"sanitized": log.Sanitized,
			"orig_len":  log.OrigLen,
		}
	}

	return minerLogs
}

// convertToLokiFormat converts log entries to Loki format
func (f *Forwarder) convertToLokiFormat(logs []*LogEntry) map[string]interface{} {
	streams := make(map[string][]map[string]interface{})

	for _, log := range logs {
		// Create stream key from labels
		streamKey := f.createStreamKey(log.Labels)

		// Create Loki log entry
		lokiEntry := map[string]interface{}{
			"ts":   fmt.Sprintf("%d", log.Timestamp),
			"line": log.Message,
		}

		// Add fields as additional labels for Loki
		labels := make(map[string]string)
		for k, v := range log.Labels {
			labels[k] = v
		}
		for k, v := range log.Fields {
			// Prefix fields to avoid conflicts
			labels["field_"+k] = v
		}
		labels["schema"] = log.Schema
		labels["sanitized"] = fmt.Sprintf("%t", log.Sanitized)

		stream := map[string]interface{}{
			"stream": labels,
			"values": [][]string{{fmt.Sprintf("%d", log.Timestamp), log.Message}},
		}

		streams[streamKey] = append(streams[streamKey], stream)
	}

	return map[string]interface{}{
		"streams": streams,
	}
}

// createStreamKey creates a unique stream key from labels
func (f *Forwarder) createStreamKey(labels map[string]string) string {
	// Use service and environment as primary stream key
	if service, ok := labels["service.name"]; ok {
		if env, ok := labels["service.environment"]; ok {
			return fmt.Sprintf("%s_%s", service, env)
		}
		return service
	}
	return "default"
}

// sendToMiner sends logs to Miner
func (f *Forwarder) sendToMiner(ctx context.Context, logs []map[string]interface{}) error {
	payload := map[string]interface{}{
		"logs": logs,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal logs for Miner: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", f.config.Miner.Endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Miner request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := f.minerClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs to Miner: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Miner returned error status: %d", resp.StatusCode)
	}

	return nil
}

// sendToLoki sends logs to Loki
func (f *Forwarder) sendToLoki(ctx context.Context, logs map[string]interface{}) error {
	jsonData, err := json.Marshal(logs)
	if err != nil {
		return fmt.Errorf("failed to marshal logs for Loki: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", f.config.Loki.Endpoint, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create Loki request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if f.config.Loki.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", f.config.Loki.TenantID)
	}

	resp, err := f.lokiClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send logs to Loki: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Loki returned error status: %d", resp.StatusCode)
	}

	return nil
}

// reportMetrics reports queue metrics periodically
func (f *Forwarder) reportMetrics() {
	defer f.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-f.ctx.Done():
			return
		case <-ticker.C:
			// Report queue sizes
			f.metrics.QueueSize.Set(float64(len(f.minerQueue)), map[string]string{"destination": "miner"})
			f.metrics.QueueSize.Set(float64(len(f.lokiQueue)), map[string]string{"destination": "loki"})
		}
	}
}
