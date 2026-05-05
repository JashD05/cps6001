-- ============================================================================
-- Chaos-Sec: Add Report Content Column Migration
-- Migration: 015_add_report_content.up.sql
-- Description: Adds a content BYTEA column to the reports table for storing
--              generated report binary data (PDF, JSON, CSV, HTML)
-- ============================================================================

ALTER TABLE reports ADD COLUMN content BYTEA;

COMMENT ON COLUMN reports.content IS 'Binary content of the generated report file (PDF, JSON, CSV, or HTML bytes)';
