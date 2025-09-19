package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPEndpoints(t *testing.T) {
	// Setup
	gin.SetMode(gin.TestMode)

	log, _ := logger.New("test", logger.Options{Format: logger.JSONLogFormat})
	metric, _ := metrics.New("test")

	// Create mock service
	serviceHandler := &service.Handler{}

	// Create HTTP server
	config := &HTTPConfig{
		Host: "127.0.0.1",
		Port: "8080",
	}

	server := NewHTTP(config, serviceHandler, log, metric)

	t.Run("health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/healthz", nil)
		w := httptest.NewRecorder()

		server.handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, "ok", response["status"])
		assert.Contains(t, response, "time")
	})

	t.Run("metrics endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/metrics", nil)
		w := httptest.NewRecorder()

		server.handler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "# HELP")
	})

	t.Run("ingest endpoint exists", func(t *testing.T) {
		// Test that the endpoint exists (will fail due to missing service, but that's expected)
		req := httptest.NewRequest("POST", "/v1/ingest", bytes.NewReader([]byte("{}")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		server.handler.ServeHTTP(w, req)

		// Should not return 404 (endpoint exists)
		assert.NotEqual(t, http.StatusNotFound, w.Code)
	})
}

func TestHTTPConfig(t *testing.T) {
	config := &HTTPConfig{
		Host: "0.0.0.0",
		Port: "8080",
	}

	assert.Equal(t, "0.0.0.0", config.Host)
	assert.Equal(t, "8080", config.Port)
}
