package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

type Website struct {
	ID                  string  `json:"id"`
	PrimaryDomain       string  `json:"primary_domain"`
	NormalizedPrimaryDomain string  `json:"normalized_primary_domain"`
	DocumentRoot        *string `json:"document_root"`
	WebServerKind       *string `json:"web_server_kind"`
	Status              string  `json:"status"`
	Confidence          string  `json:"confidence"`
	Source              string  `json:"source"`
	LastDiscoveredAt    string  `json:"last_discovered_at"`
}

type WebsiteDomain struct {
	ID                  string `json:"id"`
	WebsiteID           string `json:"website_id"`
	DomainName          string `json:"domain_name"`
	NormalizedDomainName string `json:"normalized_domain_name"`
	IsPrimary           bool   `json:"is_primary"`
	Status              string `json:"status"`
	LastDiscoveredAt    string `json:"last_discovered_at"`
}

type HostingAccount struct {
	ID       string `json:"id"`
	ServerID int64  `json:"server_id"`
	Username string `json:"username"`
	Source   string `json:"source"`
}

func main() {
	dbURL := "postgresql://neondb_owner:npg_jnyM3xQJ5lqO@ep-quiet-scene-b31gv8sg-pooler.c-4.ap-southeast-1.aws.neon.tech/neondb?sslmode=require"
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer db.Close()

	// 1. Query websites
	var websites []Website
	rows, err := db.Query("SELECT id, primary_domain, normalized_primary_domain, document_root, web_server_kind, status, confidence, source, last_discovered_at FROM websites WHERE server_id = $1", 2)
	if err != nil {
		log.Fatalf("failed to query websites: %v", err)
	}
	for rows.Next() {
		var w Website
		if err := rows.Scan(&w.ID, &w.PrimaryDomain, &w.NormalizedPrimaryDomain, &w.DocumentRoot, &w.WebServerKind, &w.Status, &w.Confidence, &w.Source, &w.LastDiscoveredAt); err != nil {
			log.Fatalf("failed to scan website: %v", err)
		}
		websites = append(websites, w)
	}

	// 2. Query website domains
	var domains []WebsiteDomain
	rows2, err := db.Query("SELECT id, website_id, domain_name, normalized_domain_name, is_primary, status, last_discovered_at FROM website_domains WHERE website_id IN (SELECT id FROM websites WHERE server_id = $1)", 2)
	if err != nil {
		log.Fatalf("failed to query website_domains: %v", err)
	}
	for rows2.Next() {
		var d WebsiteDomain
		if err := rows2.Scan(&d.ID, &d.WebsiteID, &d.DomainName, &d.NormalizedDomainName, &d.IsPrimary, &d.Status, &d.LastDiscoveredAt); err != nil {
			log.Fatalf("failed to scan domain: %v", err)
		}
		domains = append(domains, d)
	}

	// 3. Query hosting accounts
	var accounts []HostingAccount
	rows3, err := db.Query("SELECT id, server_id, username, source FROM hosting_accounts WHERE server_id = $1", 2)
	if err != nil {
		log.Fatalf("failed to query hosting_accounts: %v", err)
	}
	for rows3.Next() {
		var a HostingAccount
		if err := rows3.Scan(&a.ID, &a.ServerID, &a.Username, &a.Source); err != nil {
			log.Fatalf("failed to scan account: %v", err)
		}
		accounts = append(accounts, a)
	}

	res := map[string]interface{}{
		"total_websites":        len(websites),
		"total_website_domains": len(domains),
		"total_hosting_accounts": len(accounts),
		"websites":        websites,
		"website_domains": domains,
	}

	out, _ := json.MarshalIndent(res, "", "  ")
	fmt.Println(string(out))
}
