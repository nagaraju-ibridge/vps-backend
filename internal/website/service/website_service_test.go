package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"vpsmonitoring-backend/internal/discovery/dto"
	"vpsmonitoring-backend/internal/website/models"
	"vpsmonitoring-backend/internal/website/repository"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	assert.NoError(t, err)

	// Since we mock servers, we don't enforce foreign keys in sqlite testing
	err = db.AutoMigrate(
		&models.HostingAccount{},
		&models.Website{},
		&models.WebsiteDomain{},
	)
	assert.NoError(t, err)

	return db
}

func TestWebsiteUpsert(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewWebsiteRepository(db)
	svc := NewWebsiteService(repo, db)

	serverID := int64(1)

	// A. Create website
	websites := []dto.WebsiteCandidateDTO{
		{

			DocumentRoot:  "/var/www/example",
			WebServerKind: "nginx",
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
			Domains: []dto.WebDomainCandidateDTO{
				{DomainName: "example.com", IsPrimary: true},
				{DomainName: "www.example.com", IsPrimary: false},
			},
		},
	}

	err := svc.IngestWebsites(context.Background(), serverID, websites)
	assert.NoError(t, err)

	website, _ := repo.GetWebsite(context.Background(), serverID, "example.com")
	assert.NotNil(t, website)
	assert.Equal(t, "/var/www/example", *website.DocumentRoot)
	assert.Equal(t, "nginx", *website.WebServerKind)
	assert.Equal(t, "ACTIVE", website.Status)

	// Update existing website
	websites[0].DocumentRoot = "/var/www/new"
	err = svc.IngestWebsites(context.Background(), serverID, websites)
	assert.NoError(t, err)

	website, _ = repo.GetWebsite(context.Background(), serverID, "example.com")
	assert.Equal(t, "/var/www/new", *website.DocumentRoot)
}

func TestConfidence(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewWebsiteRepository(db)
	svc := NewWebsiteService(repo, db)

	serverID := int64(1)

	// High confidence insert
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "example.com", IsPrimary: true}},
			DocumentRoot:  "/var/www/high",
			WebServerKind: "nginx",
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
		},
	})

	// Low confidence insert
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "example.com", IsPrimary: true}},
			DocumentRoot:  "/var/www/low",
			WebServerKind: "apache",
			Confidence:    dto.ConfidenceLow,
			Source:        "process",
		},
	})

	// LOW should not overwrite HIGH
	website, _ := repo.GetWebsite(context.Background(), serverID, "example.com")
	assert.Equal(t, "/var/www/high", *website.DocumentRoot)
	assert.Equal(t, "nginx", *website.WebServerKind)

	// HIGH should overwrite LOW
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "example.com", IsPrimary: true}},
			DocumentRoot:  "/var/www/newer_high",
			WebServerKind: "caddy",
			Confidence:    dto.ConfidenceHigh,
			Source:        "caddy_config",
		},
	})

	website, _ = repo.GetWebsite(context.Background(), serverID, "example.com")
	assert.Equal(t, "/var/www/newer_high", *website.DocumentRoot)
	assert.Equal(t, "caddy", *website.WebServerKind)
}

func TestCatchAllFiltering(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewWebsiteRepository(db)
	svc := NewWebsiteService(repo, db)

	serverID := int64(1)

	websites := []dto.WebsiteCandidateDTO{
		{Domains: []dto.WebDomainCandidateDTO{{DomainName: "_", IsPrimary: true}}, Confidence: dto.ConfidenceLow},
		{Domains: []dto.WebDomainCandidateDTO{{DomainName: "default", IsPrimary: true}}, Confidence: dto.ConfidenceLow},
		{Domains: []dto.WebDomainCandidateDTO{{DomainName: "localhost", IsPrimary: true}}, Confidence: dto.ConfidenceLow},
	}

	err := svc.IngestWebsites(context.Background(), serverID, websites)
	assert.NoError(t, err)

	w, _ := repo.GetWebsite(context.Background(), serverID, "_")
	assert.Nil(t, w)
	w, _ = repo.GetWebsite(context.Background(), serverID, "default")
	assert.Nil(t, w)
	w, _ = repo.GetWebsite(context.Background(), serverID, "localhost")
	assert.Nil(t, w)
}

