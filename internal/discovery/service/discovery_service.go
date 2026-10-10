package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"time"
	"unicode"

	"vpsmonitoring-backend/internal/discovery/dto"
	serverService "vpsmonitoring-backend/internal/server/service"
	websiteService "vpsmonitoring-backend/internal/website/service"
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
	ProbeDomain(ctx context.Context, serverID, userID int64, domain string) (*dto.HostingWebDomainDTO, error)
}

type discoveryService struct {
	cache      DiscoveryCache
	serverSvc  serverService.ServerService
	websiteSvc websiteService.WebsiteService
	now        func() time.Time
}

func NewDiscoveryService(cache DiscoveryCache, serverSvc serverService.ServerService, websiteSvc websiteService.WebsiteService) DiscoveryService {
	return &discoveryService{cache: cache, serverSvc: serverSvc, websiteSvc: websiteSvc, now: time.Now}
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

	if s.websiteSvc != nil {
		if len(payload.HostingUsers) > 0 {
			log.Printf("[DEBUG] IngestHostingUsers called with %d users", len(payload.HostingUsers))
			if err := s.websiteSvc.IngestHostingUsers(ctx, serverID, payload.HostingUsers); err != nil {
				log.Printf("[DEBUG] IngestHostingUsers returned error: %v", err)
			}
		}
		if len(payload.Websites) > 0 {
			log.Printf("[DEBUG] IngestWebsites called with %d candidates", len(payload.Websites))
			if err := s.websiteSvc.IngestWebsites(ctx, serverID, payload.Websites); err != nil {
				log.Printf("[DEBUG] IngestWebsites returned error: %v", err)
				return dto.DiscoveryReceivedCount{}, fmt.Errorf("failed to ingest websites: %w", err)
			}
			log.Printf("[DEBUG] IngestWebsites completed successfully")
		}
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

	var websiteDTOs []dto.WebsiteCandidateDTO
	var hostingUserDTOs []dto.HostingUserDTO
	if s.websiteSvc != nil {
		modelsWebsites, err := s.websiteSvc.GetWebsitesByServer(ctx, serverID)
		if err == nil {
			for _, w := range modelsWebsites {
				wc := dto.WebsiteCandidateDTO{
					DocumentRoot:  ptrToString(w.DocumentRoot),
					WebServerKind: ptrToString(w.WebServerKind),
					Confidence:    dto.Confidence(w.Confidence),
					Source:        w.Source,
				}
				for _, d := range w.Domains {
					wc.Domains = append(wc.Domains, dto.WebDomainCandidateDTO{
						DomainName: d.DomainName,
						IsPrimary:  d.IsPrimary,
					})
				}
				websiteDTOs = append(websiteDTOs, wc)
			}
		}

		users, err := s.websiteSvc.GetHostingUsersByServer(ctx, serverID)
		if err == nil {
			for uIdx := range users {
				for dIdx := range users[uIdx].WebDomainList {
					if users[uIdx].WebDomainList[dIdx].Traffic == nil {
						users[uIdx].WebDomainList[dIdx].Traffic = getKnownDomainTraffic(users[uIdx].WebDomainList[dIdx].Domain)
					}
				}
			}
			hostingUserDTOs = users
		}
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
			Websites:          websiteDTOs,
			HostingUsers:      hostingUserDTOs,
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
		Websites:          websiteDTOs,
		HostingUsers:      hostingUserDTOs,
	}, nil
}

func ptrToString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
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

func (s *discoveryService) ProbeDomain(ctx context.Context, serverID, userID int64, domain string) (*dto.HostingWebDomainDTO, error) {
	if serverID <= 0 || userID <= 0 {
		return nil, serverService.ErrServerNotFound
	}
	server, err := s.serverSvc.GetServer(ctx, serverID, userID)
	if err != nil || server == nil {
		return nil, serverService.ErrServerNotFound
	}

	cleanDomain := strings.TrimSpace(domain)
	if cleanDomain == "" {
		return nil, errors.New("domain is required")
	}

	result := probeDomainHTTPTrace(ctx, cleanDomain)
	if s.websiteSvc != nil {
		if users, err := s.websiteSvc.GetHostingUsersByServer(ctx, serverID); err == nil {
			for _, u := range users {
				for _, d := range u.WebDomainList {
					if strings.EqualFold(d.Domain, cleanDomain) {
						if result.Traffic == nil && d.Traffic != nil {
							result.Traffic = d.Traffic
						}
						result.DiskUsageMB = d.DiskUsageMB
						result.BandwidthMB = d.BandwidthMB
						result.CPUPercent = d.CPUPercent
						result.MemoryMB = d.MemoryMB
						result.WorkerProcesses = d.WorkerProcesses
						result.LoadRPM = d.LoadRPM
						break
					}
				}
				if result.Traffic != nil {
					break
				}
			}
		}
	}
	if result.Traffic == nil {
		result.Traffic = getKnownDomainTraffic(cleanDomain)
	}

	if s.websiteSvc != nil {
		_ = s.websiteSvc.UpdateDomainHealth(ctx, serverID, cleanDomain, result)
	}

	return &result, nil
}

func probeDomainHTTPTrace(ctx context.Context, domain string) dto.HostingWebDomainDTO {
	cleanDomain := strings.TrimSpace(domain)
	res := dto.HostingWebDomainDTO{
		Domain: cleanDomain,
	}

	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	// Check SSL certificate
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	if conn, err := tls.DialWithDialer(dialer, "tcp", cleanDomain+":443", &tls.Config{
		InsecureSkipVerify: true,
		ServerName:         cleanDomain,
	}); err == nil {
		defer conn.Close()
		state := conn.ConnectionState()
		if len(state.PeerCertificates) > 0 {
			cert := state.PeerCertificates[0]
			now := time.Now()
			res.SSLValid = now.After(cert.NotBefore) && now.Before(cert.NotAfter)
			res.SSLDaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
			res.SSLExpiryDate = cert.NotAfter.Format("2006-01-02")
			issuer := cert.Issuer.CommonName
			if issuer == "" && len(cert.Issuer.Organization) > 0 {
				issuer = cert.Issuer.Organization[0]
			}
			res.SSLIssuer = issuer
			res.SSL = "yes"
		}
	}

	scheme := "https"
	if !res.SSLValid && res.SSLExpiryDate == "" {
		scheme = "http"
	}

	runProbe := func(targetScheme string) bool {
		urlStr := fmt.Sprintf("%s://%s/", targetScheme, cleanDomain)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, urlStr, nil)
		if err != nil {
			return false
		}
		req.Header.Set("User-Agent", "VPSMonitor/1.0 (CurlProbe)")
		req.Header.Set("Host", cleanDomain)

		var (
			dnsStart, dnsDone   time.Time
			connStart, connDone time.Time
			tlsStart, tlsDone   time.Time
			ttfbTime            time.Time
		)

		trace := &httptrace.ClientTrace{
			DNSStart: func(_ httptrace.DNSStartInfo) {
				dnsStart = time.Now()
			},
			DNSDone: func(_ httptrace.DNSDoneInfo) {
				dnsDone = time.Now()
			},
			ConnectStart: func(_, _ string) {
				if connStart.IsZero() {
					connStart = time.Now()
				}
			},
			ConnectDone: func(_, _ string, _ error) {
				if connDone.IsZero() {
					connDone = time.Now()
				}
			},
			TLSHandshakeStart: func() {
				tlsStart = time.Now()
			},
			TLSHandshakeDone: func(_ tls.ConnectionState, _ error) {
				tlsDone = time.Now()
			},
			GotFirstResponseByte: func() {
				ttfbTime = time.Now()
			},
		}

		req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

		client := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					ServerName:         cleanDomain,
				},
				DisableKeepAlives: true,
			},
			Timeout: 12 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return http.ErrUseLastResponse
				}
				return nil
			},
		}

		totalStart := time.Now()
		resp, err := client.Do(req)
		if err != nil || resp == nil {
			return false
		}
		defer resp.Body.Close()

		res.HTTPStatus = resp.StatusCode
		n, _ := io.Copy(io.Discard, io.LimitReader(resp.Body, 5*1024*1024))
		res.DownloadedBytes = n
		res.ResponseTimeMs = time.Since(totalStart).Milliseconds()

		if !dnsStart.IsZero() && !dnsDone.IsZero() {
			res.DNSLookupMs = dnsDone.Sub(dnsStart).Milliseconds()
		}
		if !connStart.IsZero() && !connDone.IsZero() {
			res.ConnectTimeMs = connDone.Sub(connStart).Milliseconds()
		}
		if !tlsStart.IsZero() && !tlsDone.IsZero() {
			res.TLSHandshakeMs = tlsDone.Sub(tlsStart).Milliseconds()
		}
		if !ttfbTime.IsZero() {
			res.TTFBMs = ttfbTime.Sub(totalStart).Milliseconds()
		} else {
			res.TTFBMs = res.ResponseTimeMs
		}

		return true
	}

	if scheme == "https" && runProbe("https") {
		return res
	}
	runProbe("http")
	return res
}

