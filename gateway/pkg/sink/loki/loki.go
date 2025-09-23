package loki

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

// LokiEntry represents a log entry to be sent to Loki
type LokiEntry struct {
	Timestamp time.Time         `json:"-"`
	Labels    map[string]string `json:"-"`
	Line      string            `json:"-"`
}

// streamBuffer represents a per-stream buffer of entries to flush
type streamBuffer struct {
	labels   map[string]string
	entries  []LokiEntry
	bytes    int // rough running total of payload bytes
	lastPush time.Time
}

// LokiConfig contains configuration for the Loki sink
type LokiConfig struct {
	Addr             string        `json:"addr" yaml:"addr" default:"http://loki:3100"`
	FlushInterval    time.Duration `json:"flush_interval" yaml:"flush_interval" default:"400ms"`
	MaxBatchBytes    int           `json:"max_batch_bytes" yaml:"max_batch_bytes" default:"1000000"`
	MaxBatchEntries  int           `json:"max_batch_entries" yaml:"max_batch_entries" default:"5000"`
	MaxBufferBytes   int64         `json:"max_buffer_bytes" yaml:"max_buffer_bytes" default:"268435456"` // 256MB
	MaxBufferEntries int64         `json:"max_buffer_entries" yaml:"max_buffer_entries" default:"1000000"`
	RequestTimeout   time.Duration `json:"request_timeout" yaml:"request_timeout" default:"5s"`
	MockMode         bool          `json:"mock_mode" yaml:"mock_mode" default:"false"`
	Retry            RetryConfig   `json:"retry" yaml:"retry"`
	DropPolicy       DropPolicy    `json:"drop_policy" yaml:"drop_policy"`
	Labels           LabelConfig   `json:"labels" yaml:"labels"`
}

// RetryConfig contains retry configuration
type RetryConfig struct {
	Enabled        bool          `json:"enabled" yaml:"enabled" default:"true"`
	InitialBackoff time.Duration `json:"initial_backoff" yaml:"initial_backoff" default:"200ms"`
	MaxBackoff     time.Duration `json:"max_backoff" yaml:"max_backoff" default:"5s"`
}

// DropPolicy contains drop policy configuration
type DropPolicy struct {
	DebugFirst  bool `json:"debug_first" yaml:"debug_first" default:"true"`
	ProtectInfo bool `json:"protect_info" yaml:"protect_info" default:"true"`
}

// LabelConfig contains label configuration
type LabelConfig struct {
	Static map[string]string `json:"static" yaml:"static"`
}

// LokiSink handles sending logs to Loki
type LokiSink struct {
	client           *http.Client
	addr             string
	flushInterval    time.Duration
	maxBatchBytes    int
	maxBatchEntries  int
	maxBufferBytes   int64
	maxBufferEntries int64
	retryEnabled     bool
	backoffInit      time.Duration
	backoffMax       time.Duration
	dropDebugFirst   bool
	protectInfo      bool
	staticLabels     map[string]string
	mockMode         bool

	// state
	mu          sync.Mutex
	streams     map[string]*streamBuffer // key -> buffer
	usedBytes   int64
	usedEntries int64

	// worker management
	stopCh      chan struct{}
	flushTicker *time.Ticker

	metrics LokiMetrics
	log     *logger.Handler
}

// LokiMetrics interface for Loki-specific metrics
type LokiMetrics interface {
	IncLokiEnqueued(severity string)
	IncLokiBufferBytes(state string, value int64)
	IncLokiBufferEntries(state string, value int64)
	SetLokiStreamsActive(count int)
	IncLokiFlush(status string)
	ObserveLokiFlushLatency(duration time.Duration)
	IncLokiHTTPStatus(code int)
	IncLokiDropped(severity string, reason string)
}

