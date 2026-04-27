-- ============================================================================
-- Chaos-Sec: Drop Performance Indexes
-- Migration: 010_add_performance_indexes.down.sql
-- Description: Drops all composite indexes and materialized view added for
--              performance optimization in the up migration.
-- ============================================================================

-- Drop the materialized view first (depends on the underlying tables)
DROP MATERIALIZED VIEW IF EXISTS mv_dashboard_summary;

-- Drop composite indexes on experiments
DROP INDEX IF EXISTS idx_perf_experiments_org_status;
DROP INDEX IF EXISTS idx_perf_experiments_org_created_at;

-- Drop composite indexes on experiment_runs
DROP INDEX IF EXISTS idx_perf_runs_exp_created_at;
DROP INDEX IF EXISTS idx_perf_runs_exp_status;

-- Drop composite index on attack_pods
DROP INDEX IF EXISTS idx_perf_pods_run_status;

-- Drop composite index on siem_validations
DROP INDEX IF EXISTS idx_perf_siem_run_matched;

-- Drop indexes on users
DROP INDEX IF EXISTS idx_perf_users_email;
DROP INDEX IF EXISTS idx_perf_users_org_id;
