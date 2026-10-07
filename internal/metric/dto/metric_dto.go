package dto

import (
	"errors"
	"fmt"
	"time"
)

// IngestMetricRequest represents the JSON payload sent by the agent to POST /api/v1/agent/metrics.
// Field names deliberately match models.MetricSnapshot from the agent to minimize translation friction.
// Notice: server_id, user_id, and agent_id are strictly excluded to enforce tenant isolation.
type IngestMetricRequest struct {
	Timestamp time.Time         `json:"timestamp"`
	CPU       CPUMetricsDTO     `json:"cpu"`
	Memory    MemoryMetricsDTO  `json:"memory"`
	Swap      SwapMetricsDTO    `json:"swap"`
	Disk      DiskMetricsDTO    `json:"disk"`
	Network   NetworkMetricsDTO `json:"network"`
}

type CPUMetricsDTO struct {
	UsagePercent float64 `json:"usage_percent"`
	Cores        int     `json:"cores"`
	Load1        float64 `json:"load_1"`
	Load5        float64 `json:"load_5"`
	Load15       float64 `json:"load_15"`
}

type MemoryMetricsDTO struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Available    uint64  `json:"available"`
	UsagePercent float64 `json:"usage_percent"`
}

type SwapMetricsDTO struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usage_percent"`
}

type MountPointDTO struct {
	Path         string  `json:"path"`
	FSType       string  `json:"fs_type"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usage_percent"`
}

type DiskMetricsDTO struct {
	MountPoints []MountPointDTO `json:"mount_points"`
}

type NetworkMetricsDTO struct {
	RXBytes   uint64 `json:"rx_bytes"`
	TXBytes   uint64 `json:"tx_bytes"`
	RXPackets uint64 `json:"rx_packets"`
	TXPackets uint64 `json:"tx_packets"`
	Errors    uint64 `json:"errors"`
	Drops     uint64 `json:"drops"`
}

// IngestMetricResponse represents the response returned upon successful ingestion.
type IngestMetricResponse struct {
	Success    bool   `json:"success"`
	MetricID   string `json:"metric_id"`
	ServerID   int64  `json:"server_id"`
	ReceivedAt string `json:"received_at"`
}

// LatestMetricResponse is a user-facing response for the newest metric snapshot of a server.
type LatestMetricResponse struct {
	ServerID    int64                `json:"server_id"`
	CollectedAt string               `json:"collected_at"`
	CPU         LatestCPUMetricsDTO  `json:"cpu"`
	Load        LatestLoadMetricsDTO `json:"load"`
	Memory      MemoryMetricsDTO     `json:"memory"`
	Swap        SwapMetricsDTO       `json:"swap"`
	Network     NetworkMetricsDTO    `json:"network"`
	Disks       []DiskMetricResponse `json:"disks"`
}

// HistoricalMetricsResponse is a user-facing response for bounded historical metric snapshots.
type HistoricalMetricsResponse struct {
	ServerID int64                   `json:"server_id"`
	Start    string                  `json:"start"`
	End      string                  `json:"end"`
	Interval string                  `json:"interval,omitempty"`
	Data     []HistoricalMetricPoint `json:"data"`
}

// HistoricalMetricPoint represents one historical metric snapshot.
type HistoricalMetricPoint struct {
	CollectedAt string               `json:"collected_at"`
	CPU         LatestCPUMetricsDTO  `json:"cpu"`
	Load        LatestLoadMetricsDTO `json:"load"`
	Memory      MemoryMetricsDTO     `json:"memory"`
	Swap        SwapMetricsDTO       `json:"swap"`
	Network     NetworkMetricsDTO    `json:"network"`
}

// AggregatedMetricsResponse is a user-facing response for bounded bucketed metric aggregates.
type AggregatedMetricsResponse struct {
	ServerID    int64                   `json:"server_id"`
	Start       string                  `json:"start"`
	End         string                  `json:"end"`
	Granularity string                  `json:"granularity"`
	Data        []AggregatedMetricPoint `json:"data"`
}

// AggregatedMetricPoint represents one generated time bucket. Aggregate fields are
// pointers so empty buckets serialize as null values instead of fabricated zeroes.
type AggregatedMetricPoint struct {
	BucketStart string                   `json:"bucket_start"`
	CPU         AggregatedCPUMetrics     `json:"cpu"`
	Load        AggregatedLoadMetrics    `json:"load"`
	Memory      AggregatedMemoryMetrics  `json:"memory"`
	Swap        AggregatedSwapMetrics    `json:"swap"`
	Network     AggregatedNetworkMetrics `json:"network"`
	Disks       []AggregatedDiskMetrics  `json:"disks"`
}

type AggregatedCPUMetrics struct {
	AverageUsagePercent *float64 `json:"average_usage_percent"`
	MinUsagePercent     *float64 `json:"min_usage_percent"`
	MaxUsagePercent     *float64 `json:"max_usage_percent"`
	P95UsagePercent     *float64 `json:"p95_usage_percent"`
	AverageCores        *float64 `json:"average_cores"`
}

type AggregatedLoadMetrics struct {
	AverageLoad1  *float64 `json:"average_load_1"`
	MinLoad1      *float64 `json:"min_load_1"`
	MaxLoad1      *float64 `json:"max_load_1"`
	AverageLoad5  *float64 `json:"average_load_5"`
	MinLoad5      *float64 `json:"min_load_5"`
	MaxLoad5      *float64 `json:"max_load_5"`
	AverageLoad15 *float64 `json:"average_load_15"`
	MinLoad15     *float64 `json:"min_load_15"`
	MaxLoad15     *float64 `json:"max_load_15"`
}

