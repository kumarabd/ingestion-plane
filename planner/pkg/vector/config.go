package vector

// Config holds Qdrant vector database configuration
type Config struct {
	Host       string `json:"host" yaml:"host"`
	Port       int    `json:"port" yaml:"port"`
	Collection string `json:"collection" yaml:"collection"`
	VectorDim  int    `json:"vector_dim" yaml:"vector_dim"`
}
