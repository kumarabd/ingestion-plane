package sampler

import (
	"time"

	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
)

// PipelineRecord is the working item after mining and before sinks
type PipelineRecord struct {
	Normalized logtypes.NormalizedLog // existing normalized record
	TemplateID string                 // filled by miner
	// These fields will be filled by sampler:
	Decision samplerv1.Decision // sampler response (action/reason/counters)
	// Shadow decision (if needed for debugging):
	Shadow bool
}

// SamplerBatch is a batch container for sampler processing
type SamplerBatch struct {
	Items []*PipelineRecord
}

// DecisionRequest represents a single decision request to the sampler
type DecisionRequest struct {
	RecordIndex int32              // Index of the input record in the caller's batch
	Timestamp   time.Time          // Event time of the log
	Labels      map[string]string  // Key labels used for policy & bucketing
	TemplateID  string             // Template identity produced by the Miner
	Severity    samplerv1.Severity // Normalized severity
	TenantID    string             // Optional multi-tenant routing key
}

// Decision represents a sampler decision response
type Decision struct {
	RecordIndex   int32                   // Echo of the input record_index for alignment
	Action        samplerv1.Action        // Action to take (KEEP/SUPPRESS)
	KeepReason    samplerv1.KeepReason    // Why it was kept (or which rule would have applied)
	Counters      *samplerv1.WindowCounts // Rolling counts for this template/service/env/severity
	SampleRate    uint32                  // Effective sampling rate (1 = keep-all; 10 = keep 1 in 10)
	PolicyVersion string                  // Policy/version identifier that produced the decision
	Note          string                  // Optional human-readable note for debugging
}
