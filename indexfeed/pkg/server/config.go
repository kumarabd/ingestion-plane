package server

// Config holds server configuration
type Config struct {
	GRPCPort string `json:"grpc_port" yaml:"grpc_port"`
}