func TestAliasMissingAndWebsiteMissing(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewWebsiteRepository(db)
	svc := NewWebsiteService(repo, db)

	serverID := int64(1)

	// Insert website with alias
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
			Domains: []dto.WebDomainCandidateDTO{
				{DomainName: "example.com", IsPrimary: true},
				{DomainName: "www.example.com"},
				{DomainName: "alias.com"},
			},
		},
	})

	// Now omit alias.com and the entire example.com website, but we must send a payload with nginx_config source to trigger stale marking.
	// Actually, let's omit alias.com from example.com, and omit example2.com entirely.
	time.Sleep(1 * time.Second)
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "example2.com", IsPrimary: true}},
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
		},
	})

	// Wait, we just omitted example.com.
	// So example.com should be MISSING, and its aliases should be MISSING.
	// Let's check example.com.
	w, _ := repo.GetWebsite(context.Background(), serverID, "example.com")
	assert.Equal(t, "MISSING", w.Status)

	var domains []models.WebsiteDomain
	db.Where("website_id = ?", w.ID).Find(&domains)
	for _, d := range domains {
		assert.Equal(t, "MISSING", d.Status) // alias is not deleted
	}

	// Now re-discover example.com but only with www.example.com
	time.Sleep(100 * time.Millisecond)
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
			Domains: []dto.WebDomainCandidateDTO{
				{DomainName: "example.com", IsPrimary: true},
				{DomainName: "www.example.com"},
			},
		},
	})

	w, _ = repo.GetWebsite(context.Background(), serverID, "example.com")
	assert.Equal(t, "ACTIVE", w.Status) // Restored

	db.Where("website_id = ? AND domain_name = ?", w.ID, "www.example.com").First(&domains)
	assert.Equal(t, "ACTIVE", domains[0].Status) // Restored

	db.Where("website_id = ? AND domain_name = ?", w.ID, "alias.com").First(&domains)
	assert.Equal(t, "MISSING", domains[0].Status) // Still missing
}

func TestSourceSegregation(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewWebsiteRepository(db)
	svc := NewWebsiteService(repo, db)

	serverID := int64(1)

	// Create nginx website
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "nginx.com", IsPrimary: true}},
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
		},
	})

	// Create apache website
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "apache.com", IsPrimary: true}},
			Confidence:    dto.ConfidenceHigh,
			Source:        "apache_config",
		},
	})

	// Now send apache discovery that omits apache.com
	// But it shouldn't mark nginx.com as MISSING
	time.Sleep(1 * time.Second)
	svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Domains: []dto.WebDomainCandidateDTO{{DomainName: "other-apache.com", IsPrimary: true}},
			Confidence:    dto.ConfidenceHigh,
			Source:        "apache_config",
		},
	})

	w, _ := repo.GetWebsite(context.Background(), serverID, "apache.com")
	assert.Equal(t, "MISSING", w.Status)

	w, _ = repo.GetWebsite(context.Background(), serverID, "nginx.com")
	assert.Equal(t, "ACTIVE", w.Status)
}

// simulate failure to test rollback
type failingRepository struct {
	repository.WebsiteRepository
}

func (f *failingRepository) UpsertWebsiteDomain(ctx context.Context, domain *models.WebsiteDomain) error {
	return errors.New("simulated domain persistence failure")
}

func (f *failingRepository) WithTx(tx *gorm.DB) repository.WebsiteRepository {
	return &failingRepository{f.WebsiteRepository.WithTx(tx)}
}

