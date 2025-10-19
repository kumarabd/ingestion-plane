package policy

import "time"

// Config holds sampling policy configuration
type Config struct {
	EMAHalfLifeMins  float64 `json:"ema_halflife_mins" yaml:"ema_halflife_mins"`
	MinSpikePerMin   uint64  `json:"min_spike_per_min" yaml:"min_spike_per_min"`
	SpikeFactor      float64 `json:"spike_factor" yaml:"spike_factor"`
	WarmupN          uint64  `json:"warmup_n" yaml:"warmup_n"`
	Log2Enabled      bool    `json:"log2_enabled" yaml:"log2_enabled"`
	NoveltyWindowMin int64   `json:"novelty_window_min" yaml:"novelty_window_min"`
	Version          string  `json:"version" yaml:"version"`
	SteadyKDebug     uint64  `json:"steady_k_debug" yaml:"steady_k_debug"`
	SteadyKInfo      uint64  `json:"steady_k_info" yaml:"steady_k_info"`
	SteadyKWarn      uint64  `json:"steady_k_warn" yaml:"steady_k_warn"`
	Budget10mDefault uint64  `json:"budget_10m_default" yaml:"budget_10m_default"`
}

// GetNoveltyWindow returns the novelty window as duration
func (c *Config) GetNoveltyWindow() time.Duration {
	return time.Duration(c.NoveltyWindowMin) * time.Minute
}
