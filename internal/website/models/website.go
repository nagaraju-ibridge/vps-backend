package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type HostingAccount struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	ServerID    int64     `gorm:"not null;uniqueIndex:uq_hosting_account_identity"`
	Username    string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_hosting_account_identity"`
	Source      string    `gorm:"type:varchar(50);not null;uniqueIndex:uq_hosting_account_identity;default:'os'"`
	Role        string    `gorm:"type:varchar(50);default:'user'"`
	Package     string    `gorm:"type:varchar(100);default:'default'"`
	WebCount    int       `gorm:"default:0"`
	DNSCount    int       `gorm:"default:0"`
	MailCount   int       `gorm:"default:0"`
	DBCount     int       `gorm:"default:0"`
	DiskUsageMB int64     `gorm:"default:0"`
	BandwidthMB int64     `gorm:"default:0"`
	IsSuspended bool      `gorm:"default:false"`
	LinuxUID    *int      `gorm:"type:integer"`
	HomeDir           string    `gorm:"type:varchar(1024)"`
	Shell             string    `gorm:"type:varchar(255)"`
	DateCreated       string    `gorm:"type:varchar(50)"`
	TimeCreated       string    `gorm:"type:varchar(50)"`
	FullName          string    `gorm:"type:varchar(255)"`
	Email             string    `gorm:"type:varchar(255)"`
	Language          string    `gorm:"type:varchar(50)"`
	Theme             string    `gorm:"type:varchar(50)"`
	WebDomainsQuota   string    `gorm:"type:varchar(100)"`
	WebAliasesQuota   string    `gorm:"type:varchar(100)"`
	DNSDomainsQuota   string    `gorm:"type:varchar(100)"`
	DNSRecordsQuota   string    `gorm:"type:varchar(100)"`
	MailDomainsQuota  string    `gorm:"type:varchar(100)"`
	MailAccountsQuota string    `gorm:"type:varchar(100)"`
	BackupsQuota      string    `gorm:"type:varchar(100)"`
	DatabasesQuota    string    `gorm:"type:varchar(100)"`
	CronJobsQuota     string    `gorm:"type:varchar(100)"`
	DiskQuota         string    `gorm:"type:varchar(100)"`
	BandwidthQuota    string    `gorm:"type:varchar(100)"`
	IPAddressesQuota  string    `gorm:"type:varchar(100)"`
	WebDomainsJSON    string    `gorm:"type:text"`
	Status            string    `gorm:"type:varchar(50);default:'ACTIVE'"`
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

func (h *HostingAccount) BeforeCreate(tx *gorm.DB) (err error) {
	if h.ID == uuid.Nil {
		h.ID = uuid.New()
	}
	return
}

type Website struct {
	ID                      uuid.UUID        `gorm:"type:uuid;primaryKey"`
	ServerID                int64            `gorm:"not null;uniqueIndex:uq_website_identity"`
	HostingAccountID        *uuid.UUID       `gorm:"type:uuid;index"`
	PrimaryDomain           string           `gorm:"type:varchar(255);not null"`
	NormalizedPrimaryDomain string           `gorm:"type:varchar(255);not null;uniqueIndex:uq_website_identity"`
	DocumentRoot            *string          `gorm:"type:varchar(1024)"`
	WebServerKind           *string          `gorm:"type:varchar(50)"`
	Status                  string           `gorm:"type:varchar(50);not null;default:'ACTIVE'"`
	Confidence              string           `gorm:"type:varchar(20);not null"`
	Source                  string           `gorm:"type:varchar(50);not null"`
	LastDiscoveredAt        time.Time        `gorm:"not null"`
	CreatedAt               time.Time        `gorm:"autoCreateTime"`
	UpdatedAt               time.Time        `gorm:"autoUpdateTime"`
	Domains                 []WebsiteDomain  `gorm:"constraint:OnDelete:CASCADE;"`
}

func (w *Website) BeforeCreate(tx *gorm.DB) (err error) {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return
}

type WebsiteDomain struct {
	ID                   uuid.UUID `gorm:"type:uuid;primaryKey"`
	WebsiteID            uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_website_domain"`
	DomainName           string    `gorm:"type:varchar(255);not null"`
	NormalizedDomainName string    `gorm:"type:varchar(255);not null;uniqueIndex:uq_website_domain"`
	IsPrimary            bool      `gorm:"not null;default:false"`
	Status               string    `gorm:"type:varchar(50);not null;default:'ACTIVE'"`
	LastDiscoveredAt     time.Time `gorm:"not null"`
	CreatedAt            time.Time `gorm:"autoCreateTime"`
	UpdatedAt            time.Time `gorm:"autoUpdateTime"`
}

func (wd *WebsiteDomain) BeforeCreate(tx *gorm.DB) (err error) {
	if wd.ID == uuid.Nil {
		wd.ID = uuid.New()
	}
	return
}
