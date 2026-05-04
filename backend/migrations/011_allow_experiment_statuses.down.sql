-- ============================================================================
-- Chaos-Sec: Revert additional experiment lifecycle statuses
-- Migration: 011_allow_experiment_statuses.down.sql
-- Description: Restores the original experiments status constraint.
-- ============================================================================

ALTER TABLE experiments
    DROP CONSTRAINT IF EXISTS chk_experiments_status;

ALTER TABLE experiments
    ADD CONSTRAINT chk_experiments_status
    CHECK (status IN ('draft', 'active', 'archived'));
