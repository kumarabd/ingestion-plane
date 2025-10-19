package types

import "time"

// SemanticQuery matches search.v1.SemanticQuery (HTTP JSON)
type SemanticQuery struct {
	Query       string            `json:"query"`
	LabelFilter map[string]string `json:"label_filter,omitempty"`
	StartTime   *time.Time        `json:"start_time,omitempty"`
	EndTime     *time.Time        `json:"end_time,omitempty"`
	TopK        uint32            `json:"top_k,omitempty"`
}

// TemplateHit matches search.v1.TemplateHit (HTTP JSON)
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

// SearchResponse matches search.v1.SearchResponse (HTTP JSON)
type SearchResponse struct {
	Hits []TemplateHit `json:"hits"`
}

// PlanRequest matches search.v1.PlanRequest (HTTP JSON)
type PlanRequest struct {
	Selected  []TemplateHit `json:"selected"`
	StartTime *time.Time    `json:"start_time,omitempty"`
	EndTime   *time.Time    `json:"end_time,omitempty"`
}

// PlanResponse matches search.v1.PlanResponse (HTTP JSON)
type PlanResponse struct {
	LogQLCandidates []string `json:"logql_candidates"`
}
