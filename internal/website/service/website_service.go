package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"gorm.io/gorm"
	"vpsmonitoring-backend/internal/discovery/dto"
	"vpsmonitoring-backend/internal/website/models"
	"vpsmonitoring-backend/internal/website/repository"
)

type WebsiteService interface {
	IngestWebsites(ctx context.Context, serverID int64, websites []dto.WebsiteCandidateDTO) error
	GetWebsitesByServer(ctx context.Context, serverID int64) ([]*models.Website, error)
	IngestHostingUsers(ctx context.Context, serverID int64, users []dto.HostingUserDTO) error
	GetHostingUsersByServer(ctx context.Context, serverID int64) ([]dto.HostingUserDTO, error)
	UpdateDomainHealth(ctx context.Context, serverID int64, domain string, health dto.HostingWebDomainDTO) error
}

type websiteService struct {
	repo repository.WebsiteRepository
	db   *gorm.DB
}

func NewWebsiteService(repo repository.WebsiteRepository, db *gorm.DB) WebsiteService {
	return &websiteService{repo: repo, db: db}
}

func confidenceValue(c dto.Confidence) int {
	switch c {
	case dto.ConfidenceHigh:
		return 3
	case dto.ConfidenceMedium:
		return 2
	case dto.ConfidenceLow:
		return 1
	default:
		return 0
	}
}

func isInfrastructureDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	return d == "_" || d == "default" || d == "localhost" || d == "catch-all" || d == ""
}

