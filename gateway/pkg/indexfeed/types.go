package indexfeed

import (
	"github.com/kumarabd/ingestion-plane/gateway/pkg/types"
)

// Re-export types from the common types package for backward compatibility
type (
	Provenance     = types.Provenance
	TemplateResult = types.TemplateResult
	MinerShadow    = types.MinerShadow
	MinerResult    = types.MinerResult
	MinedRecord    = types.MinedRecord
	MinedBatch     = types.MinedBatch
)

// Re-export constants for backward compatibility
const (
	ProvenanceUnspecified = types.ProvenanceUnspecified
	ProvenanceCache       = types.ProvenanceCache
	ProvenanceHeuristic   = types.ProvenanceHeuristic
	ProvenanceLibreLog    = types.ProvenanceLibreLog
	ProvenanceFallback    = types.ProvenanceFallback
)
