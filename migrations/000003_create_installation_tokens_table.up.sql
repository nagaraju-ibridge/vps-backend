-- Migration: 000003_create_installation_tokens_table.up.sql
-- Description: Creates the installation_tokens table for Phase 2B (Installation Token Management)
-- Relational hierarchy: User -> Server -> Installation Token -> Agent

CREATE TABLE IF NOT EXISTS installation_tokens (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    token_hash VARCHAR(255) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    is_used BOOLEAN NOT NULL DEFAULT FALSE,
    used_at TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_installation_tokens_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

-- Optimize token lookup by hash during agent registration (Phase 2C)
CREATE INDEX IF NOT EXISTS idx_installation_tokens_token_hash ON installation_tokens(token_hash);

-- Optimize lookup of tokens for a given server
CREATE INDEX IF NOT EXISTS idx_installation_tokens_server_id ON installation_tokens(server_id);

-- Optimize cleanup or filtering of expired tokens
CREATE INDEX IF NOT EXISTS idx_installation_tokens_expires_at ON installation_tokens(expires_at);
