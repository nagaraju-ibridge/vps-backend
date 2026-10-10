package repository

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"vpsmonitoring-backend/internal/website/models"
)

type WebsiteRepository interface {
	WithTx(tx *gorm.DB) WebsiteRepository
	UpsertWebsite(ctx context.Context, website *models.Website) error
	UpsertWebsiteDomain(ctx context.Context, domain *models.WebsiteDomain) error
	GetWebsite(ctx context.Context, serverID int64, normalizedPrimaryDomain string) (*models.Website, error)
	MarkStaleWebsites(ctx context.Context, serverID int64, source string, threshold time.Time) error
	MarkStaleWebsiteDomains(ctx context.Context, serverID int64, source string, threshold time.Time) error
	GetWebsitesByServer(ctx context.Context, serverID int64) ([]*models.Website, error)
	UpsertHostingAccount(ctx context.Context, account *models.HostingAccount) error
	GetHostingAccount(ctx context.Context, serverID int64, username string) (*models.HostingAccount, error)
	GetHostingAccountsByServer(ctx context.Context, serverID int64) ([]*models.HostingAccount, error)
}

type websiteRepository struct {
	db *gorm.DB
}

func NewWebsiteRepository(db *gorm.DB) WebsiteRepository {
	return &websiteRepository{db: db}
}

func (r *websiteRepository) WithTx(tx *gorm.DB) WebsiteRepository {
	return &websiteRepository{db: tx}
}

func (r *websiteRepository) UpsertWebsite(ctx context.Context, website *models.Website) error {
	// Identity is server_id + normalized_primary_domain
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "server_id"},
			{Name: "normalized_primary_domain"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"document_root",
			"web_server_kind",
			"status",
			"confidence",
			"source",
			"last_discovered_at",
			"updated_at",
		}),
	}).Create(website).Error
}

func (r *websiteRepository) UpsertWebsiteDomain(ctx context.Context, domain *models.WebsiteDomain) error {
	// Identity is website_id + normalized_domain_name
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "website_id"},
			{Name: "normalized_domain_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"is_primary",
			"status",
			"last_discovered_at",
			"updated_at",
		}),
	}).Create(domain).Error
}

func (r *websiteRepository) GetWebsite(ctx context.Context, serverID int64, normalizedPrimaryDomain string) (*models.Website, error) {
	var website models.Website
	err := r.db.WithContext(ctx).
		Where("server_id = ? AND normalized_primary_domain = ?", serverID, normalizedPrimaryDomain).
		First(&website).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &website, nil
}

func (r *websiteRepository) MarkStaleWebsites(ctx context.Context, serverID int64, source string, threshold time.Time) error {
	return r.db.WithContext(ctx).
		Model(&models.Website{}).
		Where("server_id = ? AND source = ? AND last_discovered_at < ?", serverID, source, threshold).
		Update("status", "MISSING").Error
}

func (r *websiteRepository) MarkStaleWebsiteDomains(ctx context.Context, serverID int64, source string, threshold time.Time) error {
	return r.db.WithContext(ctx).
		Table("website_domains").
		Where("website_id IN (SELECT id FROM websites WHERE server_id = ? AND source = ?) AND last_discovered_at < ?", serverID, source, threshold).
		Update("status", "MISSING").Error
}

func (r *websiteRepository) GetWebsitesByServer(ctx context.Context, serverID int64) ([]*models.Website, error) {
	var websites []*models.Website
	err := r.db.WithContext(ctx).
		Preload("Domains").
		Where("server_id = ?", serverID).
		Find(&websites).Error
	return websites, err
}

func (r *websiteRepository) UpsertHostingAccount(ctx context.Context, account *models.HostingAccount) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "server_id"},
			{Name: "username"},
			{Name: "source"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"role",
			"package",
			"web_count",
			"dns_count",
			"mail_count",
			"db_count",
			"disk_usage_mb",
			"bandwidth_mb",
			"is_suspended",
			"linux_uid",
			"home_dir",
			"shell",
			"date_created",
			"time_created",
			"full_name",
			"email",
			"language",
			"theme",
			"web_domains_quota",
			"web_aliases_quota",
			"dns_domains_quota",
			"dns_records_quota",
			"mail_domains_quota",
			"mail_accounts_quota",
			"backups_quota",
			"databases_quota",
			"cron_jobs_quota",
			"disk_quota",
			"bandwidth_quota",
			"ip_addresses_quota",
			"web_domains_json",
			"status",
			"updated_at",
		}),
	}).Create(account).Error
}

func (r *websiteRepository) GetHostingAccount(ctx context.Context, serverID int64, username string) (*models.HostingAccount, error) {
	var account models.HostingAccount
	err := r.db.WithContext(ctx).
		Where("server_id = ? AND username = ?", serverID, username).
		First(&account).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &account, nil
}

func (r *websiteRepository) GetHostingAccountsByServer(ctx context.Context, serverID int64) ([]*models.HostingAccount, error) {
	var accounts []*models.HostingAccount
	err := r.db.WithContext(ctx).
		Where("server_id = ?", serverID).
		Order("username ASC").
		Find(&accounts).Error
	return accounts, err
}

