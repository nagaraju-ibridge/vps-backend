package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/metric/models"
)

// MetricRepository defines operations for metric persistence.
type MetricRepository interface {
	AutoMigrate() error
	CreateMetric(ctx context.Context, metric *models.Metric, disks []models.MetricDisk) error
	GetLatestByServerID(ctx context.Context, serverID int64) (*models.Metric, error)
	GetHistoryByServerID(ctx context.Context, serverID int64, start time.Time, end time.Time, limit int) ([]models.Metric, error)
	GetAggregatedMetrics(ctx context.Context, serverID int64, start time.Time, end time.Time, granularity string) ([]AggregatedMetricRow, []AggregatedDiskRow, error)
}

type metricRepository struct {
	db *gorm.DB
}

// AggregatedMetricRow contains one generated bucket and nullable aggregates.
type AggregatedMetricRow struct {
	BucketStart time.Time
	CPUAvg      sql.NullFloat64
	CPUMin      sql.NullFloat64
	CPUMax      sql.NullFloat64
	CPUP95      sql.NullFloat64
	CPUCoresAvg sql.NullFloat64

	Load1Avg  sql.NullFloat64
	Load1Min  sql.NullFloat64
	Load1Max  sql.NullFloat64
	Load5Avg  sql.NullFloat64
	Load5Min  sql.NullFloat64
	Load5Max  sql.NullFloat64
	Load15Avg sql.NullFloat64
	Load15Min sql.NullFloat64
	Load15Max sql.NullFloat64

	MemoryUsageAvg     sql.NullFloat64
	MemoryUsageMin     sql.NullFloat64
	MemoryUsageMax     sql.NullFloat64
	MemoryUsageP95     sql.NullFloat64
	MemoryTotalAvg     sql.NullFloat64
	MemoryUsedAvg      sql.NullFloat64
	MemoryAvailableAvg sql.NullFloat64

	SwapUsageAvg sql.NullFloat64
	SwapUsageMin sql.NullFloat64
	SwapUsageMax sql.NullFloat64
	SwapTotalAvg sql.NullFloat64
	SwapUsedAvg  sql.NullFloat64
	SwapFreeAvg  sql.NullFloat64

	NetworkRXBytesDelta   sql.NullFloat64
	NetworkTXBytesDelta   sql.NullFloat64
	NetworkRXPacketsDelta sql.NullFloat64
	NetworkTXPacketsDelta sql.NullFloat64
	NetworkRXBytesAvg     sql.NullFloat64
	NetworkRXBytesMin     sql.NullFloat64
	NetworkRXBytesMax     sql.NullFloat64
	NetworkTXBytesAvg     sql.NullFloat64
	NetworkTXBytesMin     sql.NullFloat64
	NetworkTXBytesMax     sql.NullFloat64
	NetworkRXPacketsAvg   sql.NullFloat64
	NetworkRXPacketsMin   sql.NullFloat64
	NetworkRXPacketsMax   sql.NullFloat64
	NetworkTXPacketsAvg   sql.NullFloat64
	NetworkTXPacketsMin   sql.NullFloat64
	NetworkTXPacketsMax   sql.NullFloat64
	NetworkErrorsAvg      sql.NullFloat64
	NetworkErrorsMin      sql.NullFloat64
	NetworkErrorsMax      sql.NullFloat64
	NetworkDropsAvg       sql.NullFloat64
	NetworkDropsMin       sql.NullFloat64
	NetworkDropsMax       sql.NullFloat64
}

// AggregatedDiskRow contains one bucket and mount-point aggregate.
type AggregatedDiskRow struct {
	BucketStart time.Time
	MountPoint  string
	UsageAvg    sql.NullFloat64
	UsageMin    sql.NullFloat64
	UsageMax    sql.NullFloat64
	TotalAvg    sql.NullFloat64
	UsedAvg     sql.NullFloat64
	FreeAvg     sql.NullFloat64
}

// NewMetricRepository instantiates a new MetricRepository.
func NewMetricRepository(db *gorm.DB) MetricRepository {
	return &metricRepository{db: db}
}

// AutoMigrate migrates metrics and metric_disks schema tables.
func (r *metricRepository) AutoMigrate() error {
	if r.db == nil {
		return errors.New("database connection is nil")
	}
	return r.db.AutoMigrate(&models.Metric{}, &models.MetricDisk{})
}

