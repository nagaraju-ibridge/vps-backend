-- Migration: 000005_create_metrics_table.down.sql
-- Description: Drops metrics and metric_disks tables

DROP TABLE IF EXISTS metric_disks CASCADE;
DROP TABLE IF EXISTS metrics CASCADE;
