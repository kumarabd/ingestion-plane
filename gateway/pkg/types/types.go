package types

import (
	"time"

	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
)

// Provenance represents the source of a template result
type Provenance int32

const (
	ProvenanceUnspecified Provenance = 0
	ProvenanceCache       Provenance = 1 // found in template memory
	ProvenanceHeuristic   Provenance = 2 // grouping-tree / non-LLM heuristic
	ProvenanceLibreLog    Provenance = 3 // LibreLog / LLM-assisted synthesis
	ProvenanceFallback    Provenance = 4 // masked-preview → regex fallback
)

// TemplateResult represents the primary (authoritative) template assignment for a log
// Each input record gets exactly one TemplateResult with the following fields:
type TemplateResult struct {
	RecordIndex    int32      // Index of the input record (0-based)
	TemplateID     string     // Stable identifier for the template (e.g., hash of canonical text)
	Template       string     // Canonical masked template text (post-PII)
	Regex          string     // Regex derived from placeholders; used by planner to build LogQL
	GroupSignature string     // Optional grouping signature / bucket id (token-type sequence, etc.)
	Confidence     float32    // Confidence (0..1) for this assignment
	Provenance     Provenance // Source of this result (cache, heuristic, librelog, fallback)
	FirstSeen      time.Time  // Template lifecycle hints (as known by the miner)
	LastSeen       time.Time  // Template lifecycle hints (as known by the miner)
	Examples       []string   // Small, bounded exemplars for UI/analysis (already redacted)
}

// MinerShadow represents alternate candidates for a given input record
// These are non-authoritative alternatives that were considered but not chosen as primary
type MinerShadow struct {
	RecordIndex int32            // Index of the input record this shadow set refers to
	Candidates  []TemplateResult // Ranked alternate candidates (same structure as primary)
}

// MinerResult represents the complete miner response for a batch
type MinerResult struct {
	Results []TemplateResult // Primary authoritative results (1:1 with input records)
	Shadows []MinerShadow    // Optional alternates for drift analysis and semantic pre-indexing
}

// MinedRecord represents a pipeline item after mining (to feed sampler next)
// Each record contains:
// - The original normalized log
// - One primary TemplateResult (authoritative)
// - Optional shadow candidates (non-authoritative)
type MinedRecord struct {
	Normalized   logtypes.NormalizedLog // Original input log
	Primary      TemplateResult         // Primary authoritative result with all fields
	Shadows      []TemplateResult       // Optional alternate candidates (non-authoritative)
	FromFallback bool                   // true if we didn't call Miner (circuit/open) or timed out
	Note         string                 // debug info
}

// MinedBatch represents a batch wrapper for mining stage
type MinedBatch struct {
	Records []MinedRecord
}
