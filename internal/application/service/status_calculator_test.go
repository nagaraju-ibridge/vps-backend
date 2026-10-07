package service

import (
	"testing"
	"vpsmonitoring-backend/internal/application/dto"
)

func calcPtrBool(b bool) *bool { return &b }

func TestCalculateHealthStatus(t *testing.T) {
	tests := []struct {
		name     string
		entry    dto.ApplicationTelemetryEntry
		expected string
	}{
		{
			name: "A. Process matched + no optional checks configured -> UP",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
			},
			expected: StatusUp,
		},
		{
			name: "B. Process matched + configured port listening -> UP",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				PortConfigured:   true,
				PortListening:    calcPtrBool(true),
			},
			expected: StatusUp,
		},
		{
			name: "C. Process matched + configured HTTP succeeds -> UP",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				HTTPConfigured:   true,
				HTTPAvailable:    calcPtrBool(true),
			},
			expected: StatusUp,
		},
		{
			name: "D. Process matched + HTTP fails -> DEGRADED",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				HTTPConfigured:   true,
				HTTPAvailable:    calcPtrBool(false),
			},
			expected: StatusDegraded,
		},
		{
			name: "E. Process matched + port not listening -> DEGRADED",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				PortConfigured:   true,
				PortListening:    calcPtrBool(false),
			},
			expected: StatusDegraded,
		},
		{
			name: "F. Process explicitly NOT_MATCHED -> DOWN",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "NOT_MATCHED",
			},
			expected: StatusDown,
		},
		{
			name: "G. Process UNKNOWN + port listening -> UP",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "UNKNOWN",
				PortConfigured:   true,
				PortListening:    calcPtrBool(true),
			},
			expected: StatusUp,
		},
		{
			name: "H. Process UNKNOWN + HTTP succeeds -> UP",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "UNKNOWN",
				HTTPConfigured:   true,
				HTTPAvailable:    calcPtrBool(true),
			},
			expected: StatusUp,
		},
		{
			name: "I. Process UNKNOWN + no positive evidence -> UNKNOWN",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "UNKNOWN",
			},
			expected: StatusUnknown,
		},
		{
			name: "J. No configured optional signals -> rely on process evidence only (MATCHED)",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				PortConfigured:   false,
				HTTPConfigured:   false,
				LogConfigured:    false,
			},
			expected: StatusUp,
		},
		{
			name: "K. Log unavailable while process is healthy -> DEGRADED if log monitoring is explicitly configured",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				LogConfigured:    true,
				LogAvailable:     calcPtrBool(false),
			},
			expected: StatusDegraded,
		},
		{
			name: "L. Log unavailable when log monitoring is not configured -> ignore it",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "MATCHED",
				LogConfigured:    false, // explicitly not configured
				LogAvailable:     calcPtrBool(false),
			},
			expected: StatusUp,
		},
		{
			name: "M. Conflicting signals -> verify the documented precedence (NOT_MATCHED but PortListening=true)",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "NOT_MATCHED",
				PortConfigured:   true,
				PortListening:    calcPtrBool(true),
			},
			expected: StatusDegraded,
		},
		{
			name: "N. All meaningful signals UNKNOWN -> UNKNOWN",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "UNKNOWN",
				PortConfigured:   true,
				PortListening:    nil,
				HTTPConfigured:   true,
				HTTPAvailable:    nil,
			},
			expected: StatusUnknown,
		},
		{
			name: "O. Ensure UNKNOWN process state caused by permissions never becomes DOWN without independent strong negative evidence.",
			entry: dto.ApplicationTelemetryEntry{
				ObservationState: "UNKNOWN",
				LogConfigured:    true,
				LogAvailable:     calcPtrBool(false), // Weak negative evidence
			},
			expected: StatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalculateHealthStatus(tt.entry)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}
