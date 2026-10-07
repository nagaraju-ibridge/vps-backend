package dto

import "time"

type Confidence string

const (
	ConfidenceHigh    Confidence = "HIGH"
	ConfidenceMedium  Confidence = "MEDIUM"
	ConfidenceLow     Confidence = "LOW"
	ConfidenceUnknown Confidence = "UNKNOWN"
)

type DiscoveryPayloadDTO struct {
	CollectedAt  time.Time                 `json:"collected_at"`
	Applications []ApplicationCandidateDTO `json:"applications"`
	Services     []ServiceCandidateDTO     `json:"services"`
	Listeners    []ListenerDTO             `json:"listeners"`
	Warnings     []string                  `json:"warnings,omitempty"`
}

type ApplicationCandidateDTO struct {
	ID                  string             `json:"id"`
	Runtime             string             `json:"runtime"`
	RuntimeConfidence   Confidence         `json:"runtime_confidence"`
	Framework           string             `json:"framework"`
	FrameworkConfidence Confidence         `json:"framework_confidence"`
	Process             ProcessMetadataDTO `json:"process"`
	Ports               []ListenerDTO      `json:"ports,omitempty"`
	SystemdUnit         string             `json:"systemd_unit,omitempty"`
	Evidence            []EvidenceDTO      `json:"evidence"`
}

type ServiceCandidateDTO struct {
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Confidence  Confidence         `json:"confidence"`
	Process     ProcessMetadataDTO `json:"process"`
	Ports       []ListenerDTO      `json:"ports,omitempty"`
	SystemdUnit string             `json:"systemd_unit,omitempty"`
	Evidence    []EvidenceDTO      `json:"evidence"`
}

type ProcessMetadataDTO struct {
	PID             int64         `json:"pid"`
	ParentPID       *int64        `json:"parent_pid,omitempty"`
	Name            string        `json:"name"`
	ExePath         string        `json:"exe_path,omitempty"`
	CmdlineRedacted string        `json:"cmdline_redacted,omitempty"`
	User            string        `json:"user,omitempty"`
	StartTime       *time.Time    `json:"start_time,omitempty"`
	SystemdUnit     string        `json:"systemd_unit,omitempty"`
	Listeners       []ListenerDTO `json:"listeners,omitempty"`
}

type ListenerDTO struct {
	Protocol     string `json:"protocol"`
	LocalAddress string `json:"local_address"`
	LocalPort    int    `json:"local_port"`
	Scope        string `json:"scope"`
	PID          *int64 `json:"pid,omitempty"`
	ProcessName  string `json:"process_name,omitempty"`
}

type EvidenceDTO struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type IngestDiscoveryResponse struct {
	Status   string                 `json:"status"`
	Received DiscoveryReceivedCount `json:"received"`
}

type DiscoveryReceivedCount struct {
	Applications int `json:"applications"`
	Services     int `json:"services"`
	Listeners    int `json:"listeners"`
}

type DiscoverySnapshotResponse struct {
	ServerID          int64                     `json:"server_id"`
	SnapshotAvailable bool                      `json:"snapshot_available"`
	CollectedAt       *time.Time                `json:"collected_at,omitempty"`
	Applications      []ApplicationCandidateDTO `json:"applications"`
	Services          []ServiceCandidateDTO     `json:"services"`
	Listeners         []ListenerDTO             `json:"listeners"`
	Warnings          []string                  `json:"warnings,omitempty"`
}
