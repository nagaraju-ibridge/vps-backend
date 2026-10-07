package service

import (
	"sync"
	"time"

	"vpsmonitoring-backend/internal/discovery/dto"
)

type DiscoverySnapshot struct {
	ServerID     int64
	CollectedAt  time.Time
	Applications []dto.ApplicationCandidateDTO
	Services     []dto.ServiceCandidateDTO
	Listeners    []dto.ListenerDTO
	Warnings     []string
}

type DiscoveryCache interface {
	SetIfNotStale(serverID int64, snapshot DiscoverySnapshot) bool
	Get(serverID int64) (*DiscoverySnapshot, bool)
	Size() int
}

type discoveryCache struct {
	mu    sync.RWMutex
	store map[int64]DiscoverySnapshot
}

func NewDiscoveryCache() DiscoveryCache {
	return &discoveryCache{store: make(map[int64]DiscoverySnapshot)}
}

func (c *discoveryCache) SetIfNotStale(serverID int64, snapshot DiscoverySnapshot) bool {
	snapshot.ServerID = serverID
	snapshot.CollectedAt = snapshot.CollectedAt.UTC()
	snapshot.Applications = copyApplications(snapshot.Applications)
	snapshot.Services = copyServices(snapshot.Services)
	snapshot.Listeners = copyListeners(snapshot.Listeners)
	snapshot.Warnings = append([]string(nil), snapshot.Warnings...)

	c.mu.Lock()
	defer c.mu.Unlock()

	current, ok := c.store[serverID]
	if ok && snapshot.CollectedAt.Before(current.CollectedAt) {
		return false
	}

	c.store[serverID] = snapshot
	return true
}

func (c *discoveryCache) Get(serverID int64) (*DiscoverySnapshot, bool) {
	c.mu.RLock()
	snapshot, ok := c.store[serverID]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	snapshot.Applications = copyApplications(snapshot.Applications)
	snapshot.Services = copyServices(snapshot.Services)
	snapshot.Listeners = copyListeners(snapshot.Listeners)
	snapshot.Warnings = append([]string(nil), snapshot.Warnings...)
	return &snapshot, true
}

func (c *discoveryCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.store)
}

func copyApplications(in []dto.ApplicationCandidateDTO) []dto.ApplicationCandidateDTO {
	out := make([]dto.ApplicationCandidateDTO, len(in))
	copy(out, in)
	for i := range out {
		out[i].Ports = copyListeners(out[i].Ports)
		out[i].Process.Listeners = copyListeners(out[i].Process.Listeners)
		out[i].Evidence = append([]dto.EvidenceDTO(nil), out[i].Evidence...)
	}
	return out
}

func copyServices(in []dto.ServiceCandidateDTO) []dto.ServiceCandidateDTO {
	out := make([]dto.ServiceCandidateDTO, len(in))
	copy(out, in)
	for i := range out {
		out[i].Ports = copyListeners(out[i].Ports)
		out[i].Process.Listeners = copyListeners(out[i].Process.Listeners)
		out[i].Evidence = append([]dto.EvidenceDTO(nil), out[i].Evidence...)
	}
	return out
}

func copyListeners(in []dto.ListenerDTO) []dto.ListenerDTO {
	out := make([]dto.ListenerDTO, len(in))
	copy(out, in)
	return out
}
