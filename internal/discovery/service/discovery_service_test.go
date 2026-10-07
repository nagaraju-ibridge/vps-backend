package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"vpsmonitoring-backend/internal/discovery/dto"
	serverDTO "vpsmonitoring-backend/internal/server/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
)

func TestDiscoveryServiceIngestValidPayload(t *testing.T) {
	cache := NewDiscoveryCache()
	svc := newTestService(cache)
	payload := validPayload()

	received, err := svc.IngestDiscovery(context.Background(), 42, payload)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if received.Applications != 1 || received.Services != 1 || received.Listeners != 1 {
		t.Fatalf("unexpected counts: %+v", received)
	}

	snapshot, ok := cache.Get(42)
	if !ok {
		t.Fatal("expected cached snapshot")
	}
	if snapshot.ServerID != 42 {
		t.Fatalf("expected authenticated server id 42, got %d", snapshot.ServerID)
	}
}

func TestDiscoveryServiceValidationFailures(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*dto.DiscoveryPayloadDTO)
	}{
		{"missing timestamp", func(p *dto.DiscoveryPayloadDTO) { p.CollectedAt = time.Time{} }},
		{"future timestamp", func(p *dto.DiscoveryPayloadDTO) {
			p.CollectedAt = time.Date(2026, 10, 1, 12, 10, 0, 0, time.UTC)
		}},
		{"invalid runtime", func(p *dto.DiscoveryPayloadDTO) { p.Applications[0].Runtime = "rails" }},
		{"invalid framework", func(p *dto.DiscoveryPayloadDTO) { p.Applications[0].Framework = "made_up" }},
		{"invalid confidence", func(p *dto.DiscoveryPayloadDTO) { p.Applications[0].RuntimeConfidence = "CERTAIN" }},
		{"invalid port", func(p *dto.DiscoveryPayloadDTO) { p.Listeners[0].LocalPort = 70000 }},
		{"invalid pid", func(p *dto.DiscoveryPayloadDTO) { p.Applications[0].Process.PID = -1 }},
		{"invalid address", func(p *dto.DiscoveryPayloadDTO) { p.Listeners[0].LocalAddress = "not-an-ip" }},
		{"invalid scope", func(p *dto.DiscoveryPayloadDTO) { p.Listeners[0].Scope = "internet-ish" }},
		{"excessive applications", func(p *dto.DiscoveryPayloadDTO) {
			p.Applications = make([]dto.ApplicationCandidateDTO, MaxApplications+1)
		}},
		{"excessive listeners", func(p *dto.DiscoveryPayloadDTO) { p.Listeners = make([]dto.ListenerDTO, MaxListeners+1) }},
		{"excessive services", func(p *dto.DiscoveryPayloadDTO) { p.Services = make([]dto.ServiceCandidateDTO, MaxServices+1) }},
		{"excessive warnings", func(p *dto.DiscoveryPayloadDTO) { p.Warnings = make([]string, MaxWarnings+1) }},
		{"excessive evidence", func(p *dto.DiscoveryPayloadDTO) {
			p.Applications[0].Evidence = make([]dto.EvidenceDTO, MaxEvidencePerItem+1)
		}},
		{"excessive command line", func(p *dto.DiscoveryPayloadDTO) {
			p.Applications[0].Process.CmdlineRedacted = strings.Repeat("a", MaxCommandLineLength+1)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewDiscoveryCache()
			svc := newTestService(cache)
			payload := validPayload()
			tt.mutate(&payload)

			_, err := svc.IngestDiscovery(context.Background(), 42, payload)
			if !errors.Is(err, ErrInvalidPayload) {
				t.Fatalf("expected ErrInvalidPayload, got %v", err)
			}
			if cache.Size() != 0 {
				t.Fatal("expected validation failure not to mutate cache")
			}
		})
	}
}