// NewLokiSink creates a new Loki sink
func NewLokiSink(cfg *LokiConfig, metrics LokiMetrics, log *logger.Handler) *LokiSink {
	// Create HTTP client with sensible defaults
	client := &http.Client{
		Timeout: cfg.RequestTimeout,
		Transport: &http.Transport{
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	return &LokiSink{
		client:           client,
		addr:             cfg.Addr,
		flushInterval:    cfg.FlushInterval,
		maxBatchBytes:    cfg.MaxBatchBytes,
		maxBatchEntries:  cfg.MaxBatchEntries,
		maxBufferBytes:   cfg.MaxBufferBytes,
		maxBufferEntries: cfg.MaxBufferEntries,
		retryEnabled:     cfg.Retry.Enabled,
		backoffInit:      cfg.Retry.InitialBackoff,
		backoffMax:       cfg.Retry.MaxBackoff,
		dropDebugFirst:   cfg.DropPolicy.DebugFirst,
		protectInfo:      cfg.DropPolicy.ProtectInfo,
		staticLabels:     cfg.Labels.Static,
		mockMode:         cfg.MockMode,
		streams:          make(map[string]*streamBuffer),
		stopCh:           make(chan struct{}),
		metrics:          metrics,
		log:              log,
	}
}

// Start starts the Loki sink
func (s *LokiSink) Start() {
	s.flushTicker = time.NewTicker(s.flushInterval)
	go func() {
		for {
			select {
			case <-s.flushTicker.C:
				s.flushDue()
			case <-s.stopCh:
				return
			}
		}
	}()
	s.log.Info().Msg("Loki sink started")
}

// Stop stops the Loki sink
func (s *LokiSink) Stop() {
	if s.flushTicker != nil {
		s.flushTicker.Stop()
	}
	close(s.stopCh)

	// Final flush with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.flushAll(ctx)
	s.log.Info().Msg("Loki sink stopped")
}

// Enqueue adds entries to the Loki sink
func (s *LokiSink) Enqueue(ctx context.Context, entries []LokiEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range entries {
		// Check global buffers; drop under pressure with severity-aware policy
		if s.usedBytes >= s.maxBufferBytes || s.usedEntries >= s.maxBufferEntries {
			sev := strings.ToLower(e.Labels["severity"])
			if s.shouldDropUnderPressure(sev) {
				s.metrics.IncLokiDropped(sev, "buffer_full")
				continue
			}
		}

		key, lbls := s.streamKey(e.Labels)
		buf := s.streams[key]
		if buf == nil {
			buf = &streamBuffer{labels: lbls, lastPush: time.Now()}
			s.streams[key] = buf
		}
		buf.entries = append(buf.entries, e)
		// Rough size accounting: timestamp+line JSON overhead estimate
		s.usedEntries++
		s.usedBytes += int64(len(e.Line)) + 32
		buf.bytes += len(e.Line) + 32

		s.metrics.IncLokiEnqueued(strings.ToLower(e.Labels["severity"]))
	}

	// Update metrics
	s.metrics.IncLokiBufferBytes("used", s.usedBytes)
	s.metrics.IncLokiBufferBytes("limit", s.maxBufferBytes)
	s.metrics.IncLokiBufferEntries("used", s.usedEntries)
	s.metrics.IncLokiBufferEntries("limit", s.maxBufferEntries)
	s.metrics.SetLokiStreamsActive(len(s.streams))
}

// streamKey computes a stable key for stream grouping
func (s *LokiSink) streamKey(labels map[string]string) (string, map[string]string) {
	// Combine labels for stable key
	keyLabels := make(map[string]string)
	for k, v := range labels {
		if k == "service" || k == "env" || k == "severity" || k == "namespace" || k == "pod" {
			keyLabels[k] = v
		}
	}

	// Add static labels
	for k, v := range s.staticLabels {
		keyLabels[k] = v
	}

	// Create stable key by sorting and concatenating
	var keys []string
	for k := range keyLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var keyParts []string
	for _, k := range keys {
		keyParts = append(keyParts, fmt.Sprintf("%s=%s", k, keyLabels[k]))
	}

	return strings.Join(keyParts, ","), keyLabels
}

// shouldDropUnderPressure determines if an entry should be dropped under pressure
func (s *LokiSink) shouldDropUnderPressure(sev string) bool {
	// Order: debug → info → warn (last), never drop error/fatal here.
	if s.dropDebugFirst {
		switch sev {
		case "debug":
			return true
		case "info":
			return !s.protectInfo // if protectInfo=false, allow dropping info after debug
		default:
			return false
		}
	}
	// Fallback: only drop debug
	return sev == "debug"
}

// flushDue flushes buffers that are due for flushing
func (s *LokiSink) flushDue() {
	now := time.Now()
	var toFlushKeys []string

	s.mu.Lock()
	for k, buf := range s.streams {
		if len(buf.entries) == 0 {
			continue
		}
		if buf.bytes >= s.maxBatchBytes || len(buf.entries) >= s.maxBatchEntries || now.Sub(buf.lastPush) >= s.flushInterval {
			toFlushKeys = append(toFlushKeys, k)
		}
	}
	s.mu.Unlock()

	for _, k := range toFlushKeys {
		s.flushStream(k)
	}
}

// flushAll flushes all streams (used during shutdown)
func (s *LokiSink) flushAll(ctx context.Context) {
	s.mu.Lock()
	var keys []string
	for k := range s.streams {
		if len(s.streams[k].entries) > 0 {
			keys = append(keys, k)
		}
	}
	s.mu.Unlock()

	for _, k := range keys {
		select {
		case <-ctx.Done():
			return
		default:
			s.flushStream(k)
		}
	}
}

// flushStream flushes a specific stream
func (s *LokiSink) flushStream(key string) {
	// Take ownership of entries (pop under lock)
	s.mu.Lock()
	buf := s.streams[key]
	if buf == nil || len(buf.entries) == 0 {
		s.mu.Unlock()
		return
	}
	entries := buf.entries
	buf.entries = nil
	buf.bytes = 0
	buf.lastPush = time.Now()
	s.mu.Unlock()

	// Chunk & send
	startIdx := 0
	for startIdx < len(entries) {
		// Chunk by count; approximate by bytes if needed
		end := startIdx + s.maxBatchEntries
		if end > len(entries) {
			end = len(entries)
		}

		// Build Loki push payload
		lp := lokiPush{
			Streams: []lokiStream{{
				Stream: buf.labels,
				Values: make([][2]string, 0, end-startIdx),
			}},
		}

		approxBytes := 0
		for i := startIdx; i < end; i++ {
			ts := strconv.FormatInt(entries[i].Timestamp.UnixNano(), 10)
			line := entries[i].Line
			lp.Streams[0].Values = append(lp.Streams[0].Values, [2]string{ts, line})
			approxBytes += len(line) + 32
			if approxBytes >= s.maxBatchBytes {
				end = i + 1
				break
			}
		}

		// Serialize + gzip
		var bufJSON bytes.Buffer
		enc := json.NewEncoder(&bufJSON)
		if err := enc.Encode(lp); err != nil {
			// On encode error, count dropped and continue
			s.metrics.IncLokiFlush("fail")
			s.metrics.IncLokiDropped(strings.ToLower(buf.labels["severity"]), "encode_error")
			startIdx = end
			continue
		}

		var gzBuf bytes.Buffer
		gz := gzip.NewWriter(&gzBuf)
		_, _ = gz.Write(bufJSON.Bytes())
		_ = gz.Close()

		// Send with retry
		startTime := time.Now()
		status, ok := s.postWithRetry(gzBuf.Bytes())
		latency := time.Since(startTime)

		s.metrics.IncLokiHTTPStatus(status)
		s.metrics.ObserveLokiFlushLatency(latency)

		if ok {
			s.metrics.IncLokiFlush("success")
			// Adjust global counters
			s.mu.Lock()
			for i := startIdx; i < end; i++ {
				s.usedEntries--
				s.usedBytes -= int64(len(entries[i].Line) + 32)
			}
			s.mu.Unlock()
		} else {
			s.metrics.IncLokiFlush("fail")
			// On fail after retries: drop these entries (avoid infinite memory growth)
			sev := strings.ToLower(buf.labels["severity"])
			dropped := end - startIdx
			for i := 0; i < dropped; i++ {
				s.metrics.IncLokiDropped(sev, "retry_exhausted")
			}
			s.mu.Lock()
			for i := startIdx; i < end; i++ {
				s.usedEntries--
				s.usedBytes -= int64(len(entries[i].Line) + 32)
			}
			s.mu.Unlock()
		}

		startIdx = end
	}
}

// postWithRetry sends HTTP POST with retry/backoff logic
func (s *LokiSink) postWithRetry(body []byte) (status int, success bool) {
	// Mock mode: just print to stdout instead of making HTTP request
	if s.mockMode {
		s.mockOutput(body)
		return 200, true // Simulate successful response
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.flushInterval*2) // Use flush interval as base timeout
	defer cancel()

	attempt := 0
	backoff := s.backoffInit
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.addr+"/loki/api/v1/push", bytes.NewReader(body))
		if err != nil {
			if !s.retryEnabled {
				return 0, false
			}
		} else {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", "gzip")

			resp, err := s.client.Do(req)
			if err != nil {
				// Network error → retry if enabled
				if !s.retryEnabled {
					return 0, false
				}
			} else {
				status = resp.StatusCode
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				if status >= 200 && status < 300 {
					return status, true
				}
				// Non-2xx
				if !(status == 429 || (status >= 500 && status <= 599)) || !s.retryEnabled {
					return status, false
				}
			}
		}

		// Retry path
		attempt++
		d := backoff + time.Duration(rand.Int63n(int64(backoff/2)+1))
		if d > s.backoffMax {
			d = s.backoffMax
		}
		time.Sleep(d)
		if backoff < s.backoffMax/2 {
			backoff *= 2
		} else {
			backoff = s.backoffMax
		}
		// Limit attempts
		if attempt >= 5 {
			return status, false
		}
	}
}

