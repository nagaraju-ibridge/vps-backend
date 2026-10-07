-- Migration: 000004_create_agents_table.up.sql
-- Description: Creates the agents table for Phase 2C (Agent Registration & Telemetry)
-- Relational hierarchy: User -> Server -> Installation Token -> Agent

CREATE TABLE IF NOT EXISTS agents (
    id BIGSERIAL PRIMARY KEY,
    server_id BIGINT NOT NULL,
    agent_id VARCHAR(100) NOT NULL UNIQUE,
    token_hash VARCHAR(255) NOT NULL,
    version VARCHAR(50) NOT NULL DEFAULT '0.1.0',
    status VARCHAR(50) NOT NULL DEFAULT 'ACTIVE',
    last_seen TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT fk_agents_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE,
    CONSTRAINT uq_agents_server_id UNIQUE (server_id)
);

-- Index on agent_id for O(1) agent identification during heartbeat and telemetry
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_agent_id ON agents(agent_id);

-- Index on server_id for quick lookup of a server's active agent
CREATE INDEX IF NOT EXISTS idx_agents_server_id ON agents(server_id);

-- Index on token_hash for credential validation
CREATE INDEX IF NOT EXISTS idx_agents_token_hash ON agents(token_hash);
