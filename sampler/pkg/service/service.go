package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/policy"
	"github.com/kumarabd/ingestion-plane/sampler/pkg/redis"
)

// Service implements the Sampler gRPC service
type Service struct {
	samplerv1.UnimplementedSamplerServiceServer
	pol   *policy.Policy
	store *redis.Store
}

// New creates a new sampler service
func New(pol *policy.Policy, store *redis.Store) *Service {
	return &Service{
		pol:   pol,
		store: store,
	}
}

// Decide handles a single decision request
func (s *Service) Decide(ctx context.Context, req *samplerv1.DecisionRequest) (*samplerv1.Decision, error) {
	resp, err := s.DecideBatch(ctx, &samplerv1.DecisionBatchRequest{Items: []*samplerv1.DecisionRequest{req}})
	if err != nil {
		return nil, err
	}
	return resp.Items[0], nil
}

// DecideBatch handles batch decision requests
func (s *Service) DecideBatch(ctx context.Context, req *samplerv1.DecisionBatchRequest) (*samplerv1.DecisionBatchResponse, error) {
	if req == nil || len(req.Items) == 0 {
		log.Printf("DEBUG: Empty decision batch received")
		return &samplerv1.DecisionBatchResponse{Items: nil}, nil
	}

	log.Printf("DEBUG: Processing decision batch with %d items", len(req.Items))
	out := make([]*samplerv1.Decision, 0, len(req.Items))
	now := time.Now().UTC()

	for i, it := range req.Items {
		if it == nil {
			log.Printf("ERROR: nil item at index %d", i)
			return nil, errors.New("nil item")
		}

		tenant := nz(it.TenantId, "default")
		svc := strings.TrimSpace(it.Labels["service"])
		env := strings.TrimSpace(it.Labels["env"])
		sev := it.Severity

		log.Printf("DEBUG: Processing item %d: tenant=%s, template_id=%s, service=%s, env=%s, severity=%s",
			i+1, tenant, it.TemplateId, svc, env, sev.String())

		ts := now
		if it.Timestamp != nil && it.Timestamp.Seconds != 0 {
			ts = time.Unix(it.Timestamp.Seconds, int64(it.Timestamp.Nanos)).UTC()
			log.Printf("DEBUG: Using provided timestamp: %v", ts)
		} else {
			log.Printf("DEBUG: Using current timestamp: %v", ts)
		}

		// Touch Redis counters/state
		log.Printf("DEBUG: Touching Redis state for template %s", it.TemplateId)
		cnt, err := s.store.Touch(ctx, tenant, it.TemplateId, svc, env, sev, ts, s.pol.GetEMAAlpha())
		if err != nil {
			log.Printf("ERROR: Redis touch failed for item %d: %v", i+1, err)
			return nil, fmt.Errorf("redis touch: %w", err)
		}

		log.Printf("DEBUG: Redis state updated - curMin=%d, win10m=%d, win1h=%d, win24h=%d, total=%d, ema=%.2f",
			cnt.CurMin, cnt.Win10m, cnt.Win1h, cnt.Win24h, cnt.Total, cnt.EMA)

		// Evaluate policy ladder
		action, reason, sampleRate, note := s.evaluate(tenant, sev, cnt)

		log.Printf("DEBUG: Policy evaluation result - action=%s, reason=%s, sampleRate=%d, note='%s'",
			action.String(), reason.String(), sampleRate, note)

		out = append(out, &samplerv1.Decision{
			RecordIndex:   it.RecordIndex,
			Action:        action,
			KeepReason:    reason,
			Counters:      &samplerv1.WindowCounts{Count_10M: cnt.Win10m, Count_1H: cnt.Win1h, Count_24H: cnt.Win24h},
			SampleRate:    sampleRate,
			PolicyVersion: s.pol.Version,
			Note:          note,
		})
	}

	log.Printf("DEBUG: Decision batch processing complete: %d decisions generated", len(out))
	return &samplerv1.DecisionBatchResponse{Items: out}, nil
}