type AggregatedMemoryMetrics struct {
	AverageUsagePercent *float64 `json:"average_usage_percent"`
	MinUsagePercent     *float64 `json:"min_usage_percent"`
	MaxUsagePercent     *float64 `json:"max_usage_percent"`
	P95UsagePercent     *float64 `json:"p95_usage_percent"`
	AverageTotal        *float64 `json:"average_total"`
	AverageUsed         *float64 `json:"average_used"`
	AverageAvailable    *float64 `json:"average_available"`
}

type AggregatedSwapMetrics struct {
	AverageUsagePercent *float64 `json:"average_usage_percent"`
	MinUsagePercent     *float64 `json:"min_usage_percent"`
	MaxUsagePercent     *float64 `json:"max_usage_percent"`
	AverageTotal        *float64 `json:"average_total"`
	AverageUsed         *float64 `json:"average_used"`
	AverageFree         *float64 `json:"average_free"`
}

type AggregatedNetworkMetrics struct {
	RXBytesDelta     *float64 `json:"rx_bytes_delta"`
	TXBytesDelta     *float64 `json:"tx_bytes_delta"`
	RXPacketsDelta   *float64 `json:"rx_packets_delta"`
	TXPacketsDelta   *float64 `json:"tx_packets_delta"`
	AverageRXBytes   *float64 `json:"average_rx_bytes"`
	MinRXBytes       *float64 `json:"min_rx_bytes"`
	MaxRXBytes       *float64 `json:"max_rx_bytes"`
	AverageTXBytes   *float64 `json:"average_tx_bytes"`
	MinTXBytes       *float64 `json:"min_tx_bytes"`
	MaxTXBytes       *float64 `json:"max_tx_bytes"`
	AverageRXPackets *float64 `json:"average_rx_packets"`
	MinRXPackets     *float64 `json:"min_rx_packets"`
	MaxRXPackets     *float64 `json:"max_rx_packets"`
	AverageTXPackets *float64 `json:"average_tx_packets"`
	MinTXPackets     *float64 `json:"min_tx_packets"`
	MaxTXPackets     *float64 `json:"max_tx_packets"`
	AverageErrors    *float64 `json:"average_errors"`
	MinErrors        *float64 `json:"min_errors"`
	MaxErrors        *float64 `json:"max_errors"`
	AverageDrops     *float64 `json:"average_drops"`
	MinDrops         *float64 `json:"min_drops"`
	MaxDrops         *float64 `json:"max_drops"`
}

type AggregatedDiskMetrics struct {
	MountPoint          string   `json:"mount_point"`
	AverageUsagePercent *float64 `json:"average_usage_percent"`
	MinUsagePercent     *float64 `json:"min_usage_percent"`
	MaxUsagePercent     *float64 `json:"max_usage_percent"`
	AverageTotal        *float64 `json:"average_total"`
	AverageUsed         *float64 `json:"average_used"`
	AverageFree         *float64 `json:"average_free"`
}

type LatestCPUMetricsDTO struct {
	UsagePercent float64 `json:"usage_percent"`
	Cores        int     `json:"cores"`
}

type LatestLoadMetricsDTO struct {
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
}

type DiskMetricResponse struct {
	MountPoint   string  `json:"mount_point"`
	Filesystem   string  `json:"filesystem"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usage_percent"`
}

// Validate validates the incoming metric payload against business rules.
func (r *IngestMetricRequest) Validate() error {
	if r.Timestamp.IsZero() {
		return errors.New("timestamp is required")
	}

	// Reject unrealistic timestamps (more than 24h in the past or 1h in future)
	now := time.Now().UTC()
	if r.Timestamp.After(now.Add(1*time.Hour)) || r.Timestamp.Before(now.Add(-24*time.Hour)) {
		return errors.New("timestamp is out of acceptable time range")
	}

	// CPU validation
	if r.CPU.UsagePercent < 0 || r.CPU.UsagePercent > 100 {
		return fmt.Errorf("invalid cpu usage_percent: %f, must be between 0 and 100", r.CPU.UsagePercent)
	}
	if r.CPU.Cores < 0 {
		return fmt.Errorf("invalid cpu cores: %d, must be >= 0", r.CPU.Cores)
	}
	if r.CPU.Load1 < 0 || r.CPU.Load5 < 0 || r.CPU.Load15 < 0 {
		return errors.New("cpu load averages cannot be negative")
	}

	// Memory validation
	if r.Memory.UsagePercent < 0 || r.Memory.UsagePercent > 100 {
		return fmt.Errorf("invalid memory usage_percent: %f, must be between 0 and 100", r.Memory.UsagePercent)
	}

	// Swap validation (Total == 0 is acceptable when swap is disabled)
	if r.Swap.UsagePercent < 0 || r.Swap.UsagePercent > 100 {
		return fmt.Errorf("invalid swap usage_percent: %f, must be between 0 and 100", r.Swap.UsagePercent)
	}

	// Disk validation
	for i, mp := range r.Disk.MountPoints {
		if mp.Path == "" {
			return fmt.Errorf("mount point path at index %d cannot be empty", i)
		}
		if mp.UsagePercent < 0 || mp.UsagePercent > 100 {
			return fmt.Errorf("invalid disk usage_percent for mount %s: %f, must be between 0 and 100", mp.Path, mp.UsagePercent)
		}
	}

	return nil
}
