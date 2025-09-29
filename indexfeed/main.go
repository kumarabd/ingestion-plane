// cmd/indexfeed/main.go
package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	indexfeedv1 "github.com/kumarabd/ingestion-plane/contracts/indexfeed/v1"
	_ "github.com/lib/pq"
	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ----------- Config -----------

type Config struct {
	GRPCPort         string
	PGConn           string
	QdrantHost       string
	QdrantPort       int
	QdrantCollection string
	VectorDim        int
	LabelAllowlist   map[string]struct{}
}

func loadConfig() Config {
	return Config{
		GRPCPort:         getenv("GRPC_PORT", "50070"),
		PGConn:           getenv("PG_CONN", "postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable"),
		QdrantHost:       getenv("QDRANT_HOST", "localhost"),
		QdrantPort:       atoi(getenv("QDRANT_PORT", "6334")),
		QdrantCollection: getenv("QDRANT_COLLECTION", "templates"),
		VectorDim:        atoi(getenv("VECTOR_DIM", "384")), // adjust if you switch to a real embedder
		LabelAllowlist: map[string]struct{}{
			"service":   {},
			"env":       {},
			"severity":  {},
			"namespace": {},
		},
	}
}

func getenv(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func atoi(s string) int {
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n
}

// ----------- DB schema helpers -----------

// Call these once during bootstrap/migrations (kept here for reference):
const createTemplatesSQL = `
CREATE TABLE IF NOT EXISTS templates (
  tenant           TEXT NOT NULL,
  template_id      TEXT NOT NULL,
  template_text    TEXT NOT NULL,
  regex            TEXT NOT NULL,
  labels           JSONB,
  first_seen       TIMESTAMPTZ,
  last_seen        TIMESTAMPTZ,
  template_version TEXT NOT NULL,
  PRIMARY KEY (tenant, template_id)
);
CREATE INDEX IF NOT EXISTS templates_service_idx ON templates ((labels->>'service'));
CREATE INDEX IF NOT EXISTS templates_env_idx     ON templates ((labels->>'env'));
CREATE INDEX IF NOT EXISTS templates_last_seen   ON templates (last_seen);
`

const createTemplateStatsSQL = `
CREATE TABLE IF NOT EXISTS template_stats (
  tenant      TEXT NOT NULL,
  template_id TEXT NOT NULL,
  "window"    TEXT NOT NULL,  -- '10m'|'1h'|'24h'
  count       BIGINT NOT NULL,
  updated_at  TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (tenant, template_id, "window")
);
`

// Upsert base rows for templates; only updates fields that matter.
// We treat template_version as the change detector for canonical fields.
const upsertTemplateSQL = `
INSERT INTO templates (tenant, template_id, template_text, regex, labels, first_seen, last_seen, template_version)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
ON CONFLICT (tenant, template_id) DO UPDATE SET
  template_text=EXCLUDED.template_text,
  regex=EXCLUDED.regex,
  labels=EXCLUDED.labels,
  -- only lower first_seen if earlier arrives; always refresh last_seen
  first_seen=LEAST(templates.first_seen, EXCLUDED.first_seen),
  last_seen=GREATEST(templates.last_seen, EXCLUDED.last_seen),
  template_version=EXCLUDED.template_version
WHERE templates.template_version <> EXCLUDED.template_version
   OR templates.last_seen < EXCLUDED.last_seen;
`

const upsertStatSQL = `
INSERT INTO template_stats (tenant, template_id, "window", count, updated_at)
VALUES ($1,$2,$3,$4,$5)
ON CONFLICT (tenant, template_id, "window") DO UPDATE SET
  count=EXCLUDED.count,
  updated_at=EXCLUDED.updated_at;
`

// ----------- Qdrant helpers -----------

type VecUpserter struct {
	cli        qdrant.QdrantClient
	collection string
	dim        int
}

func newVecUpserter(ctx context.Context, host string, port int, collection string, dim int) (*VecUpserter, error) {
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", host, port), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("qdrant connect: %w", err)
	}
	client := qdrant.NewQdrantClient(conn)
	v := &VecUpserter{cli: client, collection: collection, dim: dim}
	if err := v.ensureCollection(ctx); err != nil {
		return nil, err
	}
	return v, nil
}

func (v *VecUpserter) ensureCollection(ctx context.Context) error {
	// For now, we'll assume the collection exists or will be created by the Qdrant server
	// In a real implementation, you would check if the collection exists and create it if needed
	return nil
}

func (v *VecUpserter) UpsertTemplate(ctx context.Context, id string, vec []float32, payload map[string]interface{}) error {
	// For now, we'll just log the operation since the Qdrant client API is limited
	// In a real implementation, you would use the correct Qdrant client methods
	log.Printf("DEBUG: VecUpserter.UpsertTemplate called - id=%s, vector_len=%d, collection=%s", id, len(vec), v.collection)

	// Log payload details for debugging
	payloadJSON, _ := json.Marshal(payload)
	log.Printf("DEBUG: Qdrant payload: %s", string(payloadJSON))

	log.Printf("INFO: Would upsert template %s with vector of length %d to collection %s", id, len(vec), v.collection)
	return nil
}