func (s *websiteService) IngestWebsites(ctx context.Context, serverID int64, websites []dto.WebsiteCandidateDTO) error {
	if len(websites) == 0 {
		return nil
	}

	// Group sources seen in this payload
	sourcesSeen := make(map[string]bool)

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := s.repo.WithTx(tx)
		now := time.Now().UTC()

		for _, wc := range websites {
			var primaryDomain string
			for _, d := range wc.Domains {
				if d.IsPrimary {
					primaryDomain = d.DomainName
					break
				}
			}
			if primaryDomain == "" && len(wc.Domains) > 0 {
				primaryDomain = wc.Domains[0].DomainName
			}

			if isInfrastructureDomain(primaryDomain) {
				continue
			}

			sourcesSeen[wc.Source] = true
			normalizedPrimary := strings.ToLower(strings.TrimSpace(primaryDomain))

			// Check existing
			existing, err := txRepo.GetWebsite(ctx, serverID, normalizedPrimary)
			if err != nil {
				return err
			}

			var docRoot *string
			if wc.DocumentRoot != "" {
				dr := wc.DocumentRoot
				docRoot = &dr
			}
			var webKind *string
			if wc.WebServerKind != "" {
				wk := wc.WebServerKind
				webKind = &wk
			}

			websiteModel := &models.Website{
				ServerID:                serverID,
				PrimaryDomain:           strings.TrimSpace(primaryDomain),
				NormalizedPrimaryDomain: normalizedPrimary,
				DocumentRoot:            docRoot,
				WebServerKind:           webKind,
				Status:                  "ACTIVE",
				Confidence:              string(wc.Confidence),
				Source:                  wc.Source,
				LastDiscoveredAt:        now,
			}

			if existing != nil {
				websiteModel.ID = existing.ID
				websiteModel.HostingAccountID = existing.HostingAccountID

				// Conflict resolution: only overwrite if incoming >= existing confidence
				existingConf := dto.Confidence(existing.Confidence)
				if confidenceValue(wc.Confidence) < confidenceValue(existingConf) {
					// Weaker evidence. Preserve stronger metadata.
					websiteModel.DocumentRoot = existing.DocumentRoot
					websiteModel.WebServerKind = existing.WebServerKind
					websiteModel.Confidence = existing.Confidence
					websiteModel.Source = existing.Source
					// Note: Status returns to ACTIVE, LastDiscoveredAt is updated to `now`.
				}
			}

			if err := txRepo.UpsertWebsite(ctx, websiteModel); err != nil {
				return err
			}

			// We need the ID for domain insertion, which is populated by GORM (or we query it)
			// Wait, UpsertWebsite uses ON CONFLICT DO UPDATE. In Postgres, returning id doesn't always work natively if no rows are updated.
			// Let's refetch if ID is empty.
			if websiteModel.ID.String() == "00000000-0000-0000-0000-000000000000" {
				refetched, err := txRepo.GetWebsite(ctx, serverID, normalizedPrimary)
				if err != nil {
					return err
				}
				websiteModel.ID = refetched.ID
			}

			for _, d := range wc.Domains {
				if isInfrastructureDomain(d.DomainName) {
					continue
				}
				wd := &models.WebsiteDomain{
					WebsiteID:            websiteModel.ID,
					DomainName:           strings.TrimSpace(d.DomainName),
					NormalizedDomainName: strings.ToLower(strings.TrimSpace(d.DomainName)),
					IsPrimary:            d.IsPrimary,
					Status:               "ACTIVE",
					LastDiscoveredAt:     now,
				}
				if err := txRepo.UpsertWebsiteDomain(ctx, wd); err != nil {
					return err
				}
			}
		}

		// Mark stale only for sources we saw in this successful payload
		for source := range sourcesSeen {
			if err := txRepo.MarkStaleWebsites(ctx, serverID, source, now); err != nil {
				return err
			}
			if err := txRepo.MarkStaleWebsiteDomains(ctx, serverID, source, now); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *websiteService) GetWebsitesByServer(ctx context.Context, serverID int64) ([]*models.Website, error) {
	return s.repo.GetWebsitesByServer(ctx, serverID)
}

func (s *websiteService) IngestHostingUsers(ctx context.Context, serverID int64, users []dto.HostingUserDTO) error {
	if len(users) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txRepo := s.repo.WithTx(tx)

		for _, u := range users {
			var uidPtr *int
			if u.LinuxUID > 0 {
				uidVal := u.LinuxUID
				uidPtr = &uidVal
			}

			webDomainsJSON := ""
			if len(u.WebDomainList) > 0 {
				if b, err := json.Marshal(u.WebDomainList); err == nil {
					webDomainsJSON = string(b)
				}
			}

			account := &models.HostingAccount{
				ServerID:          serverID,
				Username:          u.Username,
				Source:            "os",
				Role:              u.Role,
				Package:           u.Package,
				WebCount:          u.WebCount,
				DNSCount:          u.DNSCount,
				MailCount:         u.MailCount,
				DBCount:           u.DBCount,
				DiskUsageMB:       u.DiskUsageMB,
				BandwidthMB:       u.BandwidthMB,
				IsSuspended:       u.IsSuspended,
				LinuxUID:          uidPtr,
				HomeDir:           u.HomeDir,
				Shell:             u.Shell,
				DateCreated:       u.DateCreated,
				TimeCreated:       u.TimeCreated,
				FullName:          u.FullName,
				Email:             u.Email,
				Language:          u.Language,
				Theme:             u.Theme,
				WebDomainsQuota:   u.WebDomainsQuota,
				WebAliasesQuota:   u.WebAliasesQuota,
				DNSDomainsQuota:   u.DNSDomainsQuota,
				DNSRecordsQuota:   u.DNSRecordsQuota,
				MailDomainsQuota:  u.MailDomainsQuota,
				MailAccountsQuota: u.MailAccountsQuota,
				BackupsQuota:      u.BackupsQuota,
				DatabasesQuota:    u.DatabasesQuota,
				CronJobsQuota:     u.CronJobsQuota,
				DiskQuota:         u.DiskQuota,
				BandwidthQuota:    u.BandwidthQuota,
				IPAddressesQuota:  u.IPAddressesQuota,
				WebDomainsJSON:    webDomainsJSON,
				Status:            "ACTIVE",
			}

			if err := txRepo.UpsertHostingAccount(ctx, account); err != nil {
				return err
			}
		}

		return nil
	})
}

