package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/metric/dto"
	"vpsmonitoring-backend/internal/metric/models"
	"vpsmonitoring-backend/internal/metric/repository"
	serverRepository "vpsmonitoring-backend/internal/server/repository"
	serverService "vpsmonitoring-backend/internal/server/service"
)

var (
	ErrNoMetricsYet        = errors.New("no metrics available yet")
	ErrInvalidTimeRange    = errors.New("invalid time range")
	ErrInvalidGranularity  = errors.New("invalid granularity")
	ErrBucketLimitExceeded = errors.New("aggregation bucket limit exceeded")
)

const MaxAggregationBuckets = 5040

// MetricService defines the business logic for metric ingestion.
type MetricService interface {
	IngestMetric(ctx context.Context, agentID string, serverID int64, req dto.IngestMetricRequest) (*dto.IngestMetricResponse, error)
	GetLatestMetric(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error)
	GetHistoricalMetrics(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, limit int) (*dto.HistoricalMetricsResponse, error)
	GetAggregatedMetrics(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, granularity string) (*dto.AggregatedMetricsResponse, error)
}

type metricService struct {
	repo       repository.MetricRepository
	serverRepo serverRepository.ServerRepository
}

// NewMetricService instantiates a MetricService.
func NewMetricService(repo repository.MetricRepository, serverRepo serverRepository.ServerRepository) MetricService {
	return &metricService{repo: repo, serverRepo: serverRepo}
}

// IngestMetric maps the DTO to models, assigns authenticated agent/server identity and backend received_at,
// and invokes the repository to persist transactionally.
func (s *metricService) IngestMetric(ctx context.Context, agentID string, serverID int64, req dto.IngestMetricRequest) (*dto.IngestMetricResponse, error) {
	if agentID == "" || serverID <= 0 {
		return nil, errors.New("unauthorized: missing or invalid agent identity")
	}

	// 1. Validate payload
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// 2. Generate backend timestamp (UTC)
	receivedAt := time.Now().UTC()
	metricID := uuid.New()

	// 3. Map parent Metric model
	metric := &models.Metric{
		ID:                 metricID,
		ServerID:           serverID, // Strictly derived from AgentIdentity
		AgentID:            agentID,  // Strictly derived from AgentIdentity
		CollectedAt:        req.Timestamp.UTC(),
		ReceivedAt:         receivedAt,
		CPUUsagePercent:    req.CPU.UsagePercent,
		CPUCores:           req.CPU.Cores,
		Load1:              req.CPU.Load1,
		Load5:              req.CPU.Load5,
		Load15:             req.CPU.Load15,
		MemoryTotal:        req.Memory.Total,
		MemoryUsed:         req.Memory.Used,
		MemoryAvailable:    req.Memory.Available,
		MemoryUsagePercent: req.Memory.UsagePercent,
		SwapTotal:          req.Swap.Total,
		SwapUsed:           req.Swap.Used,
		SwapFree:           req.Swap.Free,
		SwapUsagePercent:   req.Swap.UsagePercent,
		NetworkRXBytes:     req.Network.RXBytes,
		NetworkTXBytes:     req.Network.TXBytes,
		NetworkRXPackets:   req.Network.RXPackets,
		NetworkTXPackets:   req.Network.TXPackets,
		NetworkErrors:      req.Network.Errors,
		NetworkDrops:       req.Network.Drops,
		CreatedAt:          receivedAt,
	}

	// 4. Map child disk mount points
	disks := make([]models.MetricDisk, 0, len(req.Disk.MountPoints))
	for _, mp := range req.Disk.MountPoints {
		disk := models.MetricDisk{
			ID:           uuid.New(),
			MetricID:     metricID,
			MountPoint:   mp.Path,
			Filesystem:   mp.FSType,
			Total:        mp.Total,
			Used:         mp.Used,
			Free:         mp.Free,
			UsagePercent: mp.UsagePercent,
			CreatedAt:    receivedAt,
		}
		disks = append(disks, disk)
	}

	// 5. Transactionally persist via repository
	if err := s.repo.CreateMetric(ctx, metric, disks); err != nil {
		return nil, errors.New("failed to persist metric snapshot")
	}

	return &dto.IngestMetricResponse{
		Success:    true,
		MetricID:   metricID.String(),
		ServerID:   serverID,
		ReceivedAt: receivedAt.Format(time.RFC3339),
	}, nil
}

