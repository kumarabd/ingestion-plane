package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
)

func TestEmitNormalizedLogBatch(t *testing.T) {
	// Create a mock metrics handler
	metricHandler, err := metrics.New("test_stdout")
	if err != nil {
		t.Fatalf("Failed to create metrics handler: %v", err)
	}

	// Create emitter config for stdout output
	config := &EmitterConfig{
		OutputType: "stdout",
	}

	// Create emitter
	emitter, err := NewEmitter(config, nil, metricHandler)
	if err != nil {
		t.Fatalf("Failed to create emitter: %v", err)
	}

	// Create test batch
	batch := &NormalizedLogBatch{
		Records: []NormalizedLog{
			{
				Timestamp: time.Now(),
				Labels:    map[string]string{"service": "test1"},
				Message:   "Test message 1",
				Schema:    "TEXT",
				Sanitized: false,
				Truncated: false,
				Redaction: RedactionReport{Applied: false},
			},
			{
				Timestamp: time.Now(),
				Labels:    map[string]string{"service": "test2"},
				Message:   "Test message 2",
				Schema:    "JSON",
				Sanitized: true,
				Truncated: false,
				Redaction: RedactionReport{Applied: true, Rules: []string{"email"}, Count: 1},
			},
		},
	}

	// Test batch emission
	err = emitter.EmitNormalizedLogBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("Failed to emit batch: %v", err)
	}

	// Test empty batch
	emptyBatch := &NormalizedLogBatch{Records: []NormalizedLog{}}
	err = emitter.EmitNormalizedLogBatch(context.Background(), emptyBatch)
	if err != nil {
		t.Fatalf("Failed to emit empty batch: %v", err)
	}

	// Test nil batch
	err = emitter.EmitNormalizedLogBatch(context.Background(), nil)
	if err != nil {
		t.Fatalf("Failed to emit nil batch: %v", err)
	}

	t.Log("Batch emission test completed successfully")
}

func TestEmitNormalizedLogBatchForwarder(t *testing.T) {
	// Create emitter config for forwarder output (without actual forwarder)
	config := &EmitterConfig{
		OutputType: "forwarder",
	}

	// Create emitter without metrics to avoid registration conflicts
	emitter, err := NewEmitter(config, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create emitter: %v", err)
	}

	// Create test batch
	batch := &NormalizedLogBatch{
		Records: []NormalizedLog{
			{
				Timestamp: time.Now(),
				Labels:    map[string]string{"service": "test"},
				Message:   "Test message",
				Schema:    "TEXT",
				Sanitized: false,
				Truncated: false,
				Redaction: RedactionReport{Applied: false},
			},
		},
	}

	// Test batch emission (should fail because forwarder is not initialized)
	err = emitter.EmitNormalizedLogBatch(context.Background(), batch)
	if err == nil {
		t.Error("Expected error when forwarder is not initialized")
	}

	t.Log("Forwarder batch emission test completed successfully")
}