// evaluate applies the sampling policy ladder
func (s *Service) evaluate(tenant string, sev samplerv1.Severity, c redis.Counts) (samplerv1.Action, samplerv1.KeepReason, uint32, string) {
	log.Printf("DEBUG: Evaluating policy for severity=%s, total=%d, curMin=%d, ema=%.2f",
		sev.String(), c.Total, c.CurMin, c.EMA)

	// 1) Severity floor
	if s.pol.SeverityKeep[sev] {
		log.Printf("DEBUG: Severity floor matched - keeping due to severity")
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_SEVERITY, 1, ""
	}

	// 2) Novelty (first seen or unseen beyond window)
	if s.pol.NoveltyWindow > 0 {
		// first occurrence
		if c.Total <= 1 {
			log.Printf("DEBUG: Novelty window matched - keeping due to first occurrence")
			return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_NOVEL, 1, "first-occurrence"
		}
	}

	// 3) Spike relaxation
	if c.EMA > 0 && float64(c.CurMin) >= s.pol.SpikeFactor*c.EMA && c.CurMin >= s.pol.MinSpikePerMin {
		log.Printf("DEBUG: Spike detected - curMin=%d, ema=%.2f, factor=%.2f, minSpike=%d",
			c.CurMin, c.EMA, s.pol.SpikeFactor, s.pol.MinSpikePerMin)
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_SPIKE, 1, fmt.Sprintf("cur=%d ema=%.2f", c.CurMin, c.EMA)
	}

	// 4) Warmup
	if c.Total <= s.pol.WarmupN {
		log.Printf("DEBUG: Warmup phase - total=%d/%d", c.Total, s.pol.WarmupN)
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_WARMUP, 1, fmt.Sprintf("n=%d/%d", c.Total, s.pol.WarmupN)
	}

	// 5) Log2 milestones
	if s.pol.Log2Enabled && policy.IsPowerOfTwo(c.Total) {
		log.Printf("DEBUG: Log2 milestone - total=%d is power of 2", c.Total)
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_LOG2, 1, fmt.Sprintf("n=%d", c.Total)
	}

	// 6) Budget (simple per-key budget if enabled)
	if s.pol.Budget10mDefault > 0 && c.Win10m > s.pol.Budget10mDefault &&
		(sev == samplerv1.Severity_SEVERITY_DEBUG || sev == samplerv1.Severity_SEVERITY_INFO) {
		k := s.pol.GetSteadyK(sev)
		log.Printf("DEBUG: Budget check - win10m=%d > default=%d, k=%d", c.Win10m, s.pol.Budget10mDefault, k)
		if c.Total%k == 0 {
			log.Printf("DEBUG: Budget exceeded but keep due to steadyK milestone")
			return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_BUDGET, uint32(k), fmt.Sprintf("budget>10m: %d>%d", c.Win10m, s.pol.Budget10mDefault)
		}
		log.Printf("DEBUG: Budget exceeded - suppressing")
		return samplerv1.Action_ACTION_SUPPRESS, samplerv1.KeepReason_KEEP_REASON_BUDGET, uint32(k), fmt.Sprintf("budget>10m: %d>%d", c.Win10m, s.pol.Budget10mDefault)
	}

	// 7) SteadyK floor
	k := s.pol.GetSteadyK(sev)
	log.Printf("DEBUG: SteadyK check - total=%d, k=%d, total%%k=%d", c.Total, k, c.Total%k)
	if c.Total%k == 0 {
		log.Printf("DEBUG: SteadyK milestone - keeping")
		return samplerv1.Action_ACTION_KEEP, samplerv1.KeepReason_KEEP_REASON_STEADYK, uint32(k), ""
	}
	log.Printf("DEBUG: SteadyK - suppressing")
	return samplerv1.Action_ACTION_SUPPRESS, samplerv1.KeepReason_KEEP_REASON_STEADYK, uint32(k), ""
}

func nz(s, d string) string {
	if strings.TrimSpace(s) == "" {
		return d
	}
	return s
}