func TestDiscoveryCacheStaleEqualAndEmptySnapshots(t *testing.T) {
	cache := NewDiscoveryCache()
	svc := newTestService(cache)
	first := validPayload()
	first.CollectedAt = time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)

	if _, err := svc.IngestDiscovery(context.Background(), 7, first); err != nil {
		t.Fatalf("ingest first: %v", err)
	}

	stale := validPayload()
	stale.CollectedAt = first.CollectedAt.Add(-time.Minute)
	if _, err := svc.IngestDiscovery(context.Background(), 7, stale); !errors.Is(err, ErrStaleSnapshot) {
		t.Fatalf("expected stale error, got %v", err)
	}

	equal := validPayload()
	equal.CollectedAt = first.CollectedAt
	equal.Applications[0].ID = "pid:999"
	if _, err := svc.IngestDiscovery(context.Background(), 7, equal); err != nil {
		t.Fatalf("equal timestamp should replace, got %v", err)
	}
	snapshot, _ := cache.Get(7)
	if snapshot.Applications[0].ID != "pid:999" {
		t.Fatalf("expected equal timestamp replacement, got %+v", snapshot.Applications)
	}

	empty := dto.DiscoveryPayloadDTO{CollectedAt: first.CollectedAt.Add(time.Minute)}
	if _, err := svc.IngestDiscovery(context.Background(), 7, empty); err != nil {
		t.Fatalf("empty valid snapshot should replace, got %v", err)
	}
	snapshot, _ = cache.Get(7)
	if len(snapshot.Applications) != 0 || len(snapshot.Services) != 0 || len(snapshot.Listeners) != 0 {
		t.Fatalf("expected empty snapshot replacement, got %+v", snapshot)
	}
}

func TestDiscoveryCacheMissAndConcurrentAccess(t *testing.T) {
	cache := NewDiscoveryCache()
	if _, ok := cache.Get(404); ok {
		t.Fatal("expected cache miss")
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			payload := validPayload()
			payload.CollectedAt = payload.CollectedAt.Add(time.Duration(i) * time.Second)
			_ = cache.SetIfNotStale(int64(i%3), DiscoverySnapshot{
				CollectedAt:  payload.CollectedAt,
				Applications: payload.Applications,
				Services:     payload.Services,
				Listeners:    payload.Listeners,
				Warnings:     payload.Warnings,
			})
			_, _ = cache.Get(int64(i % 3))
		}(i)
	}
	wg.Wait()

	if cache.Size() != 3 {
		t.Fatalf("expected three bounded server entries, got %d", cache.Size())
	}
}

func TestDiscoveryPayloadCannotSpoofServerIdentity(t *testing.T) {
	cache := NewDiscoveryCache()
	svc := newTestService(cache)
	payload := validPayload()
	payload.Applications[0].ID = "server:999-pid:1"

	if _, err := svc.IngestDiscovery(context.Background(), 123, payload); err != nil {
		t.Fatalf("expected ingest success, got %v", err)
	}
	if _, ok := cache.Get(999); ok {
		t.Fatal("payload content must not create server 999 cache entry")
	}
	if snapshot, ok := cache.Get(123); !ok || snapshot.ServerID != 123 {
		t.Fatalf("expected authenticated server id 123, got %+v", snapshot)
	}
}

func TestDiscoveryValidationRejectsPotentialLogInjection(t *testing.T) {
	cache := NewDiscoveryCache()
	svc := newTestService(cache)
	payload := validPayload()
	payload.Warnings = []string{"safe", "unsafe\nsecret"}

	_, err := svc.IngestDiscovery(context.Background(), 42, payload)
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected invalid payload, got %v", err)
	}
}

func TestDiscoveryServiceGetDiscoveryOwnerCacheMissAndSnapshot(t *testing.T) {
	cache := NewDiscoveryCache()
	servers := &fakeServerService{servers: map[[2]int64]*serverDTO.ServerResponse{
		{42, 7}: {ID: 42, UserID: 7, Name: "server"},
	}}
	svc := newTestServiceWithServer(cache, servers)

	resp, err := svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("expected cache miss success, got %v", err)
	}
	if resp.ServerID != 42 || resp.SnapshotAvailable {
		t.Fatalf("unexpected cache miss response: %+v", resp)
	}
	if len(resp.Applications) != 0 || len(resp.Services) != 0 || len(resp.Listeners) != 0 {
		t.Fatalf("expected empty cache miss arrays, got %+v", resp)
	}

	payload := validPayload()
	if _, err := svc.IngestDiscovery(context.Background(), 42, payload); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	resp, err = svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("expected snapshot success, got %v", err)
	}
	if !resp.SnapshotAvailable || resp.CollectedAt == nil || !resp.CollectedAt.Equal(payload.CollectedAt) {
		t.Fatalf("unexpected snapshot metadata: %+v", resp)
	}
	if len(resp.Applications) != 1 || len(resp.Services) != 1 || len(resp.Listeners) != 1 {
		t.Fatalf("unexpected snapshot counts: %+v", resp)
	}
	if resp.Applications[0].RuntimeConfidence != dto.ConfidenceHigh ||
		resp.Applications[0].Evidence[0].Value != "node" ||
		resp.Applications[0].Process.CmdlineRedacted != "node server.js --token REDACTED" {
		t.Fatalf("expected confidence/evidence/redacted command preserved, got %+v", resp.Applications[0])
	}
}

