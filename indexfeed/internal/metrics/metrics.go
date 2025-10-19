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
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		c.CounterVec.WithLabelValues(labelValues...).Inc()
	} else {
		c.CounterVec.WithLabelValues().Inc()
	}
}

// Add adds the given value to the counter
func (c *Counter) Add(delta float64, labels ...map[string]string) {
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		c.CounterVec.WithLabelValues(labelValues...).Add(delta)
	} else {
		c.CounterVec.WithLabelValues().Add(delta)
	}
}

// Observe adds a single observation to the histogram
func (h *Histogram) Observe(value float64, labels ...map[string]string) {
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		h.HistogramVec.WithLabelValues(labelValues...).Observe(value)
	} else {
		h.HistogramVec.WithLabelValues().Observe(value)
	}
}

// Set sets the gauge value
func (g *Gauge) Set(value float64, labels ...map[string]string) {
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		g.GaugeVec.WithLabelValues(labelValues...).Set(value)
	} else {
		g.GaugeVec.WithLabelValues().Set(value)
	}
}

// Inc increments the gauge
func (g *Gauge) Inc(labels ...map[string]string) {
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		g.GaugeVec.WithLabelValues(labelValues...).Inc()
	} else {
		g.GaugeVec.WithLabelValues().Inc()
	}
}

// Dec decrements the gauge
func (g *Gauge) Dec(labels ...map[string]string) {
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		g.GaugeVec.WithLabelValues(labelValues...).Dec()
	} else {
		g.GaugeVec.WithLabelValues().Dec()
	}
}

// Add adds the given value to the gauge
func (g *Gauge) Add(delta float64, labels ...map[string]string) {
	if len(labels) > 0 {
		// Convert map to label values if provided
		labelValues := make([]string, len(labels[0]))
		i := 0
		for _, v := range labels[0] {
			labelValues[i] = v
			i++
		}
		g.GaugeVec.WithLabelValues(labelValues...).Add(delta)
	} else {
		g.GaugeVec.WithLabelValues().Add(delta)
	}
}
