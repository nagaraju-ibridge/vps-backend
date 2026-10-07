package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"vpsmonitoring-backend/internal/agent/service"
)

const (
	// DefaultHeartbeatInterval is the expected cadence for agents to send heartbeats (30 seconds)
	DefaultHeartbeatInterval = 30 * time.Second

	// DefaultOfflineThreshold is the duration of silence before an agent is marked OFFLINE (90 seconds)
	DefaultOfflineThreshold = 90 * time.Second

	// DefaultWorkerInterval is how frequently the background worker checks for stale agents (30 seconds)
	DefaultWorkerInterval = 30 * time.Second
)

// OfflineWorkerConfig configures the offline detection worker execution parameters
type OfflineWorkerConfig struct {
	CheckInterval    time.Duration
	OfflineThreshold time.Duration
}

// OfflineWorker periodically evaluates online servers and marks stale ones as offline
type OfflineWorker struct {
	agentService     service.AgentService
	checkInterval    time.Duration
	offlineThreshold time.Duration
	stopChan         chan struct{}
	wg               sync.WaitGroup
}

// NewOfflineWorker initializes an OfflineWorker instance
func NewOfflineWorker(agentService service.AgentService, cfg *OfflineWorkerConfig) *OfflineWorker {
	checkInterval := DefaultWorkerInterval
	offlineThreshold := DefaultOfflineThreshold

	if cfg != nil {
		if cfg.CheckInterval > 0 {
			checkInterval = cfg.CheckInterval
		}
		if cfg.OfflineThreshold > 0 {
			offlineThreshold = cfg.OfflineThreshold
		}
	}

	return &OfflineWorker{
		agentService:     agentService,
		checkInterval:    checkInterval,
		offlineThreshold: offlineThreshold,
		stopChan:         make(chan struct{}),
	}
}

// Start begins periodic offline detection in a background goroutine
func (w *OfflineWorker) Start(ctx context.Context) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		log.Println("[INFO] offline detection worker started")

		ticker := time.NewTicker(w.checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[INFO] offline detection worker stopping via context cancellation")
				return
			case <-w.stopChan:
				log.Println("[INFO] offline detection worker stopping via Stop signal")
				return
			case <-ticker.C:
				w.CheckOffline(ctx)
			}
		}
	}()
}

// CheckOffline executes a single offline detection cycle
func (w *OfflineWorker) CheckOffline(ctx context.Context) (int64, error) {
	count, err := w.agentService.DetectOfflineAgents(ctx, w.offlineThreshold)
	if err != nil {
		log.Printf("[ERROR] failed to check offline agents: %v", err)
		return 0, err
	}

	if count > 0 {
		log.Printf("[INFO] detected stale agent/server: marked %d servers OFFLINE", count)
	}

	return count, nil
}

// Stop gracefully signals the background worker to stop and waits for completion
func (w *OfflineWorker) Stop() {
	close(w.stopChan)
	w.wg.Wait()
	log.Println("[INFO] offline detection worker stopped")
}