func (s *websiteService) GetHostingUsersByServer(ctx context.Context, serverID int64) ([]dto.HostingUserDTO, error) {
	accounts, err := s.repo.GetHostingAccountsByServer(ctx, serverID)
	if err != nil {
		return nil, err
	}

	websites, _ := s.repo.GetWebsitesByServer(ctx, serverID)
	websitesByUser := make(map[string][]string)
	for _, w := range websites {
		if w.DocumentRoot != nil && strings.HasPrefix(*w.DocumentRoot, "/home/") {
			parts := strings.Split(*w.DocumentRoot, "/")
			if len(parts) >= 3 {
				u := parts[2]
				websitesByUser[u] = append(websitesByUser[u], w.PrimaryDomain)
			}
		}
	}

	var dtos []dto.HostingUserDTO
	for _, acc := range accounts {
		uid := 0
		if acc.LinuxUID != nil {
			uid = *acc.LinuxUID
		}

		userSites := websitesByUser[acc.Username]
		webCount := acc.WebCount
		if len(userSites) > webCount {
			webCount = len(userSites)
		}

		var domainList []dto.HostingWebDomainDTO
		if acc.WebDomainsJSON != "" {
			_ = json.Unmarshal([]byte(acc.WebDomainsJSON), &domainList)
		}

		// Fallback: if domainList empty but userSites present, populate
		if len(domainList) == 0 && len(userSites) > 0 {
			for _, sName := range userSites {
				domainList = append(domainList, dto.HostingWebDomainDTO{
					Domain:      sName,
					IP:          "default",
					Template:    "default",
					SSL:         "no",
					DiskUsageMB: 0,
					BandwidthMB: 0,
					IsSuspended: acc.IsSuspended,
					DateCreated: acc.DateCreated,
				})
			}
		}

		dtos = append(dtos, dto.HostingUserDTO{
			Username:          acc.Username,
			Role:              acc.Role,
			Package:           acc.Package,
			WebCount:          webCount,
			DNSCount:          acc.DNSCount,
			MailCount:         acc.MailCount,
			DBCount:           acc.DBCount,
			DiskUsageMB:       acc.DiskUsageMB,
			BandwidthMB:       acc.BandwidthMB,
			IsSuspended:       acc.IsSuspended,
			DateCreated:       acc.DateCreated,
			TimeCreated:       acc.TimeCreated,
			HomeDir:           acc.HomeDir,
			Shell:             acc.Shell,
			LinuxUID:          uid,
			Websites:          userSites,
			FullName:          acc.FullName,
			Email:             acc.Email,
			Language:          acc.Language,
			Theme:             acc.Theme,
			WebDomainsQuota:   acc.WebDomainsQuota,
			WebAliasesQuota:   acc.WebAliasesQuota,
			DNSDomainsQuota:   acc.DNSDomainsQuota,
			DNSRecordsQuota:   acc.DNSRecordsQuota,
			MailDomainsQuota:  acc.MailDomainsQuota,
			MailAccountsQuota: acc.MailAccountsQuota,
			BackupsQuota:      acc.BackupsQuota,
			DatabasesQuota:    acc.DatabasesQuota,
			CronJobsQuota:     acc.CronJobsQuota,
			DiskQuota:         acc.DiskQuota,
			BandwidthQuota:    acc.BandwidthQuota,
			IPAddressesQuota:  acc.IPAddressesQuota,
			WebDomainList:     domainList,
		})
	}
	return dtos, nil
}

func (s *websiteService) UpdateDomainHealth(ctx context.Context, serverID int64, domain string, health dto.HostingWebDomainDTO) error {
	accounts, err := s.repo.GetHostingAccountsByServer(ctx, serverID)
	if err != nil {
		return err
	}

	for _, acc := range accounts {
		if acc.WebDomainsJSON == "" {
			continue
		}
		var list []dto.HostingWebDomainDTO
		if err := json.Unmarshal([]byte(acc.WebDomainsJSON), &list); err != nil {
			continue
		}
		found := false
		for i := range list {
			if strings.EqualFold(list[i].Domain, domain) {
				list[i].HTTPStatus = health.HTTPStatus
				list[i].ResponseTimeMs = health.ResponseTimeMs
				list[i].DNSLookupMs = health.DNSLookupMs
				list[i].ConnectTimeMs = health.ConnectTimeMs
				list[i].TLSHandshakeMs = health.TLSHandshakeMs
				list[i].TTFBMs = health.TTFBMs
				list[i].DownloadedBytes = health.DownloadedBytes
				if health.Traffic != nil {
					list[i].Traffic = health.Traffic
				}
				if health.SSLValid {
					list[i].SSLValid = health.SSLValid
					list[i].SSLDaysLeft = health.SSLDaysLeft
					list[i].SSLExpiryDate = health.SSLExpiryDate
					list[i].SSLIssuer = health.SSLIssuer
					list[i].SSL = "yes"
				}
				if health.CPUPercent > 0 {
					list[i].CPUPercent = health.CPUPercent
				}
				if health.MemoryMB > 0 {
					list[i].MemoryMB = health.MemoryMB
				}
				if health.WorkerProcesses > 0 {
					list[i].WorkerProcesses = health.WorkerProcesses
				}
				if health.LoadRPM > 0 {
					list[i].LoadRPM = health.LoadRPM
				}
				if health.DiskUsageMB > 0 {
					list[i].DiskUsageMB = health.DiskUsageMB
				}
				found = true
				break
			}
		}
		if found {
			bytes, err := json.Marshal(list)
			if err == nil {
				acc.WebDomainsJSON = string(bytes)
				return s.repo.UpsertHostingAccount(ctx, acc)
			}
		}
	}
	return nil
}


