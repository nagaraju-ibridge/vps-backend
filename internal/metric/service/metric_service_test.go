package service_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/metric/dto"
	"vpsmonitoring-backend/internal/metric/models"
	"vpsmonitoring-backend/internal/metric/repository"
	"vpsmonitoring-backend/internal/metric/service"
	serverModels "vpsmonitoring-backend/internal/server/models"
)

type mockMetricRepository struct {
	createdMetric        *models.Metric
	createdDisks         []models.MetricDisk
	createErr            error
	latestMetric         *models.Metric
	latestErr            error
	historyMetrics       []models.Metric
	historyErr           error
	historyCalled        bool
	historyServerID      int64
	historyStart         time.Time
	historyEnd           time.Time
	historyLimit         int
	aggregateRows        []repository.AggregatedMetricRow
	aggregateDiskRows    []repository.AggregatedDiskRow
	aggregateErr         error
	aggregateCalled      bool
	aggregateServerID    int64
	aggregateStart       time.Time
	aggregateEnd         time.Time
	aggregateGranularity string
}

func (m *mockMetricRepository) AutoMigrate() error {
	return nil
}

func (m *mockMetricRepository) CreateMetric(ctx context.Context, metric *models.Metric, disks []models.MetricDisk) error {
	if m.createErr != nil {
		return m.createErr
	}
	m.createdMetric = metric
	m.createdDisks = disks
	return nil
}

func (m *mockMetricRepository) GetLatestByServerID(ctx context.Context, serverID int64) (*models.Metric, error) {
	if m.latestErr != nil {
		return nil, m.latestErr
	}
	if m.latestMetric != nil {
		return m.latestMetric, nil
	}
	return m.createdMetric, nil
}

func (m *mockMetricRepository) GetHistoryByServerID(ctx context.Context, serverID int64, start time.Time, end time.Time, limit int) ([]models.Metric, error) {
	m.historyCalled = true
	m.historyServerID = serverID
	m.historyStart = start
	m.historyEnd = end
	m.historyLimit = limit
	if m.historyErr != nil {
		return nil, m.historyErr
	}
	return m.historyMetrics, nil
}

func (m *mockMetricRepository) GetAggregatedMetrics(ctx context.Context, serverID int64, start time.Time, end time.Time, granularity string) ([]repository.AggregatedMetricRow, []repository.AggregatedDiskRow, error) {
	m.aggregateCalled = true
	m.aggregateServerID = serverID
	m.aggregateStart = start
	m.aggregateEnd = end
	m.aggregateGranularity = granularity
	if m.aggregateErr != nil {
		return nil, nil, m.aggregateErr
	}
	return m.aggregateRows, m.aggregateDiskRows, nil
}

type mockServerRepository struct {
	servers map[int64]*serverModels.Server
	err     error
}

func (m *mockServerRepository) AutoMigrate() error {
	return nil
}

func (m *mockServerRepository) Create(ctx context.Context, server *serverModels.Server) error {
	return nil
}

func (m *mockServerRepository) GetByID(ctx context.Context, id int64) (*serverModels.Server, error) {
	return nil, nil
}

func (m *mockServerRepository) GetByIDAndUserID(ctx context.Context, id int64, userID int64) (*serverModels.Server, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.servers == nil {
		return nil, nil
	}
	server, ok := m.servers[id]
	if !ok || server.UserID != userID {
		return nil, nil
	}
	cp := *server
	return &cp, nil
}

func (m *mockServerRepository) GetByUserID(ctx context.Context, userID int64) ([]serverModels.Server, error) {
	return nil, nil
}

func (m *mockServerRepository) Update(ctx context.Context, server *serverModels.Server) error {
	return nil
}

func (m *mockServerRepository) Delete(ctx context.Context, id int64, userID int64) error {
	return nil
}

