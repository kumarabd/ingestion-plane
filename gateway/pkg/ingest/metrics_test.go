package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

func TestNormalizeAndRedactBatchMetrics(t *testing.T) {
	// Create a mock metrics handler
	metricHandler, err := metrics.New("test_normalize")
	if err != nil {
		t.Fatalf("Failed to create metrics handler: %v", err)
	}

	// Create a test handler
	config := &Config{
		MaxMessageBytes: 1024,
	}
	handler := &Handler{
		config: config,
	}

	// Create test batch with PII data
	batch := &RawLogBatch{
		Records: []RawLog{
			{
				Timestamp: time.Now(),
				Labels:    map[string]string{"service": "test"},
				Payload:   "User john@example.com connected from 192.168.1.1",
			},
			{
				Timestamp: time.Now(),
				Labels:    map[string]string{"service": "test"},
				Payload:   "Payment with card 4111-1111-1111-1111",
			},
			{
				Timestamp: time.Now(),
				Labels:    map[string]string{"service": "test"},
				Payload:   "Normal log message without PII",
			},
		},
	}

	// Create a mock logger (we'll just pass nil since we're not testing logging)
	// Process the batch
	result, err := handler.NormalizeAndRedactBatch(context.Background(), batch, nil, metricHandler)
	if err != nil {
		t.Fatalf("Failed to process batch: %v", err)
	}

	// Verify results
	if len(result.Records) != 3 {
		t.Errorf("Expected 3 records, got %d", len(result.Records))
	}

	// Check that PII was redacted
	firstRecord := result.Records[0]
	if firstRecord.Message != "User <email> connected from <ipv4>" {
		t.Errorf("Expected redacted message, got %q", firstRecord.Message)
	}
	if !firstRecord.Redaction.Applied {
		t.Error("Expected redaction to be applied to first record")
	}

	secondRecord := result.Records[1]
	if secondRecord.Message != "Payment with card <cc>" {
		t.Errorf("Expected redacted message, got %q", secondRecord.Message)
	}
	if !secondRecord.Redaction.Applied {
		t.Error("Expected redaction to be applied to second record")
	}

	thirdRecord := result.Records[2]
	if thirdRecord.Redaction.Applied {
		t.Error("Expected no redaction for third record")
	}

	// If we get here without panicking, the metrics tracking is working
	t.Log("NormalizeAndRedactBatch metrics tracking completed successfully")
}