// GetLatestMetric verifies server ownership and returns the latest metric snapshot.
func (s *metricService) GetLatestMetric(ctx context.Context, serverID int64, userID int64) (*dto.LatestMetricResponse, error) {
	if serverID <= 0 || userID <= 0 {
		return nil, serverService.ErrServerNotFound
	}
	if s.serverRepo == nil {
		return nil, errors.New("server repository is nil")
	}

	server, err := s.serverRepo.GetByIDAndUserID(ctx, serverID, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, serverService.ErrServerNotFound
	}

	metric, err := s.repo.GetLatestByServerID(ctx, serverID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoMetricsYet
		}
		return nil, err
	}
	if metric == nil {
		return nil, ErrNoMetricsYet
	}

	resp := ToLatestMetricResponse(metric)
	return &resp, nil
}

// GetHistoricalMetrics verifies server ownership and returns historical metric snapshots.
func (s *metricService) GetHistoricalMetrics(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, limit int) (*dto.HistoricalMetricsResponse, error) {
	if serverID <= 0 || userID <= 0 {
		return nil, serverService.ErrServerNotFound
	}
	if start.After(end) {
		return nil, ErrInvalidTimeRange
	}
	if s.serverRepo == nil {
		return nil, errors.New("server repository is nil")
	}

	server, err := s.serverRepo.GetByIDAndUserID(ctx, serverID, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, serverService.ErrServerNotFound
	}

	metrics, err := s.repo.GetHistoryByServerID(ctx, serverID, start, end, limit)
	if err != nil {
		return nil, err
	}

	resp := ToHistoricalMetricsResponse(serverID, start, end, "", metrics)
	return &resp, nil
}

// GetAggregatedMetrics verifies server ownership and returns bucketed aggregates.
func (s *metricService) GetAggregatedMetrics(ctx context.Context, serverID int64, userID int64, start time.Time, end time.Time, granularity string) (*dto.AggregatedMetricsResponse, error) {
	if serverID <= 0 || userID <= 0 {
		return nil, serverService.ErrServerNotFound
	}
	if start.After(end) {
		return nil, ErrInvalidTimeRange
	}
	interval, err := aggregationDuration(granularity)
	if err != nil {
		return nil, err
	}
	buckets := bucketCount(start, end, interval)
	if buckets > MaxAggregationBuckets {
		return nil, fmt.Errorf("%w: maximum allowed buckets is %d", ErrBucketLimitExceeded, MaxAggregationBuckets)
	}
	if s.serverRepo == nil {
		return nil, errors.New("server repository is nil")
	}

	server, err := s.serverRepo.GetByIDAndUserID(ctx, serverID, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, serverService.ErrServerNotFound
	}

	if start.Equal(end) {
		resp := dto.AggregatedMetricsResponse{
			ServerID:    serverID,
			Start:       start.UTC().Format(time.RFC3339),
			End:         end.UTC().Format(time.RFC3339),
			Granularity: granularity,
			Data:        []dto.AggregatedMetricPoint{},
		}
		return &resp, nil
	}

	metricRows, diskRows, err := s.repo.GetAggregatedMetrics(ctx, serverID, start, end, granularity)
	if err != nil {
		return nil, err
	}
	resp := ToAggregatedMetricsResponse(serverID, start, end, granularity, metricRows, diskRows)
	return &resp, nil
}

// ToLatestMetricResponse maps the database model to a frontend-safe DTO.
func ToLatestMetricResponse(metric *models.Metric) dto.LatestMetricResponse {
	if metric == nil {
		return dto.LatestMetricResponse{}
	}

	disks := make([]dto.DiskMetricResponse, 0, len(metric.Disks))
	for _, disk := range metric.Disks {
		disks = append(disks, dto.DiskMetricResponse{
			MountPoint:   disk.MountPoint,
			Filesystem:   disk.Filesystem,
			Total:        disk.Total,
			Used:         disk.Used,
			Free:         disk.Free,
			UsagePercent: disk.UsagePercent,
		})
	}

	return dto.LatestMetricResponse{
		ServerID:    metric.ServerID,
		CollectedAt: metric.CollectedAt.UTC().Format(time.RFC3339),
		CPU: dto.LatestCPUMetricsDTO{
			UsagePercent: metric.CPUUsagePercent,
			Cores:        metric.CPUCores,
		},
		Load: dto.LatestLoadMetricsDTO{
			Load1:  metric.Load1,
			Load5:  metric.Load5,
			Load15: metric.Load15,
		},
		Memory: dto.MemoryMetricsDTO{
			Total:        metric.MemoryTotal,
			Used:         metric.MemoryUsed,
			Available:    metric.MemoryAvailable,
			UsagePercent: metric.MemoryUsagePercent,
		},
		Swap: dto.SwapMetricsDTO{
			Total:        metric.SwapTotal,
			Used:         metric.SwapUsed,
			Free:         metric.SwapFree,
			UsagePercent: metric.SwapUsagePercent,
		},
		Network: dto.NetworkMetricsDTO{
			RXBytes:   metric.NetworkRXBytes,
			TXBytes:   metric.NetworkTXBytes,
			RXPackets: metric.NetworkRXPackets,
			TXPackets: metric.NetworkTXPackets,
			Errors:    metric.NetworkErrors,
			Drops:     metric.NetworkDrops,
		},
		Disks: disks,
	}
}

