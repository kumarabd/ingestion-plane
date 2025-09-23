package indexfeed

import (
	"context"
	"crypto/sha1"
	"fmt"
	"math/rand"
	"time"

	"github.com/kumarabd/gokit/logger"
	indexcandidatev1 "github.com/kumarabd/ingestion-plane/contracts/indexfeed/v1"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Client interface for publishing template candidates to the indexfeed service
type Client interface {
	ProcessMinedRecord(ctx context.Context, minedRecord types.MinedRecord) error
	Close(ctx context.Context) error
}

type grpcClient struct {
	log        *logger.Handler
	cli        indexcandidatev1.CandidateIngestClient
	timeout    time.Duration
	maxRetries int
	baseDelay  time.Duration
}

// NewClient creates a new indexfeed client with gRPC connection
func NewClient(cfg *Config, log *logger.Handler) (Client, error) {
	// Create gRPC connection
	conn, err := grpc.NewClient(cfg.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to indexfeed at %s: %w", cfg.Addr, err)
	}

	// Create gRPC client
	cli := indexcandidatev1.NewCandidateIngestClient(conn)

	return &grpcClient{
		log:        log,
		cli:        cli,
		timeout:    cfg.Timeout,
		maxRetries: cfg.MaxRetries,
		baseDelay:  cfg.RetryBaseDelay,
	}, nil
}

// ProcessMinedRecord processes a mined record and submits it to the indexfeed service
func (c *grpcClient) ProcessMinedRecord(ctx context.Context, minedRecord types.MinedRecord) error {
	if c.cli == nil {
		c.log.Debug().Str("template_id", minedRecord.Primary.TemplateID).Msg("IndexFeed client not available, skipping submission")
		return nil
	}

	// Submit the mined record to the indexfeed service
	err := c.PublishCandidate(ctx, minedRecord)
	if err != nil {
		c.log.Error().Err(err).Str("template_id", minedRecord.Primary.TemplateID).Msg("Failed to publish candidate to indexfeed")
		return err
	}

	c.log.Debug().Str("template_id", minedRecord.Primary.TemplateID).Msg("Successfully published candidate to indexfeed")
	return nil
}

// Close gracefully shuts down the processor
func (c *grpcClient) Close(ctx context.Context) error {
	// No resources to close in the simplified processor
	return nil
}

// PublishCandidate publishes a mined record as a template candidate
func (c *grpcClient) PublishCandidate(ctx context.Context, minedRecord types.MinedRecord) error {
	// Extract tenant from labels (or use default)
	tenant := minedRecord.Normalized.Labels["tenant"]
	if tenant == "" {
		tenant = "default"
	}

	// Convert MinedRecord to TemplateCandidate
	candidate := convertMinedRecordToCandidate(minedRecord, tenant)

	// Create batch with single candidate
	batch := &indexcandidatev1.TemplateCandidateBatch{
		Items: []*indexcandidatev1.TemplateCandidate{candidate},
	}

	var lastErr error
	var ack *indexcandidatev1.PublishAck

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		// per-attempt timeout
		tctx, cancel := context.WithTimeout(ctx, c.timeout)
		ack, lastErr = c.cli.Publish(tctx, batch)
		cancel()

		if lastErr == nil {
			// Success - log the result
			if ack.Rejected > 0 {
				return fmt.Errorf("indexfeed rejected %d candidates: %s", ack.Rejected, ack.Note)
			}
			return nil
		}

		// jittered backoff
		if attempt < c.maxRetries {
			d := c.baseDelay + time.Duration(rand.Intn(25))*time.Millisecond
			timer := time.NewTimer(d)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}

	return lastErr
}

// convertMinedRecordToCandidate converts a MinedRecord to TemplateCandidate
func convertMinedRecordToCandidate(minedRecord types.MinedRecord, tenant string) *indexcandidatev1.TemplateCandidate {
	primary := minedRecord.Primary

	// Convert exemplars
	exemplars := make([]*indexcandidatev1.Exemplar, 0, len(primary.Examples))
	for _, example := range primary.Examples {
		exemplars = append(exemplars, &indexcandidatev1.Exemplar{
			Message: example,
			// Note: other fields like ts, namespace, pod, stream are not available
		})
	}

	// Convert provenance
	var provenance indexcandidatev1.Provenance
	switch primary.Provenance {
	case types.ProvenanceCache:
		provenance = indexcandidatev1.Provenance_PROVENANCE_CACHE
	case types.ProvenanceHeuristic:
		provenance = indexcandidatev1.Provenance_PROVENANCE_HEURISTIC
	case types.ProvenanceLibreLog:
		provenance = indexcandidatev1.Provenance_PROVENANCE_LIBRELOG
	case types.ProvenanceFallback:
		provenance = indexcandidatev1.Provenance_PROVENANCE_FALLBACK
	default:
		provenance = indexcandidatev1.Provenance_PROVENANCE_UNSPECIFIED
	}

	// Generate template version (hash of template + regex)
	templateVersion := generateTemplateVersion(primary.Template, primary.Regex)

	// Create labels map with tenant
	labels := make(map[string]string)
	for k, v := range minedRecord.Normalized.Labels {
		labels[k] = v
	}
	labels["tenant"] = tenant

	return &indexcandidatev1.TemplateCandidate{
		Tenant:          tenant,
		TemplateId:      primary.TemplateID,
		TemplateText:    primary.Template,
		Regex:           primary.Regex,
		Labels:          labels,
		Stats:           nil, // RollingStats not available in MinedRecord
		FirstSeen:       timestamppb.New(primary.FirstSeen),
		LastSeen:        timestamppb.New(primary.LastSeen),
		Provenance:      provenance,
		TemplateVersion: templateVersion,
		Exemplars:       exemplars,
		OccurredAt:      timestamppb.New(time.Now()),
		SourceService:   "gateway",
		// ProducerInstance and SeqNoHint are optional and can be left empty
	}
}

// generateTemplateVersion creates a simple hash-based version
func generateTemplateVersion(template, regex string) string {
	// Simple hash-based versioning
	h := sha1.New()
	h.Write([]byte(template + "\n" + regex))
	return fmt.Sprintf("%x", h.Sum(nil))[:16] // Use first 16 chars
}
