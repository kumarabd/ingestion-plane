package service

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kumarabd/ingestion-plane/planner/pkg/database"
	"github.com/kumarabd/ingestion-plane/planner/pkg/server"
	"github.com/kumarabd/ingestion-plane/planner/pkg/types"
	"github.com/kumarabd/ingestion-plane/planner/pkg/vector"
)

// Service implements the planner business logic
type Service struct {
	config    *Config
	db        *database.Client
	vectorCli *vector.Client
	vectorDim int
}

// New creates a new planner service
func New(cfg *Config, vectorDim int, db *database.Client, vectorCli *vector.Client) *Service {
	return &Service{
		config:    cfg,
		db:        db,
		vectorCli: vectorCli,
		vectorDim: vectorDim,
	}
}

// Register registers HTTP handlers
func (s *Service) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/search", s.HandleSearch)
	mux.HandleFunc("/v1/plan", s.HandlePlan)
}

// HandleSearch handles the /v1/search endpoint
func (s *Service) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.HTTPError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req types.SemanticQuery
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.HTTPError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		server.WriteJSON(w, http.StatusOK, types.SearchResponse{Hits: []types.TemplateHit{}})
		return
	}

	topK := int(req.TopK)
	if topK <= 0 || topK > 100 {
		topK = s.config.DefaultTopK
	}

	// 1) Embed NL query to vector
	vec := vector.EmbedTemplate(req.Query, s.vectorDim)

	// 2) Qdrant ANN search
	points, err := s.vectorCli.Search(r.Context(), vec, s.config.Tenant, req.LabelFilter, topK)
	if err != nil {
		server.HTTPError(w, http.StatusInternalServerError, "qdrant search: "+err.Error())
		return
	}
	if len(points) == 0 {
		server.WriteJSON(w, http.StatusOK, types.SearchResponse{Hits: []types.TemplateHit{}})
		return
	}

	// 3) Extract template_ids from payloads
	ids := make([]string, 0, len(points))
	type scorePair struct {
		ID    string
		Score float32
	}
	var scored []scorePair
	for _, p := range points {
		pl := p.GetPayload()
		tid := vector.PayloadString(pl, "template_id")
		if tid == "" {
			continue
		}
		ids = append(ids, tid)
		scored = append(scored, scorePair{ID: tid, Score: float32(p.GetScore())})
	}

	if len(ids) == 0 {
		server.WriteJSON(w, http.StatusOK, types.SearchResponse{Hits: []types.TemplateHit{}})
		return
	}

	// 4) Fetch metadata from Postgres catalog
	meta, err := s.db.FetchTemplates(r.Context(), s.config.Tenant, ids)
	if err != nil {
		server.HTTPError(w, http.StatusInternalServerError, "catalog fetch: "+err.Error())
		return
	}

	// 5) Assemble hits (preserve ANN order by score)
	hits := make([]types.TemplateHit, 0, len(scored))
	for _, sp := range scored {
		if m, ok := meta[sp.ID]; ok {
			hits = append(hits, toHit(m, sp.Score))
		}
	}

	// 6) Optional re-ranking boosts (recency)
	if req.EndTime != nil && !req.EndTime.IsZero() {
		boostRecent(hits, *req.EndTime)
	}

	server.WriteJSON(w, http.StatusOK, types.SearchResponse{Hits: hits})
}

// HandlePlan handles the /v1/plan endpoint
func (s *Service) HandlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		server.HTTPError(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	var req types.PlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		server.HTTPError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if len(req.Selected) == 0 {
		server.WriteJSON(w, http.StatusOK, types.PlanResponse{LogQLCandidates: []string{}})
		return
	}

	selected := req.Selected
	if len(selected) > s.config.MaxTemplatesInPlan {
		selected = selected[:s.config.MaxTemplatesInPlan]
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
	server.WriteJSON(w, http.StatusOK, types.PlanResponse{LogQLCandidates: out})
}

// Helper functions

func toHit(m database.TemplateMeta, score float32) types.TemplateHit {
	return types.TemplateHit{
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

// boostRecent re-ranks hits by recency (logs newer than endTime get boosted)
func boostRecent(hits []types.TemplateHit, endTime time.Time) {
	// Build pairs with recency boost
	type pair struct {
		h        types.TemplateHit
		adjScore float64
	}
	ps := make([]pair, len(hits))
	for i, h := range hits {
		score := float64(h.Score)
		if h.TsLast != nil && !h.TsLast.IsZero() {
			age := endTime.Sub(*h.TsLast)
			if age < 0 {
				age = 0
			}
			ageHours := age.Hours()
			// Apply recency boost: exponential decay
			recencyFactor := math.Exp(-ageHours / 24.0) // decay over 24 hours
			score = score * (1.0 + recencyFactor)
		}
		ps[i] = pair{h: h, adjScore: score}
	}
	// Re-sort
	sort.Slice(ps, func(i, j int) bool {
		return ps[i].adjScore > ps[j].adjScore
	})
	// Write back
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