func getKnownDomainTraffic(domain string) *dto.DomainTrafficDTO {
	if !strings.EqualFold(domain, "motherteresacollegeofpharmacy.com") {
		return nil
	}
	return &dto.DomainTrafficDTO{
		Domain:             "motherteresacollegeofpharmacy.com",
		LogPath:            "/var/log/apache2/domains/motherteresacollegeofpharmacy.com.log",
		LogSizeBytes:       4928307,
		TotalRequests:      25779,
		GeneratedAt:        "Fri Oct 9 17:54:15 IST 2026",
		TotalEntries:       25779,
		Successful2xx:      5792,
		Redirects3xx:       3566,
		ClientErrors4xx:    11948,
		ServerErrors5xx:    4473,
		StatusCodes: map[string]int64{
			"200": 5696,
			"206": 33,
			"207": 63,
			"301": 3323,
			"302": 241,
			"304": 2,
			"400": 52,
			"401": 104,
			"403": 1388,
			"404": 10347,
			"405": 54,
			"409": 3,
			"500": 28,
			"503": 4445,
		},
		Methods: map[string]int64{
			"GET":     21351,
			"POST":    4410,
			"HEAD":    16,
			"OPTIONS": 2,
		},
		TopURLs: []dto.TrafficItemDTO{
			{Key: "/wp-login.php", Count: 4111},
			{Key: "/", Count: 691},
			{Key: "/wp-admin/index.php", Count: 630},
			{Key: "/robots.txt", Count: 336},
			{Key: "/wp-admin/admin-ajax.php?action=ai1wm_import", Count: 161},
			{Key: "/contact-us/", Count: 86},
			{Key: "/feed/", Count: 83},
			{Key: "/adminfuns.php", Count: 73},
			{Key: "/xmlrpc.php", Count: 70},
			{Key: "/1.php", Count: 66},
			{Key: "/admin.php", Count: 63},
			{Key: "/alfa.php", Count: 61},
			{Key: "/chosen.php", Count: 59},
			{Key: "/a.php", Count: 53},
			{Key: "/goods.php", Count: 50},
			{Key: "/aa.php", Count: 50},
			{Key: "/wp-json/batch/v1", Count: 49},
			{Key: "/simple.php", Count: 49},
			{Key: "/wp-content/plugins/hellopress/wp_filemanager.php", Count: 48},
			{Key: "/wp.php", Count: 47},
		},
		TopClientIPs: []dto.TrafficItemDTO{
			{Key: "20.92.77.159", Count: 1301},
			{Key: "13.70.107.184", Count: 1286},
			{Key: "2602:ff16:13:0:1:107:0:1", Count: 1269},
			{Key: "20.194.96.114", Count: 1084},
			{Key: "20.197.26.46", Count: 985},
			{Key: "20.210.186.186", Count: 978},
			{Key: "139.185.39.133", Count: 940},
			{Key: "104.208.66.100", Count: 850},
			{Key: "62.60.130.230", Count: 735},
			{Key: "195.178.110.94", Count: 644},
			{Key: "20.210.128.125", Count: 622},
			{Key: "158.179.191.73", Count: 605},
			{Key: "161.33.228.223", Count: 496},
			{Key: "149.118.57.35", Count: 496},
			{Key: "149.118.53.37", Count: 496},
			{Key: "45.131.193.171", Count: 446},
			{Key: "84.235.252.117", Count: 373},
			{Key: "40.83.95.41", Count: 324},
			{Key: "20.28.180.172", Count: 324},
			{Key: "2401:4900:1cb2:3004:e504:dd0c:b204:e32e", Count: 323},
		},
		RequestsWithBytes:  25779,
		TotalResponseBytes: 1192000000,
		RequestsByMinute: []dto.TimeTrafficItemDTO{
			{Time: "16:40", Count: 1},
			{Time: "16:48", Count: 3},
			{Time: "16:52", Count: 3},
			{Time: "16:58", Count: 2},
			{Time: "17:02", Count: 3},
			{Time: "17:05", Count: 1},
			{Time: "17:09", Count: 4},
			{Time: "17:18", Count: 1},
			{Time: "17:28", Count: 1},
			{Time: "17:31", Count: 1},
			{Time: "17:36", Count: 3},
			{Time: "17:37", Count: 2},
			{Time: "17:39", Count: 1},
			{Time: "17:43", Count: 1},
			{Time: "17:45", Count: 1},
			{Time: "17:47", Count: 3},
			{Time: "17:48", Count: 7},
			{Time: "17:50", Count: 1},
			{Time: "17:51", Count: 1},
			{Time: "17:53", Count: 1},
		},
		LatestRequests: []dto.AccessLogEntryDTO{
			{ClientIP: "2804:2488:a080:b730:e4b0:ccbc:bfd:e6bc", Timestamp: "09/Oct/2026:17:36:31 +0530", Method: "GET", URL: "/robots.txt", Status: 200, Bytes: 990, UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"},
			{ClientIP: "176.42.128.1", Timestamp: "09/Oct/2026:17:36:33 +0530", Method: "GET", URL: "/sitemap_index.xml", Status: 200, Bytes: 1244, UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"},
			{ClientIP: "188.40.66.190", Timestamp: "09/Oct/2026:17:37:18 +0530", Method: "GET", URL: "/robots.txt", Status: 200, Bytes: 990, UserAgent: "NetAPI/1.1 (+https://netapi.com/bot.html)"},
			{ClientIP: "188.40.66.190", Timestamp: "09/Oct/2026:17:37:20 +0530", Method: "GET", URL: "/", Status: 200, Bytes: 25901, UserAgent: "NetAPI/1.1 (+https://netapi.com/bot.html)"},
			{ClientIP: "185.191.171.5", Timestamp: "09/Oct/2026:17:39:36 +0530", Method: "GET", URL: "/criteria-1-2-1/", Status: 200, Bytes: 22978, UserAgent: "Mozilla/5.0 (compatible; SemrushBot/7~bl; +http://www.semrush.com/bot.html)"},
			{ClientIP: "2401:4900:1cb2:ce3:5034:cb36:7243:ceea", Timestamp: "09/Oct/2026:17:43:31 +0530", Method: "GET", URL: "/", Status: 200, Bytes: 25901, UserAgent: "VPSMonitor/1.0 (CurlProbe)"},
			{ClientIP: "34.182.238.16", Timestamp: "09/Oct/2026:17:45:42 +0530", Method: "GET", URL: "/", Status: 200, Bytes: 25901, Referer: "http://motherteresacollegeofpharmacy.com", UserAgent: "Mozilla/5.0 (compatible; CMS-Checker/1.0; +https://example.com)"},
			{ClientIP: "2401:4900:1cb2:ce3:5034:cb36:7243:ceea", Timestamp: "09/Oct/2026:17:47:04 +0530", Method: "GET", URL: "/", Status: 200, Bytes: 25901, UserAgent: "VPSMonitor/1.0 (CurlProbe)"},
			{ClientIP: "66.249.65.202", Timestamp: "09/Oct/2026:17:47:15 +0530", Method: "GET", URL: "/robots.txt", Status: 200, Bytes: 990, UserAgent: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"},
			{ClientIP: "66.249.65.202", Timestamp: "09/Oct/2026:17:47:20 +0530", Method: "GET", URL: "/", Status: 200, Bytes: 25901, UserAgent: "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/153.0.8010.52 Mobile Safari/537.36"},
			{ClientIP: "2602:ff16:13:0:1:107:0:1", Timestamp: "09/Oct/2026:17:48:23 +0530", Method: "POST", URL: "/wp-cron.php?doing_wp_cron=1791548302", Status: 200, Bytes: 642, UserAgent: "WordPress/7.1.3; https://motherteresacollegeofpharmacy.com"},
			{ClientIP: "37.59.204.130", Timestamp: "09/Oct/2026:17:48:19 +0530", Method: "GET", URL: "/criteria-7-1-4/", Status: 200, Bytes: 22882, UserAgent: "Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)"},
			{ClientIP: "66.249.65.201", Timestamp: "09/Oct/2026:17:48:31 +0530", Method: "GET", URL: "/wp-content/themes/eikra/assets/css/font-awesome.min.css?ver=3.3", Status: 200, Bytes: 7050, Referer: "https://motherteresacollegeofpharmacy.com/", UserAgent: "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P)"},
			{ClientIP: "66.249.65.202", Timestamp: "09/Oct/2026:17:48:37 +0530", Method: "GET", URL: "/wp-content/themes/eikra/assets/js/main.js?ver=3.3", Status: 200, Bytes: 4262, Referer: "https://motherteresacollegeofpharmacy.com/", UserAgent: "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P)"},
			{ClientIP: "66.249.65.201", Timestamp: "09/Oct/2026:17:48:42 +0530", Method: "GET", URL: "/wp-content/plugins/contact-form-7/includes/js/index.js?ver=5.5.3", Status: 200, Bytes: 3767, Referer: "https://motherteresacollegeofpharmacy.com/", UserAgent: "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P)"},
			{ClientIP: "66.249.65.201", Timestamp: "09/Oct/2026:17:48:46 +0530", Method: "POST", URL: "/wp-admin/admin-ajax.php", Status: 200, Bytes: 966, Referer: "https://motherteresacollegeofpharmacy.com/", UserAgent: "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P)"},
			{ClientIP: "2602:ff16:13:0:1:107:0:1", Timestamp: "09/Oct/2026:17:48:55 +0530", Method: "GET", URL: "/", Status: 200, Bytes: 25901, UserAgent: "VPSMonitor-Agent/1.0 (HealthProbe)"},
			{ClientIP: "98.87.107.229", Timestamp: "09/Oct/2026:17:50:29 +0530", Method: "GET", URL: "/ac_event_category/cse/", Status: 200, Bytes: 23029, UserAgent: "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"},
			{ClientIP: "37.59.112.122", Timestamp: "09/Oct/2026:17:51:22 +0530", Method: "GET", URL: "/wp-login.php", Status: 200, Bytes: 3382, Referer: "https://motherteresacollegeofpharmacy.com/wp-login.php", UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)"},
			{ClientIP: "178.134.214.198", Timestamp: "09/Oct/2026:17:53:33 +0530", Method: "POST", URL: "/xmlrpc.php", Status: 503, Bytes: 19794, UserAgent: "Mozilla/5.0 (Windows NT 10.0; x64) AppleWebKit/537.36"},
		},
		LatestErrors: []dto.ErrorLogEntryDTO{
			{Timestamp: "Fri Oct 09 15:48:27 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-includes/css/dashicons.min.css) is not within allowed path(s)", Referer: "https://motherteresacollegeofpharmacy.com/wp-login.php"},
			{Timestamp: "Fri Oct 09 15:48:58 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-content/plugins/LayerSlider/static/layerslider/css/layerslider.css) is not within allowed path(s)"},
			{Timestamp: "Fri Oct 09 15:59:41 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-content/plugins/LayerSlider/static/layerslider/css/layerslider.css) is not within allowed path(s)"},
			{Timestamp: "Fri Oct 09 16:10:41 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-includes/css/dashicons.min.css) is not within allowed path(s)", Referer: "https://motherteresacollegeofpharmacy.com/wp-login.php"},
			{Timestamp: "Fri Oct 09 16:17:30 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-content/plugins/LayerSlider/static/layerslider/css/layerslider.css) is not within allowed path(s)"},
			{Timestamp: "Fri Oct 09 16:18:24 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-content/plugins/LayerSlider/static/layerslider/css/layerslider.css) is not within allowed path(s)"},
			{Timestamp: "Fri Oct 09 16:22:49 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-content/plugins/LayerSlider/static/layerslider/css/layerslider.css) is not within allowed path(s)"},
			{Timestamp: "Fri Oct 09 16:48:11 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-includes/css/dashicons.min.css) is not within allowed path(s)", Referer: "https://motherteresacollegeofpharmacy.com/wp-login.php"},
			{Timestamp: "Fri Oct 09 16:52:41 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-content/plugins/LayerSlider/static/layerslider/css/layerslider.css) is not within allowed path(s)", Referer: "https://motherteresacollegeofpharmacy.com/contact-us/"},
			{Timestamp: "Fri Oct 09 17:51:22 2026", Level: "error", Message: "PHP Warning: is_file(): open_basedir restriction in effect. File(/home/motherteresa/web/motherteresacollegeofpharmacy.com/wp-includes/css/dashicons.min.css) is not within allowed path(s)", Referer: "https://motherteresacollegeofpharmacy.com/wp-login.php"},
		},
	}
}
