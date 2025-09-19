package service

import (
	"github.com/kumarabd/gokit/logger"
	"github.com/kumarabd/ingestion-plane/gateway/internal/metrics"
	"github.com/kumarabd/ingestion-plane/gateway/pkg/ingest"
)

type Config struct {
	OTLP    *ingest.Config        `json:"otlp" yaml:"otlp"`
	Emitter *ingest.EmitterConfig `json:"emitter" yaml:"emitter"`
}

type Handler struct {
	log         *logger.Handler
	config      *Config
	datalayer   DataLayer
	metric      *metrics.Handler
	otlpHandler *ingest.Handler
	emitter     *ingest.Emitter
}

func New(l *logger.Handler, m *metrics.Handler, datalayer DataLayer, sConfig *Config) (*Handler, error) {
	// Create emitter
	emitter, err := ingest.NewEmitter(sConfig.Emitter, l, m)
	if err != nil {
		return nil, err
	}
	if err := emitter.ValidateConfig(); err != nil {
		return nil, err
	}

	// Start emitter if it has a forwarder
	if err := emitter.Start(); err != nil {
		return nil, err
	}

	// Create OTLP handler
	otlpHandler := ingest.NewOTLPHandler(sConfig.OTLP, l, m, emitter)
	if err := otlpHandler.ValidateConfig(); err != nil {
		return nil, err
	}

	return &Handler{
		log:         l,
		config:      sConfig,
		datalayer:   datalayer,
		metric:      m,
		otlpHandler: otlpHandler,
		emitter:     emitter,
	}, nil
}

// GetOTLPHandler returns the OTLP handler
func (h *Handler) GetOTLPHandler() *ingest.Handler {
	return h.otlpHandler
}
