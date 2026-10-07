-- Migration: 000005_create_metrics_table.up.sql
-- Description: Creates metrics and metric_disks tables for Phase 2C.7 Metric Ingestion

-- Main metrics table capturing CPU, Memory, Swap, Network, and timestamps
CREATE TABLE IF NOT EXISTS metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id BIGINT NOT NULL,
    agent_id VARCHAR(100) NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    received_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- CPU metrics
    cpu_usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    cpu_cores INTEGER NOT NULL DEFAULT 0,
    load_1 DOUBLE PRECISION NOT NULL DEFAULT 0,
    load_5 DOUBLE PRECISION NOT NULL DEFAULT 0,
    load_15 DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- Memory metrics
    memory_total BIGINT NOT NULL DEFAULT 0,
    memory_used BIGINT NOT NULL DEFAULT 0,
    memory_available BIGINT NOT NULL DEFAULT 0,
    memory_usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- Swap metrics
    swap_total BIGINT NOT NULL DEFAULT 0,
    swap_used BIGINT NOT NULL DEFAULT 0,
    swap_free BIGINT NOT NULL DEFAULT 0,
    swap_usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,

    -- Network metrics
    network_rx_bytes BIGINT NOT NULL DEFAULT 0,
    network_tx_bytes BIGINT NOT NULL DEFAULT 0,
    network_rx_packets BIGINT NOT NULL DEFAULT 0,
    network_tx_packets BIGINT NOT NULL DEFAULT 0,
    network_errors BIGINT NOT NULL DEFAULT 0,
    network_drops BIGINT NOT NULL DEFAULT 0,

    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_metrics_server FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

-- Index for server queries
CREATE INDEX IF NOT EXISTS idx_metrics_server_id ON metrics(server_id);

-- Index for agent queries
CREATE INDEX IF NOT EXISTS idx_metrics_agent_id ON metrics(agent_id);

-- Index for time-range queries
CREATE INDEX IF NOT EXISTS idx_metrics_collected_at ON metrics(collected_at);

-- Composite index for fast server time-series queries
CREATE INDEX IF NOT EXISTS idx_metrics_server_collected_desc ON metrics(server_id, collected_at DESC);

-- Relational child table for mount point disk metrics
CREATE TABLE IF NOT EXISTS metric_disks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    metric_id UUID NOT NULL,
    mount_point VARCHAR(255) NOT NULL,
    filesystem VARCHAR(100) DEFAULT '',
    total BIGINT NOT NULL DEFAULT 0,
    used BIGINT NOT NULL DEFAULT 0,
    free BIGINT NOT NULL DEFAULT 0,
    usage_percent DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_metric_disks_metric FOREIGN KEY (metric_id) REFERENCES metrics(id) ON DELETE CASCADE
);

-- Index on metric_id for joins and cascade deletes
CREATE INDEX IF NOT EXISTS idx_metric_disks_metric_id ON metric_disks(metric_id);
