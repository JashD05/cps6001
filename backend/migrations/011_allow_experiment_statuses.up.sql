-- ============================================================================
-- Chaos-Sec: Allow additional experiment lifecycle statuses
-- Migration: 011_allow_experiment_statuses.up.sql
-- Description: Expands the experiments status constraint to support lifecycle
--              values used by the application UI and stop/start flows.
-- ============================================================================

ALTER TABLE experiments
    DROP CONSTRAINT IF EXISTS chk_experiments_status;

ALTER TABLE experiments
    ADD CONSTRAINT chk_experiments_status
    CHECK (status IN ('draft', 'active', 'running', 'stopped', 'archived'));
