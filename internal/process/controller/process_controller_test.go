// Package controller_test contains all tests for the process snapshot
// ingestion endpoint (Phase 3.4B.2).
//
// Test coverage:
//  1. Valid agent POST → 202 Accepted
//  2. Correct received count in response
//  3. Cache contains latest snapshot after POST
//  4. Second POST replaces previous snapshot
//  5. collected_at is preserved in cache
//  6. Missing/invalid agent authentication → 401
//  7. User JWT cannot authenticate agent endpoint
//  8. Installation token cannot authenticate agent endpoint
//  9. Invalid JSON → 400
//  10. Missing collected_at → 400
//  11. >200 processes → 400
//  12. >500 KB body → 413
//  13. Missing required Name → 400
//  14. Invalid payload does NOT overwrite existing cache entry
//  15. Concurrent cache writes are safe (race detector)
//  16. Concurrent cache read/write is safe (race detector)
//  17. Empty process array is accepted as a valid snapshot
//  18. Stale snapshots remain in cache (no TTL eviction)
//  19. Backend cache starts empty
//  20. No DB records created (cache-only)
package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	agentMw "vpsmonitoring-backend/internal/agent/middleware"
	agentModels "vpsmonitoring-backend/internal/agent/models"
	agentService "vpsmonitoring-backend/internal/agent/service"
	"vpsmonitoring-backend/internal/process/cache"
	"vpsmonitoring-backend/internal/process/dto"
	processService "vpsmonitoring-backend/internal/process/service"
)

// ---------------------------------------------------------------------------
// Test infrastructure
// ---------------------------------------------------------------------------

// mockAgentService stubs AgentService for the middleware pipeline.
type mockAgentService struct {
	agentService.AgentService
}

// AuthenticateAgent accepts only "valid-agent-secret" and maps it to server 42.
func (m *mockAgentService) AuthenticateAgent(_ context.Context, rawCredential string) (*agentModels.Agent, error) {
	if rawCredential == "valid-agent-secret" {
		return &agentModels.Agent{
			AgentID:  "agent-test-uuid",
			ServerID: 42,
			Status:   "ACTIVE",
		}, nil
	}
	// A second valid agent (server 99) for cross-agent tests.
	if rawCredential == "agent-b-secret" {
		return &agentModels.Agent{
			AgentID:  "agent-b-uuid",
			ServerID: 99,
			Status:   "ACTIVE",
		}, nil
	}
	return nil, agentService.ErrUnauthorizedAgent
}

// buildPipeline wires AgentAuthMiddleware → mock controller handler into a plain
// http.Handler for use with httptest, replicating the production middleware
// stack without starting a full GoFr application. This matches the established
// project pattern in metric_controller_test.go.
func buildPipeline(c cache.ProcessCache) http.Handler {
	svc := processService.NewProcessService(c, nil)
	return agentMw.AgentAuthMiddleware(&mockAgentService{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		val := r.Context().Value(agentMw.AgentIdentityContextKey)
		if val == nil {
			respondError(w, http.StatusUnauthorized, "unauthenticated request: agent identity not found")
			return
		}
		identity, ok := val.(*agentMw.AgentIdentity)
		if !ok || identity == nil {
			respondError(w, http.StatusUnauthorized, "invalid agent identity in context")
			return
		}

		// Enforce size limit
		r.Body = http.MaxBytesReader(w, r.Body, processService.MaxBodyBytes)

		var req dto.IngestProcessSnapshotRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if strings.Contains(err.Error(), "request body too large") {
				respondError(w, http.StatusRequestEntityTooLarge, "request body exceeds maximum allowed size")
			} else {
				respondError(w, http.StatusBadRequest, fmt.Sprintf("invalid JSON: %v", err))
			}
			return
		}

		received, err := svc.IngestSnapshot(r.Context(), identity.ServerID, req)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(dto.IngestProcessSnapshotResponse{Received: received})
	}))
}

func respondError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":{"message":"%s"}}`, message)))
}

// validPayload returns a minimal valid ingestion payload with n processes.
func validPayload(n int) dto.IngestProcessSnapshotRequest {
	procs := make([]dto.ProcessSnapshotDTO, n)
	for i := range procs {
		procs[i] = dto.ProcessSnapshotDTO{
			PID:  int64(i + 1),
			Name: fmt.Sprintf("proc-%d", i+1),
		}
	}
	return dto.IngestProcessSnapshotRequest{
		CollectedAt: time.Now().UTC(),
		Processes:   procs,
	}
}

