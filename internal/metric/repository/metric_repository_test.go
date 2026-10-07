package repository_test

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/metric/models"
	"vpsmonitoring-backend/internal/metric/repository"
)

func newMockMetricRepository(t *testing.T) (repository.MetricRepository, sqlmock.Sqlmock) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(postgres.New(postgres.Config{
		Conn:                 sqlDB,
		PreferSimpleProtocol: true,
	}), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open gorm db: %v", err)
	}

	return repository.NewMetricRepository(db), mock
}

func newMetricRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "server_id", "agent_id", "collected_at", "received_at",
		"cpu_usage_percent", "cpu_cores", "load_1", "load_5", "load_15",
		"memory_total", "memory_used", "memory_available", "memory_usage_percent",
		"swap_total", "swap_used", "swap_free", "swap_usage_percent",
		"network_rx_bytes", "network_tx_bytes", "network_rx_packets", "network_tx_packets",
		"network_errors", "network_drops", "created_at",
	})
}

func addMetricRow(rows *sqlmock.Rows, id uuid.UUID, serverID int64, collectedAt time.Time) *sqlmock.Rows {
	return rows.AddRow(
		id, serverID, "agent-123", collectedAt, collectedAt.Add(time.Second),
		10.0, 2, 0.1, 0.2, 0.3,
		uint64(8000), uint64(3200), uint64(4800), 40.0,
		uint64(2000), uint64(100), uint64(1900), 5.0,
		uint64(10000), uint64(20000), uint64(100), uint64(200),
		uint64(1), uint64(2), collectedAt.Add(2*time.Second),
	)
}

