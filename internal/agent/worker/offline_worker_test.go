package worker_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	agentDto "vpsmonitoring-backend/internal/agent/dto"
	agentService "vpsmonitoring-backend/internal/agent/service"
	"vpsmonitoring-backend/internal/agent/worker"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

// mockServiceForWorker implements agentService.AgentService
type mockServiceForWorker struct {
	agentService.AgentService
	servers         map[int64]*serverModels.Server
	detectError     error
	detectCallCount int
	mu              sync.Mutex
}

func newMockServiceForWorker() *mockServiceForWorker {
	return &mockServiceForWorker{
		servers: make(map[int64]*serverModels.Server),
	}
}

func (m *mockServiceForWorker) DetectOfflineAgents(ctx context.Context, offlineThreshold time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.detectCallCount++
	if m.detectError != nil {
		return 0, m.detectError
	}

	thresholdTime := time.Now().UTC().Add(-offlineThreshold)
	var count int64
	for _, srv := range m.servers {
		if srv.DeletedAt.Valid {
			continue
		}
		if srv.AgentStatus == serverModels.StatusOnline && srv.LastSeen != nil && srv.LastSeen.Before(thresholdTime) {
			srv.AgentStatus = serverModels.StatusOffline
			count++
		}
	}
	return count, nil
}

func (m *mockServiceForWorker) Heartbeat(ctx context.Context, agentID string, serverID int64, req agentDto.HeartbeatRequest) (*agentDto.HeartbeatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	srv, exists := m.servers[serverID]
	if !exists || srv.DeletedAt.Valid {
		return nil, agentService.ErrServerNotFound
	}

	now := time.Now().UTC()
	srv.LastSeen = &now
	srv.AgentStatus = serverModels.StatusOnline

	return &agentDto.HeartbeatResponse{
		AgentID:  agentID,
		ServerID: serverID,
		Status:   serverModels.StatusOnline,
		LastSeen: now.Format(time.RFC3339),
	}, nil
}