func TestMetricService_IngestMetric(t *testing.T) {
	t.Run("Valid authenticated metric submission succeeds and assigns tenant identity", func(t *testing.T) {
		repo := &mockMetricRepository{}
		svc := service.NewMetricService(repo, &mockServerRepository{})

		req := dto.IngestMetricRequest{
			Timestamp: time.Now().UTC(),
			CPU: dto.CPUMetricsDTO{
				UsagePercent: 25.5,
				Cores:        4,
				Load1:        0.5,
				Load5:        0.4,
				Load15:       0.3,
			},
			Memory: dto.MemoryMetricsDTO{
				Total:        8000000000,
				Used:         4000000000,
				Available:    4000000000,
				UsagePercent: 50.0,
			},
			Swap: dto.SwapMetricsDTO{
				Total:        2000000000,
				Used:         100000000,
				Free:         1900000000,
				UsagePercent: 5.0,
			},
			Disk: dto.DiskMetricsDTO{
				MountPoints: []dto.MountPointDTO{
					{
						Path:         "/",
						FSType:       "ext4",
						Total:        100000000000,
						Used:         50000000000,
						Free:         50000000000,
						UsagePercent: 50.0,
					},
					{
						Path:         "/mnt/data",
						FSType:       "ext4",
						Total:        500000000000,
						Used:         100000000000,
						Free:         400000000000,
						UsagePercent: 20.0,
					},
				},
			},
			Network: dto.NetworkMetricsDTO{
				RXBytes:   1000,
				TXBytes:   2000,
				RXPackets: 10,
				TXPackets: 20,
				Errors:    0,
				Drops:     0,
			},
		}

		resp, err := svc.IngestMetric(context.Background(), "agent-12345", 99, req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !resp.Success {
			t.Errorf("expected success true, got false")
		}
		if resp.ServerID != 99 {
			t.Errorf("expected server_id 99, got %d", resp.ServerID)
		}

		// Verify repository received models with derived tenant identity
		if repo.createdMetric == nil {
			t.Fatalf("expected metric to be persisted")
		}
		if repo.createdMetric.ServerID != 99 {
			t.Errorf("expected persisted ServerID 99, got %d", repo.createdMetric.ServerID)
		}
		if repo.createdMetric.AgentID != "agent-12345" {
			t.Errorf("expected persisted AgentID agent-12345, got %s", repo.createdMetric.AgentID)
		}
		if repo.createdMetric.ReceivedAt.IsZero() {
			t.Errorf("expected ReceivedAt to be set by backend")
		}
		if repo.createdMetric.CPUUsagePercent != 25.5 {
			t.Errorf("expected CPUUsagePercent 25.5, got %f", repo.createdMetric.CPUUsagePercent)
		}
		if len(repo.createdDisks) != 2 {
			t.Errorf("expected 2 mount points persisted, got %d", len(repo.createdDisks))
		}
		for _, disk := range repo.createdDisks {
			if disk.MetricID != repo.createdMetric.ID {
				t.Errorf("disk MetricID %s does not match parent metric ID %s", disk.MetricID, repo.createdMetric.ID)
			}
		}
	})

	t.Run("Missing agent identity returns unauthorized error", func(t *testing.T) {
		repo := &mockMetricRepository{}
		svc := service.NewMetricService(repo, &mockServerRepository{})

		req := dto.IngestMetricRequest{
			Timestamp: time.Now().UTC(),
		}

		_, err := svc.IngestMetric(context.Background(), "", 99, req)
		if err == nil {
			t.Errorf("expected error for empty agentID, got nil")
		}

		_, err = svc.IngestMetric(context.Background(), "agent-123", 0, req)
		if err == nil {
			t.Errorf("expected error for zero serverID, got nil")
		}
	})

	t.Run("Zero swap is accepted", func(t *testing.T) {
		repo := &mockMetricRepository{}
		svc := service.NewMetricService(repo, &mockServerRepository{})

		req := dto.IngestMetricRequest{
			Timestamp: time.Now().UTC(),
			CPU: dto.CPUMetricsDTO{
				UsagePercent: 10,
				Cores:        2,
			},
			Memory: dto.MemoryMetricsDTO{
				Total:        4000000000,
				Used:         2000000000,
				Available:    2000000000,
				UsagePercent: 50.0,
			},
			Swap: dto.SwapMetricsDTO{
				Total:        0,
				Used:         0,
				Free:         0,
				UsagePercent: 0,
			},
		}

		_, err := svc.IngestMetric(context.Background(), "agent-123", 42, req)
		if err != nil {
			t.Fatalf("expected zero swap to succeed, got: %v", err)
		}
	})

	t.Run("Invalid percentages rejected", func(t *testing.T) {
		repo := &mockMetricRepository{}
		svc := service.NewMetricService(repo, &mockServerRepository{})

		req := dto.IngestMetricRequest{
			Timestamp: time.Now().UTC(),
			CPU: dto.CPUMetricsDTO{
				UsagePercent: 105.0, // Invalid!
			},
		}

		_, err := svc.IngestMetric(context.Background(), "agent-123", 42, req)
		if err == nil {
			t.Fatalf("expected error for CPU usage > 100, got nil")
		}
	})

	t.Run("Repository failure rolls back and returns clean error", func(t *testing.T) {
		repo := &mockMetricRepository{
			createErr: errors.New("database connection refused"),
		}
		svc := service.NewMetricService(repo, &mockServerRepository{})

		req := dto.IngestMetricRequest{
			Timestamp: time.Now().UTC(),
			CPU: dto.CPUMetricsDTO{
				UsagePercent: 20,
				Cores:        2,
			},
		}

		_, err := svc.IngestMetric(context.Background(), "agent-123", 42, req)
		if err == nil {
			t.Fatalf("expected error on repo failure, got nil")
		}
		if err.Error() != "failed to persist metric snapshot" {
			t.Errorf("expected sanitized error, got %v", err)
		}
	})
}

func TestMetricService_GetLatestMetric(t *testing.T) {
	collectedAt := time.Date(2026, 9, 29, 12, 30, 0, 0, time.UTC)
	metricID := uuid.New()
	latestMetric := &models.Metric{
		ID:                 metricID,
		ServerID:           42,
		AgentID:            "agent-123",
		CollectedAt:        collectedAt,
		ReceivedAt:         collectedAt.Add(2 * time.Second),
		CPUUsagePercent:    37.5,
		CPUCores:           4,
		Load1:              0.7,
		Load5:              0.6,
		Load15:             0.5,
		MemoryTotal:        8000,
		MemoryUsed:         3200,
		MemoryAvailable:    4800,
		MemoryUsagePercent: 40,
		SwapTotal:          2000,
		SwapUsed:           100,
		SwapFree:           1900,
		SwapUsagePercent:   5,
		NetworkRXBytes:     10000,
		NetworkTXBytes:     20000,
		NetworkRXPackets:   100,
		NetworkTXPackets:   200,
		NetworkErrors:      1,
		NetworkDrops:       2,
		Disks: []models.MetricDisk{
			{
				ID:           uuid.New(),
				MetricID:     metricID,
				MountPoint:   "/",
				Filesystem:   "ext4",
				Total:        100000,
				Used:         25000,
				Free:         75000,
				UsagePercent: 25,
			},
		},
	}

	t.Run("User owns server and latest metric exists", func(t *testing.T) {
		metricRepo := &mockMetricRepository{latestMetric: latestMetric}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		resp, err := svc.GetLatestMetric(context.Background(), 42, 7)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.ServerID != 42 {
			t.Errorf("expected server_id 42, got %d", resp.ServerID)
		}
		if resp.CollectedAt != collectedAt.Format(time.RFC3339) {
			t.Errorf("expected collected_at %s, got %s", collectedAt.Format(time.RFC3339), resp.CollectedAt)
		}
		if resp.CPU.UsagePercent != 37.5 || resp.CPU.Cores != 4 {
			t.Errorf("cpu values not mapped correctly: %+v", resp.CPU)
		}
		if resp.Load.Load1 != 0.7 || resp.Load.Load5 != 0.6 || resp.Load.Load15 != 0.5 {
			t.Errorf("load values not mapped correctly: %+v", resp.Load)
		}
		if resp.Memory.Total != 8000 || resp.Memory.Used != 3200 || resp.Memory.Available != 4800 || resp.Memory.UsagePercent != 40 {
			t.Errorf("memory values not mapped correctly: %+v", resp.Memory)
		}
		if resp.Swap.Total != 2000 || resp.Swap.Used != 100 || resp.Swap.Free != 1900 || resp.Swap.UsagePercent != 5 {
			t.Errorf("swap values not mapped correctly: %+v", resp.Swap)
		}
		if resp.Network.RXBytes != 10000 || resp.Network.TXBytes != 20000 || resp.Network.RXPackets != 100 || resp.Network.TXPackets != 200 || resp.Network.Errors != 1 || resp.Network.Drops != 2 {
			t.Errorf("network values not mapped correctly: %+v", resp.Network)
		}
		if len(resp.Disks) != 1 {
			t.Fatalf("expected 1 disk, got %d", len(resp.Disks))
		}
		disk := resp.Disks[0]
		if disk.MountPoint != "/" || disk.Filesystem != "ext4" || disk.Total != 100000 || disk.Used != 25000 || disk.Free != 75000 || disk.UsagePercent != 25 {
			t.Errorf("disk values not mapped correctly: %+v", disk)
		}
	})

	t.Run("User owns server and no metric exists", func(t *testing.T) {
		metricRepo := &mockMetricRepository{latestErr: gorm.ErrRecordNotFound}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetLatestMetric(context.Background(), 42, 7)
		if !errors.Is(err, service.ErrNoMetricsYet) {
			t.Fatalf("expected ErrNoMetricsYet, got %v", err)
		}
	})

	t.Run("User does not own server", func(t *testing.T) {
		metricRepo := &mockMetricRepository{latestMetric: latestMetric}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 99, Name: "Other user's server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetLatestMetric(context.Background(), 42, 7)
		if err == nil {
			t.Fatalf("expected ownership error, got nil")
		}
		if errors.Is(err, service.ErrNoMetricsYet) {
			t.Fatalf("expected server ownership error, got no-metrics error")
		}
	})

	t.Run("Repository returns unexpected database error", func(t *testing.T) {
		dbErr := errors.New("database unavailable")
		metricRepo := &mockMetricRepository{latestErr: dbErr}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetLatestMetric(context.Background(), 42, 7)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected database error, got %v", err)
		}
	})
}