func TestDiscoveryServiceGetDiscoveryOwnershipFailures(t *testing.T) {
	cache := NewDiscoveryCache()
	servers := &fakeServerService{servers: map[[2]int64]*serverDTO.ServerResponse{
		{42, 7}: {ID: 42, UserID: 7, Name: "server"},
	}}
	svc := newTestServiceWithServer(cache, servers)

	if _, err := svc.GetDiscovery(context.Background(), 42, 8); !errors.Is(err, serverService.ErrServerNotFound) {
		t.Fatalf("expected cross-user not found, got %v", err)
	}
	if _, err := svc.GetDiscovery(context.Background(), 999, 7); !errors.Is(err, serverService.ErrServerNotFound) {
		t.Fatalf("expected nonexistent not found, got %v", err)
	}

	servers.err = errors.New("database down")
	if _, err := svc.GetDiscovery(context.Background(), 42, 7); err == nil || err.Error() != "database down" {
		t.Fatalf("expected ownership lookup failure, got %v", err)
	}
}

func TestDiscoveryServiceGetDiscoveryEmptySnapshotAndDefensiveCopy(t *testing.T) {
	cache := NewDiscoveryCache()
	servers := &fakeServerService{servers: map[[2]int64]*serverDTO.ServerResponse{
		{42, 7}: {ID: 42, UserID: 7, Name: "server"},
	}}
	svc := newTestServiceWithServer(cache, servers)

	empty := dto.DiscoveryPayloadDTO{CollectedAt: time.Date(2026, 10, 1, 11, 30, 0, 0, time.UTC)}
	if _, err := svc.IngestDiscovery(context.Background(), 42, empty); err != nil {
		t.Fatalf("ingest empty: %v", err)
	}
	resp, err := svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("get empty: %v", err)
	}
	if !resp.SnapshotAvailable || len(resp.Applications) != 0 || len(resp.Services) != 0 || len(resp.Listeners) != 0 {
		t.Fatalf("unexpected empty snapshot response: %+v", resp)
	}

	payload := validPayload()
	payload.CollectedAt = empty.CollectedAt.Add(time.Minute)
	if _, err := svc.IngestDiscovery(context.Background(), 42, payload); err != nil {
		t.Fatalf("ingest payload: %v", err)
	}
	resp, err = svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("get payload: %v", err)
	}
	resp.Applications[0].Runtime = "mutated"
	resp.Listeners[0].LocalPort = 1

	again, err := svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("get again: %v", err)
	}
	if again.Applications[0].Runtime == "mutated" || again.Listeners[0].LocalPort == 1 {
		t.Fatalf("response mutation changed cache: %+v", again)
	}
}

func TestDiscoveryServiceGetDiscoveryDoesNotServeRejectedStaleSnapshot(t *testing.T) {
	cache := NewDiscoveryCache()
	servers := &fakeServerService{servers: map[[2]int64]*serverDTO.ServerResponse{
		{42, 7}: {ID: 42, UserID: 7, Name: "server"},
	}}
	svc := newTestServiceWithServer(cache, servers)

	newer := validPayload()
	newer.CollectedAt = time.Date(2026, 10, 1, 11, 30, 0, 0, time.UTC)
	if _, err := svc.IngestDiscovery(context.Background(), 42, newer); err != nil {
		t.Fatalf("ingest newer: %v", err)
	}
	stale := validPayload()
	stale.CollectedAt = newer.CollectedAt.Add(-time.Hour)
	stale.Applications[0].Runtime = "python"
	if _, err := svc.IngestDiscovery(context.Background(), 42, stale); !errors.Is(err, ErrStaleSnapshot) {
		t.Fatalf("expected stale rejection, got %v", err)
	}

	resp, err := svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if resp.Applications[0].Runtime != "nodejs" {
		t.Fatalf("stale snapshot was served: %+v", resp.Applications[0])
	}
}

