package cache

import (
	"time"

	gocache "github.com/patrickmn/go-cache"
)

type Config struct {
	Enabled bool `yaml:"enabled"`
	TTL     int  `yaml:"ttl"`
	Cleanup int  `yaml:"cleanup"`
}

type Service struct {
	cache *gocache.Cache
}

func NewService(cfg *Config) *Service {
	if !cfg.Enabled {
		return nil
	}

	c := gocache.New(
		time.Duration(cfg.TTL)*time.Second,
		time.Duration(cfg.Cleanup)*time.Second,
	)

	return &Service{cache: c}
}

func (s *Service) Get(key string) (interface{}, bool) {
	if s.cache == nil {
		return nil, false
	}
	return s.cache.Get(key)
}

func (s *Service) Set(key string, value interface{}, ttl time.Duration) {
	if s.cache == nil {
		return
	}
	s.cache.Set(key, value, ttl)
}

func (s *Service) Delete(key string) {
	if s.cache == nil {
		return
	}
	s.cache.Delete(key)
}

func (s *Service) Flush() {
	if s.cache == nil {
		return
	}
	s.cache.Flush()
}
