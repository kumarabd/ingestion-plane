package miner

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	commonv1 "github.com/kumarabd/ingestion-plane/contracts/common/v1"
	ingestv1 "github.com/kumarabd/ingestion-plane/contracts/ingest/v1"
	minerv1 "github.com/kumarabd/ingestion-plane/contracts/miner/v1"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/logtypes"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Client interface for analyzing normalized logs
type Client interface {
	AnalyzeBatch(ctx context.Context, recs []logtypes.NormalizedLog) (*types.MinerResult, error)
}

type grpcClient struct {
	cli        minerv1.MinerServiceClient
	timeout    time.Duration
	maxRetries int
	baseDelay  time.Duration
}

// NewClient creates a new miner client with gRPC connection
func NewClient(cfg *Config) (Client, error) {
	// Create gRPC connection
	conn, err := grpc.NewClient(cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to miner at %s: %w", cfg.Addr, err)
	}

	// Create gRPC client
	cli := minerv1.NewMinerServiceClient(conn)

	return &grpcClient{
		cli:        cli,
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		baseDelay:  cfg.RetryBaseDelay,
	}, nil
}

// AnalyzeBatch analyzes a batch of normalized logs
func (m *grpcClient) AnalyzeBatch(ctx context.Context, recs []logtypes.NormalizedLog) (*types.MinerResult, error) {
	if len(recs) == 0 {
		return &types.MinerResult{Results: []types.TemplateResult{}, Shadows: []types.MinerShadow{}}, nil
	}

	// Build request
	areq := &minerv1.AnalyzeRequest{Records: make([]*ingestv1.NormalizedLog, 0, len(recs))}
	for i := range recs {
		nl := recs[i]
		protoRec := toProtoNormalized(nl)
		areq.Records = append(areq.Records, protoRec)

	}

	var lastErr error
	var aresp *minerv1.AnalyzeResponse

	for attempt := 0; attempt <= m.maxRetries; attempt++ {
		// per-attempt timeout
		tctx, cancel := context.WithTimeout(ctx, m.timeout)
		aresp, lastErr = m.cli.Analyze(tctx, areq)
		cancel()

		if lastErr == nil {
			// translate results and shadows
			return fromProtoAnalyzeResponse(aresp), nil
		}

		// jittered backoff
		if attempt < m.maxRetries {
			d := m.baseDelay + time.Duration(rand.Intn(25))*time.Millisecond
			timer := time.NewTimer(d)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			case <-timer.C:
			}
		}
	}

	return nil, lastErr
}

// toProtoNormalized converts a NormalizedLog to protobuf format
func toProtoNormalized(nl logtypes.NormalizedLog) *ingestv1.NormalizedLog {
	// Map your fields to proto; ensure timestamp set.
	ts := timestamppb.New(nl.Timestamp)
	if nl.Timestamp.IsZero() {
		ts = timestamppb.Now()
	}

	// Convert redaction report
	redaction := &ingestv1.RedactionReport{
		Applied: nl.Redaction.Applied,
		Rules:   nl.Redaction.Rules,
		Count:   uint32(nl.Redaction.Count),
	}

	// Map schema (assuming string to enum conversion)
	var schema commonv1.Schema
	switch nl.Schema {
	case "JSON":
		schema = commonv1.Schema_SCHEMA_JSON
	case "LOGFMT":
		schema = commonv1.Schema_SCHEMA_LOGFMT
	case "TEXT":
		schema = commonv1.Schema_SCHEMA_TEXT
	default:
		schema = commonv1.Schema_SCHEMA_TEXT
	}

	return &ingestv1.NormalizedLog{
		Timestamp: ts,
		Labels:    nl.Labels,
		Message:   nl.Message,
		Schema:    schema,
		Sanitized: nl.Sanitized,
		Truncated: nl.Truncated,
		Redaction: redaction,
		// Note: fields, severity, original_length, correlation_id are not in our current NormalizedLog struct
		// They would need to be added if required by the proto
	}
}

// fromProtoAnalyzeResponse converts protobuf response to MinerResult
func fromProtoAnalyzeResponse(resp *minerv1.AnalyzeResponse) *types.MinerResult {
	// Convert primary results
	results := make([]types.TemplateResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = types.TemplateResult{
			RecordIndex:    r.GetRecordIndex(),
			TemplateID:     r.GetTemplateId(),
			Template:       r.GetTemplate(),
			Regex:          r.GetRegex(),
			GroupSignature: r.GetGroupSignature(),
			Confidence:     r.GetConfidence(),
			Provenance:     types.Provenance(r.GetProvenance()),
			FirstSeen:      r.GetFirstSeen().AsTime(),
			LastSeen:       r.GetLastSeen().AsTime(),
			Examples:       append([]string(nil), r.GetExamples()...),
		}
	}

	// Convert shadows
	shadows := make([]types.MinerShadow, len(resp.Shadows))
	for i, s := range resp.Shadows {
		candidates := make([]types.TemplateResult, len(s.GetCandidates()))
		for j, c := range s.GetCandidates() {
			candidates[j] = types.TemplateResult{
				RecordIndex:    c.GetRecordIndex(),
				TemplateID:     c.GetTemplateId(),
				Template:       c.GetTemplate(),
				Regex:          c.GetRegex(),
				GroupSignature: c.GetGroupSignature(),
				Confidence:     c.GetConfidence(),
				Provenance:     types.Provenance(c.GetProvenance()),
				FirstSeen:      c.GetFirstSeen().AsTime(),
				LastSeen:       c.GetLastSeen().AsTime(),
				Examples:       append([]string(nil), c.GetExamples()...),
			}
		}
		shadows[i] = types.MinerShadow{
			RecordIndex: s.GetRecordIndex(),
			Candidates:  candidates,
		}
	}

	return &types.MinerResult{
		Results: results,
		Shadows: shadows,
	}
}