func TestOfflineWorker_Scenarios(t *testing.T) {
	t.Run("TEST 1 - ONLINE server with recent heartbeat remains ONLINE", func(t *testing.T) {
		svc := newMockServiceForWorker()
		recentTime := time.Now().UTC().Add(-20 * time.Second)
		svc.servers[1] = &serverModels.Server{
			ID:          1,
			AgentStatus: serverModels.StatusOnline,
			LastSeen:    &recentTime,
		}

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			CheckInterval:    10 * time.Millisecond,
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 servers marked offline, got %d", count)
		}
		if svc.servers[1].AgentStatus != serverModels.StatusOnline {
			t.Errorf("expected server to remain ONLINE, got %s", svc.servers[1].AgentStatus)
		}
	})

	t.Run("TEST 2 - ONLINE server with stale heartbeat transitions to OFFLINE", func(t *testing.T) {
		svc := newMockServiceForWorker()
		staleTime := time.Now().UTC().Add(-120 * time.Second)
		svc.servers[2] = &serverModels.Server{
			ID:          2,
			AgentStatus: serverModels.StatusOnline,
			LastSeen:    &staleTime,
		}

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 server marked offline, got %d", count)
		}
		if svc.servers[2].AgentStatus != serverModels.StatusOffline {
			t.Errorf("expected server to become OFFLINE, got %s", svc.servers[2].AgentStatus)
		}
	})

	t.Run("TEST 3 - OFFLINE server remains OFFLINE without repeated update", func(t *testing.T) {
		svc := newMockServiceForWorker()
		staleTime := time.Now().UTC().Add(-200 * time.Second)
		svc.servers[3] = &serverModels.Server{
			ID:          3,
			AgentStatus: serverModels.StatusOffline,
			LastSeen:    &staleTime,
		}

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 servers marked offline on already offline server, got %d", count)
		}
		if svc.servers[3].AgentStatus != serverModels.StatusOffline {
			t.Errorf("expected server to remain OFFLINE, got %s", svc.servers[3].AgentStatus)
		}
	})

	t.Run("TEST 4 - Boundary condition (89s vs 91s)", func(t *testing.T) {
		svc := newMockServiceForWorker()
		now := time.Now().UTC()
		time89 := now.Add(-89 * time.Second)
		time91 := now.Add(-91 * time.Second)

		svc.servers[10] = &serverModels.Server{
			ID:          10,
			AgentStatus: serverModels.StatusOnline,
			LastSeen:    &time89,
		}
		svc.servers[11] = &serverModels.Server{
			ID:          11,
			AgentStatus: serverModels.StatusOnline,
			LastSeen:    &time91,
		}

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 1 {
			t.Errorf("expected exactly 1 server marked offline at boundary, got %d", count)
		}
		if svc.servers[10].AgentStatus != serverModels.StatusOnline {
			t.Errorf("server 10 (89s) should remain ONLINE, got %s", svc.servers[10].AgentStatus)
		}
		if svc.servers[11].AgentStatus != serverModels.StatusOffline {
			t.Errorf("server 11 (91s) should become OFFLINE, got %s", svc.servers[11].AgentStatus)
		}
	})

	t.Run("TEST 5 - Deleted server is not modified", func(t *testing.T) {
		svc := newMockServiceForWorker()
		staleTime := time.Now().UTC().Add(-150 * time.Second)
		svc.servers[5] = &serverModels.Server{
			ID:          5,
			AgentStatus: serverModels.StatusOnline,
			LastSeen:    &staleTime,
			DeletedAt:   gorm.DeletedAt{Time: time.Now().UTC(), Valid: true},
		}

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 deleted servers modified, got %d", count)
		}
		if svc.servers[5].AgentStatus != serverModels.StatusOnline {
			t.Errorf("deleted server should not be modified, got %s", svc.servers[5].AgentStatus)
		}
	})

	t.Run("TEST 6 - Multiple agents with mixed statuses", func(t *testing.T) {
		svc := newMockServiceForWorker()
		now := time.Now().UTC()
		recent := now.Add(-10 * time.Second)
		stale := now.Add(-120 * time.Second)

		svc.servers[100] = &serverModels.Server{ID: 100, AgentStatus: serverModels.StatusOnline, LastSeen: &recent}
		svc.servers[101] = &serverModels.Server{ID: 101, AgentStatus: serverModels.StatusOnline, LastSeen: &stale}
		svc.servers[102] = &serverModels.Server{ID: 102, AgentStatus: serverModels.StatusOffline, LastSeen: &stale}

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 server marked offline, got %d", count)
		}
		if svc.servers[100].AgentStatus != serverModels.StatusOnline {
			t.Errorf("server 100 should remain ONLINE, got %s", svc.servers[100].AgentStatus)
		}
		if svc.servers[101].AgentStatus != serverModels.StatusOffline {
			t.Errorf("server 101 should become OFFLINE, got %s", svc.servers[101].AgentStatus)
		}
		if svc.servers[102].AgentStatus != serverModels.StatusOffline {
			t.Errorf("server 102 should remain OFFLINE, got %s", svc.servers[102].AgentStatus)
		}
	})

	t.Run("TEST 7 - Recovery from OFFLINE to ONLINE through heartbeat", func(t *testing.T) {
		svc := newMockServiceForWorker()
		staleTime := time.Now().UTC().Add(-200 * time.Second)
		svc.servers[200] = &serverModels.Server{
			ID:          200,
			AgentStatus: serverModels.StatusOffline,
			LastSeen:    &staleTime,
		}

		// Agent sends heartbeat
		resp, err := svc.Heartbeat(context.Background(), "agent-recovery-200", 200, agentDto.HeartbeatRequest{})
		if err != nil {
			t.Fatalf("expected nil error on heartbeat, got %v", err)
		}
		if resp.Status != serverModels.StatusOnline {
			t.Errorf("expected status ONLINE in heartbeat response, got %s", resp.Status)
		}
		if svc.servers[200].AgentStatus != serverModels.StatusOnline {
			t.Errorf("expected server 200 status to be restored to ONLINE, got %s", svc.servers[200].AgentStatus)
		}
	})

	t.Run("TEST 8 - Worker database error handled safely without crash", func(t *testing.T) {
		svc := newMockServiceForWorker()
		svc.detectError = errors.New("simulated database connectivity failure")

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			OfflineThreshold: 90 * time.Second,
		})

		count, err := w.CheckOffline(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if count != 0 {
			t.Errorf("expected 0 count on error, got %d", count)
		}
		// Worker remains safe and functional
	})

	t.Run("TEST 9 - Worker lifecycle startup and graceful shutdown", func(t *testing.T) {
		svc := newMockServiceForWorker()

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			CheckInterval:    10 * time.Millisecond,
			OfflineThreshold: 90 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		w.Start(ctx)

		// Wait briefly for at least one tick
		time.Sleep(35 * time.Millisecond)

		// Graceful shutdown via Stop
		w.Stop()
		cancel()

		svc.mu.Lock()
		calls := svc.detectCallCount
		svc.mu.Unlock()

		if calls < 1 {
			t.Errorf("expected at least 1 check call during ticker run, got %d", calls)
		}
	})

	t.Run("TEST 9b - Worker cancellation via Context", func(t *testing.T) {
		svc := newMockServiceForWorker()

		w := worker.NewOfflineWorker(svc, &worker.OfflineWorkerConfig{
			CheckInterval:    10 * time.Millisecond,
			OfflineThreshold: 90 * time.Second,
		})

		ctx, cancel := context.WithCancel(context.Background())
		w.Start(ctx)

		time.Sleep(20 * time.Millisecond)
		cancel()

		// Allow worker to finish via ctx.Done()
		time.Sleep(20 * time.Millisecond)
	})
}
