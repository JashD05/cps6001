-- ============================================================================
-- Chaos-Sec: Drop Report Content Column Migration
-- Migration: 015_add_report_content.down.sql
-- Description: Drops the content BYTEA column from the reports table
-- ============================================================================

ALTER TABLE reports DROP COLUMN IF EXISTS content;
