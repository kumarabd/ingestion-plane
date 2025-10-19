package policy

import (
	"math"

	samplerv1 "github.com/kumarabd/ingestion-plane/contracts/sampler/v1"
)

// Policy defines sampling rules
type Policy struct {
	SeverityKeep     map[samplerv1.Severity]bool
	NoveltyWindow    int64 // in minutes
	WarmupN          uint64
	Log2Enabled      bool
	SteadyK          map[samplerv1.Severity]uint64
	SpikeFactor      float64
	MinSpikePerMin   uint64
	EMAHalfLifeMins  float64
	Budget10mDefault uint64
	Version          string
}

// New creates a new policy from config
func New(cfg *Config) *Policy {
	return &Policy{
		SeverityKeep: map[samplerv1.Severity]bool{
			samplerv1.Severity_SEVERITY_ERROR: true,
			samplerv1.Severity_SEVERITY_FATAL: true,
			samplerv1.Severity_SEVERITY_WARN:  false,
		},
		NoveltyWindow: cfg.NoveltyWindowMin,
		WarmupN:       cfg.WarmupN,
		Log2Enabled:   cfg.Log2Enabled,
		SteadyK: map[samplerv1.Severity]uint64{
			samplerv1.Severity_SEVERITY_DEBUG: cfg.SteadyKDebug,
			samplerv1.Severity_SEVERITY_INFO:  cfg.SteadyKInfo,
			samplerv1.Severity_SEVERITY_WARN:  cfg.SteadyKWarn,
		},
		SpikeFactor:      cfg.SpikeFactor,
		MinSpikePerMin:   cfg.MinSpikePerMin,
		EMAHalfLifeMins:  cfg.EMAHalfLifeMins,
		Budget10mDefault: cfg.Budget10mDefault,
		Version:          cfg.Version,
	}
}

// GetEMAAlpha calculates the EMA alpha from half-life
func (p *Policy) GetEMAAlpha() float64 {
	if p.EMAHalfLifeMins <= 0 {
		return 0.2
	}
	// alpha = 1 - 0.5^(1/halflife)
	return 1.0 - math.Pow(0.5, 1.0/p.EMAHalfLifeMins)
}

// GetSteadyK returns the steady-state sampling rate for a severity
func (p *Policy) GetSteadyK(sev samplerv1.Severity) uint64 {
	k := p.SteadyK[sev]
	if k == 0 {
		k = 100
	}
	return k
}

// IsPowerOfTwo checks if a number is a power of 2
func IsPowerOfTwo(n uint64) bool {
	return n != 0 && (n&(n-1)) == 0
}