func TestMetricService_GetHistoricalMetrics(t *testing.T) {
	start := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)
	limit := 25

	firstMetric := models.Metric{
		ID:                 uuid.New(),
		ServerID:           42,
		AgentID:            "agent-123",
		CollectedAt:        start.Add(time.Minute),
		ReceivedAt:         start.Add(time.Minute + time.Second),
		CPUUsagePercent:    10.5,
		CPUCores:           2,
		Load1:              0.1,
		Load5:              0.2,
		Load15:             0.3,
		MemoryTotal:        8000,
		MemoryUsed:         3000,
		MemoryAvailable:    5000,
		MemoryUsagePercent: 37.5,
		SwapTotal:          1000,
		SwapUsed:           50,
		SwapFree:           950,
		SwapUsagePercent:   5,
		NetworkRXBytes:     100,
		NetworkTXBytes:     200,
		NetworkRXPackets:   10,
		NetworkTXPackets:   20,
		NetworkErrors:      0,
		NetworkDrops:       1,
	}
	secondMetric := models.Metric{
		ID:                 uuid.New(),
		ServerID:           42,
		AgentID:            "agent-123",
		CollectedAt:        start.Add(2 * time.Minute),
		ReceivedAt:         start.Add(2*time.Minute + time.Second),
		CPUUsagePercent:    20.5,
		CPUCores:           2,
		Load1:              0.4,
		Load5:              0.5,
		Load15:             0.6,
		MemoryTotal:        8000,
		MemoryUsed:         4000,
		MemoryAvailable:    4000,
		MemoryUsagePercent: 50,
		SwapTotal:          1000,
		SwapUsed:           100,
		SwapFree:           900,
		SwapUsagePercent:   10,
		NetworkRXBytes:     300,
		NetworkTXBytes:     400,
		NetworkRXPackets:   30,
		NetworkTXPackets:   40,
		NetworkErrors:      1,
		NetworkDrops:       2,
	}

	t.Run("Owner can retrieve historical metrics with correct parameters", func(t *testing.T) {
		metricRepo := &mockMetricRepository{
			historyMetrics: []models.Metric{firstMetric, secondMetric},
		}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		resp, err := svc.GetHistoricalMetrics(context.Background(), 42, 7, start, end, limit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !metricRepo.historyCalled {
			t.Fatalf("expected historical repository to be called")
		}
		if metricRepo.historyServerID != 42 || !metricRepo.historyStart.Equal(start) || !metricRepo.historyEnd.Equal(end) || metricRepo.historyLimit != limit {
			t.Fatalf("historical repository received wrong params: serverID=%d start=%s end=%s limit=%d",
				metricRepo.historyServerID, metricRepo.historyStart, metricRepo.historyEnd, metricRepo.historyLimit)
		}
		if resp.ServerID != 42 {
			t.Fatalf("expected server_id 42, got %d", resp.ServerID)
		}
		if resp.Start != start.Format(time.RFC3339) || resp.End != end.Format(time.RFC3339) {
			t.Fatalf("expected start/end to be preserved, got start=%s end=%s", resp.Start, resp.End)
		}
		if len(resp.Data) != 2 {
			t.Fatalf("expected 2 historical metrics, got %d", len(resp.Data))
		}
		if resp.Data[0].CollectedAt != firstMetric.CollectedAt.Format(time.RFC3339) || resp.Data[1].CollectedAt != secondMetric.CollectedAt.Format(time.RFC3339) {
			t.Fatalf("expected chronological order to be preserved, got %+v", resp)
		}
		if resp.Data[0].CPU.UsagePercent != firstMetric.CPUUsagePercent || resp.Data[1].CPU.UsagePercent != secondMetric.CPUUsagePercent {
			t.Fatalf("historical CPU values were not mapped correctly: %+v", resp)
		}
	})

	t.Run("Non-owner cannot retrieve historical metrics and repository is not called", func(t *testing.T) {
		metricRepo := &mockMetricRepository{
			historyMetrics: []models.Metric{firstMetric},
		}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 99, Name: "Other user's server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetHistoricalMetrics(context.Background(), 42, 7, start, end, limit)
		if err == nil {
			t.Fatalf("expected ownership error, got nil")
		}
		if metricRepo.historyCalled {
			t.Fatalf("expected historical repository not to be called for non-owner")
		}
	})

	t.Run("Non-existent server uses existing ownership error and repository is not called", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetHistoricalMetrics(context.Background(), 999, 7, start, end, limit)
		if err == nil {
			t.Fatalf("expected server not found error, got nil")
		}
		if metricRepo.historyCalled {
			t.Fatalf("expected historical repository not to be called for missing server")
		}
	})

	t.Run("Empty historical result returns empty collection and no error", func(t *testing.T) {
		metricRepo := &mockMetricRepository{
			historyMetrics: []models.Metric{},
		}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		resp, err := svc.GetHistoricalMetrics(context.Background(), 42, 7, start, end, limit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty historical response, got %d", len(resp.Data))
		}
	})

	t.Run("Invalid time range returns validation error and repository is not called", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetHistoricalMetrics(context.Background(), 42, 7, end, start, limit)
		if !errors.Is(err, service.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
		}
		if metricRepo.historyCalled {
			t.Fatalf("expected historical repository not to be called for invalid time range")
		}
	})

	t.Run("Repository error is returned", func(t *testing.T) {
		dbErr := errors.New("database unavailable")
		metricRepo := &mockMetricRepository{
			historyErr: dbErr,
		}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 7, Name: "Owned server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetHistoricalMetrics(context.Background(), 42, 7, start, end, limit)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected repository error, got %v", err)
		}
	})
}

