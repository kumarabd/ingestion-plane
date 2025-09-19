package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Handler struct {
	RequestsReceived     *prometheus.CounterVec
	IngestBatchesTotal   *prometheus.CounterVec
	IngestRecordsTotal   *prometheus.CounterVec
	IngestRejectedTotal  *prometheus.CounterVec
	IngestHandlerLatency *prometheus.HistogramVec
}

type Options struct {
	// Additional labels necessary
}

func New(name string) (*Handler, error) {
	return &Handler{
		RequestsReceived: promauto.NewCounterVec(prometheus.CounterOpts{
			Name:        "http_requests_received",
			ConstLabels: map[string]string{
				// Add labels
			},
			Help: "The total number of http requests received",
		}, []string{"status"}),
		IngestBatchesTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_batches_total",
			Help: "The total number of batches ingested",
		}, []string{"source"}),
		IngestRecordsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_records_total",
			Help: "The total number of records ingested",
		}, []string{"source", "schema"}),
		IngestRejectedTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Name: "ingest_rejected_total",
			Help: "The total number of batches rejected",
		}, []string{"reason"}),
		IngestHandlerLatency: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ingest_handler_latency_seconds",
			Help:    "The latency of ingest handler requests",
			Buckets: prometheus.DefBuckets,
		}, []string{"source", "success"}),
	}, nil
}

// IncIngestBatchesTotal increments the ingest batches counter
func (h *Handler) IncIngestBatchesTotal(source string) {
	h.IngestBatchesTotal.WithLabelValues(source).Inc()
}

// IncIngestRecordsTotal increments the ingest records counter
func (h *Handler) IncIngestRecordsTotal(source, schema string) {
	h.IngestRecordsTotal.WithLabelValues(source, schema).Inc()
}

// IncIngestRejectedTotal increments the ingest rejected counter
func (h *Handler) IncIngestRejectedTotal(reason string) {
	h.IngestRejectedTotal.WithLabelValues(reason).Inc()
}

// ObserveIngestHandlerLatency records the latency of ingest handler requests
func (h *Handler) ObserveIngestHandlerLatency(duration time.Duration, source string, success bool) {
	successStr := "true"
	if !success {
		successStr = "false"
	}
	h.IngestHandlerLatency.WithLabelValues(source, successStr).Observe(duration.Seconds())
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

// IncrementCounter increments a counter with the given name and labels
func (h *Handler) IncrementCounter(name string, labels map[string]string) {
	// This is a simplified implementation - in a real system you'd want to
	// manage named counters properly
	// For now, we'll just increment the existing RequestsReceived counter
	h.RequestsReceived.WithLabelValues("increment").Inc()
}

// AddHistogram adds a value to a histogram with the given name and labels
func (h *Handler) AddHistogram(name string, value float64, labels map[string]string) {
	// This is a simplified implementation - in a real system you'd want to
	// manage named histograms properly
	// For now, we'll just observe the value in the existing latency histogram
	h.IngestHandlerLatency.WithLabelValues("histogram", "true").Observe(value)
}
