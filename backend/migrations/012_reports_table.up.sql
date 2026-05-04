-- ============================================================================
-- Chaos-Sec: Reports Table Migration
-- Migration: 012_reports_table.up.sql
-- Description: Creates the reports table for storing generated experiment reports
-- ============================================================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- Reports table
-- ============================================================================
CREATE TABLE reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,

    -- Report metadata
    title VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL DEFAULT 'experiment',
    format VARCHAR(20) NOT NULL DEFAULT 'pdf',
    description TEXT,

    -- Experiment references
    experiment_ids JSONB DEFAULT '[]'::jsonb,
    date_range_from TIMESTAMPTZ,
    date_range_to TIMESTAMPTZ,

    -- Status tracking
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    error_message TEXT,

    -- Generated file info
    download_url VARCHAR(500),
    file_size BIGINT,

    -- Provenance
    generated_by UUID NOT NULL REFERENCES users(id),

    -- Timestamps
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Constraints
    CONSTRAINT chk_reports_type CHECK (type IN ('experiment', 'compliance', 'executive', 'trend')),
    CONSTRAINT chk_reports_format CHECK (format IN ('pdf', 'csv', 'json', 'html')),
    CONSTRAINT chk_reports_status CHECK (status IN ('pending', 'generating', 'ready', 'error'))
);

-- ============================================================================
-- Indexes
-- ============================================================================

-- Index for listing reports by organization (most common query)
CREATE INDEX idx_reports_organization_id ON reports(organization_id);

-- Index for filtering by type
CREATE INDEX idx_reports_type ON reports(type);

-- Index for filtering by status
CREATE INDEX idx_reports_status ON reports(status);

-- Index for sorting by creation date
CREATE INDEX idx_reports_created_at ON reports(created_at DESC);

-- Index for filtering by date range
CREATE INDEX idx_reports_date_range ON reports(date_range_from, date_range_to);

-- Composite index for common query patterns
CREATE INDEX idx_reports_org_status ON reports(organization_id, status);
CREATE INDEX idx_reports_org_created ON reports(organization_id, created_at DESC);

-- ============================================================================
-- Comments
-- ============================================================================
COMMENT ON TABLE reports IS 'Stores generated experiment reports in various formats (PDF, CSV, JSON, HTML)';
COMMENT ON COLUMN reports.experiment_ids IS 'JSON array of experiment IDs included in this report';
COMMENT ON COLUMN reports.date_range_from IS 'Start date of the reporting period';
COMMENT ON COLUMN reports.date_range_to IS 'End date of the reporting period';
COMMENT ON COLUMN reports.status IS 'Report generation status: pending, generating, ready, or error';
COMMENT ON COLUMN reports.download_url IS 'Pre-signed or internal URL for downloading the report file';
COMMENT ON COLUMN reports.file_size IS 'Size of the generated report file in bytes';

-- ============================================================================
-- Trigger for updated_at auto-update
-- ============================================================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER trigger_reports_updated_at
    BEFORE UPDATE ON reports
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
