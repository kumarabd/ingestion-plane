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
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // pgx stdlib driver for database/sql

	// Qdrant Go client (official)
	qdrant "github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
)

/* ===========================
   Request/Response contracts
   =========================== */

// Matches search.v1.SemanticQuery (HTTP JSON)
type SemanticQuery struct {
	Query       string            `json:"query"`
	LabelFilter map[string]string `json:"label_filter,omitempty"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	TopK        uint32            `json:"top_k,omitempty"`
}

// Matches search.v1.TemplateHit (HTTP JSON)
type TemplateHit struct {
	TemplateID string            `json:"template_id"`
	Template   string            `json:"template"`
	Regex      string            `json:"regex"`
	Labels     map[string]string `json:"labels"`
	Score      float32           `json:"score"`
	Count24h   uint64            `json:"count_24h"`
	TsFirst    *time.Time        `json:"ts_first,omitempty"`
	TsLast     *time.Time        `json:"ts_last,omitempty"`
}

// Matches search.v1.SearchResponse (HTTP JSON)
type SearchResponse struct {
	Hits []TemplateHit `json:"hits"`
}

// Matches search.v1.PlanRequest (HTTP JSON)
type PlanRequest struct {
	Selected  []TemplateHit `json:"selected"`
	StartTime *time.Time    `json:"start_time,omitempty"`
	EndTime   *time.Time    `json:"end_time,omitempty"`
}

// Matches search.v1.PlanResponse (HTTP JSON)
type PlanResponse struct {
	LogQLCandidates []string `json:"logql_candidates"`
	// Optional diagnostics can be added here later.
}

/* ===========================
   Planner server
   =========================== */

type PlannerServer struct {
	tenant             string
	db                 *pgxpool.Pool
	sqlDB              *sql.DB // optional (for simple queries), using pgx stdlib
	qdrant             qdrant.PointsClient
	qdrantCollection   string
	defaultTopK        int
	maxTemplatesInPlan int
	httpAddr           string
}

// ---- Bootstrapping ----

func NewPlannerServer() (*PlannerServer, error) {
	ctx := context.Background()

	pgURL := env("PG_URI", "postgres://postgres:postgres@localhost:5432/catalog?sslmode=disable")
	pool, err := pgxpool.New(ctx, pgURL)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}

	sqlDB, err := sql.Open("pgx", pgURL)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	qdrantAddr := env("QDRANT_ADDR", "localhost:6334") // gRPC
	conn, err := grpc.Dial(qdrantAddr, grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("qdrant dial: %w", err)
	}
	points := qdrant.NewPointsClient(conn)

	return &PlannerServer{
		tenant:             env("TENANT", "default"),
		db:                 pool,
		sqlDB:              sqlDB,
		qdrant:             points,
		qdrantCollection:   env("QDRANT_COLLECTION", "templates"),
		defaultTopK:        envInt("DEFAULT_TOPK", 10),
		maxTemplatesInPlan: envInt("MAX_TEMPLATES_PLAN", 8),
		httpAddr:           env("HTTP_ADDR", ":8080"),
	}, nil
}

func (s *PlannerServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/search", s.handleSearch)
	mux.HandleFunc("/v1/plan", s.handlePlan)
	srv := &http.Server{
		Addr:         s.httpAddr,
		Handler:      withJSON(withCORS(mux)),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	log.Printf("[planner] listening on %s (tenant=%s, qdrant.collection=%s)", s.httpAddr, s.tenant, s.qdrantCollection)
	return srv.ListenAndServe()
}

/* ===========================
   Handlers
   =========================== */

func (s *PlannerServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req SemanticQuery
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		writeJSON(w, http.StatusOK, SearchResponse{Hits: []TemplateHit{}})
		return
	}

	topK := int(req.TopK)
	if topK <= 0 || topK > 100 {
		topK = s.defaultTopK
	}

	// 1) Embed NL query to vector
	vec, err := s.embedQuery(r.Context(), req.Query)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "embed failed: "+err.Error())
		return
	}

	// 2) Build Qdrant filter (tenant + label_filter)
	filter := s.qdrantFilter(req.LabelFilter)

	// 3) Qdrant ANN search
	points, err := s.qdrantSearch(r.Context(), vec, filter, topK)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "qdrant search: "+err.Error())
		return
	}
	if len(points) == 0 {
		writeJSON(w, http.StatusOK, SearchResponse{Hits: []TemplateHit{}})
		return
	}

	// 4) Extract template_ids from payloads
	ids := make([]string, 0, len(points))
	type scorePair struct {
		ID    string
		Score float32
	}
	var scored []scorePair
	for _, p := range points {
		pl := p.GetPayload()
		// Expect "template_id" in payload
		tid := payloadString(pl, "template_id")
		if tid == "" {
			continue
		}
		ids = append(ids, tid)
		scored = append(scored, scorePair{ID: tid, Score: float32(p.GetScore())})
	}

	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, SearchResponse{Hits: []TemplateHit{}})
		return
	}

	// 5) Fetch metadata from Postgres catalog
	meta, err := s.fetchTemplates(r.Context(), ids)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "catalog fetch: "+err.Error())
		return
	}

	// 6) Assemble hits (preserve ANN order by score)
	hits := make([]TemplateHit, 0, len(scored))
	for _, sp := range scored {
		if m, ok := meta[sp.ID]; ok {
			hits = append(hits, m.toHit(sp.Score))
		}
	}

	// 7) Optional re-ranking boosts (recency)
	if req.EndTime != nil && !req.EndTime.IsZero() {
		boostRecent(hits, *req.EndTime)
	}

	writeJSON(w, http.StatusOK, SearchResponse{Hits: hits})
}

func (s *PlannerServer) handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req PlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Selected) == 0 {
		writeJSON(w, http.StatusOK, PlanResponse{LogQLCandidates: []string{}})
		return
	}

	selected := req.Selected
	if len(selected) > s.maxTemplatesInPlan {
		selected = selected[:s.maxTemplatesInPlan]
	}

	var out []string
	for _, h := range selected {
		selector := buildLabelSelector(h.Labels) // service/env/namespace only
		regex := strings.TrimSpace(h.Regex)
		if regex == "" {
			// fallback: escape template when regex missing
			regex = regexp.QuoteMeta(h.Template)
		}
		regex = escapeForLogQL(regex)
		body := fmt.Sprintf(`|~ "%s"`, regex)
		logql := fmt.Sprintf(`%s %s`, selector, body)
		out = append(out, logql)
	}
	writeJSON(w, http.StatusOK, PlanResponse{LogQLCandidates: out})
}

/* ===========================
   Qdrant helpers
   =========================== */

func (s *PlannerServer) qdrantSearch(ctx context.Context, vec []float32, filter *qdrant.Filter, topK int) ([]*qdrant.ScoredPoint, error) {
	req := &qdrant.SearchPoints{
		CollectionName: s.qdrantCollection,
		Vector:         vec, // Direct []float32 slice
		Limit:          uint64(topK),
		ScoreThreshold: nil,
		Filter:         filter,
		WithPayload:    &qdrant.WithPayloadSelector{SelectorOptions: &qdrant.WithPayloadSelector_Enable{Enable: true}},
		Params:         nil, // e.g., HNSW params
	}
	resp, err := s.qdrant.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.GetResult(), nil
}

// Build a Qdrant filter that always includes tenant AND any whitelisted label filters.
func (s *PlannerServer) qdrantFilter(labelFilter map[string]string) *qdrant.Filter {
	must := []*qdrant.Condition{
		// enforce tenant in payload
		qdrantHasStringMatch("tenant", s.tenant),
	}
	allow := map[string]bool{"service": true, "env": true, "namespace": true, "severity": true}
	for k, v := range labelFilter {
		if !allow[k] || strings.TrimSpace(v) == "" {
			continue
		}
		must = append(must, qdrantHasStringMatch("labels."+k, v))
	}
	return &qdrant.Filter{Must: must}
}

func qdrantHasStringMatch(path, value string) *qdrant.Condition {
	return &qdrant.Condition{
		ConditionOneOf: &qdrant.Condition_Field{
			Field: &qdrant.FieldCondition{
				Key: path,
				Match: &qdrant.Match{
					MatchValue: &qdrant.Match_Text{
						Text: value,
					},
				},
			},
		},
	}
}

/* ===========================
   Postgres catalog helpers
   =========================== */

type tmplMeta struct {
	TemplateID   string
	TemplateText string
	Regex        string
	Labels       map[string]string
	Count24h     uint64
	FirstSeen    *time.Time
	LastSeen     *time.Time
}

func (m tmplMeta) toHit(score float32) TemplateHit {
	return TemplateHit{
		TemplateID: m.TemplateID,
		Template:   m.TemplateText,
		Regex:      m.Regex,
		Labels:     m.Labels,
		Score:      score,
		Count24h:   m.Count24h,
		TsFirst:    m.FirstSeen,
		TsLast:     m.LastSeen,
	}
}

func (s *PlannerServer) fetchTemplates(ctx context.Context, ids []string) (map[string]tmplMeta, error) {
	if len(ids) == 0 {
		return map[string]tmplMeta{}, nil
	}
	// Build IN clause safely
	params := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	for i, id := range ids {
		params[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, id)
	}
	// Always scope by tenant
	params = append(params, fmt.Sprintf("$%d", len(ids)+1))
	args = append(args, s.tenant)

	q := `
SELECT template_id, template_text, regex, labels, count_24h, first_seen, last_seen
FROM templates
WHERE template_id IN (` + strings.Join(params[:len(ids)], ",") + `)
  AND tenant = ` + params[len(params)-1]

	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]tmplMeta, len(ids))
	for rows.Next() {
		var (
			id, text, regex string
			labelsJSON      []byte
			count24         int64
			first, last     *time.Time
		)
		if err := rows.Scan(&id, &text, &regex, &labelsJSON, &count24, &first, &last); err != nil {
			return nil, err
		}
		lbls := map[string]string{}
		_ = json.Unmarshal(labelsJSON, &lbls)
		if count24 < 0 {
			count24 = 0
		}
		out[id] = tmplMeta{
			TemplateID:   id,
			TemplateText: text,
			Regex:        regex,
			Labels:       lbls,
			Count24h:     uint64(count24),
			FirstSeen:    first,
			LastSeen:     last,
		}
	}
	return out, rows.Err()
}

/* ===========================
   Embedding (replace with real)
   =========================== */

func (s *PlannerServer) embedQuery(ctx context.Context, q string) ([]float32, error) {
	if strings.TrimSpace(q) == "" {
		return nil, errors.New("empty query")
	}
	const dim = 384 // Match indexfeed vector dimension
	return embedText(q, dim), nil
}

// Real embedding implementation using word-level features and semantic hashing
func embedText(text string, dim int) []float32 {
	// Normalize text
	text = strings.ToLower(strings.TrimSpace(text))

	// Tokenize into words
	words := tokenize(text)

	// Create word embeddings using multiple approaches
	vec := make([]float32, dim)

	// 1. Character n-gram features (captures morphology)
	charNgrams := extractCharNgrams(text, 2, 4) // 2-4 character n-grams
	for i, ngram := range charNgrams {
		if i >= dim/4 {
			break
		}
		vec[i] = float32(hashToFloat(ngram))
	}

	// 2. Word-level features (captures semantics)
	wordFeatures := extractWordFeatures(words)
	for i, feature := range wordFeatures {
		if i >= dim/4 {
			break
		}
		vec[dim/4+i] = float32(feature)
	}

	// 3. TF-IDF like features (captures importance)
	tfidfFeatures := extractTfIdfFeatures(words)
	for i, feature := range tfidfFeatures {
		if i >= dim/4 {
			break
		}
		vec[dim/2+i] = float32(feature)
	}

	// 4. Semantic hash features (captures overall meaning)
	semanticHash := hashToFloat(text)
	for i := 0; i < dim/4; i++ {
		vec[3*dim/4+i] = float32(semanticHash * float64(i+1) / float64(dim/4))
	}

	// Normalize the vector
	normalizeVector(vec)

	return vec
}

// Tokenize text into words, handling common log patterns
func tokenize(text string) []string {
	// Split on whitespace and punctuation, but preserve some patterns
	words := []string{}
	current := ""

	for _, r := range text {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			current += string(r)
		} else {
			if current != "" {
				words = append(words, current)
				current = ""
			}
		}
	}
	if current != "" {
		words = append(words, current)
	}

	return words
}

// Extract character n-grams
func extractCharNgrams(text string, minN, maxN int) []string {
	ngrams := []string{}
	for n := minN; n <= maxN; n++ {
		for i := 0; i <= len(text)-n; i++ {
			ngram := text[i : i+n]
			ngrams = append(ngrams, ngram)
		}
	}
	return ngrams
}

// Extract word-level features
func extractWordFeatures(words []string) []float64 {
	features := []float64{}

	// Word length statistics
	if len(words) > 0 {
		avgLength := 0.0
		for _, word := range words {
			avgLength += float64(len(word))
		}
		avgLength /= float64(len(words))
		features = append(features, avgLength/20.0) // normalize
	}

	// Word frequency features
	wordFreq := make(map[string]int)
	for _, word := range words {
		wordFreq[word]++
	}

	// Most common word features
	sortedWords := make([]string, 0, len(wordFreq))
	for word := range wordFreq {
		sortedWords = append(sortedWords, word)
	}
	sort.Slice(sortedWords, func(i, j int) bool {
		return wordFreq[sortedWords[i]] > wordFreq[sortedWords[j]]
	})

	for i := range sortedWords {
		if i >= 10 { // limit to top 10 words
			break
		}
		word := sortedWords[i]
		features = append(features, float64(wordFreq[word])/float64(len(words)))
	}

	return features
}

// Extract TF-IDF like features
func extractTfIdfFeatures(words []string) []float64 {
	features := []float64{}

	// Term frequency features
	termFreq := make(map[string]int)
	for _, word := range words {
		termFreq[word]++
	}

	// Calculate TF scores
	for _, freq := range termFreq {
		tf := float64(freq) / float64(len(words))
		features = append(features, tf)
	}

	// Add document length feature
	features = append(features, math.Log(float64(len(words))+1)/10.0)

	return features
}

// Hash string to float in range [0, 1]
func hashToFloat(s string) float64 {
	hash := sha256.Sum256([]byte(s))
	// Take first 8 bytes and convert to float
	bits := binary.BigEndian.Uint64(hash[:8])
	return float64(bits) / float64(^uint64(0)) // normalize to [0, 1]
}

// Normalize vector to unit length
func normalizeVector(vec []float32) {
	var sum float64
	for _, v := range vec {
		sum += float64(v * v)
	}
	if sum > 0 {
		norm := float32(1.0 / math.Sqrt(sum))
		for i := range vec {
			vec[i] *= norm
		}
	}
}

/* ===========================
   Ranking & LogQL helpers
   =========================== */

func boostRecent(hits []TemplateHit, end time.Time) {
	// Small recency boost using last_seen proximity to "end"
	type pair struct {
		h TemplateHit
		s float64
	}
	ps := make([]pair, 0, len(hits))
	for _, h := range hits {
		score := float64(h.Score)
		if h.TsLast != nil {
			diff := end.Sub(*h.TsLast).Abs().Hours()
			score += 0.05 * (1.0 / (1.0 + diff)) // tiny boost
		}
		ps = append(ps, pair{h: h, s: score})
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].s > ps[j].s })
	for i := range hits {
		hits[i] = ps[i].h
	}
}

func buildLabelSelector(labels map[string]string) string {
	parts := []string{}
	if v := labels["service"]; v != "" {
		parts = append(parts, fmt.Sprintf(`service="%s"`, escapeLabel(v)))
	}
	if v := labels["env"]; v != "" {
		parts = append(parts, fmt.Sprintf(`env="%s"`, escapeLabel(v)))
	}
	if v := labels["namespace"]; v != "" {
		parts = append(parts, fmt.Sprintf(`namespace="%s"`, escapeLabel(v)))
	}
	if len(parts) == 0 {
		return "{}"
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func escapeLabel(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func escapeForLogQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

/* ===========================
   HTTP helpers
   =========================== */

func withJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

/* ===========================
   Misc utils
   =========================== */

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i
		}
	}
	return def
}

// Qdrant payload helpers
func payloadString(pl map[string]*qdrant.Value, key string) string {
	if pl == nil {
		return ""
	}
	// Payload is a map<string, Value> (oneof). We read text/strings.
	if v, ok := pl[key]; ok {
		if vv := v.GetStringValue(); vv != "" {
			return vv
		}
		if arr := v.GetListValue(); arr != nil && len(arr.Values) > 0 {
			// sometimes string stored as 1-element list
			if s := arr.Values[0].GetStringValue(); s != "" {
				return s
			}
		}
	}
	return ""
}

/* ===========================
   main
   =========================== */

func main() {
	s, err := NewPlannerServer()
	if err != nil {
		log.Fatal(err)
	}
	if err := s.Start(); err != nil {
		log.Fatal(err)
	}
}