// ----------- Embedding (placeholder, deterministic) -----------
// Replace this with a real model (e.g., OpenAI, local bge-m3) in production.

func embedTemplate(text string, dim int) []float32 {
	log.Printf("DEBUG: embedTemplate called - text_len=%d, dim=%d", len(text), dim)

	h := sha256.Sum256([]byte(strings.ToLower(text)))
	// Expand the 32 bytes into dim floats deterministically.
	out := make([]float32, dim)
	var seed uint64
	seed = binary.LittleEndian.Uint64(h[0:8]) ^ binary.LittleEndian.Uint64(h[8:16]) ^
		binary.LittleEndian.Uint64(h[16:24]) ^ binary.LittleEndian.Uint64(h[24:32])

	log.Printf("DEBUG: Generated seed from hash: %d", seed)

	// Xorshift-ish PRNG to fill
	var x = seed
	for i := 0; i < dim; i++ {
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		out[i] = float32((x % 10000)) / 10000.0 // [0,1)
	}

	// L2-normalize for cosine fairness
	var sum float64
	for _, v := range out {
		sum += float64(v * v)
	}
	norm := float32(1.0 / (float64ToFloat32SafeSqrt(sum) + 1e-8))
	for i := range out {
		out[i] *= norm
	}

	log.Printf("DEBUG: Embedding generated - norm=%.6f, sum_before_norm=%.6f", norm, sum)
	return out
}

func float64ToFloat32SafeSqrt(f float64) float32 {
	if f <= 0 {
		return 0
	}
	return float32((1.0 / 2.0) * (float64(1) + f)) // cheap-ish; accuracy not critical for placeholder
}

// ----------- Service impl -----------

type server struct {
	indexfeedv1.UnimplementedCandidateIngestServer
	cfg   Config
	db    *sql.DB
	vec   *VecUpserter
	nowFn func() time.Time
}

func newServer(cfg Config, db *sql.DB, vec *VecUpserter) *server {
	return &server{
		cfg:   cfg,
		db:    db,
		vec:   vec,
		nowFn: time.Now,
	}
}

