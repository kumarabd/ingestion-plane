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
	log.Printf("INFO: Service ready state changed to: %t", ready)
}

// handleHealth handles /healthz endpoint
func (hs *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	log.Printf("DEBUG: Health check request received from %s", r.RemoteAddr)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
	log.Printf("DEBUG: Health check response: OK")
}

// handleReady handles /readyz endpoint
func (hs *HealthServer) handleReady(w http.ResponseWriter, r *http.Request) {
	hs.mu.RLock()
	ready := hs.ready
	hs.mu.RUnlock()

	log.Printf("DEBUG: Readiness check request received from %s (ready=%t)", r.RemoteAddr, ready)

	w.Header().Set("Content-Type", "application/json")
	if !ready {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "not ready",
			"time":   time.Now().UTC(),
		})
		log.Printf("DEBUG: Readiness check response: NOT READY (503)")
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ready",
		"time":   time.Now().UTC(),
	})
	log.Printf("DEBUG: Readiness check response: READY (200)")
}
