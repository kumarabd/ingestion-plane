package cache

import (
	"time"

	cache_pkg "github.com/patrickmn/go-cache"
)

type Handler struct {
	client *cache_pkg.Cache
}

func New() (*Handler, error) {
	client := cache_pkg.New(5*time.Minute, 10*time.Minute)
	return &Handler{
		client: client,
	}, nil
}

func (h *Handler) Ping() (bool, error) {
	return true, nil
}
