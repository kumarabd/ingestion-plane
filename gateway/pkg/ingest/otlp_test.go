package ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
)

// mockEmitter is a mock implementation of NormalizedLogEmitter
type mockEmitter struct {
	logs []*NormalizedLog
}

func (m *mockEmitter) EmitNormalizedLog(ctx context.Context, log *NormalizedLog) error {
	m.logs = append(m.logs, log)
	return nil
}

func TestOTLPHandler_HandleOTLPHTTP(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)

	log, _ := logger.New("test", logger.Options{Format: logger.JSONLogFormat})
	metric, _ := metrics.New("test")

	config := &Config{
		MaxLogSize:     1024,
		MaxBatchSize:   100,
		MaxLabels:      50,
		MaxFields:      100,
		RequestTimeout: 30 * time.Second,
		ValidateUTF8:   true,
		AllowedSchemas: []string{"JSON", "LOGFMT", "TEXT"},
	}

	mockEmitter := &mockEmitter{}
	handler := NewOTLPHandler(config, log, metric, mockEmitter)

	// Create test OTLP request
	req := plogotlp.NewExportRequest()
	logs := req.Logs()
	resourceLogs := logs.ResourceLogs().AppendEmpty()
	scopeLogs := resourceLogs.ScopeLogs().AppendEmpty()
	logRecord := scopeLogs.LogRecords().AppendEmpty()

	// Set log record data
	logRecord.SetTimestamp(pcommon.NewTimestampFromTime(time.Now()))
	logRecord.Body().SetStr("Test log message")
	logRecord.SetSeverityNumber(plog.SeverityNumberInfo)
	logRecord.SetSeverityText("INFO")

	// Add resource attributes
	resourceLogs.Resource().Attributes().PutStr("service.name", "test-service")
	resourceLogs.Resource().Attributes().PutStr("service.version", "1.0.0")

	// Add log attributes
	logRecord.Attributes().PutStr("user_id", "12345")
	logRecord.Attributes().PutStr("request_id", "req-abc-123")

	// Marshal request
	reqBytes, err := req.MarshalProto()
	require.NoError(t, err)

	// Create HTTP request
	httpReq := httptest.NewRequest("POST", "/v1/logs", bytes.NewReader(reqBytes))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin context
	c, _ := gin.CreateTestContext(w)
	c.Request = httpReq

	// Handle request
	handler.HandleOTLPHTTP(c)

	// Assertions
	assert.Equal(t, http.StatusOK, w.Code)

	// Check response
	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "success", response["status"])
	assert.Equal(t, float64(1), response["processed_logs"])

	// Check emitted log
	require.Len(t, mockEmitter.logs, 1)
	emittedLog := mockEmitter.logs[0]

	assert.Equal(t, "Test log message", emittedLog.Message)
	assert.Equal(t, "TEXT", emittedLog.Schema)
	assert.False(t, emittedLog.Sanitized)
	assert.Equal(t, uint32(len("Test log message")), emittedLog.OrigLen)

	// Check labels
	assert.Equal(t, "test-service", emittedLog.Labels["service.name"])
	assert.Equal(t, "1.0.0", emittedLog.Labels["service.version"])

	// Check fields
	assert.Equal(t, "12345", emittedLog.Fields["user_id"])
	assert.Equal(t, "req-abc-123", emittedLog.Fields["request_id"])
	assert.Equal(t, "INFO", emittedLog.Fields["severity_text"])
}

func TestOTLPHandler_ValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				MaxLogSize:     1024,
				MaxBatchSize:   100,
				MaxLabels:      50,
				MaxFields:      100,
				RequestTimeout: 30 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "invalid max log size",
			config: &Config{
				MaxLogSize:     -1,
				MaxBatchSize:   100,
				MaxLabels:      50,
				MaxFields:      100,
				RequestTimeout: 30 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid max batch size",
			config: &Config{
				MaxLogSize:     1024,
				MaxBatchSize:   0,
				MaxLabels:      50,
				MaxFields:      100,
				RequestTimeout: 30 * time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, _ := logger.New("test", logger.Options{Format: logger.JSONLogFormat})
			metric, _ := metrics.New("test")
			mockEmitter := &mockEmitter{}
			handler := NewOTLPHandler(tt.config, log, metric, mockEmitter)

			err := handler.ValidateConfig()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOTLPHandler_RequestTooLarge(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)

	log, _ := logger.New("test", logger.Options{Format: logger.JSONLogFormat})
	metric, _ := metrics.New("test")

	config := &Config{
		MaxLogSize:     100, // Very small limit
		MaxBatchSize:   100,
		MaxLabels:      50,
		MaxFields:      100,
		RequestTimeout: 30 * time.Second,
		ValidateUTF8:   true,
		AllowedSchemas: []string{"JSON", "LOGFMT", "TEXT"},
	}

	mockEmitter := &mockEmitter{}
	handler := NewOTLPHandler(config, log, metric, mockEmitter)

	// Create large request body
	largeBody := make([]byte, 200) // Larger than MaxLogSize

	// Create HTTP request
	httpReq := httptest.NewRequest("POST", "/v1/logs", bytes.NewReader(largeBody))
	httpReq.Header.Set("Content-Type", "application/x-protobuf")

	// Create response recorder
	w := httptest.NewRecorder()

	// Create Gin context
	c, _ := gin.CreateTestContext(w)
	c.Request = httpReq

	// Handle request
	handler.HandleOTLPHTTP(c)

	// Assertions
	assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)

	// Check response
	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "Request body too large", response["error"])
}