// CreateMetric inserts a parent Metric and child MetricDisk rows transactionally.
func (r *metricRepository) CreateMetric(ctx context.Context, metric *models.Metric, disks []models.MetricDisk) error {
	if r.db == nil {
		return errors.New("database connection is nil")
	}
	if metric == nil {
		return errors.New("metric cannot be nil")
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Assign ID if empty
		if metric.ID == uuid.Nil {
			metric.ID = uuid.New()
		}

		// 2. Insert parent Metric row
		if err := tx.Create(metric).Error; err != nil {
			return err
		}

		// 3. Insert child MetricDisk rows if any
		if len(disks) > 0 {
			for i := range disks {
				if disks[i].ID == uuid.Nil {
					disks[i].ID = uuid.New()
				}
				disks[i].MetricID = metric.ID
			}

			if err := tx.Create(&disks).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// GetLatestByServerID retrieves the most recent metric snapshot for a server with its disk records.
func (r *metricRepository) GetLatestByServerID(ctx context.Context, serverID int64) (*models.Metric, error) {
	if r.db == nil {
		return nil, errors.New("database connection is nil")
	}

	var metric models.Metric
	err := r.db.WithContext(ctx).
		Preload("Disks").
		Where("server_id = ?", serverID).
		Order("collected_at DESC").
		First(&metric).Error

	if err != nil {
		return nil, err
	}

	return &metric, nil
}

// GetHistoryByServerID retrieves bounded historical metric snapshots for a server in chronological order.
func (r *metricRepository) GetHistoryByServerID(ctx context.Context, serverID int64, start time.Time, end time.Time, limit int) ([]models.Metric, error) {
	if r.db == nil {
		return nil, errors.New("database connection is nil")
	}

	var metrics []models.Metric
	err := r.db.WithContext(ctx).
		Where("server_id = ? AND collected_at >= ? AND collected_at <= ?", serverID, start, end).
		Order("collected_at ASC").
		Limit(limit).
		Find(&metrics).Error

	if err != nil {
		return nil, err
	}

	return metrics, nil
}

// GetAggregatedMetrics retrieves bucketed aggregates for metrics and disks.
func (r *metricRepository) GetAggregatedMetrics(ctx context.Context, serverID int64, start time.Time, end time.Time, granularity string) ([]AggregatedMetricRow, []AggregatedDiskRow, error) {
	if r.db == nil {
		return nil, nil, errors.New("database connection is nil")
	}
	if start.Equal(end) {
		return []AggregatedMetricRow{}, []AggregatedDiskRow{}, nil
	}

	interval := aggregationInterval(granularity)
	metricSQL := `
WITH buckets AS (
	SELECT generate_series($2::timestamptz, $3::timestamptz - interval '1 microsecond', $4::interval) AS bucket_start
),
bucket_metrics AS (
	SELECT
		b.bucket_start,
		m.*
	FROM buckets b
	LEFT JOIN metrics m
		ON m.server_id = $1
		AND m.collected_at >= b.bucket_start
		AND m.collected_at < b.bucket_start + $4::interval
)
SELECT
	bucket_start,
	AVG(cpu_usage_percent), MIN(cpu_usage_percent), MAX(cpu_usage_percent), percentile_cont(0.95) WITHIN GROUP (ORDER BY cpu_usage_percent), AVG(cpu_cores),
	AVG(load_1), MIN(load_1), MAX(load_1), AVG(load_5), MIN(load_5), MAX(load_5), AVG(load_15), MIN(load_15), MAX(load_15),
	AVG(memory_usage_percent), MIN(memory_usage_percent), MAX(memory_usage_percent), percentile_cont(0.95) WITHIN GROUP (ORDER BY memory_usage_percent), AVG(memory_total), AVG(memory_used), AVG(memory_available),
	AVG(swap_usage_percent), MIN(swap_usage_percent), MAX(swap_usage_percent), AVG(swap_total), AVG(swap_used), AVG(swap_free),
	GREATEST((MAX(network_rx_bytes) - MIN(network_rx_bytes))::double precision, 0), GREATEST((MAX(network_tx_bytes) - MIN(network_tx_bytes))::double precision, 0),
	GREATEST((MAX(network_rx_packets) - MIN(network_rx_packets))::double precision, 0), GREATEST((MAX(network_tx_packets) - MIN(network_tx_packets))::double precision, 0),
	AVG(network_rx_bytes), MIN(network_rx_bytes), MAX(network_rx_bytes), AVG(network_tx_bytes), MIN(network_tx_bytes), MAX(network_tx_bytes),
	AVG(network_rx_packets), MIN(network_rx_packets), MAX(network_rx_packets), AVG(network_tx_packets), MIN(network_tx_packets), MAX(network_tx_packets),
	AVG(network_errors), MIN(network_errors), MAX(network_errors), AVG(network_drops), MIN(network_drops), MAX(network_drops)
FROM bucket_metrics
GROUP BY bucket_start
ORDER BY bucket_start ASC`

	rows, err := r.db.WithContext(ctx).Raw(metricSQL, serverID, start, end, interval).Rows()
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var metricRows []AggregatedMetricRow
	for rows.Next() {
		var row AggregatedMetricRow
		if err := rows.Scan(
			&row.BucketStart,
			&row.CPUAvg, &row.CPUMin, &row.CPUMax, &row.CPUP95, &row.CPUCoresAvg,
			&row.Load1Avg, &row.Load1Min, &row.Load1Max, &row.Load5Avg, &row.Load5Min, &row.Load5Max, &row.Load15Avg, &row.Load15Min, &row.Load15Max,
			&row.MemoryUsageAvg, &row.MemoryUsageMin, &row.MemoryUsageMax, &row.MemoryUsageP95, &row.MemoryTotalAvg, &row.MemoryUsedAvg, &row.MemoryAvailableAvg,
			&row.SwapUsageAvg, &row.SwapUsageMin, &row.SwapUsageMax, &row.SwapTotalAvg, &row.SwapUsedAvg, &row.SwapFreeAvg,
			&row.NetworkRXBytesDelta, &row.NetworkTXBytesDelta, &row.NetworkRXPacketsDelta, &row.NetworkTXPacketsDelta,
			&row.NetworkRXBytesAvg, &row.NetworkRXBytesMin, &row.NetworkRXBytesMax, &row.NetworkTXBytesAvg, &row.NetworkTXBytesMin, &row.NetworkTXBytesMax,
			&row.NetworkRXPacketsAvg, &row.NetworkRXPacketsMin, &row.NetworkRXPacketsMax, &row.NetworkTXPacketsAvg, &row.NetworkTXPacketsMin, &row.NetworkTXPacketsMax,
			&row.NetworkErrorsAvg, &row.NetworkErrorsMin, &row.NetworkErrorsMax, &row.NetworkDropsAvg, &row.NetworkDropsMin, &row.NetworkDropsMax,
		); err != nil {
			return nil, nil, err
		}
		metricRows = append(metricRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	diskSQL := `
WITH buckets AS (
	SELECT generate_series($2::timestamptz, $3::timestamptz - interval '1 microsecond', $4::interval) AS bucket_start
)
SELECT
	b.bucket_start,
	d.mount_point,
	AVG(d.usage_percent), MIN(d.usage_percent), MAX(d.usage_percent),
	AVG(d.total), AVG(d.used), AVG(d.free)
FROM buckets b
JOIN metrics m
	ON m.server_id = $1
	AND m.collected_at >= b.bucket_start
	AND m.collected_at < b.bucket_start + $4::interval
JOIN metric_disks d
	ON d.metric_id = m.id
GROUP BY b.bucket_start, d.mount_point
ORDER BY b.bucket_start ASC, d.mount_point ASC`

	diskRowsRaw, err := r.db.WithContext(ctx).Raw(diskSQL, serverID, start, end, interval).Rows()
	if err != nil {
		return nil, nil, err
	}
	defer diskRowsRaw.Close()

	var diskRows []AggregatedDiskRow
	for diskRowsRaw.Next() {
		var row AggregatedDiskRow
		if err := diskRowsRaw.Scan(&row.BucketStart, &row.MountPoint, &row.UsageAvg, &row.UsageMin, &row.UsageMax, &row.TotalAvg, &row.UsedAvg, &row.FreeAvg); err != nil {
			return nil, nil, err
		}
		diskRows = append(diskRows, row)
	}
	if err := diskRowsRaw.Err(); err != nil {
		return nil, nil, err
	}

	return metricRows, diskRows, nil
}

func aggregationInterval(granularity string) string {
	switch granularity {
	case "5m":
		return "5 minutes"
	case "1h":
		return "1 hour"
	case "d":
		return "1 day"
	default:
		return granularity
	}
}
