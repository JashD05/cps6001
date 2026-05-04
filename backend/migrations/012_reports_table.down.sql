-- ============================================================================
-- Chaos-Sec: Reports Table Down Migration
-- Migration: 012_reports_table.down.sql
-- Description: Drops the reports table and associated objects
-- ============================================================================

-- Drop the trigger first (needs to be dropped before the function)
DROP TRIGGER IF EXISTS trigger_reports_updated_at ON reports;

-- Drop the update function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop the reports table
DROP TABLE IF EXISTS reports;
