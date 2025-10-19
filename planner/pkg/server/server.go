package server

import (
	"encoding/json"
	"log"
	"net/http"
)

// Server represents the HTTP server
type Server struct {
	config  *Config
	handler http.Handler
	srv     *http.Server
}

// New creates a new HTTP server
func New(cfg *Config, handler http.Handler) *Server {
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	return &Server{
		config:  cfg,
		handler: handler,
		srv:     srv,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	log.Printf("INFO: Planner HTTP server listening on %s", s.config.HTTPAddr)
	return s.srv.ListenAndServe()
}

// HTTP middleware and helpers

// WithJSON sets JSON content type
func WithJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		next.ServeHTTP(w, r)
	})
}

// WithCORS adds CORS headers
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WriteJSON writes JSON response
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// HTTPError writes JSON error response
func HTTPError(w http.ResponseWriter, code int, msg string) {
	WriteJSON(w, code, map[string]any{"error": msg})
}