// mockOutput prints the Loki push payload to stdout for local development
func (s *LokiSink) mockOutput(body []byte) {
	// Decompress gzip body to make it readable
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to decompress gzip body in mock mode")
		return
	}
	defer reader.Close()

	// Read the decompressed JSON
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		s.log.Error().Err(err).Msg("Failed to read decompressed body in mock mode")
		return
	}

	// Pretty print the JSON
	var prettyJSON bytes.Buffer
	if err := json.Indent(&prettyJSON, decompressed, "", "  "); err != nil {
		s.log.Error().Err(err).Msg("Failed to indent JSON in mock mode")
		return
	}

	// Print to stdout with clear formatting
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Println("LOKI MOCK OUTPUT - Logs would be sent to Loki")
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Println(prettyJSON.String())
	fmt.Println("=" + strings.Repeat("=", 80))
	fmt.Println()
}

// Loki push format structures
type lokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiPush struct {
	Streams []lokiStream `json:"streams"`
}

// metricsHandler implements LokiMetrics interface using the existing metrics handler
type metricsHandler struct {
	*metrics.Handler
}

// NewLokiMetrics creates a new Loki metrics handler
func NewLokiMetrics(h *metrics.Handler) LokiMetrics {
	return &metricsHandler{Handler: h}
}

