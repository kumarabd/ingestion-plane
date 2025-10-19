package server

// Config holds gRPC server configuration
type Config struct {
	GRPCPort string `json:"grpc_port" yaml:"grpc_port"`
}
