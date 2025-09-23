package sampler

import (
	"sync"
	"time"
)

// Breaker implements a simple circuit breaker pattern
type Breaker struct {
	mu          sync.RWMutex
	failures    int
	maxFailures int
	resetTime   time.Time
	timeout     time.Duration
}

// NewBreaker creates a new circuit breaker
func NewBreaker(maxFailures int, timeout time.Duration) *Breaker {
	return &Breaker{
		maxFailures: maxFailures,
		timeout:     timeout,
	}
}

// Open returns true if the circuit breaker is open
func (b *Breaker) Open() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.failures < b.maxFailures {
		return false
	}

	// Check if timeout has passed
	if time.Now().After(b.resetTime) {
		return false
	}

	return true
}

// Success resets the failure count
func (b *Breaker) Success() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures = 0
	b.resetTime = time.Time{}
}

// Fail increments the failure count and sets reset time
func (b *Breaker) Fail() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.failures++
	if b.failures >= b.maxFailures {
		b.resetTime = time.Now().Add(b.timeout)
	}
}
