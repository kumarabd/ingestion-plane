package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

// HealthServer provides HTTP health check endpoints
type HealthServer struct {
	server *http.Server
	ready  bool
	mu     sync.RWMutex
}

// NewHealthServer creates a new health check HTTP server
func NewHealthServer(port string) *HealthServer {
	hs := &HealthServer{
		ready: false,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", hs.handleHealth)
	mux.HandleFunc("/readyz", hs.handleReady)

	hs.server = &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return hs
}

// Start starts the health server
func (hs *HealthServer) Start() error {
	log.Printf("INFO: Health server listening on %s", hs.server.Addr)
	return hs.server.ListenAndServe()
}

// SetReady marks the service as ready
func (hs *HealthServer) SetReady(ready bool) {
	hs.mu.Lock()
	hs.ready = ready
	hs.mu.Unlock()
}

// handleHealth handles /healthz endpoint
func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}

// handleReady handles /readyz endpoint
func (hs *HealthServer) handleReady(w http.ResponseWriter, r *http.Request) {
	hs.mu.RLock()
	ready := hs.ready
	hs.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not ready",
			"time":   time.Now().UTC(),
		})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
		"time":   time.Now().UTC(),
	})
}