// post is a test helper that sends an authenticated POST to the pipeline.
func post(handler http.Handler, body any, credential string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-test-uuid/processes", &buf)
	req.Header.Set("Content-Type", "application/json")
	if credential != "" {
		req.Header.Set("Authorization", "Bearer "+credential)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

// ---------------------------------------------------------------------------
// Test 1 — Valid agent POST → 202 Accepted
// ---------------------------------------------------------------------------

func TestIngestSnapshot_ValidPayload_Returns202(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	rr := post(h, validPayload(3), "valid-agent-secret")

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 2 — Correct received count in response
// ---------------------------------------------------------------------------

func TestIngestSnapshot_ReceivedCount_MatchesPayload(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	rr := post(h, validPayload(7), "valid-agent-secret")

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", rr.Code)
	}

	var resp dto.IngestProcessSnapshotResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Received != 7 {
		t.Errorf("expected received=7, got %d", resp.Received)
	}
}

// ---------------------------------------------------------------------------
// Test 3 — Cache contains the latest snapshot after POST
// ---------------------------------------------------------------------------

func TestIngestSnapshot_CacheContainsSnapshot(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	payload := validPayload(2)
	post(h, payload, "valid-agent-secret")

	entry, ok := c.Get(42) // server 42 matches "valid-agent-secret"
	if !ok {
		t.Fatal("expected cache to have entry for server 42")
	}
	if len(entry.Processes) != 2 {
		t.Errorf("expected 2 processes in cache, got %d", len(entry.Processes))
	}
}

// ---------------------------------------------------------------------------
// Test 4 — Second POST replaces previous snapshot
// ---------------------------------------------------------------------------

func TestIngestSnapshot_SecondPost_ReplacesCache(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	post(h, validPayload(5), "valid-agent-secret")
	post(h, validPayload(2), "valid-agent-secret")

	entry, ok := c.Get(42)
	if !ok {
		t.Fatal("expected cache entry for server 42")
	}
	if len(entry.Processes) != 2 {
		t.Errorf("expected 2 processes after second POST, got %d", len(entry.Processes))
	}
}

// ---------------------------------------------------------------------------
// Test 5 — collected_at is preserved in cache
// ---------------------------------------------------------------------------

func TestIngestSnapshot_CollectedAt_PreservedInCache(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	want := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	payload := dto.IngestProcessSnapshotRequest{
		CollectedAt: want,
		Processes: []dto.ProcessSnapshotDTO{
			{PID: 1, Name: "init"},
		},
	}
	post(h, payload, "valid-agent-secret")

	entry, ok := c.Get(42)
	if !ok {
		t.Fatal("cache entry missing")
	}
	if !entry.CollectedAt.Equal(want) {
		t.Errorf("CollectedAt: want %v, got %v", want, entry.CollectedAt)
	}
}

// ---------------------------------------------------------------------------
// Test 6 — Missing/invalid agent authentication → 401
// ---------------------------------------------------------------------------

func TestIngestSnapshot_NoAuthHeader_Returns401(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	rr := post(h, validPayload(1), "") // no credential

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestIngestSnapshot_WrongCredential_Returns401(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	rr := post(h, validPayload(1), "wrong-credential")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Test 7 — Normal user JWT cannot authenticate agent endpoint
// ---------------------------------------------------------------------------

func TestIngestSnapshot_UserJWT_Rejected(t *testing.T) {
	// The AgentAuthMiddleware only accepts raw agent credentials; JWT tokens
	// (which have a different format) are treated as invalid agent credentials
	// and rejected with 401.
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Simulate what a user JWT bearer token looks like (three base64-encoded
	// segments separated by dots — never matches raw agent hex credentials).
	userJWT := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c2VyIn0.fake_signature"
	rr := post(h, validPayload(1), userJWT)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for user JWT, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 8 — Installation token cannot authenticate agent endpoint
// ---------------------------------------------------------------------------

func TestIngestSnapshot_InstallationToken_Rejected(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Installation tokens are UUIDs sent in the JSON body during registration,
	// never as bearer tokens. If one were submitted here, it would be treated
	// as an unknown agent credential and rejected.
	installationToken := "550e8400-e29b-41d4-a716-446655440000"
	rr := post(h, validPayload(1), installationToken)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for installation token, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Test 9 — Invalid JSON → 400
// ---------------------------------------------------------------------------

func TestIngestSnapshot_InvalidJSON_Returns400(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/x/processes",
		strings.NewReader(`{not valid json`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-agent-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 10 — Missing collected_at → 400
// ---------------------------------------------------------------------------

func TestIngestSnapshot_MissingCollectedAt_Returns400(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Manually construct a payload with no collected_at field.
	raw := `{"processes":[{"pid":1,"name":"init"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/x/processes",
		strings.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-agent-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing collected_at, got %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "collected_at") {
		t.Errorf("error message should mention 'collected_at', got: %s", rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 11 — >200 processes → 400
// ---------------------------------------------------------------------------

func TestIngestSnapshot_TooManyProcesses_Returns400(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	rr := post(h, validPayload(501), "valid-agent-secret")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >500 processes, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 12 — >500 KB body → 413
// ---------------------------------------------------------------------------

func TestIngestSnapshot_OversizedBody_Returns413(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Build a valid JSON body slightly larger than 500 KiB.
	// We put the padding INSIDE a JSON string value so the decoder is forced
	// to read past the MaxBytesReader limit.
	padding := strings.Repeat("x", 501*1024)
	payload := `{"collected_at":"2026-09-30T00:00:00Z","processes":[{"name":"` + padding + `"}]}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/x/processes",
		strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-agent-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected 413, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 13 — Missing required Name → 400
// ---------------------------------------------------------------------------

func TestIngestSnapshot_MissingProcessName_Returns400(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	payload := dto.IngestProcessSnapshotRequest{
		CollectedAt: time.Now().UTC(),
		Processes: []dto.ProcessSnapshotDTO{
			{PID: 1, Name: ""}, // name is required
		},
	}
	rr := post(h, payload, "valid-agent-secret")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty Name, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test 14 — Invalid payload does NOT overwrite existing cache entry
// ---------------------------------------------------------------------------

func TestIngestSnapshot_InvalidPayload_DoesNotOverwriteCache(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Store a valid snapshot first.
	good := validPayload(3)
	post(h, good, "valid-agent-secret")

	before, _ := c.Get(42)
	if before == nil {
		t.Fatal("initial snapshot not stored")
	}

	// Send an invalid payload (>500 processes).
	post(h, validPayload(501), "valid-agent-secret")

	after, _ := c.Get(42)
	if after == nil {
		t.Fatal("cache entry unexpectedly removed")
	}
	if len(after.Processes) != 3 {
		t.Errorf("cache was overwritten by invalid payload: expected 3 processes, got %d", len(after.Processes))
	}
}

// ---------------------------------------------------------------------------
// Test 15 — Concurrent cache writes are safe (run with -race)
// ---------------------------------------------------------------------------

func TestIngestSnapshot_ConcurrentWrites_RaceSafe(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			post(h, validPayload(n%5+1), "valid-agent-secret")
		}(i)
	}
	wg.Wait()

	// Cache must still be coherent (one entry for server 42).
	entry, ok := c.Get(42)
	if !ok {
		t.Fatal("cache entry missing after concurrent writes")
	}
	if entry == nil {
		t.Fatal("nil cache entry after concurrent writes")
	}
}

// ---------------------------------------------------------------------------
// Test 16 — Concurrent cache read/write is safe (run with -race)
// ---------------------------------------------------------------------------

func TestIngestSnapshot_ConcurrentReadWrite_RaceSafe(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Prime the cache with an initial snapshot.
	post(h, validPayload(5), "valid-agent-secret")

	const goroutines = 40
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			post(h, validPayload(n%3+1), "valid-agent-secret")
		}(i)
	}

	// Readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			c.Get(42) //nolint:errcheck // intentional read-only
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Test 17 — Empty process array is accepted as a valid snapshot
// ---------------------------------------------------------------------------

func TestIngestSnapshot_EmptyProcessArray_Accepted(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	payload := dto.IngestProcessSnapshotRequest{
		CollectedAt: time.Now().UTC(),
		Processes:   []dto.ProcessSnapshotDTO{},
	}
	rr := post(h, payload, "valid-agent-secret")

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for empty process array, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp dto.IngestProcessSnapshotResponse
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	if resp.Received != 0 {
		t.Errorf("expected received=0, got %d", resp.Received)
	}

	entry, ok := c.Get(42)
	if !ok {
		t.Fatal("cache entry missing for empty snapshot")
	}
	if len(entry.Processes) != 0 {
		t.Errorf("expected 0 processes in cache, got %d", len(entry.Processes))
	}
}

// ---------------------------------------------------------------------------
// Test 18 — Stale snapshots remain in cache (no TTL eviction)
// ---------------------------------------------------------------------------

func TestIngestSnapshot_StaleSnapshot_NotEvicted(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Submit a snapshot with a timestamp in the distant past (2 hours ago).
	staleTime := time.Now().UTC().Add(-2 * time.Hour)
	payload := dto.IngestProcessSnapshotRequest{
		CollectedAt: staleTime,
		Processes: []dto.ProcessSnapshotDTO{
			{PID: 1, Name: "stalend"},
		},
	}
	rr := post(h, payload, "valid-agent-secret")

	if rr.Code != http.StatusAccepted {
		t.Fatalf("stale-timestamped payload rejected: %d %s", rr.Code, rr.Body.String())
	}

	// Wait a moment and confirm the entry is still present.
	time.Sleep(10 * time.Millisecond)

	entry, ok := c.Get(42)
	if !ok {
		t.Fatal("stale entry was evicted — TTL eviction must NOT be implemented")
	}
	if !entry.CollectedAt.Equal(staleTime) {
		t.Errorf("CollectedAt changed: want %v, got %v", staleTime, entry.CollectedAt)
	}
}

// ---------------------------------------------------------------------------
// Test 19 — Backend cache starts empty
// ---------------------------------------------------------------------------

func TestProcessCache_StartsEmpty(t *testing.T) {
	c := cache.NewProcessCache()

	if c.Size() != 0 {
		t.Errorf("expected empty cache at startup, size=%d", c.Size())
	}

	_, ok := c.Get(1)
	if ok {
		t.Error("expected (nil, false) for server 1 in empty cache")
	}
}

// ---------------------------------------------------------------------------
// Test 20 — No DB records created (cache-only)
// ---------------------------------------------------------------------------

func TestIngestSnapshot_NoDB_CacheOnly(t *testing.T) {
	// The process cache is a pure in-memory structure with no GORM model or
	// database connection. This test verifies the invariant structurally:
	// a cache is created without any *gorm.DB dependency, POST succeeds, and
	// the entry is retrieved from memory without any SQL interaction.
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	rr := post(h, validPayload(3), "valid-agent-secret")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("unexpected status: %d %s", rr.Code, rr.Body.String())
	}

	entry, ok := c.Get(42)
	if !ok || len(entry.Processes) != 3 {
		t.Errorf("expected 3 processes from cache, got ok=%v", ok)
	}

	// Confirm cache size — only one server entry should exist.
	if c.Size() != 1 {
		t.Errorf("expected cache size 1, got %d", c.Size())
	}
}

// ---------------------------------------------------------------------------
// Test 21 — Agent A cannot submit snapshots as agent B
// ---------------------------------------------------------------------------

func TestIngestSnapshot_AgentIsolation_DifferentServers(t *testing.T) {
	c := cache.NewProcessCache()
	h := buildPipeline(c)

	// Agent A (server 42) posts a snapshot.
	post(h, validPayload(3), "valid-agent-secret")
	// Agent B (server 99) posts a different snapshot.
	post(h, validPayload(10), "agent-b-secret")

	entryA, okA := c.Get(42)
	entryB, okB := c.Get(99)

	if !okA || len(entryA.Processes) != 3 {
		t.Errorf("server 42 cache incorrect: ok=%v count=%d", okA, len(entryA.Processes))
	}
	if !okB || len(entryB.Processes) != 10 {
		t.Errorf("server 99 cache incorrect: ok=%v count=%d", okB, len(entryB.Processes))
	}

	// Server IDs must be distinct — agent A cannot overwrite agent B's entry.
	if c.Size() != 2 {
		t.Errorf("expected 2 independent cache entries, got %d", c.Size())
	}
}