func TestMetricService_GetAggregatedMetrics(t *testing.T) {
	start := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	ownedServers := &mockServerRepository{
		servers: map[int64]*serverModels.Server{
			42: {ID: 42, UserID: 7, Name: "Owned server"},
		},
	}

	t.Run("Owner can retrieve aggregate metrics with correct parameters", func(t *testing.T) {
		row := repository.AggregatedMetricRow{
			BucketStart:           start,
			CPUAvg:                sql.NullFloat64{Float64: 12.5, Valid: true},
			CPUMin:                sql.NullFloat64{Float64: 10, Valid: true},
			CPUMax:                sql.NullFloat64{Float64: 15, Valid: true},
			NetworkRXBytesDelta:   sql.NullFloat64{Float64: 256, Valid: true},
			NetworkTXPacketsDelta: sql.NullFloat64{Float64: 4, Valid: true},
		}
		diskRow := repository.AggregatedDiskRow{
			BucketStart: start,
			MountPoint:  "/",
			UsageAvg:    sql.NullFloat64{Float64: 55.5, Valid: true},
		}
		metricRepo := &mockMetricRepository{
			aggregateRows:     []repository.AggregatedMetricRow{row},
			aggregateDiskRows: []repository.AggregatedDiskRow{diskRow},
		}
		svc := service.NewMetricService(metricRepo, ownedServers)

		resp, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, end, "5m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !metricRepo.aggregateCalled {
			t.Fatalf("expected aggregate repository to be called")
		}
		if metricRepo.aggregateServerID != 42 || !metricRepo.aggregateStart.Equal(start) || !metricRepo.aggregateEnd.Equal(end) || metricRepo.aggregateGranularity != "5m" {
			t.Fatalf("aggregate repository received wrong params: serverID=%d start=%s end=%s granularity=%s",
				metricRepo.aggregateServerID, metricRepo.aggregateStart, metricRepo.aggregateEnd, metricRepo.aggregateGranularity)
		}
		if resp.ServerID != 42 || resp.Start != start.Format(time.RFC3339) || resp.End != end.Format(time.RFC3339) || resp.Granularity != "5m" {
			t.Fatalf("aggregate envelope not mapped correctly: %+v", resp)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 aggregate point, got %d", len(resp.Data))
		}
		point := resp.Data[0]
		if point.CPU.AverageUsagePercent == nil || *point.CPU.AverageUsagePercent != 12.5 {
			t.Fatalf("expected CPU average 12.5, got %+v", point.CPU.AverageUsagePercent)
		}
		if point.Network.RXBytesDelta == nil || *point.Network.RXBytesDelta != 256 {
			t.Fatalf("expected RX delta 256, got %+v", point.Network.RXBytesDelta)
		}
		if len(point.Disks) != 1 || point.Disks[0].MountPoint != "/" || point.Disks[0].AverageUsagePercent == nil || *point.Disks[0].AverageUsagePercent != 55.5 {
			t.Fatalf("disk aggregates not mapped correctly: %+v", point.Disks)
		}
	})

	t.Run("Empty bucket nullable fields remain nil and disks are empty", func(t *testing.T) {
		metricRepo := &mockMetricRepository{
			aggregateRows: []repository.AggregatedMetricRow{{BucketStart: start}},
		}
		svc := service.NewMetricService(metricRepo, ownedServers)

		resp, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, end, "1h")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected one empty generated bucket, got %d", len(resp.Data))
		}
		point := resp.Data[0]
		if point.CPU.AverageUsagePercent != nil || point.Memory.AverageUsagePercent != nil || point.Network.RXBytesDelta != nil {
			t.Fatalf("expected nil aggregate fields for empty bucket, got %+v", point)
		}
		if point.Disks == nil {
			t.Fatalf("expected empty disk collection, got nil")
		}
		if len(point.Disks) != 0 {
			t.Fatalf("expected no disk rows for empty bucket, got %d", len(point.Disks))
		}
	})

	t.Run("Start equal end returns empty response without repository call", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		svc := service.NewMetricService(metricRepo, ownedServers)

		resp, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, start, "5m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metricRepo.aggregateCalled {
			t.Fatalf("expected repository not to be called for empty range")
		}
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty aggregate response, got %d", len(resp.Data))
		}
	})

	t.Run("Exactly maximum bucket count is accepted", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		svc := service.NewMetricService(metricRepo, ownedServers)

		_, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, start.Add(service.MaxAggregationBuckets*5*time.Minute), "5m")
		if err != nil {
			t.Fatalf("expected max bucket count to be accepted, got %v", err)
		}
		if !metricRepo.aggregateCalled {
			t.Fatalf("expected repository call for max bucket range")
		}
	})

	t.Run("Bucket count above maximum is rejected before ownership repository query", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		svc := service.NewMetricService(metricRepo, ownedServers)

		_, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, start.Add((service.MaxAggregationBuckets+1)*5*time.Minute), "5m")
		if !errors.Is(err, service.ErrBucketLimitExceeded) {
			t.Fatalf("expected ErrBucketLimitExceeded, got %v", err)
		}
		if metricRepo.aggregateCalled {
			t.Fatalf("expected aggregate repository not to be called when bucket limit is exceeded")
		}
	})

	t.Run("Invalid granularity is rejected", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		svc := service.NewMetricService(metricRepo, ownedServers)

		_, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, end, "10m")
		if !errors.Is(err, service.ErrInvalidGranularity) {
			t.Fatalf("expected ErrInvalidGranularity, got %v", err)
		}
		if metricRepo.aggregateCalled {
			t.Fatalf("expected aggregate repository not to be called for invalid granularity")
		}
	})

	t.Run("Non-owner cannot retrieve aggregate metrics", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		serverRepo := &mockServerRepository{
			servers: map[int64]*serverModels.Server{
				42: {ID: 42, UserID: 99, Name: "Other user's server"},
			},
		}
		svc := service.NewMetricService(metricRepo, serverRepo)

		_, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, end, "5m")
		if err == nil {
			t.Fatalf("expected ownership error, got nil")
		}
		if metricRepo.aggregateCalled {
			t.Fatalf("expected aggregate repository not to be called for non-owner")
		}
	})

	t.Run("Invalid time range returns validation error", func(t *testing.T) {
		metricRepo := &mockMetricRepository{}
		svc := service.NewMetricService(metricRepo, ownedServers)

		_, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, end, start, "5m")
		if !errors.Is(err, service.ErrInvalidTimeRange) {
			t.Fatalf("expected ErrInvalidTimeRange, got %v", err)
		}
		if metricRepo.aggregateCalled {
			t.Fatalf("expected aggregate repository not to be called for invalid time range")
		}
	})

	t.Run("Repository error is returned", func(t *testing.T) {
		dbErr := errors.New("database unavailable")
		metricRepo := &mockMetricRepository{aggregateErr: dbErr}
		svc := service.NewMetricService(metricRepo, ownedServers)

		_, err := svc.GetAggregatedMetrics(context.Background(), 42, 7, start, end, "5m")
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected repository error, got %v", err)
		}
	})
}

