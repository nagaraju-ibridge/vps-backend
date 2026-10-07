package cache

import (
	"sync"
	"time"

	"vpsmonitoring-backend/internal/systemd/dto"
)

type SystemdCacheEntry struct {
	CollectedAt time.Time
	Services    []dto.ServiceSnapshotDTO
}

type SystemdCache interface {
	Set(serverID int64, collectedAt time.Time, services []dto.ServiceSnapshotDTO)
	Get(serverID int64) (*SystemdCacheEntry, bool)
	Size() int
}

type systemdCache struct {
	mu    sync.RWMutex
	store map[int64]*SystemdCacheEntry
}

func NewSystemdCache() SystemdCache {
	return &systemdCache{
		store: make(map[int64]*SystemdCacheEntry),
	}
}

func (c *systemdCache) Set(serverID int64, collectedAt time.Time, services []dto.ServiceSnapshotDTO) {
	snapshot := make([]dto.ServiceSnapshotDTO, len(services))
	copy(snapshot, services)

	c.mu.Lock()
	c.store[serverID] = &SystemdCacheEntry{
		CollectedAt: collectedAt,
		Services:    snapshot,
	}
	c.mu.Unlock()
}

func (c *systemdCache) Get(serverID int64) (*SystemdCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.store[serverID]
	c.mu.RUnlock()
	return entry, ok
}

func (c *systemdCache) Size() int {
	c.mu.RLock()
	n := len(c.store)
	c.mu.RUnlock()
	return n
}
