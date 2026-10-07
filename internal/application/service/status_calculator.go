package service

import (
	"vpsmonitoring-backend/internal/application/dto"
)

const (
	StatusUp       = "UP"
	StatusDown     = "DOWN"
	StatusDegraded = "DEGRADED"
	StatusUnknown  = "UNKNOWN"
)

// CalculateHealthStatus deterministically computes the overall application health
// based on the raw telemetry signals provided by the agent.
func CalculateHealthStatus(entry dto.ApplicationTelemetryEntry) string {
	strongPositives := 0
	strongNegatives := 0
	weakNegatives := 0

	// 1. Evaluate Process Signal
	if entry.ObservationState == "MATCHED" {
		strongPositives++
	} else if entry.ObservationState == "NOT_MATCHED" {
		strongNegatives++
	}
	// UNKNOWN adds nothing to positives or negatives

	// 2. Evaluate Port Signal
	if entry.PortConfigured && entry.PortListening != nil {
		if *entry.PortListening {
			strongPositives++
		} else {
			strongNegatives++
		}
	}

	// 3. Evaluate HTTP Signal
	if entry.HTTPConfigured && entry.HTTPAvailable != nil {
		if *entry.HTTPAvailable {
			strongPositives++
		} else {
			strongNegatives++
		}
	}

	// 4. Evaluate Log Signal
	// Treat access logs as supporting health evidence, not proof by itself.
	if entry.LogConfigured && entry.LogAvailable != nil {
		if !*entry.LogAvailable {
			weakNegatives++
		}
	}

	// 5. Apply Precedence Rules

	// If there is ANY strong positive evidence, the application is at least partially running.
	if strongPositives > 0 {
		// If there is conflicting strong negative evidence (e.g., port failed but process matched)
		// or weak negative evidence (log missing), we consider it DEGRADED.
		if strongNegatives > 0 || weakNegatives > 0 {
			return StatusDegraded
		}
		// All evaluated configured signals are positive or neutral
		return StatusUp
	}

	// If there is NO strong positive evidence, but there IS strong negative evidence
	// (e.g., Process is NOT_MATCHED, or Process is UNKNOWN but Port is NOT listening).
	if strongNegatives > 0 {
		return StatusDown
	}

	// If we have no strong positives and no strong negatives (e.g., everything is UNKNOWN),
	// we cannot reliably determine the status.
	// We also don't let weak negatives (like log missing) drive a DOWN state alone.
	return StatusUnknown
}
