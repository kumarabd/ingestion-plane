package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Handler struct {
}

type Options struct {
	// Additional labels necessary
}

func New(name string) (*Handler, error) {
	return &Handler{}, nil
}

// Counter represents a Prometheus counter
type Counter struct {
	*prometheus.CounterVec
}

// Histogram represents a Prometheus histogram
type Histogram struct {
	*prometheus.HistogramVec
}

// Gauge represents a Prometheus gauge
type Gauge struct {
	*prometheus.GaugeVec
}

// NewCounter creates a new counter metric
func (h *Handler) NewCounter(name, help string) *Counter {
	counter := promauto.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: help,
	}, []string{})
	return &Counter{counter}
}

// NewHistogram creates a new histogram metric
func (h *Handler) NewHistogram(name, help string) *Histogram {
	histogram := promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: prometheus.DefBuckets,
	}, []string{})
	return &Histogram{histogram}
}

// NewGauge creates a new gauge metric
func (h *Handler) NewGauge(name, help string) *Gauge {
	gauge := promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: help,
	}, []string{})
	return &Gauge{gauge}
}

// Inc increments the counter
func (c *Counter) Inc(labels ...map[string]string) {
	c.CounterVec.WithLabelValues().Inc()
}

// Observe adds a single observation to the histogram
func (h *Histogram) Observe(value float64, labels ...map[string]string) {
	h.HistogramVec.WithLabelValues().Observe(value)
}

// Set sets the gauge value
func (g *Gauge) Set(value float64, labels ...map[string]string) {
	g.GaugeVec.WithLabelValues().Set(value)
}