func TestTransactionRollback(t *testing.T) {
	db := setupTestDB(t)
	realRepo := repository.NewWebsiteRepository(db)
	failRepo := &failingRepository{WebsiteRepository: realRepo}
	svc := NewWebsiteService(failRepo, db)

	serverID := int64(1)

	// This should fail during domain upsert and rollback website creation
	err := svc.IngestWebsites(context.Background(), serverID, []dto.WebsiteCandidateDTO{
		{
			Confidence:    dto.ConfidenceHigh,
			Source:        "nginx_config",
			Domains: []dto.WebDomainCandidateDTO{
				{DomainName: "rollback.com", IsPrimary: true},
				{DomainName: "www.rollback.com"},
			},
		},
	})

	assert.Error(t, err)
	assert.Equal(t, "simulated domain persistence failure", err.Error())

	// Verify website was rolled back and doesn't exist
	w, _ := realRepo.GetWebsite(context.Background(), serverID, "rollback.com")
	assert.Nil(t, w)
}

func TestHostingUserIngestAndRetrieval(t *testing.T) {
	db := setupTestDB(t)
	repo := repository.NewWebsiteRepository(db)
	svc := NewWebsiteService(repo, db)

	serverID := int64(42)

	users := []dto.HostingUserDTO{
		{
			Username:          "ibridge",
			FullName:          "ibridgedigital",
			Email:             "srujan@ibridge.digital",
			Role:              "user",
			Package:           "backupdefault",
			Language:          "en",
			Theme:             "dark",
			WebCount:          11,
			DNSCount:          15,
			MailCount:         6,
			DBCount:           40,
			DiskUsageMB:       36835,
			BandwidthMB:       32497,
			IsSuspended:       false,
			DateCreated:       "2022-08-28",
			TimeCreated:       "10:03:28",
			Shell:             "bash",
			LinuxUID:          1001,
			WebDomainsQuota:   "11/unlimited",
			WebAliasesQuota:   "10/unlimited",
			DNSDomainsQuota:   "15/unlimited",
			DNSRecordsQuota:   "208/unlimited",
			MailDomainsQuota:  "6/unlimited",
			MailAccountsQuota: "0/unlimited",
			BackupsQuota:      "1/1",
			DatabasesQuota:    "40/unlimited",
			CronJobsQuota:     "1/unlimited",
			DiskQuota:         "36835/unlimited",
			BandwidthQuota:    "32497/unlimited",
			IPAddressesQuota:  "1/0",
			WebDomainList: []dto.HostingWebDomainDTO{
				{
					Domain:      "payment.ibridge.digital",
					IP:          "209.182.233.252",
					Template:    "default",
					SSL:         "yes",
					DiskUsageMB: 357,
					BandwidthMB: 10,
					IsSuspended: false,
					DateCreated: "2025-02-26",
				},
				{
					Domain:      "ibridge.digital",
					IP:          "209.182.233.252",
					Template:    "default",
					SSL:         "yes",
					DiskUsageMB: 9522,
					BandwidthMB: 5835,
					IsSuspended: false,
					DateCreated: "2025-02-26",
				},
			},
		},
	}

	err := svc.IngestHostingUsers(context.Background(), serverID, users)
	assert.NoError(t, err)

	retrieved, err := svc.GetHostingUsersByServer(context.Background(), serverID)
	assert.NoError(t, err)
	assert.Len(t, retrieved, 1)

	u := retrieved[0]
	assert.Equal(t, "ibridge", u.Username)
	assert.Equal(t, "ibridgedigital", u.FullName)
	assert.Equal(t, "srujan@ibridge.digital", u.Email)
	assert.Equal(t, "backupdefault", u.Package)
	assert.Equal(t, "11/unlimited", u.WebDomainsQuota)
	assert.Equal(t, "40/unlimited", u.DatabasesQuota)
	assert.Equal(t, "36835/unlimited", u.DiskQuota)
	assert.Len(t, u.WebDomainList, 2)
	assert.Equal(t, "payment.ibridge.digital", u.WebDomainList[0].Domain)
	assert.Equal(t, "209.182.233.252", u.WebDomainList[0].IP)
	assert.Equal(t, "yes", u.WebDomainList[0].SSL)
	assert.Equal(t, int64(357), u.WebDomainList[0].DiskUsageMB)
}

