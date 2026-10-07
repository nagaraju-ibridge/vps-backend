package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"unicode"

	"vpsmonitoring-backend/internal/discovery/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
)

const (
	MaxApplications      = 100
	MaxServices          = 100
	MaxListeners         = 300
	MaxEvidencePerItem   = 10
	MaxWarnings          = 20
	MaxCommandLineLength = 512
	MaxStringLength      = 512
	MaxEvidenceLength    = 256
	MaxWarningLength     = 256
	FutureSkew           = 5 * time.Minute
)

var (
	ErrInvalidPayload   = errors.New("invalid discovery payload")
	ErrStaleSnapshot    = errors.New("stale discovery snapshot")
	ErrSnapshotNotFound = errors.New("discovery snapshot not found")
)

type DiscoveryService interface {
	IngestDiscovery(ctx context.Context, serverID int64, payload dto.DiscoveryPayloadDTO) (dto.DiscoveryReceivedCount, error)
	GetDiscovery(ctx context.Context, serverID, userID int64) (*dto.DiscoverySnapshotResponse, error)
}

type discoveryService struct {
	cache     DiscoveryCache
	serverSvc serverService.ServerService
	now       func() time.Time
}

func NewDiscoveryService(cache DiscoveryCache, serverSvc serverService.ServerService) DiscoveryService {
	return &discoveryService{cache: cache, serverSvc: serverSvc, now: time.Now}
}

func (s *discoveryService) IngestDiscovery(ctx context.Context, serverID int64, payload dto.DiscoveryPayloadDTO) (dto.DiscoveryReceivedCount, error) {
	if err := s.validate(payload); err != nil {
		return dto.DiscoveryReceivedCount{}, err
	}

	snapshot := DiscoverySnapshot{
		ServerID:     serverID,
		CollectedAt:  payload.CollectedAt.UTC(),
		Applications: payload.Applications,
		Services:     payload.Services,
		Listeners:    payload.Listeners,
		Warnings:     payload.Warnings,
	}

	if ok := s.cache.SetIfNotStale(serverID, snapshot); !ok {
		return dto.DiscoveryReceivedCount{}, ErrStaleSnapshot
	}

	return dto.DiscoveryReceivedCount{
		Applications: len(payload.Applications),
		Services:     len(payload.Services),
		Listeners:    len(payload.Listeners),
	}, nil
}

func (s *discoveryService) GetDiscovery(ctx context.Context, serverID, userID int64) (*dto.DiscoverySnapshotResponse, error) {
	if serverID <= 0 || userID <= 0 {
		return nil, serverService.ErrServerNotFound
	}
	if s.serverSvc == nil {
		return nil, errors.New("server service is nil")
	}

	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil {
		return nil, err
	}
	if server == nil {
		return nil, serverService.ErrServerNotFound
	}

	snapshot, ok := s.cache.Get(serverID)
	if !ok {
		return &dto.DiscoverySnapshotResponse{
			ServerID:          serverID,
			SnapshotAvailable: false,
			Applications:      []dto.ApplicationCandidateDTO{},
			Services:          []dto.ServiceCandidateDTO{},
			Listeners:         []dto.ListenerDTO{},
			Warnings:          []string{},
		}, nil
	}

	collectedAt := snapshot.CollectedAt.UTC()
	return &dto.DiscoverySnapshotResponse{
		ServerID:          serverID,
		SnapshotAvailable: true,
		CollectedAt:       &collectedAt,
		Applications:      copyApplications(snapshot.Applications),
		Services:          copyServices(snapshot.Services),
		Listeners:         copyListeners(snapshot.Listeners),
		Warnings:          append([]string(nil), snapshot.Warnings...),
	}, nil
}

func (s *discoveryService) validate(payload dto.DiscoveryPayloadDTO) error {
	if payload.CollectedAt.IsZero() {
		return fmt.Errorf("%w: collected_at is required", ErrInvalidPayload)
	}
	if payload.CollectedAt.After(s.now().UTC().Add(FutureSkew)) {
		return fmt.Errorf("%w: collected_at is in the future", ErrInvalidPayload)
	}
	if len(payload.Applications) > MaxApplications {
		return fmt.Errorf("%w: applications exceeds %d", ErrInvalidPayload, MaxApplications)
	}
	if len(payload.Services) > MaxServices {
		return fmt.Errorf("%w: services exceeds %d", ErrInvalidPayload, MaxServices)
	}
	if len(payload.Listeners) > MaxListeners {
		return fmt.Errorf("%w: listeners exceeds %d", ErrInvalidPayload, MaxListeners)
	}
	if len(payload.Warnings) > MaxWarnings {
		return fmt.Errorf("%w: warnings exceeds %d", ErrInvalidPayload, MaxWarnings)
	}

	for i, warning := range payload.Warnings {
		if err := validateSafeString(warning, MaxWarningLength); err != nil {
			return fmt.Errorf("%w: warning[%d]", ErrInvalidPayload, i)
		}
	}

	for i, listener := range payload.Listeners {
		if err := validateListener(listener); err != nil {
			return fmt.Errorf("%w: listener[%d]: %v", ErrInvalidPayload, i, err)
		}
	}
	for i, app := range payload.Applications {
		if err := validateApplication(app); err != nil {
			return fmt.Errorf("%w: application[%d]: %v", ErrInvalidPayload, i, err)
		}
	}
	for i, svc := range payload.Services {
		if err := validateService(svc); err != nil {
			return fmt.Errorf("%w: service[%d]: %v", ErrInvalidPayload, i, err)
		}
	}

	return nil
}

