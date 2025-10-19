package service

// Config holds service-specific configuration
type Config struct {
	Tenant             string `json:"tenant" yaml:"tenant"`
	DefaultTopK        int    `json:"default_topk" yaml:"default_topk"`
	MaxTemplatesInPlan int    `json:"max_templates_in_plan" yaml:"max_templates_in_plan"`
}