// ToLatestMetricResponses maps metric models to response DTOs while preserving repository order.
func ToLatestMetricResponses(metrics []models.Metric) []dto.LatestMetricResponse {
	responses := make([]dto.LatestMetricResponse, 0, len(metrics))
	for i := range metrics {
		responses = append(responses, ToLatestMetricResponse(&metrics[i]))
	}
	return responses
}

// ToHistoricalMetricsResponse maps metric models to a historical response envelope.
func ToHistoricalMetricsResponse(serverID int64, start time.Time, end time.Time, interval string, metrics []models.Metric) dto.HistoricalMetricsResponse {
	points := make([]dto.HistoricalMetricPoint, 0, len(metrics))
	for i := range metrics {
		points = append(points, ToHistoricalMetricPoint(&metrics[i]))
	}

	return dto.HistoricalMetricsResponse{
		ServerID: serverID,
		Start:    start.UTC().Format(time.RFC3339),
		End:      end.UTC().Format(time.RFC3339),
		Interval: interval,
		Data:     points,
	}
}

// ToHistoricalMetricPoint maps one metric model to a historical data point.
func ToHistoricalMetricPoint(metric *models.Metric) dto.HistoricalMetricPoint {
	if metric == nil {
		return dto.HistoricalMetricPoint{}
	}

	return dto.HistoricalMetricPoint{
		CollectedAt: metric.CollectedAt.UTC().Format(time.RFC3339),
		CPU: dto.LatestCPUMetricsDTO{
			UsagePercent: metric.CPUUsagePercent,
			Cores:        metric.CPUCores,
		},
		Load: dto.LatestLoadMetricsDTO{
			Load1:  metric.Load1,
			Load5:  metric.Load5,
			Load15: metric.Load15,
		},
		Memory: dto.MemoryMetricsDTO{
			Total:        metric.MemoryTotal,
			Used:         metric.MemoryUsed,
			Available:    metric.MemoryAvailable,
			UsagePercent: metric.MemoryUsagePercent,
		},
		Swap: dto.SwapMetricsDTO{
			Total:        metric.SwapTotal,
			Used:         metric.SwapUsed,
			Free:         metric.SwapFree,
			UsagePercent: metric.SwapUsagePercent,
		},
		Network: dto.NetworkMetricsDTO{
			RXBytes:   metric.NetworkRXBytes,
			TXBytes:   metric.NetworkTXBytes,
			RXPackets: metric.NetworkRXPackets,
			TXPackets: metric.NetworkTXPackets,
			Errors:    metric.NetworkErrors,
			Drops:     metric.NetworkDrops,
		},
	}
}

