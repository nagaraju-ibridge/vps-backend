package models

import (
	"time"

	"github.com/google/uuid"
)

// Metric represents a single collected snapshot of server metrics.
type Metric struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	ServerID    int64     `gorm:"not null;index" json:"server_id"`
	AgentID     string    `gorm:"type:varchar(100);not null;index" json:"agent_id"`
	CollectedAt time.Time `gorm:"not null;index" json:"collected_at"`
	ReceivedAt  time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"received_at"`

	// CPU
	CPUUsagePercent float64 `gorm:"type:double precision;not null;default:0" json:"cpu_usage_percent"`
	CPUCores        int     `gorm:"type:integer;not null;default:0" json:"cpu_cores"`
	Load1           float64 `gorm:"column:load_1;type:double precision;not null;default:0" json:"load_1"`
	Load5           float64 `gorm:"column:load_5;type:double precision;not null;default:0" json:"load_5"`
	Load15          float64 `gorm:"column:load_15;type:double precision;not null;default:0" json:"load_15"`

	// Memory
	MemoryTotal        uint64  `gorm:"type:bigint;not null;default:0" json:"memory_total"`
	MemoryUsed         uint64  `gorm:"type:bigint;not null;default:0" json:"memory_used"`
	MemoryAvailable    uint64  `gorm:"type:bigint;not null;default:0" json:"memory_available"`
	MemoryUsagePercent float64 `gorm:"type:double precision;not null;default:0" json:"memory_usage_percent"`

	// Swap
	SwapTotal        uint64  `gorm:"type:bigint;not null;default:0" json:"swap_total"`
	SwapUsed         uint64  `gorm:"type:bigint;not null;default:0" json:"swap_used"`
	SwapFree         uint64  `gorm:"type:bigint;not null;default:0" json:"swap_free"`
	SwapUsagePercent float64 `gorm:"type:double precision;not null;default:0" json:"swap_usage_percent"`

	// Network
	NetworkRXBytes   uint64 `gorm:"type:bigint;not null;default:0" json:"network_rx_bytes"`
	NetworkTXBytes   uint64 `gorm:"type:bigint;not null;default:0" json:"network_tx_bytes"`
	NetworkRXPackets uint64 `gorm:"type:bigint;not null;default:0" json:"network_rx_packets"`
	NetworkTXPackets uint64 `gorm:"type:bigint;not null;default:0" json:"network_tx_packets"`
	NetworkErrors    uint64 `gorm:"type:bigint;not null;default:0" json:"network_errors"`
	NetworkDrops     uint64 `gorm:"type:bigint;not null;default:0" json:"network_drops"`

	CreatedAt time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	// Disks relationship
	Disks []MetricDisk `gorm:"foreignKey:MetricID;constraint:OnDelete:CASCADE" json:"disks,omitempty"`
}

func (Metric) TableName() string {
	return "metrics"
}

// MetricDisk represents disk usage on a specific mount point for a metric snapshot.
type MetricDisk struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	MetricID     uuid.UUID `gorm:"type:uuid;not null;index" json:"metric_id"`
	MountPoint   string    `gorm:"type:varchar(255);not null" json:"mount_point"`
	Filesystem   string    `gorm:"type:varchar(100);default:''" json:"filesystem"`
	Total        uint64    `gorm:"type:bigint;not null;default:0" json:"total"`
	Used         uint64    `gorm:"type:bigint;not null;default:0" json:"used"`
	Free         uint64    `gorm:"type:bigint;not null;default:0" json:"free"`
	UsagePercent float64   `gorm:"type:double precision;not null;default:0" json:"usage_percent"`
	CreatedAt    time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

func (MetricDisk) TableName() string {
	return "metric_disks"
}
