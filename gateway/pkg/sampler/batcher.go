package sampler

import (
	"context"
	"strings"
	"time"

	"github.com/kumarabd/gokit/logger"
	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

// SamplerMetrics interface for sampler-specific metrics
type SamplerMetrics interface {
	ObserveSamplerRPC(latency time.Duration, status string)
	IncSamplerDecisions(action string, reason string)
	IncSamplerEnforced(action string)
	IncSamplerFallback()
	ObserveSamplerBatch(size int)
}

// SamplerBatcher handles batching pipeline records for sampler processing
type SamplerBatcher struct {
	cfg         *SamplerConfig
	enforcement *EnforcementConfig
	client      SamplerClient
	metrics     SamplerMetrics
	log         *logger.Handler

	// Channels
	inputCh            <-chan *PipelineRecord
	outputKeptCh       chan<- *PipelineRecord
	outputSuppressedCh chan<- *PipelineRecord

	// Control
	stopCh chan struct{}
}

// NewSamplerBatcher creates a new sampler batcher
func NewSamplerBatcher(
	cfg *SamplerConfig,
	enforcement *EnforcementConfig,
	client SamplerClient,
	metrics SamplerMetrics,
	log *logger.Handler,
	inputCh <-chan *PipelineRecord,
	outputKeptCh chan<- *PipelineRecord,
	outputSuppressedCh chan<- *PipelineRecord,
) *SamplerBatcher {
	return &SamplerBatcher{
		cfg:                cfg,
		enforcement:        enforcement,
		client:             client,
		metrics:            metrics,
		log:                log,
		inputCh:            inputCh,
		outputKeptCh:       outputKeptCh,
		outputSuppressedCh: outputSuppressedCh,
		stopCh:             make(chan struct{}),
	}
}

// Start starts the sampler batcher
func (b *SamplerBatcher) Start() {
	go b.run()
}

// Stop stops the sampler batcher
func (b *SamplerBatcher) Stop() {
	close(b.stopCh)
}

// run is the main processing loop for the batcher
func (b *SamplerBatcher) run() {
	buf := make([]*PipelineRecord, 0, b.cfg.MaxBatch)
	ticker := time.NewTicker(b.cfg.MaxBatchWait)
	defer ticker.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}

		b.metrics.ObserveSamplerBatch(len(buf))

		// Process batch through sampler
		err := b.client.DecideBatch(context.Background(), buf)
		if err != nil {
			b.log.Error().Err(err).Msg("Sampler batch processing failed")
			b.metrics.IncSamplerFallback()
		}

		// Process decisions and apply enforcement
		for _, rec := range buf {
			action := rec.Decision.Action
			reason := rec.Decision.KeepReason

			// Metrics
			actionStr := strings.ToLower(action.String()[7:])  // Remove "ACTION_" prefix
			reasonStr := strings.ToLower(reason.String()[11:]) // Remove "KEEP_REASON_" prefix
			b.metrics.IncSamplerDecisions(actionStr, reasonStr)

			// Check if enforcement applies
			enforce := b.shouldEnforce(rec.Normalized.Labels, rec.Normalized.Schema) // Using Schema as severity

			if enforce {
				b.metrics.IncSamplerEnforced(actionStr)
				if action == samplerv1.Action_ACTION_SUPPRESS {
					// Suppressed path
					b.outputSuppressedCh <- rec
					continue
				}
			} else {
				// Shadow mode - mark as shadow but still route to kept
				rec.Shadow = true
			}

			// Route to kept path
			b.outputKeptCh <- rec
		}

		buf = buf[:0]
	}

	for {
		select {
		case <-b.stopCh:
			flush()
			return
		case rec, ok := <-b.inputCh:
			if !ok {
				flush()
				return
			}
			buf = append(buf, rec)
			if len(buf) >= b.cfg.MaxBatch {
				flush()
				ticker.Reset(b.cfg.MaxBatchWait)
			}
		case <-ticker.C:
			flush()
		}
	}
}

// shouldEnforce determines if enforcement should be applied based on severity and namespace
func (b *SamplerBatcher) shouldEnforce(labels map[string]string, severity string) bool {
	// 1) Per-severity flags
	switch strings.ToLower(severity) {
	case "debug":
		if b.enforcement.Debug {
			return true
		}
	case "info":
		if b.enforcement.Info {
			return true
		}
	case "warn", "warning":
		if b.enforcement.Warn {
			return true
		}
	case "error":
		if b.enforcement.Error {
			return true
		}
	case "fatal":
		return true // Always enforce at highest severity
	}

	// 2) Optional per-namespace override
	if ns, ok := labels["k8s_namespace"]; ok && len(b.enforcement.ByNamespace) > 0 {
		if enforce, exists := b.enforcement.ByNamespace[ns]; exists {
			return enforce
		}
	}

	return false
}

// metricsHandler implements SamplerMetrics interface using the existing metrics handler
type metricsHandler struct {
	*metrics.Handler
}

// NewSamplerMetrics creates a new sampler metrics handler
func NewSamplerMetrics(h *metrics.Handler) SamplerMetrics {
	return &metricsHandler{Handler: h}
}

// ObserveSamplerRPC records sampler RPC latency and status
func (m *metricsHandler) ObserveSamplerRPC(latency time.Duration, status string) {
	success := "true"
	if status != "ok" {
		success = "false"
	}
	m.IngestHandlerLatency.WithLabelValues("sampler", success).Observe(latency.Seconds())
}

// IncSamplerDecisions increments sampler decisions counter
func (m *metricsHandler) IncSamplerDecisions(action string, reason string) {
	// Using existing metrics infrastructure
	m.IncIngestRecordsTotal("sampler_"+action, reason)
}

// IncSamplerEnforced increments sampler enforced counter
func (m *metricsHandler) IncSamplerEnforced(action string) {
	m.IncIngestBatchesTotal("sampler_enforced_" + action)
}

// IncSamplerFallback increments sampler fallback counter
func (m *metricsHandler) IncSamplerFallback() {
	m.IncIngestRejectedTotal("sampler_fallback")
}

// ObserveSamplerBatch records sampler batch size
func (m *metricsHandler) ObserveSamplerBatch(size int) {
	m.IngestHandlerLatency.WithLabelValues("sampler_batch", "true").Observe(float64(size))
}
