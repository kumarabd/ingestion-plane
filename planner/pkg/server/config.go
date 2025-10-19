package server

import "time"

// Config holds HTTP server configuration
type Config struct {
	HTTPAddr     string        `json:"http_addr" yaml:"http_addr"`
	ReadTimeout  time.Duration `json:"read_timeout" yaml:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`
}
