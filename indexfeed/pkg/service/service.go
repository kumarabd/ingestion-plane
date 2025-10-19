package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	indexfeedv1 "github.com/kumarabd/ingestion-plane/contracts/indexfeed/v1"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/database"
	"github.com/kumarabd/ingestion-plane/indexfeed/pkg/vector"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Service implements the IndexFeed gRPC service
type Service struct {
	indexfeedv1.UnimplementedCandidateIngestServer
	cfg       *Config
	vectorDim int
	db        *database.Client
	vec       *vector.Upserter
	nowFn     func() time.Time
}

// New creates a new IndexFeed service
func New(cfg *Config, vectorDim int, db *database.Client, vec *vector.Upserter) *Service {
	return &Service{
		cfg:       cfg,
		vectorDim: vectorDim,
		db:        db,
		vec:       vec,
		nowFn:     time.Now,
	}
}

// Publish handles the publishing of template candidates
func (s *Service) Publish(ctx context.Context, req *indexfeedv1.TemplateCandidateBatch) (*indexfeedv1.PublishAck, error) {
	if req == nil || len(req.Items) == 0 {
		log.Printf("DEBUG: Empty batch received")
		return &indexfeedv1.PublishAck{Accepted: 0, Rejected: 0, Note: "empty batch"}, nil
	}

	log.Printf("DEBUG: Processing batch with %d candidates", len(req.Items))
	var accepted, rejected uint32
	var firstErr error

	for i, c := range req.Items {
		log.Printf("DEBUG: Processing candidate %d/%d: tenant=%s, template_id=%s",
			i+1, len(req.Items), c.Tenant, c.TemplateId)

		if err := s.handleCandidate(ctx, c); err != nil {
			rejected++
			log.Printf("ERROR: Candidate %d rejected: %v", i+1, err)
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		accepted++
		log.Printf("DEBUG: Candidate %d accepted successfully", i+1)
	}

	note := ""
	if firstErr != nil {
		note = firstErr.Error()
	}

	log.Printf("DEBUG: Batch processing complete: %d accepted, %d rejected", accepted, rejected)
	return &indexfeedv1.PublishAck{
		Accepted: accepted,
		Rejected: rejected,
		Note:     note,
	}, nil
}

// handleCandidate processes a single template candidate
func (s *Service) handleCandidate(ctx context.Context, c *indexfeedv1.TemplateCandidate) error {
	// 1) Validate required fields
	if c == nil {
		log.Printf("ERROR: nil candidate received")
		return errors.New("nil candidate")
	}
	tenant := c.Tenant
	if tenant == "" {
		tenant = "default"
		log.Printf("DEBUG: Empty tenant, using default")
	}

	log.Printf("DEBUG: Validating candidate - template_id='%s', template_text_len=%d, regex='%s'",
		c.TemplateId, len(c.TemplateText), c.Regex)

	if c.TemplateId == "" || c.TemplateText == "" || c.Regex == "" {
		log.Printf("ERROR: Missing required fields - template_id='%s', template_text='%s', regex='%s'",
			c.TemplateId, c.TemplateText, c.Regex)
		return fmt.Errorf("missing required fields (template_id/text/regex)")
	}

	// 2) Sanitize labels (allowlist only)
	allowlist := s.cfg.GetLabelAllowlistMap()
	lbls := map[string]string{}
	for k, v := range c.Labels {
		if _, ok := allowlist[strings.ToLower(k)]; ok {
			lbls[strings.ToLower(k)] = v
		}
	}
	lblJSON, _ := json.Marshal(lbls)
	log.Printf("DEBUG: Sanitized labels: %s", string(lblJSON))

	// 3) Upsert template row
	firstSeen := tsOrNow(c.FirstSeen, s.nowFn)
	lastSeen := tsOrNow(c.LastSeen, s.nowFn)

	log.Printf("DEBUG: Upserting template - tenant=%s, template_id=%s, version=%s, first_seen=%v, last_seen=%v",
		tenant, c.TemplateId, nullIfEmpty(c.TemplateVersion), firstSeen, lastSeen)

	if err := s.db.UpsertTemplate(ctx, tenant, c.TemplateId, c.TemplateText, c.Regex, lbls, firstSeen, lastSeen, nullIfEmpty(c.TemplateVersion)); err != nil {
		log.Printf("ERROR: Failed to upsert template: %v", err)
		return fmt.Errorf("upsert template: %w", err)
	}
	log.Printf("DEBUG: Template upserted successfully")

	// 4) Upsert rolling stats if provided
	now := s.nowFn().UTC()
	if c.Stats != nil {
		log.Printf("DEBUG: Processing stats - 10m=%d, 1h=%d, 24h=%d", c.Stats.Count_10M, c.Stats.Count_1H, c.Stats.Count_24H)
		if c.Stats.Count_10M > 0 {
			if err := s.db.UpsertStat(ctx, tenant, c.TemplateId, "10m", int64(c.Stats.Count_10M), now); err != nil {
				log.Printf("ERROR: Failed to upsert 10m stat: %v", err)
				return fmt.Errorf("upsert stat 10m: %w", err)
			}
		}
		if c.Stats.Count_1H > 0 {
			if err := s.db.UpsertStat(ctx, tenant, c.TemplateId, "1h", int64(c.Stats.Count_1H), now); err != nil {
				log.Printf("ERROR: Failed to upsert 1h stat: %v", err)
				return fmt.Errorf("upsert stat 1h: %w", err)
			}
		}
		if c.Stats.Count_24H > 0 {
			if err := s.db.UpsertStat(ctx, tenant, c.TemplateId, "24h", int64(c.Stats.Count_24H), now); err != nil {
				log.Printf("ERROR: Failed to upsert 24h stat: %v", err)
				return fmt.Errorf("upsert stat 24h: %w", err)
			}
		}
		log.Printf("DEBUG: Stats upserted successfully")
	} else {
		log.Printf("DEBUG: No stats provided for candidate")
	}

	// 5) (Re)embed and upsert into Qdrant
	log.Printf("DEBUG: Generating embedding for template text (len=%d)", len(c.TemplateText))
	vec := vector.EmbedTemplate(c.TemplateText, s.vectorDim)
	pointID := c.TemplateId // Use template ID directly, store tenant in payload
	payload := map[string]interface{}{
		"tenant":           tenant,
		"template_id":      c.TemplateId,
		"template_version": c.TemplateVersion,
		"labels":           lbls,
		"last_seen":        lastSeen.Format(time.RFC3339Nano),
	}

	log.Printf("DEBUG: Upserting to Qdrant - point_id=%s, vector_dim=%d", pointID, len(vec))
	if err := s.vec.UpsertTemplate(ctx, pointID, vec, payload); err != nil {
		log.Printf("ERROR: Failed to upsert to Qdrant: %v", err)
		return fmt.Errorf("qdrant upsert: %w", err)
	}
	log.Printf("DEBUG: Qdrant upsert successful")

	return nil
}

// tsOrNow returns the timestamp or now if nil/zero
func tsOrNow(ts *timestamppb.Timestamp, nowFn func() time.Time) time.Time {
	if ts == nil {
		log.Printf("DEBUG: tsOrNow: nil timestamp, using now")
		return nowFn().UTC()
	}
	// Generated types: google.protobuf.Timestamp → has Seconds/Nanos
	type tsLike interface {
		GetSeconds() int64
		GetNanos() int32
	}
	if t, ok := any(ts).(tsLike); ok {
		sec := t.GetSeconds()
		nano := t.GetNanos()
		if sec == 0 && nano == 0 {
			log.Printf("DEBUG: tsOrNow: zero timestamp, using now")
			return nowFn().UTC()
		}
		result := time.Unix(sec, int64(nano)).UTC()
		log.Printf("DEBUG: tsOrNow: using provided timestamp: %v", result)
		return result
	}
	log.Printf("DEBUG: tsOrNow: invalid timestamp type, using now")
	return nowFn().UTC()
}

// nullIfEmpty returns "v0" if string is empty, otherwise returns the string
func nullIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		log.Printf("DEBUG: nullIfEmpty: empty string, returning 'v0'")
		return "v0"
	}
	log.Printf("DEBUG: nullIfEmpty: returning original string: '%s'", s)
	return s
}