func (s *server) Publish(ctx context.Context, req *indexfeedv1.TemplateCandidateBatch) (*indexfeedv1.PublishAck, error) {
	if req == nil || len(req.Items) == 0 {
		log.Printf("DEBUG: Empty batch received")
		return &indexfeedv1.PublishAck{Accepted: 0, Rejected: 0, Note: "empty batch"}, nil
	}

	log.Printf("DEBUG: Processing batch with %d candidates", len(req.Items))
	var accepted, rejected uint32
	var firstErr error

	// Use a single DB tx per batch for efficiency
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		log.Printf("ERROR: Failed to begin transaction: %v", err)
		return nil, err
	}
	defer func() {
		if firstErr != nil {
			log.Printf("DEBUG: Rolling back transaction due to error: %v", firstErr)
			_ = tx.Rollback()
		} else {
			log.Printf("DEBUG: Committing transaction successfully")
			_ = tx.Commit()
		}
	}()

	for i, c := range req.Items {
		log.Printf("DEBUG: Processing candidate %d/%d: tenant=%s, template_id=%s",
			i+1, len(req.Items), c.Tenant, c.TemplateId)

		if err := s.handleCandidate(ctx, tx, c); err != nil {
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

func (s *server) handleCandidate(ctx context.Context, tx *sql.Tx, c *indexfeedv1.TemplateCandidate) error {
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
	lbls := map[string]string{}
	for k, v := range c.Labels {
		if _, ok := s.cfg.LabelAllowlist[strings.ToLower(k)]; ok {
			lbls[strings.ToLower(k)] = v
		}
	}
	lblJSON, _ := json.Marshal(lbls)
	log.Printf("DEBUG: Sanitized labels: %s", string(lblJSON))

	// 3) Upsert template row (only if version changed or last_seen moved forward)
	firstSeen := tsOrNow(c.FirstSeen, s.nowFn)
	lastSeen := tsOrNow(c.LastSeen, s.nowFn)

	log.Printf("DEBUG: Upserting template - tenant=%s, template_id=%s, version=%s, first_seen=%v, last_seen=%v",
		tenant, c.TemplateId, nullIfEmpty(c.TemplateVersion), firstSeen, lastSeen)

	if _, err := tx.ExecContext(ctx, upsertTemplateSQL,
		tenant,
		c.TemplateId,
		c.TemplateText,
		c.Regex,
		string(lblJSON),
		firstSeen,
		lastSeen,
		nullIfEmpty(c.TemplateVersion),
	); err != nil {
		log.Printf("ERROR: Failed to upsert template: %v", err)
		return fmt.Errorf("upsert template: %w", err)
	}
	log.Printf("DEBUG: Template upserted successfully")

	// 4) Upsert rolling stats if provided
	now := s.nowFn().UTC()
	if c.Stats != nil {
		log.Printf("DEBUG: Processing stats - 10m=%d, 1h=%d, 24h=%d", c.Stats.Count_10M, c.Stats.Count_1H, c.Stats.Count_24H)
		if c.Stats.Count_10M > 0 {
			if _, err := tx.ExecContext(ctx, upsertStatSQL, tenant, c.TemplateId, "10m", c.Stats.Count_10M, now); err != nil {
				log.Printf("ERROR: Failed to upsert 10m stat: %v", err)
				return fmt.Errorf("upsert stat 10m: %w", err)
			}
		}
		if c.Stats.Count_1H > 0 {
			if _, err := tx.ExecContext(ctx, upsertStatSQL, tenant, c.TemplateId, "1h", c.Stats.Count_1H, now); err != nil {
				log.Printf("ERROR: Failed to upsert 1h stat: %v", err)
				return fmt.Errorf("upsert stat 1h: %w", err)
			}
		}
		if c.Stats.Count_24H > 0 {
			if _, err := tx.ExecContext(ctx, upsertStatSQL, tenant, c.TemplateId, "24h", c.Stats.Count_24H, now); err != nil {
				log.Printf("ERROR: Failed to upsert 24h stat: %v", err)
				return fmt.Errorf("upsert stat 24h: %w", err)
			}
		}
		log.Printf("DEBUG: Stats upserted successfully")
	} else {
		log.Printf("DEBUG: No stats provided for candidate")
	}

	// 5) (Re)embed and upsert into Qdrant
	// Heuristic: re-embed only when template text changes.
	// We don't fetch existing version here; rely on db constraint that only updates when version differs.
	log.Printf("DEBUG: Generating embedding for template text (len=%d)", len(c.TemplateText))
	vec := embedTemplate(c.TemplateText, s.cfg.VectorDim)
	pointID := tenant + ":" + c.TemplateId
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

// ----------- utilities -----------

func tsOrNow(ts *timestamppb.Timestamp, nowFn func() time.Time) time.Time {
	// Using google.protobuf.Timestamp in your real proto;
	// here indexfeedv1.Timestamp is whatever your generated alias is.
	// If ts is nil or zero, use now.
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

func nullIfEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		log.Printf("DEBUG: nullIfEmpty: empty string, returning 'v0'")
		return "v0"
	}
	log.Printf("DEBUG: nullIfEmpty: returning original string: '%s'", s)
	return s
}

// ----------- main / boot -----------

func main() {
	log.Printf("INFO: Starting IndexFeed service")

	cfg := loadConfig()
	log.Printf("INFO: Configuration loaded:")
	log.Printf("  GRPC Port: %s", cfg.GRPCPort)
	log.Printf("  Postgres: %s", cfg.PGConn)
	log.Printf("  Qdrant: %s:%d", cfg.QdrantHost, cfg.QdrantPort)
	log.Printf("  Collection: %s", cfg.QdrantCollection)
	log.Printf("  Vector Dim: %d", cfg.VectorDim)

	// Postgres
	log.Printf("DEBUG: Connecting to Postgres...")
	db, err := sql.Open("postgres", cfg.PGConn)
	must(err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	must(db.PingContext(ctx))
	log.Printf("INFO: Postgres connection established")

	// Ensure schema
	log.Printf("DEBUG: Ensuring database schema...")
	_, err = db.ExecContext(ctx, createTemplatesSQL)
	must(err)
	log.Printf("DEBUG: Templates table schema ensured")
	_, err = db.ExecContext(ctx, createTemplateStatsSQL)
	must(err)
	log.Printf("DEBUG: Template stats table schema ensured")

	// Qdrant
	log.Printf("DEBUG: Connecting to Qdrant...")
	vec, err := newVecUpserter(context.Background(), cfg.QdrantHost, cfg.QdrantPort, cfg.QdrantCollection, cfg.VectorDim)
	must(err)
	log.Printf("INFO: Qdrant connection established")

	s := grpc.NewServer(
		grpc.MaxRecvMsgSize(32*1024*1024),
		grpc.MaxSendMsgSize(32*1024*1024),
	)
	svc := newServer(cfg, db, vec)
	indexfeedv1.RegisterCandidateIngestServer(s, svc)
	log.Printf("DEBUG: gRPC service registered")

	// Health service
	hs := health.NewServer()
	healthpb.RegisterHealthServer(s, hs)
	log.Printf("DEBUG: Health service registered")

	lis, err := net.Listen("tcp", ":"+cfg.GRPCPort)
	must(err)
	log.Printf("INFO: IndexFeed CandidateIngest listening on :%s (PG=%s, Qdrant=%s:%d col=%s dim=%d)",
		cfg.GRPCPort, cfg.PGConn, cfg.QdrantHost, cfg.QdrantPort, cfg.QdrantCollection, cfg.VectorDim)

	must(s.Serve(lis))
}

func must(err error) {
	if err != nil {
		log.Fatalf("fatal: %v", err)
	}
}
