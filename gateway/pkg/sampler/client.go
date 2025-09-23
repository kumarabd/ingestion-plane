package sampler

import (
	"context"
	"fmt"
	"strings"
	"time"

	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// SamplerClient interface for sampler service
type SamplerClient interface {
	DecideBatch(ctx context.Context, items []*PipelineRecord) error
}

type samplerClient struct {
	cli     samplerv1.SamplerServiceClient
	cfg     *SamplerConfig
	breaker *Breaker
}

// NewSamplerClient creates a new sampler client
func NewSamplerClient(cfg *SamplerConfig) (SamplerClient, error) {
	// Create gRPC connection
	conn, err := grpc.Dial(cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to sampler at %s: %w", cfg.Addr, err)
	}

	// Create gRPC client
	cli := samplerv1.NewSamplerServiceClient(conn)

	return &samplerClient{
		cli:     cli,
		cfg:     cfg,
		breaker: NewBreaker(5, 30*time.Second), // 5 failures, 30s timeout
	}, nil
}

// DecideBatch processes a batch of pipeline records through the sampler
func (sc *samplerClient) DecideBatch(ctx context.Context, items []*PipelineRecord) error {
	if len(items) == 0 {
		return nil
	}

	// Check circuit breaker
	if sc.breaker.Open() {
		// Fallback decisions
		for i, it := range items {
			it.Decision = fallbackDecision(it, int32(i))
		}
		return nil
	}

	// Build request
	req := &samplerv1.DecisionBatchRequest{
		Items: make([]*samplerv1.DecisionRequest, 0, len(items)),
	}

	for i, it := range items {
		// Map severity string to enum
		sev := mapSeverity(it.Normalized.Schema) // Using Schema as severity placeholder
		dr := &samplerv1.DecisionRequest{
			RecordIndex: int32(i), // Index of the input record in the caller's batch
			Timestamp:   timestamppb.New(it.Normalized.Timestamp),
			Labels:      pickPolicyLabels(it.Normalized.Labels),
			TemplateId:  it.TemplateID,
			Severity:    sev,
			TenantId:    extractTenantID(it.Normalized.Labels),
		}
		req.Items = append(req.Items, dr)
	}

	// Call sampler with timeout
	ctx, cancel := context.WithTimeout(ctx, sc.cfg.Timeout)
	defer cancel()

	start := time.Now()
	resp, err := sc.cli.DecideBatch(ctx, req)
	duration := time.Since(start)

	if err != nil || resp == nil || len(resp.Items) != len(items) {
		sc.breaker.Fail()
		// Fallback decisions
		for i, it := range items {
			it.Decision = fallbackDecision(it, int32(i))
		}

		// Log error for debugging
		if err != nil {
			return fmt.Errorf("sampler batch failed: %w", err)
		}

		return fmt.Errorf("sampler batch failed: invalid response")
	}

	sc.breaker.Success()

	// Map results back to items using record_index for proper ordering
	decisionMap := make(map[int32]*samplerv1.Decision)
	for _, d := range resp.Items {
		if d != nil {
			decisionMap[d.RecordIndex] = d
		}
	}

	// Apply decisions in order, handling missing decisions
	for i, it := range items {
		recordIndex := int32(i)
		if decision, exists := decisionMap[recordIndex]; exists {
			it.Decision = samplerv1.Decision{
				RecordIndex:   decision.RecordIndex,
				Action:        decision.Action,
				KeepReason:    decision.KeepReason,
				Counters:      decision.Counters,
				SampleRate:    decision.SampleRate,
				PolicyVersion: decision.PolicyVersion,
				Note:          decision.Note,
			}
		} else {
			// Fallback if no decision found for this record
			it.Decision = fallbackDecision(it, recordIndex)
		}
	}

	// TODO: Add metrics for RPC latency
	_ = duration

	return nil
}

// pickPolicyLabels selects relevant labels for policy decisions
func pickPolicyLabels(all map[string]string) map[string]string {
	sel := make(map[string]string, 5)
	for _, k := range []string{"service", "env", "severity", "k8s_namespace", "pod"} {
		if v, ok := all[k]; ok {
			sel[k] = v
		}
	}
	return sel
}

// mapSeverity maps severity string to protobuf enum
func mapSeverity(sev string) samplerv1.Severity {
	switch strings.ToLower(sev) {
	case "debug":
		return samplerv1.Severity_SEVERITY_DEBUG
	case "info":
		return samplerv1.Severity_SEVERITY_INFO
	case "warn", "warning":
		return samplerv1.Severity_SEVERITY_WARN
	case "error":
		return samplerv1.Severity_SEVERITY_ERROR
	case "fatal":
		return samplerv1.Severity_SEVERITY_FATAL
	default:
		return samplerv1.Severity_SEVERITY_UNSPECIFIED
	}
}

// extractTenantID extracts tenant ID from labels
func extractTenantID(labels map[string]string) string {
	if tenant, ok := labels["tenant"]; ok {
		return tenant
	}
	if tenant, ok := labels["tenant_id"]; ok {
		return tenant
	}
	return "" // Empty if single-tenant
}

// fallbackDecision creates a conservative fallback decision
func fallbackDecision(it *PipelineRecord, recordIndex int32) samplerv1.Decision {
	return samplerv1.Decision{
		RecordIndex:   recordIndex,
		Action:        samplerv1.Action_ACTION_KEEP,
		KeepReason:    samplerv1.KeepReason_KEEP_REASON_WARMUP,
		Counters:      &samplerv1.WindowCounts{}, // zeroed
		SampleRate:    1,                         // Keep all in fallback
		PolicyVersion: "fallback",
		Note:          "fallback decision",
	}
}