func TestDiscoveryServiceGetDiscoveryNoCredentialLeakageOrSpoofing(t *testing.T) {
	cache := NewDiscoveryCache()
	servers := &fakeServerService{servers: map[[2]int64]*serverDTO.ServerResponse{
		{42, 7}: {ID: 42, UserID: 7, Name: "server"},
	}}
	svc := newTestServiceWithServer(cache, servers)
	payload := validPayload()
	payload.Applications[0].Process.CmdlineRedacted = "node server.js --password REDACTED"
	payload.Applications[0].ID = "user_id=999"

	if _, err := svc.IngestDiscovery(context.Background(), 42, payload); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	resp, err := svc.GetDiscovery(context.Background(), 42, 7)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	serialized := resp.Applications[0].Process.CmdlineRedacted + " " + strings.Join(resp.Warnings, " ")
	if strings.Contains(serialized, "secret") || strings.Contains(serialized, "credential") {
		t.Fatalf("response leaked secret-like content: %q", serialized)
	}
	if resp.ServerID != 42 {
		t.Fatalf("server id spoofed, got %d", resp.ServerID)
	}
}

func newTestService(cache DiscoveryCache) DiscoveryService {
	return newTestServiceWithServer(cache, &fakeServerService{})
}

func newTestServiceWithServer(cache DiscoveryCache, serverSvc serverService.ServerService) DiscoveryService {
	return &discoveryService{
		cache:     cache,
		serverSvc: serverSvc,
		now: func() time.Time {
			return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		},
	}
}

type fakeServerService struct {
	serverService.ServerService
	servers map[[2]int64]*serverDTO.ServerResponse
	err     error
}

func (s *fakeServerService) GetServer(ctx context.Context, id int64, userID int64) (*serverDTO.ServerResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	if srv, ok := s.servers[[2]int64{id, userID}]; ok {
		return srv, nil
	}
	return nil, serverService.ErrServerNotFound
}

func validPayload() dto.DiscoveryPayloadDTO {
	pid := int64(101)
	collectedAt := time.Date(2026, 10, 1, 11, 0, 0, 0, time.UTC)
	listener := dto.ListenerDTO{
		Protocol:     "tcp",
		LocalAddress: "127.0.0.1",
		LocalPort:    3000,
		Scope:        "loopback",
		PID:          &pid,
		ProcessName:  "node",
	}
	process := dto.ProcessMetadataDTO{
		PID:             pid,
		Name:            "node",
		ExePath:         "/usr/bin/node",
		CmdlineRedacted: "node server.js --token REDACTED",
		User:            "app",
		Listeners:       []dto.ListenerDTO{listener},
	}
	return dto.DiscoveryPayloadDTO{
		CollectedAt: collectedAt,
		Applications: []dto.ApplicationCandidateDTO{{
			ID:                  "pid:101",
			Runtime:             "nodejs",
			RuntimeConfidence:   dto.ConfidenceHigh,
			Framework:           "unknown",
			FrameworkConfidence: dto.ConfidenceUnknown,
			Process:             process,
			Ports:               []dto.ListenerDTO{listener},
			Evidence:            []dto.EvidenceDTO{{Kind: "process_name", Value: "node"}},
		}},
		Services: []dto.ServiceCandidateDTO{{
			ID:         "pid:202",
			Kind:       "nginx",
			Confidence: dto.ConfidenceHigh,
			Process: dto.ProcessMetadataDTO{
				PID:  202,
				Name: "nginx",
			},
			Evidence: []dto.EvidenceDTO{{Kind: "service_name", Value: "nginx"}},
		}},
		Listeners: []dto.ListenerDTO{listener},
		Warnings:  []string{"permission denied for process metadata"},
	}
}
