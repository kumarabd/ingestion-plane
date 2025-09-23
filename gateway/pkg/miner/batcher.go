package miner

import (
	"context"
	"time"

	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/types"
)

// Metrics interface for miner-specific metrics
type Metrics interface {
	ObserveMinerRPCLatency(duration time.Duration, success bool)
	IncMinerRPCFailures()
	IncMinerBatchFlush(reason string)
	IncMinerFallback()
	SetMinerInputQueueDepth(depth int)
	SetMinerOutputQueueDepth(depth int)
}

// Batcher handles batching normalized logs for miner processing
type Batcher struct {
	maxBatch int
	maxWait  time.Duration
	inputCh  <-chan logtypes.NormalizedLog // comes from preproc stage
	outputCh chan<- types.MinedRecord      // goes to sampler stage
	mc       Client
	metrics  Metrics
	log      *logger.Handler
	stopCh   chan struct{}
}

// NewBatcher creates a new miner batcher
func NewBatcher(
	maxBatch int,
	maxWait time.Duration,
	inputCh <-chan logtypes.NormalizedLog,
	outputCh chan<- types.MinedRecord,
	mc Client,
	metrics Metrics,
	log *logger.Handler,
) *Batcher {
	return &Batcher{
		maxBatch: maxBatch,
		maxWait:  maxWait,
		inputCh:  inputCh,
		outputCh: outputCh,
		mc:       mc,
		metrics:  metrics,
		log:      log,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the miner batcher
func (b *Batcher) Start() {
	go b.run()
}

// Stop stops the miner batcher
func (b *Batcher) Stop() {
	close(b.stopCh)
}

// run is the main processing loop for the batcher
func (b *Batcher) run() {
	buf := make([]logtypes.NormalizedLog, 0, b.maxBatch)
	timer := time.NewTimer(b.maxWait)
	defer timer.Stop()

	flush := func() {
		if len(buf) == 0 {
			return
		}

		start := time.Now()
		result, err := b.mc.AnalyzeBatch(context.Background(), buf)
		duration := time.Since(start)

		// metrics
		if b.metrics != nil {
			ok := (err == nil) && (result != nil) && (len(result.Results) == len(buf))
			b.metrics.ObserveMinerRPCLatency(duration, ok)
			if err != nil {
				b.metrics.IncMinerRPCFailures()
			}
		}

		if err != nil || result == nil {
			// fallback: attach FromFallback=true with a dummy template
			if b.metrics != nil {
				b.metrics.IncMinerFallback()
			}

			for i := range buf {
				b.outputCh <- types.MinedRecord{
					Normalized: buf[i],
					Primary: types.TemplateResult{
						RecordIndex:    int32(i),
						TemplateID:     "unknown",
						Template:       "fallback_template",
						Regex:          ".*",
						GroupSignature: "",
						Confidence:     0.0,
						Provenance:     types.ProvenanceFallback,
						FirstSeen:      time.Now(),
						LastSeen:       time.Now(),
						Examples:       []string{},
					},
					Shadows:      []types.TemplateResult{},
					FromFallback: true,
					Note:         firstErr(err),
				}
			}
		} else {
			// Validate that we have results for all input records
			expectedCount := len(buf)
			actualCount := len(result.Results)

			if actualCount != expectedCount {
				b.log.Warn().
					Int("expected_results", expectedCount).
					Int("actual_results", actualCount).
					Msg("Miner returned different number of results than input records")
			}

			// Create maps by record index for proper ordering
			primaryMap := make(map[int32]types.TemplateResult)
			shadowMap := make(map[int32][]types.TemplateResult)

			// Map primary results by record_index
			for _, primary := range result.Results {
				// Validate record_index is within expected range
				if primary.RecordIndex < 0 || primary.RecordIndex >= int32(expectedCount) {
					b.log.Warn().
						Int32("record_index", primary.RecordIndex).
						Int("expected_range", expectedCount).
						Msg("Miner returned invalid record_index")
					continue
				}
				primaryMap[primary.RecordIndex] = primary
			}

			// Map shadows by record_index
			for _, shadow := range result.Shadows {
				// Validate record_index is within expected range
				if shadow.RecordIndex < 0 || shadow.RecordIndex >= int32(expectedCount) {
					b.log.Warn().
						Int32("record_index", shadow.RecordIndex).
						Int("expected_range", expectedCount).
						Msg("Miner returned invalid shadow record_index")
					continue
				}
				shadowMap[shadow.RecordIndex] = shadow.Candidates
			}

			// Process each input record in order, using record_index to find corresponding results
			// Each input record gets:
			// - One primary TemplateResult (authoritative) with all fields:
			//   * record_index, template_id, template, regex
			//   * group_signature, confidence, provenance
			//   * first_seen, last_seen, examples
			// - Optional shadows[] with alternate candidates (non-authoritative)
			for i := range buf {
				recordIndex := int32(i)
				primary, exists := primaryMap[recordIndex]
				if !exists {
					// Fallback if miner didn't return a result for this record
					b.log.Debug().
						Int32("record_index", recordIndex).
						Msg("No miner result found for record, using fallback")

					primary = types.TemplateResult{
						RecordIndex:    recordIndex,
						TemplateID:     "missing",
						Template:       "missing_template",
						Regex:          ".*",
						GroupSignature: "",
						Confidence:     0.0,
						Provenance:     types.ProvenanceFallback,
						FirstSeen:      time.Now(),
						LastSeen:       time.Now(),
						Examples:       []string{},
					}
				}

				shadows := shadowMap[recordIndex]
				if shadows == nil {
					shadows = []types.TemplateResult{}
				}

				b.outputCh <- types.MinedRecord{
					Normalized:   buf[i],
					Primary:      primary,
					Shadows:      shadows,
					FromFallback: !exists, // Mark as fallback if miner didn't return a result
				}
			}
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-b.stopCh:
			flush()
			return
		case nl, ok := <-b.inputCh:
			if !ok {
				flush()
				return
			}
			buf = append(buf, nl)
			if len(buf) >= b.maxBatch {
				if b.metrics != nil {
					b.metrics.IncMinerBatchFlush("full")
				}
				flush()
				if !timer.Stop() {
					<-timer.C
				}
				timer.Reset(b.maxWait)
			}
		case <-timer.C:
			if b.metrics != nil {
				b.metrics.IncMinerBatchFlush("timer")
			}
			flush()
			timer.Reset(b.maxWait)
		}
	}
}

// firstErr returns the first error message or empty string
func firstErr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// metricsHandler implements Metrics interface using the existing metrics handler
type metricsHandler struct {
	*metrics.Handler
}

// NewMetrics creates a new miner metrics handler
func NewMetrics(h *metrics.Handler) Metrics {
	return &metricsHandler{Handler: h}
}

// ObserveMinerRPCLatency records miner RPC latency
func (m *metricsHandler) ObserveMinerRPCLatency(duration time.Duration, success bool) {
	// Use existing histogram with miner-specific labels
	successStr := "true"
	if !success {
		successStr = "false"
	}
	m.IngestHandlerLatency.WithLabelValues("miner", successStr).Observe(duration.Seconds())
}

// IncMinerRPCFailures increments miner RPC failures counter
func (m *metricsHandler) IncMinerRPCFailures() {
	m.IncIngestRejectedTotal("miner_rpc_failure")
}

// IncMinerBatchFlush increments miner batch flush counter
func (m *metricsHandler) IncMinerBatchFlush(reason string) {
	m.IncIngestBatchesTotal("miner_flush_" + reason)
}

// IncMinerFallback increments miner fallback counter
func (m *metricsHandler) IncMinerFallback() {
	m.IncIngestRejectedTotal("miner_fallback")
}

// SetMinerInputQueueDepth sets the miner input queue depth
func (m *metricsHandler) SetMinerInputQueueDepth(depth int) {
	// This would need a gauge metric - for now just log it
	// In a real implementation, you'd add a gauge to the metrics handler
}

// SetMinerOutputQueueDepth sets the miner output queue depth
func (m *metricsHandler) SetMinerOutputQueueDepth(depth int) {
	// This would need a gauge metric - for now just log it
	// In a real implementation, you'd add a gauge to the metrics handler
}
