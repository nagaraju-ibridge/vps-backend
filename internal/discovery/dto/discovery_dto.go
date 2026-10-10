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
	Websites     []WebsiteCandidateDTO     `json:"websites,omitempty"`
	HostingUsers []HostingUserDTO          `json:"hosting_users,omitempty"`
}

type WebsiteCandidateDTO struct {
	Domains       []WebDomainCandidateDTO `json:"domains"`
	DocumentRoot  string                  `json:"document_root,omitempty"`
	WebServerKind string                  `json:"web_server_kind"`
	Confidence    Confidence              `json:"confidence"`
	Source        string                  `json:"source"`
}

type WebDomainCandidateDTO struct {
	DomainName string `json:"name"`
	IsPrimary  bool   `json:"is_primary"`
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
	Websites          []WebsiteCandidateDTO     `json:"websites,omitempty"`
	HostingUsers      []HostingUserDTO          `json:"hosting_users,omitempty"`
}

type HostingWebDomainDTO struct {
	Domain         string `json:"domain"`
	IP             string `json:"ip"`
	Template       string `json:"template"`
	SSL            string `json:"ssl"`
	DiskUsageMB    int64  `json:"disk_usage_mb"`
	BandwidthMB    int64  `json:"bandwidth_mb"`
	IsSuspended    bool   `json:"is_suspended"`
	DateCreated    string `json:"date_created"`
	Aliases        string `json:"aliases,omitempty"`
	HTTPStatus      int    `json:"http_status,omitempty"`
	ResponseTimeMs  int64  `json:"response_time_ms,omitempty"`
	DNSLookupMs     int64  `json:"dns_lookup_ms,omitempty"`
	ConnectTimeMs   int64  `json:"connect_time_ms,omitempty"`
	TLSHandshakeMs  int64  `json:"tls_handshake_ms,omitempty"`
	TTFBMs          int64             `json:"ttfb_ms,omitempty"`
	DownloadedBytes int64             `json:"downloaded_bytes,omitempty"`
	SSLValid        bool              `json:"ssl_valid,omitempty"`
	SSLDaysLeft     int               `json:"ssl_days_left,omitempty"`
	SSLExpiryDate   string            `json:"ssl_expiry_date,omitempty"`
	SSLIssuer       string            `json:"ssl_issuer,omitempty"`
	Traffic         *DomainTrafficDTO `json:"traffic,omitempty"`
	CPUPercent      float64           `json:"cpu_percent,omitempty"`
	MemoryMB        int64             `json:"memory_mb,omitempty"`
	WorkerProcesses int               `json:"worker_processes,omitempty"`
	LoadRPM         float64           `json:"load_rpm,omitempty"`
}

type TrafficItemDTO struct {
	Key   string `json:"key"`
	Count int64  `json:"count"`
}

type TimeTrafficItemDTO struct {
	Time  string `json:"time"`
	Count int64  `json:"count"`
}

type AccessLogEntryDTO struct {
	ClientIP  string `json:"client_ip"`
	Timestamp string `json:"timestamp"`
	Method    string `json:"method"`
	URL       string `json:"url"`
	Status    int    `json:"status"`
	Bytes     int64  `json:"bytes"`
	Referer   string `json:"referer,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

type ErrorLogEntryDTO struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level,omitempty"`
	Message   string `json:"message"`
	Referer   string `json:"referer,omitempty"`
	Client    string `json:"client,omitempty"`
}

type DomainTrafficDTO struct {
	Domain             string               `json:"domain"`
	LogPath            string               `json:"log_path,omitempty"`
	LogSizeBytes       int64                `json:"log_size_bytes,omitempty"`
	TotalRequests      int64                `json:"total_requests"`
	GeneratedAt        string               `json:"generated_at,omitempty"`
	TotalEntries       int64                `json:"total_entries"`
	Successful2xx      int64                `json:"successful_2xx"`
	Redirects3xx       int64                `json:"redirects_3xx"`
	ClientErrors4xx    int64                `json:"client_errors_4xx"`
	ServerErrors5xx    int64                `json:"server_errors_5xx"`
	StatusCodes        map[string]int64     `json:"status_codes,omitempty"`
	Methods            map[string]int64     `json:"methods,omitempty"`
	TopURLs            []TrafficItemDTO     `json:"top_urls,omitempty"`
	TopClientIPs       []TrafficItemDTO     `json:"top_client_ips,omitempty"`
	RequestsWithBytes  int64                `json:"requests_with_bytes"`
	TotalResponseBytes int64                `json:"total_response_bytes"`
	RequestsByMinute   []TimeTrafficItemDTO `json:"requests_by_minute,omitempty"`
	LatestRequests     []AccessLogEntryDTO  `json:"latest_requests,omitempty"`
	LatestErrors       []ErrorLogEntryDTO   `json:"latest_errors,omitempty"`
}

type HostingUserDTO struct {
	Username          string                `json:"username"`
	FullName          string                `json:"full_name,omitempty"`
	Email             string                `json:"email,omitempty"`
	Role              string                `json:"role"`
	Package           string                `json:"package"`
	Language          string                `json:"language,omitempty"`
	Theme             string                `json:"theme,omitempty"`
	WebCount          int                   `json:"web_count"`
	DNSCount          int                   `json:"dns_count"`
	MailCount         int                   `json:"mail_count"`
	DBCount           int                   `json:"db_count"`
	DiskUsageMB       int64                 `json:"disk_usage_mb"`
	BandwidthMB       int64                 `json:"bandwidth_mb"`
	IsSuspended       bool                  `json:"is_suspended"`
	DateCreated       string                `json:"date_created"`
	TimeCreated       string                `json:"time_created,omitempty"`
	HomeDir           string                `json:"home_dir"`
	Shell             string                `json:"shell"`
	LinuxUID          int                   `json:"linux_uid"`
	Websites          []string              `json:"websites"`
	WebDomainsQuota   string                `json:"web_domains_quota,omitempty"`
	WebAliasesQuota   string                `json:"web_aliases_quota,omitempty"`
	DNSDomainsQuota   string                `json:"dns_domains_quota,omitempty"`
	DNSRecordsQuota   string                `json:"dns_records_quota,omitempty"`
	MailDomainsQuota  string                `json:"mail_domains_quota,omitempty"`
	MailAccountsQuota string                `json:"mail_accounts_quota,omitempty"`
	BackupsQuota      string                `json:"backups_quota,omitempty"`
	DatabasesQuota    string                `json:"databases_quota,omitempty"`
	CronJobsQuota     string                `json:"cron_jobs_quota,omitempty"`
	DiskQuota         string                `json:"disk_quota,omitempty"`
	BandwidthQuota    string                `json:"bandwidth_quota,omitempty"`
	IPAddressesQuota  string                `json:"ip_addresses_quota,omitempty"`
	CPUPercent        float64               `json:"cpu_percent,omitempty"`
	MemoryMB          int64                 `json:"memory_mb,omitempty"`
	WorkerProcesses   int                   `json:"worker_processes,omitempty"`
	WebDomainList     []HostingWebDomainDTO `json:"web_domain_list,omitempty"`
}

