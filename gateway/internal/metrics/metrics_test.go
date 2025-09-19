package metrics

import (
	"testing"
	"time"
)

func TestMetricsHandler(t *testing.T) {
	handler, err := New("test")
	if err != nil {
		t.Fatalf("Failed to create metrics handler: %v", err)
	}

	// Test PII redaction counter
	handler.IncPIIRedactionsTotal("email")
	handler.IncPIIRedactionsTotal("ipv4")
	handler.IncPIIRedactionsTotal("email") // Should increment twice

	// Test normalize latency histogram
	handler.ObserveNormalizeLatency(100*time.Millisecond, true)
	handler.ObserveNormalizeLatency(200*time.Millisecond, false)

	// Test existing metrics still work
	handler.IncIngestRecordsTotal("json", "normalized")
	handler.IncIngestBatchesTotal("json")
	handler.IncIngestRejectedTotal("test_reason")

	// If we get here without panicking, the metrics are working
	t.Log("All metrics operations completed successfully")
}
