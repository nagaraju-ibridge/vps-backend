// Package dto defines the request and response data transfer objects for the
// process snapshot ingestion endpoint (Phase 3.4B.2).
//
// Security contract:
//   - Only the approved fields are parsed.  No cmdline, environ, open files,
//     sockets, passwords, .env contents, source code, SSH keys, or database
//     credentials are accepted.
//   - Optional fields use pointer types so an absent JSON key becomes nil
//     rather than a zero value.
package dto

import "time"

// IngestProcessSnapshotRequest is the payload the agent sends to
// POST /api/v1/agents/{agentId}/processes.
//
// collected_at must be a valid RFC-3339 timestamp.
// processes must contain at most MaxProcessCount entries.
type IngestProcessSnapshotRequest struct {
	// CollectedAt is the UTC timestamp at which the agent started collection.
	CollectedAt time.Time `json:"collected_at"`

	// Processes is the list of process snapshots (at most 200).
	Processes []ProcessSnapshotDTO `json:"processes"`
}

// ProcessSnapshotDTO represents a single read-only view of one running process
// as reported by the agent.
//
// Required fields: PID, Name.
// All other fields are optional (pointer types); absent fields are nil.
type ProcessSnapshotDTO struct {
	// PID is the OS process identifier. Required.
	PID int64 `json:"pid"`

	// ParentPID is the PID of the parent process. Nil when unavailable.
	ParentPID *int64 `json:"parent_pid,omitempty"`

	// Name is the process executable name (e.g. "nginx"). Required.
	Name string `json:"name"`

	// CPUPercent is the raw CPU usage %. May exceed 100 on multi-core hosts.
	// Nil when unavailable. Values are never clamped.
	CPUPercent *float64 `json:"cpu_percent,omitempty"`

	// MemoryBytes is the Resident Set Size in bytes. Nil when unavailable.
	MemoryBytes *uint64 `json:"memory_bytes,omitempty"`

	// MemoryPercent is (RSS / total RAM) * 100. Nil when unavailable.
	MemoryPercent *float64 `json:"memory_percent,omitempty"`

	// Status is the process state string (e.g. "R", "S"). Nil when unavailable.
	Status *string `json:"status,omitempty"`

	// StartTime is the UTC process creation timestamp. Nil when unavailable.
	StartTime *time.Time `json:"start_time,omitempty"`

	// UptimeSeconds is seconds elapsed since StartTime. Nil when unavailable.
	UptimeSeconds *int64 `json:"uptime_seconds,omitempty"`

	// Threads is the thread count. Nil when unavailable.
	Threads *int32 `json:"threads,omitempty"`

	// User is the operating-system user owning the process (e.g. "vocc", "mysql").
	User string `json:"user,omitempty"`

	// VirtBytes is the virtual memory size in bytes (VIRT in top). Nil when unavailable.
	VirtBytes *uint64 `json:"virt_bytes,omitempty"`

	// Cmdline is the process executable command (e.g. "php-fpm8.2", "mariadbd").
	Cmdline string `json:"cmdline,omitempty"`
}

// IngestProcessSnapshotResponse is returned on a successful 202 Accepted.
type IngestProcessSnapshotResponse struct {
	// Received is the count of accepted process entries stored in the cache.
	Received int `json:"received"`
}

// GetProcessSnapshotResponse is returned by GET /api/v1/user/servers/{serverId}/processes.
type GetProcessSnapshotResponse struct {
	// CollectedAt is the UTC timestamp at which the agent started collection.
	CollectedAt time.Time `json:"collected_at"`

	// Processes is the list of process snapshots (at most 100 per API limit).
	Processes []ProcessSnapshotDTO `json:"processes"`

	// Pagination metadata is calculated after sorting the full cached snapshot.
	Total      int `json:"total"`
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	TotalPages int `json:"total_pages"`
}