func TestHistoricalMetricMapping(t *testing.T) {
	start := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Minute)
	firstCollectedAt := start.Add(time.Minute)
	secondCollectedAt := start.Add(2 * time.Minute)

	firstMetric := models.Metric{
		ID:                 uuid.New(),
		ServerID:           42,
		CollectedAt:        firstCollectedAt,
		CPUUsagePercent:    11.25,
		CPUCores:           2,
		Load1:              1.11,
		Load5:              2.22,
		Load15:             3.33,
		MemoryTotal:        1024,
		MemoryUsed:         512,
		MemoryAvailable:    512,
		MemoryUsagePercent: 50,
		SwapTotal:          2048,
		SwapUsed:           128,
		SwapFree:           1920,
		SwapUsagePercent:   6.25,
		NetworkRXBytes:     1000,
		NetworkTXBytes:     2000,
		NetworkRXPackets:   10,
		NetworkTXPackets:   20,
		NetworkErrors:      1,
		NetworkDrops:       2,
	}
	secondMetric := models.Metric{
		ID:              uuid.New(),
		ServerID:        42,
		CollectedAt:     secondCollectedAt,
		CPUUsagePercent: 22.5,
		CPUCores:        4,
	}

	t.Run("One historical metric maps all public metric groups", func(t *testing.T) {
		resp := service.ToHistoricalMetricsResponse(42, start, end, "60s", []models.Metric{firstMetric})

		if resp.ServerID != 42 || resp.Start != start.Format(time.RFC3339) || resp.End != end.Format(time.RFC3339) || resp.Interval != "60s" {
			t.Fatalf("historical envelope not mapped correctly: %+v", resp)
		}
		if len(resp.Data) != 1 {
			t.Fatalf("expected 1 historical point, got %d", len(resp.Data))
		}

		point := resp.Data[0]
		if point.CollectedAt != firstCollectedAt.Format(time.RFC3339) {
			t.Fatalf("expected collected_at %s, got %s", firstCollectedAt.Format(time.RFC3339), point.CollectedAt)
		}
		if point.CPU.UsagePercent != 11.25 || point.CPU.Cores != 2 {
			t.Fatalf("cpu values not mapped correctly: %+v", point.CPU)
		}
		if point.Load.Load1 != 1.11 || point.Load.Load5 != 2.22 || point.Load.Load15 != 3.33 {
			t.Fatalf("load values not mapped correctly: %+v", point.Load)
		}
		if point.Memory.Total != 1024 || point.Memory.Used != 512 || point.Memory.Available != 512 || point.Memory.UsagePercent != 50 {
			t.Fatalf("memory values not mapped correctly: %+v", point.Memory)
		}
		if point.Swap.Total != 2048 || point.Swap.Used != 128 || point.Swap.Free != 1920 || point.Swap.UsagePercent != 6.25 {
			t.Fatalf("swap values not mapped correctly: %+v", point.Swap)
		}
		if point.Network.RXBytes != 1000 || point.Network.TXBytes != 2000 || point.Network.RXPackets != 10 || point.Network.TXPackets != 20 || point.Network.Errors != 1 || point.Network.Drops != 2 {
			t.Fatalf("network values not mapped correctly: %+v", point.Network)
		}
	})

	t.Run("Multiple historical metrics preserve repository order", func(t *testing.T) {
		resp := service.ToHistoricalMetricsResponse(42, start, end, "", []models.Metric{secondMetric, firstMetric})

		if len(resp.Data) != 2 {
			t.Fatalf("expected 2 historical points, got %d", len(resp.Data))
		}
		if resp.Data[0].CollectedAt != secondCollectedAt.Format(time.RFC3339) || resp.Data[1].CollectedAt != firstCollectedAt.Format(time.RFC3339) {
			t.Fatalf("expected mapper to preserve input order, got %+v", resp.Data)
		}
	})

	t.Run("Empty historical collection maps to empty data", func(t *testing.T) {
		resp := service.ToHistoricalMetricsResponse(42, start, end, "", []models.Metric{})

		if resp.Data == nil {
			t.Fatalf("expected empty data collection, got nil")
		}
		if len(resp.Data) != 0 {
			t.Fatalf("expected empty data collection, got %d", len(resp.Data))
		}
	})
}

