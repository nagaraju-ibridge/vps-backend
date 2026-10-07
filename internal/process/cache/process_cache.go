// Package cache provides the thread-safe in-memory latest-process-snapshot
// store for Phase 3.4B.2.
//
// Design decisions (Phase 3.4A / 3.4A.1 / 3.4A.2):
//
//   - One entry per server, keyed by server ID.
//   - Each entry holds the full process list and the collected_at timestamp.
//   - Entries are overwritten atomically on every successful agent POST.
//   - Stale entries (older than the 60-second freshness window) are NOT evicted.
//     The 60-second threshold is a client-side freshness interpretation only;
//     the cache retains all received snapshots until replaced.
//   - No snapshot entry → the cache returns (nil, false); callers should respond
//     with 503 Service Unavailable.
//   - Backend restart → cache is empty; returns (nil, false) for all servers.
//   - No database persistence, no external cache, no background eviction job.
//   - Concurrent writes (multiple agent goroutines) and concurrent reads
//     (future user-facing GET handler) are safe via sync.RWMutex.
package cache

import (
	"sync"
	"time"

	"vpsmonitoring-backend/internal/process/dto"
)

// ProcessCacheEntry holds one server's latest process snapshot.
type ProcessCacheEntry struct {
	// CollectedAt is the UTC timestamp reported by the agent.
	CollectedAt time.Time

	// Processes is a defensive copy of the agent-submitted snapshot.
	Processes []dto.ProcessSnapshotDTO
}

// ProcessCache is a thread-safe in-memory store of the latest process snapshot
// per server.
type ProcessCache interface {
	// Set atomically replaces the snapshot for the given server ID.
	// processes must be a pre-validated, pre-copied slice.
	Set(serverID int64, collectedAt time.Time, processes []dto.ProcessSnapshotDTO)

	// Get returns the latest snapshot for serverID.
	// Returns (entry, true) when a snapshot exists; (nil, false) otherwise.
	Get(serverID int64) (*ProcessCacheEntry, bool)

	// Size returns the current number of server entries in the cache.
	// Used for diagnostics and tests.
	Size() int
}

type processCache struct {
	mu    sync.RWMutex
	store map[int64]*ProcessCacheEntry
}

// NewProcessCache creates a new, empty ProcessCache.
func NewProcessCache() ProcessCache {
	return &processCache{
		store: make(map[int64]*ProcessCacheEntry),
	}
}

// Set atomically replaces the snapshot for serverID.
// A defensive copy of processes is stored so the caller cannot mutate the cache
// after this call.
func (c *processCache) Set(serverID int64, collectedAt time.Time, processes []dto.ProcessSnapshotDTO) {
	// Make a defensive copy before acquiring the lock to keep the critical
	// section as short as possible.
	snapshot := make([]dto.ProcessSnapshotDTO, len(processes))
	copy(snapshot, processes)

	c.mu.Lock()
	c.store[serverID] = &ProcessCacheEntry{
		CollectedAt: collectedAt,
		Processes:   snapshot,
	}
	c.mu.Unlock()
}

// Get returns the latest snapshot for serverID, or (nil, false) when absent.
func (c *processCache) Get(serverID int64) (*ProcessCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.store[serverID]
	c.mu.RUnlock()
	return entry, ok
}

// Size returns the number of server entries currently in the cache.
func (c *processCache) Size() int {
	c.mu.RLock()
	n := len(c.store)
	c.mu.RUnlock()
	return n
}
