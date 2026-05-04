-- ============================================================================
-- Chaos-Sec: Allow additional experiment run statuses
-- Migration: 014_allow_experiment_run_statuses.up.sql
-- Description: Expands the experiment_runs status constraint to support
--              'queued' and 'timed_out' statuses used by the worker pool
--              and scheduler when experiments are waiting for a worker slot
--              or have exceeded the pod timeout.
-- ============================================================================

ALTER TABLE experiment_runs
    DROP CONSTRAINT IF EXISTS chk_experiment_runs_status;

ALTER TABLE experiment_runs
    ADD CONSTRAINT chk_experiment_runs_status
    CHECK (status IN ('pending', 'queued', 'running', 'completed', 'failed', 'cancelled', 'timed_out'));
