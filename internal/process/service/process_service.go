// Package service implements the process snapshot ingestion business logic
// for Phase 3.4B.2.
package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"vpsmonitoring-backend/internal/process/cache"
	"vpsmonitoring-backend/internal/process/dto"
	serverservice "vpsmonitoring-backend/internal/server/service"
)

const (
	// MaxProcessCount is the maximum number of process entries accepted per
	// agent POST. Payloads with more entries are rejected with 400.
	MaxProcessCount = 500

	// MaxBodyBytes is the maximum permitted request body size (500 KiB).
	// The HTTP handler enforces this before passing the payload here.
	MaxBodyBytes = 500 * 1024
)

var (
	// ErrMissingCollectedAt is returned when collected_at is absent or zero.
	ErrMissingCollectedAt = errors.New("collected_at is required and must be a valid RFC-3339 timestamp")

	// ErrTooManyProcesses is returned when the payload exceeds MaxProcessCount.
	ErrTooManyProcesses = fmt.Errorf("payload exceeds maximum allowed process count of %d", MaxProcessCount)

	// ErrInvalidProcess is returned when a required process field is missing.
	ErrInvalidProcess = errors.New("each process entry must have a valid pid and a non-empty name")

	// ErrSnapshotMissing is returned when no snapshot is found in cache (503).
	ErrSnapshotMissing = &snapshotMissingError{}

	// ErrInvalidSort is returned when the sort parameter is unrecognized.
	ErrInvalidSort = errors.New("invalid sort parameter")

	// ErrInvalidLimit is returned when the limit parameter is out of bounds.
	ErrInvalidLimit = errors.New("limit must be between 1 and 100")

	// ErrInvalidPage is returned when the requested page is not positive.
	ErrInvalidPage = errors.New("page must be greater than or equal to 1")
)

type snapshotMissingError struct{}

func (e *snapshotMissingError) Error() string {
	return "snapshot_missing"
}

func (e *snapshotMissingError) StatusCode() int {
	return 503
}

// ProcessService defines the business logic for agent process snapshot ingestion.
type ProcessService interface {
	// IngestSnapshot validates the payload and stores it in the cache.
	// Returns the number of accepted process entries on success.
	// Returns a descriptive error on validation failure; the cache is never
	// modified when an error occurs.
	IngestSnapshot(ctx context.Context, serverID int64, req dto.IngestProcessSnapshotRequest) (int, error)

	// GetSnapshot returns the sorted and limited processes for a server if owned by userID.
	GetSnapshot(ctx context.Context, serverID, userID int64, page, limit int, sortStr string) (*dto.GetProcessSnapshotResponse, error)
}

type processService struct {
	cache      cache.ProcessCache
	serverServ serverservice.ServerService
}

// NewProcessService creates a ProcessService backed by the given cache and server service.
func NewProcessService(c cache.ProcessCache, s serverservice.ServerService) ProcessService {
	return &processService{cache: c, serverServ: s}
}

// IngestSnapshot validates req and atomically stores the snapshot in the cache.
//
// Validation rules:
//   - collected_at must be non-zero.
//   - processes must be a valid array (empty array is accepted).
//   - len(processes) must be ≤ MaxProcessCount; payloads with more are rejected.
//   - Each process entry must have PID ≥ 0 and a non-empty Name.
//   - Optional fields may be absent/nil.
//
// On ANY validation failure the existing cached snapshot for serverID is
// preserved unchanged.
func (s *processService) IngestSnapshot(ctx context.Context, serverID int64, req dto.IngestProcessSnapshotRequest) (int, error) {
	// 1. Validate collected_at
	if req.CollectedAt.IsZero() {
		return 0, ErrMissingCollectedAt
	}

	// 2. Validate process count limit
	if len(req.Processes) > MaxProcessCount {
		return 0, ErrTooManyProcesses
	}

	// 3. Validate individual process entries
	for i, p := range req.Processes {
		if strings.TrimSpace(p.Name) == "" {
			return 0, fmt.Errorf("%w: entry[%d] (pid=%d) has empty name", ErrInvalidProcess, i, p.PID)
		}
		if p.PID < 0 {
			return 0, fmt.Errorf("%w: entry[%d] has negative pid %d", ErrInvalidProcess, i, p.PID)
		}
	}

	// 4. All validation passed — atomically update the cache.
	// The cache.Set implementation makes a defensive copy of the slice.
	s.cache.Set(serverID, req.CollectedAt.UTC(), req.Processes)

	return len(req.Processes), nil
}