func validateApplication(app dto.ApplicationCandidateDTO) error {
	if err := validateSafeString(app.ID, MaxStringLength); err != nil {
		return errors.New("invalid id")
	}
	if !validRuntime(app.Runtime) {
		return errors.New("invalid runtime")
	}
	if !validConfidence(app.RuntimeConfidence) || !validConfidence(app.FrameworkConfidence) {
		return errors.New("invalid confidence")
	}
	if !validFramework(app.Framework) {
		return errors.New("invalid framework")
	}
	if err := validateProcess(app.Process); err != nil {
		return err
	}
	if len(app.Ports) > MaxListeners {
		return errors.New("too many ports")
	}
	for _, port := range app.Ports {
		if err := validateListener(port); err != nil {
			return err
		}
	}
	return validateEvidence(app.Evidence)
}

func validateService(svc dto.ServiceCandidateDTO) error {
	if err := validateSafeString(svc.ID, MaxStringLength); err != nil {
		return errors.New("invalid id")
	}
	if !validServiceKind(svc.Kind) {
		return errors.New("invalid service kind")
	}
	if !validConfidence(svc.Confidence) {
		return errors.New("invalid confidence")
	}
	if err := validateProcess(svc.Process); err != nil {
		return err
	}
	if len(svc.Ports) > MaxListeners {
		return errors.New("too many ports")
	}
	for _, port := range svc.Ports {
		if err := validateListener(port); err != nil {
			return err
		}
	}
	return validateEvidence(svc.Evidence)
}

func validateProcess(process dto.ProcessMetadataDTO) error {
	if process.PID < 0 {
		return errors.New("invalid pid")
	}
	if strings.TrimSpace(process.Name) == "" {
		return errors.New("process name is required")
	}
	for _, value := range []string{process.Name, process.ExePath, process.User, process.SystemdUnit} {
		if err := validateSafeString(value, MaxStringLength); err != nil {
			return errors.New("invalid process metadata")
		}
	}
	if len(process.CmdlineRedacted) > MaxCommandLineLength {
		return errors.New("command line exceeds maximum length")
	}
	if err := validateSafeString(process.CmdlineRedacted, MaxCommandLineLength); err != nil {
		return errors.New("invalid command line")
	}
	if len(process.Listeners) > MaxListeners {
		return errors.New("too many process listeners")
	}
	for _, listener := range process.Listeners {
		if err := validateListener(listener); err != nil {
			return err
		}
	}
	return nil
}

func validateListener(listener dto.ListenerDTO) error {
	if listener.Protocol != "tcp" {
		return errors.New("invalid protocol")
	}
	if listener.LocalPort <= 0 || listener.LocalPort > 65535 {
		return errors.New("invalid port")
	}
	if strings.TrimSpace(listener.LocalAddress) == "" || net.ParseIP(listener.LocalAddress) == nil {
		return errors.New("invalid address")
	}
	if !validScope(listener.Scope) {
		return errors.New("invalid scope")
	}
	if listener.PID != nil && *listener.PID < 0 {
		return errors.New("invalid pid")
	}
	if err := validateSafeString(listener.ProcessName, MaxStringLength); err != nil {
		return errors.New("invalid process name")
	}
	return nil
}

func validateEvidence(evidence []dto.EvidenceDTO) error {
	if len(evidence) > MaxEvidencePerItem {
		return errors.New("too much evidence")
	}
	for _, item := range evidence {
		if err := validateSafeString(item.Kind, MaxEvidenceLength); err != nil {
			return errors.New("invalid evidence")
		}
		if err := validateSafeString(item.Value, MaxEvidenceLength); err != nil {
			return errors.New("invalid evidence")
		}
	}
	return nil
}

func validateSafeString(value string, maxLen int) error {
	if len(value) > maxLen {
		return errors.New("string too long")
	}
	for _, r := range value {
		if r == '\n' || r == '\r' || r == 0 || (!unicode.IsPrint(r) && !unicode.IsSpace(r)) {
			return errors.New("unsafe string")
		}
	}
	return nil
}

func validConfidence(conf dto.Confidence) bool {
	switch conf {
	case dto.ConfidenceHigh, dto.ConfidenceMedium, dto.ConfidenceLow, dto.ConfidenceUnknown:
		return true
	default:
		return false
	}
}

func validRuntime(runtime string) bool {
	switch runtime {
	case "unknown", "java", "nodejs", "python", "php", "dotnet", "go":
		return true
	default:
		return false
	}
}

func validFramework(framework string) bool {
	switch framework {
	case "unknown", "spring_boot", "nextjs", "express", "nestjs", "django", "fastapi", "flask", "laravel", "wordpress", "aspnet_core":
		return true
	default:
		return false
	}
}

func validServiceKind(kind string) bool {
	switch kind {
	case "unknown", "nginx", "apache", "caddy", "postgresql", "mysql", "mariadb", "mongodb", "redis", "php_fpm":
		return true
	default:
		return false
	}
}

func validScope(scope string) bool {
	switch scope {
	case "unknown", "loopback", "private", "public", "unspecified":
		return true
	default:
		return false
	}
}