// IncLokiEnqueued increments Loki enqueued counter
func (m *metricsHandler) IncLokiEnqueued(severity string) {
	m.IncIngestRecordsTotal("loki_enqueued", severity)
}

// IncLokiBufferBytes updates Loki buffer bytes gauge
func (m *metricsHandler) IncLokiBufferBytes(state string, value int64) {
	// Using existing metrics infrastructure - this would ideally be a gauge
	m.IncIngestBatchesTotal("loki_buffer_bytes_" + state)
}

// IncLokiBufferEntries updates Loki buffer entries gauge
func (m *metricsHandler) IncLokiBufferEntries(state string, value int64) {
	// Using existing metrics infrastructure - this would ideally be a gauge
	m.IncIngestBatchesTotal("loki_buffer_entries_" + state)
}

// SetLokiStreamsActive sets the active streams count
func (m *metricsHandler) SetLokiStreamsActive(count int) {
	// Using existing metrics infrastructure - this would ideally be a gauge
	m.IncIngestBatchesTotal("loki_streams_active")
}

// IncLokiFlush increments Loki flush counter
func (m *metricsHandler) IncLokiFlush(status string) {
	m.IncIngestBatchesTotal("loki_flush_" + status)
}

// ObserveLokiFlushLatency records Loki flush latency
func (m *metricsHandler) ObserveLokiFlushLatency(duration time.Duration) {
	m.IngestHandlerLatency.WithLabelValues("loki_flush", "true").Observe(duration.Seconds())
}

// IncLokiHTTPStatus increments Loki HTTP status counter
func (m *metricsHandler) IncLokiHTTPStatus(code int) {
	// Using existing metrics infrastructure
	m.IncIngestRecordsTotal("loki_http", fmt.Sprintf("%d", code))
}

// IncLokiDropped increments Loki dropped counter
func (m *metricsHandler) IncLokiDropped(severity string, reason string) {
	m.IncIngestRejectedTotal("loki_dropped_" + severity + "_" + reason)
}