func TestLatestMetricMapping_BackwardCompatibility(t *testing.T) {
	collectedAt := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	metricID := uuid.New()
	metric := &models.Metric{
		ID:              metricID,
		ServerID:        42,
		CollectedAt:     collectedAt,
		CPUUsagePercent: 11.25,
		CPUCores:        2,
		Disks: []models.MetricDisk{
			{
				ID:           uuid.New(),
				MetricID:     metricID,
				MountPoint:   "/",
				Filesystem:   "ext4",
				Total:        100,
				Used:         25,
				Free:         75,
				UsagePercent: 25,
			},
		},
	}

	resp := service.ToLatestMetricResponse(metric)

	if resp.ServerID != 42 || resp.CollectedAt != collectedAt.Format(time.RFC3339) {
		t.Fatalf("latest response identity changed unexpectedly: %+v", resp)
	}
	if resp.CPU.UsagePercent != 11.25 || resp.CPU.Cores != 2 {
		t.Fatalf("latest CPU mapping changed unexpectedly: %+v", resp.CPU)
	}
	if len(resp.Disks) != 1 || resp.Disks[0].MountPoint != "/" || resp.Disks[0].Filesystem != "ext4" {
		t.Fatalf("latest disk mapping changed unexpectedly: %+v", resp.Disks)
	}
}
