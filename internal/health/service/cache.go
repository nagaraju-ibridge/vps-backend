package service

import (
	"sync"

	"vpsmonitoring-backend/internal/health/dto"
)

// HealthResultCache stores the latest health results for each server and config.
// It is thread-safe.
type HealthResultCache struct {
	mu sync.RWMutex
	// map of server_id -> map of config_id -> result
	store map[int64]map[int64]dto.HealthCheckResultDTO
}

// NewHealthResultCache creates a new in-memory cache.
func NewHealthResultCache() *HealthResultCache {
	return &HealthResultCache{
		store: make(map[int64]map[int64]dto.HealthCheckResultDTO),
	}
}

// Set stores a single result in the cache safely, avoiding stale overwrites.
func (c *HealthResultCache) Set(serverID int64, result dto.HealthCheckResultDTO) {
	c.mu.Lock()
	defer c.mu.Unlock()

	serverStore, exists := c.store[serverID]
	if !exists {
		serverStore = make(map[int64]dto.HealthCheckResultDTO)
		c.store[serverID] = serverStore
	}

	// Avoid overwriting a newer result with a stale one
	if existing, ok := serverStore[result.ConfigID]; ok {
		if result.CollectedAt.Before(existing.CollectedAt) {
			return
		}
	}

	serverStore[result.ConfigID] = result
}

// GetByServerID returns a list of the latest results for the given server.
func (c *HealthResultCache) GetByServerID(serverID int64) []dto.HealthCheckResultDTO {
	c.mu.RLock()
	defer c.mu.RUnlock()

	serverStore, exists := c.store[serverID]
	if !exists {
		return []dto.HealthCheckResultDTO{}
	}

	results := make([]dto.HealthCheckResultDTO, 0, len(serverStore))
	for _, res := range serverStore {
		results = append(results, res)
	}

	return results
}