// GetSnapshot retrieves the latest snapshot, checks server ownership, and sorts/limits the result.
func (s *processService) GetSnapshot(ctx context.Context, serverID, userID int64, page, limit int, sortStr string) (*dto.GetProcessSnapshotResponse, error) {
	// 1. Verify ownership
	server, err := s.serverServ.GetServer(ctx, serverID, userID)
	if err != nil {
		if errors.Is(err, serverservice.ErrServerNotFound) {
			// Do not leak existence of other users' servers. Treat as 404 (which GetServer already handles as ErrServerNotFound)
			return nil, err
		}
		return nil, err
	}
	if server == nil {
		return nil, serverservice.ErrServerNotFound
	}

	// 2. Validate Limit
	if limit <= 0 || limit > 100 {
		return nil, ErrInvalidLimit
	}
	if page <= 0 {
		return nil, ErrInvalidPage
	}

	// 3. Validate Sort
	switch sortStr {
	case "cpu_desc", "cpu_asc", "memory_desc", "memory_asc", "pid":
		// valid
	case "":
		sortStr = "cpu_desc" // default
	default:
		return nil, ErrInvalidSort
	}

	// 4. Retrieve from cache
	entry, ok := s.cache.Get(serverID)
	if !ok || entry == nil {
		return nil, ErrSnapshotMissing
	}

	// Make a defensive copy before sorting/limiting to avoid mutating the cached entry
	processes := make([]dto.ProcessSnapshotDTO, len(entry.Processes))
	copy(processes, entry.Processes)

	total := len(processes)
	totalPages := 0
	if total > 0 {
		totalPages = (total + limit - 1) / limit
	}

	// 5. If empty, return immediately
	if len(processes) == 0 {
		return &dto.GetProcessSnapshotResponse{
			CollectedAt: entry.CollectedAt,
			Processes:   []dto.ProcessSnapshotDTO{},
			Total:       0,
			Page:        page,
			PageSize:    limit,
			TotalPages:  0,
		}, nil
	}

	// 6. Sort processes
	sortProcesses(processes, sortStr)

	// 7. Apply the requested page after sorting the complete snapshot.
	start := (page - 1) * limit
	if start >= total {
		return &dto.GetProcessSnapshotResponse{CollectedAt: entry.CollectedAt, Processes: []dto.ProcessSnapshotDTO{}, Total: total, Page: page, PageSize: limit, TotalPages: totalPages}, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	pagedProcesses := processes[start:end]

	return &dto.GetProcessSnapshotResponse{
		CollectedAt: entry.CollectedAt,
		Processes:   pagedProcesses,
		Total:       total,
		Page:        page,
		PageSize:    limit,
		TotalPages:  totalPages,
	}, nil
}

func sortProcesses(p []dto.ProcessSnapshotDTO, sortStr string) {
	sort.SliceStable(p, func(i, j int) bool {
		pi, pj := p[i], p[j]

		switch sortStr {
		case "cpu_desc":
			var cpuI, cpuJ float64
			if pi.CPUPercent != nil {
				cpuI = *pi.CPUPercent
			} else {
				cpuI = -1.0 // missing goes to bottom
			}
			if pj.CPUPercent != nil {
				cpuJ = *pj.CPUPercent
			} else {
				cpuJ = -1.0
			}
			if cpuI != cpuJ {
				return cpuI > cpuJ
			}
		case "cpu_asc":
			var cpuI, cpuJ float64
			if pi.CPUPercent != nil {
				cpuI = *pi.CPUPercent
			} else {
				cpuI = -1.0
			}
			if pj.CPUPercent != nil {
				cpuJ = *pj.CPUPercent
			} else {
				cpuJ = -1.0
			}
			// For asc, missing (-1) will naturally go to the top, which might be desired,
			// or we push it to bottom. We'll push nil to bottom by checking nilness first.
			if pi.CPUPercent == nil && pj.CPUPercent != nil {
				return false // i goes after j
			}
			if pi.CPUPercent != nil && pj.CPUPercent == nil {
				return true // i goes before j
			}
			if cpuI != cpuJ {
				return cpuI < cpuJ
			}
		case "memory_desc":
			var memI, memJ uint64
			if pi.MemoryBytes != nil {
				memI = *pi.MemoryBytes
			}
			if pj.MemoryBytes != nil {
				memJ = *pj.MemoryBytes
			}
			if memI != memJ {
				return memI > memJ
			}
		case "memory_asc":
			var memI, memJ uint64
			if pi.MemoryBytes != nil {
				memI = *pi.MemoryBytes
			}
			if pj.MemoryBytes != nil {
				memJ = *pj.MemoryBytes
			}
			if pi.MemoryBytes == nil && pj.MemoryBytes != nil {
				return false
			}
			if pi.MemoryBytes != nil && pj.MemoryBytes == nil {
				return true
			}
			if memI != memJ {
				return memI < memJ
			}
		case "pid":
			if pi.PID != pj.PID {
				return pi.PID < pj.PID
			}
		}

		// Fallback stable sorting by PID
		return pi.PID < pj.PID
	})
}