// ToAggregatedMetricsResponse maps aggregate repository rows to a user-facing response.
func ToAggregatedMetricsResponse(serverID int64, start time.Time, end time.Time, granularity string, rows []repository.AggregatedMetricRow, diskRows []repository.AggregatedDiskRow) dto.AggregatedMetricsResponse {
	disksByBucket := make(map[time.Time][]dto.AggregatedDiskMetrics)
	for _, disk := range diskRows {
		bucket := disk.BucketStart.UTC()
		disksByBucket[bucket] = append(disksByBucket[bucket], dto.AggregatedDiskMetrics{
			MountPoint:          disk.MountPoint,
			AverageUsagePercent: ptr(disk.UsageAvg),
			MinUsagePercent:     ptr(disk.UsageMin),
			MaxUsagePercent:     ptr(disk.UsageMax),
			AverageTotal:        ptr(disk.TotalAvg),
			AverageUsed:         ptr(disk.UsedAvg),
			AverageFree:         ptr(disk.FreeAvg),
		})
	}

	points := make([]dto.AggregatedMetricPoint, 0, len(rows))
	for _, row := range rows {
		bucket := row.BucketStart.UTC()
		disks := disksByBucket[bucket]
		if disks == nil {
			disks = []dto.AggregatedDiskMetrics{}
		}
		points = append(points, dto.AggregatedMetricPoint{
			BucketStart: bucket.Format(time.RFC3339),
			CPU: dto.AggregatedCPUMetrics{
				AverageUsagePercent: ptr(row.CPUAvg),
				MinUsagePercent:     ptr(row.CPUMin),
				MaxUsagePercent:     ptr(row.CPUMax),
				P95UsagePercent:     ptr(row.CPUP95),
				AverageCores:        ptr(row.CPUCoresAvg),
			},
			Load: dto.AggregatedLoadMetrics{
				AverageLoad1:  ptr(row.Load1Avg),
				MinLoad1:      ptr(row.Load1Min),
				MaxLoad1:      ptr(row.Load1Max),
				AverageLoad5:  ptr(row.Load5Avg),
				MinLoad5:      ptr(row.Load5Min),
				MaxLoad5:      ptr(row.Load5Max),
				AverageLoad15: ptr(row.Load15Avg),
				MinLoad15:     ptr(row.Load15Min),
				MaxLoad15:     ptr(row.Load15Max),
			},
			Memory: dto.AggregatedMemoryMetrics{
				AverageUsagePercent: ptr(row.MemoryUsageAvg),
				MinUsagePercent:     ptr(row.MemoryUsageMin),
				MaxUsagePercent:     ptr(row.MemoryUsageMax),
				P95UsagePercent:     ptr(row.MemoryUsageP95),
				AverageTotal:        ptr(row.MemoryTotalAvg),
				AverageUsed:         ptr(row.MemoryUsedAvg),
				AverageAvailable:    ptr(row.MemoryAvailableAvg),
			},
			Swap: dto.AggregatedSwapMetrics{
				AverageUsagePercent: ptr(row.SwapUsageAvg),
				MinUsagePercent:     ptr(row.SwapUsageMin),
				MaxUsagePercent:     ptr(row.SwapUsageMax),
				AverageTotal:        ptr(row.SwapTotalAvg),
				AverageUsed:         ptr(row.SwapUsedAvg),
				AverageFree:         ptr(row.SwapFreeAvg),
			},
			Network: dto.AggregatedNetworkMetrics{
				RXBytesDelta:     ptr(row.NetworkRXBytesDelta),
				TXBytesDelta:     ptr(row.NetworkTXBytesDelta),
				RXPacketsDelta:   ptr(row.NetworkRXPacketsDelta),
				TXPacketsDelta:   ptr(row.NetworkTXPacketsDelta),
				AverageRXBytes:   ptr(row.NetworkRXBytesAvg),
				MinRXBytes:       ptr(row.NetworkRXBytesMin),
				MaxRXBytes:       ptr(row.NetworkRXBytesMax),
				AverageTXBytes:   ptr(row.NetworkTXBytesAvg),
				MinTXBytes:       ptr(row.NetworkTXBytesMin),
				MaxTXBytes:       ptr(row.NetworkTXBytesMax),
				AverageRXPackets: ptr(row.NetworkRXPacketsAvg),
				MinRXPackets:     ptr(row.NetworkRXPacketsMin),
				MaxRXPackets:     ptr(row.NetworkRXPacketsMax),
				AverageTXPackets: ptr(row.NetworkTXPacketsAvg),
				MinTXPackets:     ptr(row.NetworkTXPacketsMin),
				MaxTXPackets:     ptr(row.NetworkTXPacketsMax),
				AverageErrors:    ptr(row.NetworkErrorsAvg),
				MinErrors:        ptr(row.NetworkErrorsMin),
				MaxErrors:        ptr(row.NetworkErrorsMax),
				AverageDrops:     ptr(row.NetworkDropsAvg),
				MinDrops:         ptr(row.NetworkDropsMin),
				MaxDrops:         ptr(row.NetworkDropsMax),
			},
			Disks: disks,
		})
	}

	return dto.AggregatedMetricsResponse{
		ServerID:    serverID,
		Start:       start.UTC().Format(time.RFC3339),
		End:         end.UTC().Format(time.RFC3339),
		Granularity: granularity,
		Data:        points,
	}
}

func ptr(v sql.NullFloat64) *float64 {
	if !v.Valid {
		return nil
	}
	return &v.Float64
}

func aggregationDuration(granularity string) (time.Duration, error) {
	switch granularity {
	case "5m":
		return 5 * time.Minute, nil
	case "1h":
		return time.Hour, nil
	case "d":
		return 24 * time.Hour, nil
	default:
		return 0, ErrInvalidGranularity
	}
}

func bucketCount(start, end time.Time, interval time.Duration) int {
	if !end.After(start) {
		return 0
	}
	return int(math.Ceil(float64(end.Sub(start)) / float64(interval)))
}
