package service

// Config holds service-specific configuration
type Config struct {
	LabelAllowlist []string `json:"label_allowlist" yaml:"label_allowlist"`
}

// GetLabelAllowlistMap returns the label allowlist as a map for efficient lookups
func (c *Config) GetLabelAllowlistMap() map[string]struct{} {
	result := make(map[string]struct{})
	for _, label := range c.LabelAllowlist {
		result[label] = struct{}{}
	}
	return result
}
