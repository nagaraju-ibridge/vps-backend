package dto_test

import (
	"testing"
	"time"

	"vpsmonitoring-backend/internal/metric/dto"
)

func TestIngestMetricRequest_Validate(t *testing.T) {
	validTimestamp := time.Now().UTC()

	tests := []struct {
		name    string
		req     dto.IngestMetricRequest
		wantErr bool
	}{
		{
			name: "Valid payload",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				CPU: dto.CPUMetricsDTO{
					UsagePercent: 45.0,
					Cores:        8,
					Load1:        1.2,
					Load5:        1.0,
					Load15:       0.8,
				},
				Memory: dto.MemoryMetricsDTO{
					UsagePercent: 60.0,
				},
				Swap: dto.SwapMetricsDTO{
					UsagePercent: 10.0,
				},
				Disk: dto.DiskMetricsDTO{
					MountPoints: []dto.MountPointDTO{
						{Path: "/", UsagePercent: 40.0},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "Zero timestamp rejected",
			req: dto.IngestMetricRequest{
				Timestamp: time.Time{},
			},
			wantErr: true,
		},
		{
			name: "Future timestamp > 1 hour rejected",
			req: dto.IngestMetricRequest{
				Timestamp: time.Now().UTC().Add(2 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "Past timestamp > 24 hours rejected",
			req: dto.IngestMetricRequest{
				Timestamp: time.Now().UTC().Add(-25 * time.Hour),
			},
			wantErr: true,
		},
		{
			name: "Negative CPU usage percent rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				CPU:       dto.CPUMetricsDTO{UsagePercent: -1.0},
			},
			wantErr: true,
		},
		{
			name: "CPU usage percent > 100 rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				CPU:       dto.CPUMetricsDTO{UsagePercent: 100.1},
			},
			wantErr: true,
		},
		{
			name: "Negative CPU cores rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				CPU:       dto.CPUMetricsDTO{Cores: -1},
			},
			wantErr: true,
		},
		{
			name: "Negative load average rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				CPU:       dto.CPUMetricsDTO{Load1: -0.1},
			},
			wantErr: true,
		},
		{
			name: "High load average (> 100) accepted",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				CPU:       dto.CPUMetricsDTO{Load1: 150.0}, // Valid on 128 core machines
			},
			wantErr: false,
		},
		{
			name: "Memory usage percent > 100 rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				Memory:    dto.MemoryMetricsDTO{UsagePercent: 101.0},
			},
			wantErr: true,
		},
		{
			name: "Swap usage percent > 100 rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				Swap:      dto.SwapMetricsDTO{UsagePercent: 105.0},
			},
			wantErr: true,
		},
		{
			name: "Disk mount with empty path rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				Disk: dto.DiskMetricsDTO{
					MountPoints: []dto.MountPointDTO{
						{Path: "", UsagePercent: 20.0},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "Disk usage percent > 100 rejected",
			req: dto.IngestMetricRequest{
				Timestamp: validTimestamp,
				Disk: dto.DiskMetricsDTO{
					MountPoints: []dto.MountPointDTO{
						{Path: "/", UsagePercent: 101.0},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