func TestMetricRepository_GetLatestByServerID(t *testing.T) {
	ctx := context.Background()
	serverID := int64(42)
	olderMetricID := uuid.New()
	latestMetricID := uuid.New()
	collectedAt := time.Date(2026, 9, 29, 12, 30, 0, 0, time.UTC)
	receivedAt := collectedAt.Add(2 * time.Second)
	createdAt := collectedAt.Add(3 * time.Second)

	t.Run("returns newest metric ordered by collected_at desc with disk records preloaded", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		metricRows := sqlmock.NewRows([]string{
			"id", "server_id", "agent_id", "collected_at", "received_at",
			"cpu_usage_percent", "cpu_cores", "load_1", "load_5", "load_15",
			"memory_total", "memory_used", "memory_available", "memory_usage_percent",
			"swap_total", "swap_used", "swap_free", "swap_usage_percent",
			"network_rx_bytes", "network_tx_bytes", "network_rx_packets", "network_tx_packets",
			"network_errors", "network_drops", "created_at",
		}).AddRow(
			latestMetricID, serverID, "agent-123", collectedAt, receivedAt,
			37.5, 4, 0.7, 0.6, 0.5,
			uint64(8000), uint64(3200), uint64(4800), 40.0,
			uint64(2000), uint64(100), uint64(1900), 5.0,
			uint64(10000), uint64(20000), uint64(100), uint64(200),
			uint64(1), uint64(2), createdAt,
		)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metrics" WHERE server_id = $1 ORDER BY collected_at DESC,"metrics"."id" LIMIT $2`)).
			WithArgs(serverID, 1).
			WillReturnRows(metricRows)

		diskRows := sqlmock.NewRows([]string{
			"id", "metric_id", "mount_point", "filesystem", "total", "used", "free", "usage_percent", "created_at",
		}).AddRow(
			uuid.New(), latestMetricID, "/", "ext4", uint64(100000), uint64(25000), uint64(75000), 25.0, createdAt,
		)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metric_disks" WHERE "metric_disks"."metric_id" = $1`)).
			WithArgs(latestMetricID).
			WillReturnRows(diskRows)

		metric, err := repo.GetLatestByServerID(ctx, serverID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metric.ID != latestMetricID {
			t.Fatalf("expected latest metric ID %s, got %s; older metric ID was %s", latestMetricID, metric.ID, olderMetricID)
		}
		if metric.CollectedAt != collectedAt {
			t.Fatalf("expected collected_at %s, got %s", collectedAt, metric.CollectedAt)
		}
		if len(metric.Disks) != 1 {
			t.Fatalf("expected disk records to be preloaded, got %d", len(metric.Disks))
		}
		if metric.Disks[0].MetricID != latestMetricID || metric.Disks[0].MountPoint != "/" {
			t.Fatalf("unexpected disk preload result: %+v", metric.Disks[0])
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("no metric returns gorm ErrRecordNotFound", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metrics" WHERE server_id = $1 ORDER BY collected_at DESC,"metrics"."id" LIMIT $2`)).
			WithArgs(serverID, 1).
			WillReturnError(gorm.ErrRecordNotFound)

		_, err := repo.GetLatestByServerID(ctx, serverID)
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("expected gorm.ErrRecordNotFound, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("repository query error is returned", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)
		dbErr := errors.New("database unavailable")

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metrics" WHERE server_id = $1 ORDER BY collected_at DESC,"metrics"."id" LIMIT $2`)).
			WithArgs(serverID, 1).
			WillReturnError(dbErr)

		_, err := repo.GetLatestByServerID(ctx, serverID)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected database error, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}

func TestMetricRepository_CreateMetric(t *testing.T) {
	ctx := context.Background()
	metricID := uuid.New()
	collectedAt := time.Date(2026, 9, 29, 12, 30, 0, 0, time.UTC)

	t.Run("persists metric and disk records transactionally", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)
		metric := &models.Metric{
			ID:          metricID,
			ServerID:    42,
			AgentID:     "agent-123",
			CollectedAt: collectedAt,
			ReceivedAt:  collectedAt.Add(time.Second),
		}
		disks := []models.MetricDisk{
			{
				MountPoint:   "/",
				Filesystem:   "ext4",
				Total:        100000,
				Used:         25000,
				Free:         75000,
				UsagePercent: 25,
			},
		}

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "metrics"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(metricID))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "metric_disks"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(uuid.New()))
		mock.ExpectCommit()

		if err := repo.CreateMetric(ctx, metric, disks); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if disks[0].MetricID != metricID {
			t.Fatalf("expected disk MetricID to be assigned parent metric ID %s, got %s", metricID, disks[0].MetricID)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("disk insert failure rolls back transaction", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)
		metric := &models.Metric{
			ID:          metricID,
			ServerID:    42,
			AgentID:     "agent-123",
			CollectedAt: collectedAt,
			ReceivedAt:  collectedAt.Add(time.Second),
		}
		disks := []models.MetricDisk{{MountPoint: "/", Filesystem: "ext4"}}
		diskErr := errors.New("disk insert failed")

		mock.ExpectBegin()
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "metrics"`)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(metricID))
		mock.ExpectQuery(regexp.QuoteMeta(`INSERT INTO "metric_disks"`)).
			WillReturnError(diskErr)
		mock.ExpectRollback()

		err := repo.CreateMetric(ctx, metric, disks)
		if !errors.Is(err, diskErr) {
			t.Fatalf("expected disk insert error, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}

func TestMetricRepository_GetHistoryByServerID(t *testing.T) {
	ctx := context.Background()
	serverID := int64(1)
	start := time.Date(2026, 9, 30, 10, 1, 0, 0, time.UTC)
	end := time.Date(2026, 9, 30, 10, 2, 0, 0, time.UTC)
	limit := 100

	historyQuery := regexp.QuoteMeta(`SELECT * FROM "metrics" WHERE server_id = $1 AND collected_at >= $2 AND collected_at <= $3 ORDER BY collected_at ASC LIMIT $4`)

	t.Run("returns records within requested range in chronological order with inclusive boundaries", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		rows := newMetricRows()
		addMetricRow(rows, uuid.New(), serverID, start)
		addMetricRow(rows, uuid.New(), serverID, end)

		mock.ExpectQuery(historyQuery).
			WithArgs(serverID, start, end, limit).
			WillReturnRows(rows)

		metrics, err := repo.GetHistoryByServerID(ctx, serverID, start, end, limit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metrics) != 2 {
			t.Fatalf("expected 2 metrics, got %d", len(metrics))
		}
		if !metrics[0].CollectedAt.Equal(start) {
			t.Fatalf("expected first metric at start boundary %s, got %s", start, metrics[0].CollectedAt)
		}
		if !metrics[1].CollectedAt.Equal(end) {
			t.Fatalf("expected second metric at end boundary %s, got %s", end, metrics[1].CollectedAt)
		}
		if metrics[0].CPUUsagePercent != 10.0 || metrics[0].CPUCores != 2 {
			t.Fatalf("expected historical CPU fields to be preserved, got %+v", metrics[0])
		}
		if metrics[0].Load1 != 0.1 || metrics[0].Load5 != 0.2 || metrics[0].Load15 != 0.3 {
			t.Fatalf("expected historical load fields to be preserved, got %+v", metrics[0])
		}
		if metrics[0].MemoryTotal != 8000 || metrics[0].MemoryUsed != 3200 || metrics[0].MemoryAvailable != 4800 || metrics[0].MemoryUsagePercent != 40.0 {
			t.Fatalf("expected historical memory fields to be preserved, got %+v", metrics[0])
		}
		if metrics[0].SwapTotal != 2000 || metrics[0].SwapUsed != 100 || metrics[0].SwapFree != 1900 || metrics[0].SwapUsagePercent != 5.0 {
			t.Fatalf("expected historical swap fields to be preserved, got %+v", metrics[0])
		}
		if metrics[0].NetworkRXBytes != 10000 || metrics[0].NetworkTXBytes != 20000 || metrics[0].NetworkRXPackets != 100 || metrics[0].NetworkTXPackets != 200 || metrics[0].NetworkErrors != 1 || metrics[0].NetworkDrops != 2 {
			t.Fatalf("expected historical network fields to be preserved, got %+v", metrics[0])
		}
		if len(metrics[0].Disks) != 0 || len(metrics[1].Disks) != 0 {
			t.Fatalf("expected historical query not to preload disks, got %+v", metrics)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("always filters by requested server ID", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)
		serverOneMetricID := uuid.New()

		rows := newMetricRows()
		addMetricRow(rows, serverOneMetricID, serverID, start)

		mock.ExpectQuery(historyQuery).
			WithArgs(serverID, start, end, limit).
			WillReturnRows(rows)

		metrics, err := repo.GetHistoryByServerID(ctx, serverID, start, end, limit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metrics) != 1 {
			t.Fatalf("expected 1 server-filtered metric, got %d", len(metrics))
		}
		if metrics[0].ServerID != serverID || metrics[0].ID != serverOneMetricID {
			t.Fatalf("expected only server %d metric %s, got %+v", serverID, serverOneMetricID, metrics[0])
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("applies supplied limit", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)
		requestedLimit := 2

		rows := newMetricRows()
		addMetricRow(rows, uuid.New(), serverID, start)
		addMetricRow(rows, uuid.New(), serverID, start.Add(time.Minute))

		mock.ExpectQuery(historyQuery).
			WithArgs(serverID, start, end, requestedLimit).
			WillReturnRows(rows)

		metrics, err := repo.GetHistoryByServerID(ctx, serverID, start, end, requestedLimit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metrics) != requestedLimit {
			t.Fatalf("expected %d metrics, got %d", requestedLimit, len(metrics))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("empty range returns empty collection and no error", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		mock.ExpectQuery(historyQuery).
			WithArgs(serverID, start, end, limit).
			WillReturnRows(newMetricRows())

		metrics, err := repo.GetHistoryByServerID(ctx, serverID, start, end, limit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metrics) != 0 {
			t.Fatalf("expected empty metric collection, got %d", len(metrics))
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("database query error is returned", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)
		dbErr := errors.New("database unavailable")

		mock.ExpectQuery(historyQuery).
			WithArgs(serverID, start, end, limit).
			WillReturnError(dbErr)

		_, err := repo.GetHistoryByServerID(ctx, serverID, start, end, limit)
		if !errors.Is(err, dbErr) {
			t.Fatalf("expected database error, got %v", err)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}

func TestMetricRepository_GetAggregatedMetrics(t *testing.T) {
	ctx := context.Background()
	serverID := int64(42)
	start := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute)

	t.Run("uses PostgreSQL bucket aggregation and independent disk query", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		metricColumns := make([]string, 50)
		metricValues := make([]driver.Value, 50)
		for i := range metricColumns {
			metricColumns[i] = "c" + string(rune('a'+i%26)) + string(rune('a'+i/26))
			metricValues[i] = nil
		}
		metricValues[0] = start
		metricValues[1] = 12.5
		metricValues[2] = 10.0
		metricValues[3] = 15.0
		metricValues[28] = 256.0

		mock.ExpectQuery(regexp.QuoteMeta("WITH buckets AS")).
			WithArgs(serverID, start, end, "5 minutes").
			WillReturnRows(sqlmock.NewRows(metricColumns).AddRow(metricValues...))

		diskRows := sqlmock.NewRows([]string{
			"bucket_start", "mount_point", "usage_avg", "usage_min", "usage_max", "total_avg", "used_avg", "free_avg",
		}).AddRow(start, "/", 55.5, 50.0, 60.0, 1000.0, 555.0, 445.0)

		mock.ExpectQuery(regexp.QuoteMeta("WITH buckets AS")).
			WithArgs(serverID, start, end, "5 minutes").
			WillReturnRows(diskRows)

		metricRows, aggregateDiskRows, err := repo.GetAggregatedMetrics(ctx, serverID, start, end, "5m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metricRows) != 1 {
			t.Fatalf("expected 1 metric aggregate row, got %d", len(metricRows))
		}
		if !metricRows[0].BucketStart.Equal(start) || !metricRows[0].CPUAvg.Valid || metricRows[0].CPUAvg.Float64 != 12.5 {
			t.Fatalf("metric aggregate row not scanned correctly: %+v", metricRows[0])
		}
		if !metricRows[0].NetworkRXBytesDelta.Valid || metricRows[0].NetworkRXBytesDelta.Float64 != 256 {
			t.Fatalf("network delta not scanned correctly: %+v", metricRows[0].NetworkRXBytesDelta)
		}
		if len(aggregateDiskRows) != 1 || aggregateDiskRows[0].MountPoint != "/" || !aggregateDiskRows[0].UsageAvg.Valid || aggregateDiskRows[0].UsageAvg.Float64 != 55.5 {
			t.Fatalf("disk aggregate row not scanned correctly: %+v", aggregateDiskRows)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("empty range returns no rows without querying", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		metricRows, diskRows, err := repo.GetAggregatedMetrics(ctx, serverID, start, start, "5m")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metricRows) != 0 || len(diskRows) != 0 {
			t.Fatalf("expected empty rows for empty range, got metrics=%d disks=%d", len(metricRows), len(diskRows))
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unexpected sql query: %v", err)
		}
	})
}

func TestMetricRepository_LoadFieldsMapFromDatabaseColumns(t *testing.T) {
	ctx := context.Background()
	serverID := int64(42)
	metricID := uuid.New()
	collectedAt := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	start := collectedAt.Add(-time.Minute)
	end := collectedAt.Add(time.Minute)
	limit := 10

	t.Run("history query scans load_1 load_5 load_15 into Load1 Load5 Load15", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		rows := newMetricRows()
		rows.AddRow(
			metricID, serverID, "agent-123", collectedAt, collectedAt.Add(time.Second),
			12.5, 4, 1.11, 2.22, 3.33,
			uint64(8000), uint64(3200), uint64(4800), 40.0,
			uint64(2000), uint64(100), uint64(1900), 5.0,
			uint64(10000), uint64(20000), uint64(100), uint64(200),
			uint64(1), uint64(2), collectedAt.Add(2*time.Second),
		)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metrics" WHERE server_id = $1 AND collected_at >= $2 AND collected_at <= $3 ORDER BY collected_at ASC LIMIT $4`)).
			WithArgs(serverID, start, end, limit).
			WillReturnRows(rows)

		metrics, err := repo.GetHistoryByServerID(ctx, serverID, start, end, limit)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(metrics) != 1 {
			t.Fatalf("expected 1 metric, got %d", len(metrics))
		}
		if metrics[0].Load1 != 1.11 || metrics[0].Load5 != 2.22 || metrics[0].Load15 != 3.33 {
			t.Fatalf("load fields mapped incorrectly: Load1=%v Load5=%v Load15=%v", metrics[0].Load1, metrics[0].Load5, metrics[0].Load15)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})

	t.Run("latest query scans load_1 load_5 load_15 into Load1 Load5 Load15", func(t *testing.T) {
		repo, mock := newMockMetricRepository(t)

		rows := newMetricRows()
		rows.AddRow(
			metricID, serverID, "agent-123", collectedAt, collectedAt.Add(time.Second),
			12.5, 4, 1.11, 2.22, 3.33,
			uint64(8000), uint64(3200), uint64(4800), 40.0,
			uint64(2000), uint64(100), uint64(1900), 5.0,
			uint64(10000), uint64(20000), uint64(100), uint64(200),
			uint64(1), uint64(2), collectedAt.Add(2*time.Second),
		)

		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metrics" WHERE server_id = $1 ORDER BY collected_at DESC,"metrics"."id" LIMIT $2`)).
			WithArgs(serverID, 1).
			WillReturnRows(rows)
		mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "metric_disks" WHERE "metric_disks"."metric_id" = $1`)).
			WithArgs(metricID).
			WillReturnRows(sqlmock.NewRows([]string{
				"id", "metric_id", "mount_point", "filesystem", "total", "used", "free", "usage_percent", "created_at",
			}))

		metric, err := repo.GetLatestByServerID(ctx, serverID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if metric.Load1 != 1.11 || metric.Load5 != 2.22 || metric.Load15 != 3.33 {
			t.Fatalf("load fields mapped incorrectly: Load1=%v Load5=%v Load15=%v", metric.Load1, metric.Load5, metric.Load15)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sql expectations: %v", err)
		}
	})
}
