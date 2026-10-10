CREATE TABLE hosting_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    username VARCHAR(255) NOT NULL,
    source VARCHAR(50) NOT NULL,
    linux_uid INTEGER,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uq_hosting_account_identity UNIQUE (server_id, username, source)
);

CREATE TABLE websites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id BIGINT NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    hosting_account_id UUID REFERENCES hosting_accounts(id) ON DELETE SET NULL,
    primary_domain VARCHAR(255) NOT NULL,
    normalized_primary_domain VARCHAR(255) NOT NULL,
    document_root VARCHAR(1024),
    web_server_kind VARCHAR(50),
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    confidence VARCHAR(20) NOT NULL,
    source VARCHAR(50) NOT NULL,
    last_discovered_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uq_website_identity UNIQUE (server_id, normalized_primary_domain)
);

CREATE TABLE website_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    website_id UUID NOT NULL REFERENCES websites(id) ON DELETE CASCADE,
    domain_name VARCHAR(255) NOT NULL,
    normalized_domain_name VARCHAR(255) NOT NULL,
    is_primary BOOLEAN NOT NULL DEFAULT false,
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    last_discovered_at TIMESTAMP WITH TIME ZONE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT uq_website_domain UNIQUE (website_id, normalized_domain_name)
);

CREATE INDEX idx_hosting_accounts_server_username ON hosting_accounts(server_id, username);
CREATE INDEX idx_websites_server_domain ON websites(server_id, normalized_primary_domain);
CREATE INDEX idx_websites_hosting_account ON websites(hosting_account_id);
CREATE INDEX idx_website_domains_website_normalized ON website_domains(website_id, normalized_domain_name);
