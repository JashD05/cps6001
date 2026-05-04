-- ============================================================================
-- Chaos-Sec: Revert experiment run statuses
-- Migration: 014_allow_experiment_run_statuses.down.sql
-- Description: Reverts the experiment_runs status constraint back to the
--              original set, removing 'queued' and 'timed_out'.
-- ============================================================================

ALTER TABLE experiment_runs
    DROP CONSTRAINT IF EXISTS chk_experiment_runs_status;

ALTER TABLE experiment_runs
    ADD CONSTRAINT chk_experiment_runs_status
    CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled'));
