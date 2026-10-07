-- Migration: 000002_create_servers_table.up.sql
-- Description: Creates the servers table for VPS server management in Phase 2A

CREATE TABLE IF NOT EXISTS servers (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,
    hostname VARCHAR(255) DEFAULT NULL,
    ip_address VARCHAR(100) DEFAULT NULL,
    os VARCHAR(100) DEFAULT NULL,
    architecture VARCHAR(50) DEFAULT NULL,
    agent_id VARCHAR(100) DEFAULT NULL,
    agent_status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    last_seen TIMESTAMPTZ DEFAULT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at TIMESTAMPTZ DEFAULT NULL,
    CONSTRAINT fk_servers_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_servers_user_id ON servers(user_id);
CREATE INDEX IF NOT EXISTS idx_servers_agent_id ON servers(agent_id);
CREATE INDEX IF NOT EXISTS idx_servers_deleted_at ON servers(deleted_at);
