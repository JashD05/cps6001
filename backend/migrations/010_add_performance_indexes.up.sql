-- ============================================================================
-- Chaos-Sec: Performance Indexes and Dashboard Materialized View
-- Migration: 010_add_performance_indexes.up.sql
-- Description: Adds composite indexes for common query patterns and a
--              materialized view for dashboard summary statistics.
-- ============================================================================

-- ---------------------------------------------------------------------------
-- Composite indexes for the `experiments` table
-- ---------------------------------------------------------------------------

-- Supports filtering experiments by organization and status (list filtering).
-- Queries like: SELECT * FROM experiments WHERE organization_id = ? AND status = ?
CREATE INDEX IF NOT EXISTS idx_experiments_org_status
    ON experiments (organization_id, status);

-- Supports sorting experiments by creation date within an organization.
-- Queries like: SELECT * FROM experiments WHERE organization_id = ? ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_experiments_org_created_at_desc
    ON experiments (organization_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Composite indexes for the `experiment_runs` table
-- ---------------------------------------------------------------------------

-- Supports retrieving run history for an experiment sorted by creation time.
-- Queries like: SELECT * FROM experiment_runs WHERE experiment_id = ? ORDER BY created_at DESC
-- Note: a similar index idx_runs_exp_created already exists; this is retained for
-- naming consistency with the performance optimization set.
CREATE INDEX IF NOT EXISTS idx_experiment_runs_exp_created_at_desc
    ON experiment_runs (experiment_id, created_at DESC);

-- Supports filtering runs by experiment and status (status filtering).
-- Queries like: SELECT * FROM experiment_runs WHERE experiment_id = ? AND status = ?
CREATE INDEX IF NOT EXISTS idx_experiment_runs_exp_status
    ON experiment_runs (experiment_id, status);

-- ---------------------------------------------------------------------------
-- Composite indexes for the `attack_pods` table
-- ---------------------------------------------------------------------------

-- Supports querying pod status within a specific run.
-- Queries like: SELECT * FROM attack_pods WHERE run_id = ? AND status = ?
CREATE INDEX IF NOT EXISTS idx_attack_pods_run_status
    ON attack_pods (run_id, status);

-- ---------------------------------------------------------------------------
-- Composite indexes for the `siem_validations` table
-- ---------------------------------------------------------------------------

-- Supports querying SIEM validation results for a run, filtered by match status.
-- Queries like: SELECT * FROM siem_validations WHERE run_id = ? AND matched = ?
CREATE INDEX IF NOT EXISTS idx_siem_validations_run_matched
    ON siem_validations (run_id, matched);

-- ---------------------------------------------------------------------------
-- Indexes for the `users` table
-- ---------------------------------------------------------------------------

-- Supports fast email lookups during login/authentication.
-- Note: A UNIQUE constraint on users(email) already provides an implicit index;
-- this explicit index ensures a dedicated, clearly-named entry for login queries.
CREATE INDEX IF NOT EXISTS idx_users_email_lookup
    ON users (email);

-- Supports listing organization members efficiently.
-- Queries like: SELECT * FROM users WHERE organization_id = ?
CREATE INDEX IF NOT EXISTS idx_users_org_member
    ON users (organization_id);

-- ---------------------------------------------------------------------------
-- Additional performance indexes for high-frequency queries
-- ---------------------------------------------------------------------------

-- Supports filtering audit logs by organization and timestamp (most recent first).
-- Dashboard activity feed pattern:
--   SELECT * FROM audit_logs WHERE organization_id = ? ORDER BY timestamp DESC
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_timestamp
    ON audit_logs (organization_id, timestamp DESC);

-- Supports filtering experiment runs by status with most recent first.
-- Pattern: SELECT * FROM experiment_runs WHERE status = 'running' ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_experiment_runs_status_created_at
    ON experiment_runs (status, created_at DESC);

-- Supports filtering attack pods by status for real-time monitoring.
-- Pattern: SELECT * FROM attack_pods WHERE status = 'running'
CREATE INDEX IF NOT EXISTS idx_attack_pods_status_started_at
    ON attack_pods (status, started_at DESC NULLS LAST);

-- Supports checking for un-validated SIEM results per run.
-- Pattern: SELECT * FROM siem_validations WHERE run_id = ? AND validation_status = 'pending'
CREATE INDEX IF NOT EXISTS idx_siem_validations_run_status
    ON siem_validations (run_id, validation_status);

-- ---------------------------------------------------------------------------
-- Materialized view: dashboard_summary
-- ---------------------------------------------------------------------------
-- Provides pre-computed per-organization statistics for the main dashboard.
-- Refresh with: REFRESH MATERIALIZED VIEW CONCURRENTLY dashboard_summary;
-- This view aggregates experiment counts, run statistics, and average durations
-- so the dashboard API can serve a single query instead of multiple joins.
-- ---------------------------------------------------------------------------

CREATE MATERIALIZED VIEW IF NOT EXISTS dashboard_summary AS
SELECT
    o.id                                                                          AS organization_id,
    o.name                                                                        AS organization_name,
    o.status                                                                      AS organization_status,
    COUNT(DISTINCT e.id)                                                          AS total_experiments,
    COUNT(DISTINCT e.id) FILTER (WHERE e.status = 'active')                       AS active_experiments,
    COUNT(DISTINCT e.id) FILTER (WHERE e.status = 'draft')                         AS draft_experiments,
    COUNT(DISTINCT e.id) FILTER (WHERE e.status = 'archived')                      AS archived_experiments,
    COUNT(er.id)                                                                  AS total_runs,
    COUNT(er.id) FILTER (WHERE er.status = 'completed')                            AS completed_runs,
    COUNT(er.id) FILTER (WHERE er.status = 'failed')                               AS failed_runs,
    COUNT(er.id) FILTER (WHERE er.status = 'running')                              AS running_runs,
    COUNT(er.id) FILTER (WHERE er.status = 'pending')                              AS pending_runs,
    COUNT(er.id) FILTER (WHERE er.status = 'cancelled')                            AS cancelled_runs,
    COALESCE(AVG(er.duration_ms) FILTER (WHERE er.status = 'completed'), 0)      AS avg_run_duration_ms,
    COALESCE(MAX(er.duration_ms) FILTER (WHERE er.status = 'completed'), 0)       AS max_run_duration_ms,
    COALESCE(MIN(er.duration_ms) FILTER (WHERE er.status = 'completed'), 0)       AS min_run_duration_ms,
    MAX(er.created_at)                                                            AS last_run_at,
    COUNT(DISTINCT sv.id) FILTER (WHERE sv.matched = true)                         AS total_successful_validations,
    COUNT(DISTINCT sv.id) FILTER (WHERE sv.matched = false)                        AS total_failed_validations,
    COUNT(DISTINCT u.id)                                                          AS total_members,
    COUNT(DISTINCT u.id) FILTER (WHERE u.is_active = true)                         AS active_members
FROM organizations o
    LEFT JOIN users u ON u.organization_id = o.id
    LEFT JOIN experiments e ON e.organization_id = o.id
    LEFT JOIN experiment_runs er ON er.experiment_id = e.id
    LEFT JOIN siem_validations sv ON sv.run_id = er.id
GROUP BY o.id, o.name, o.status;

-- Unique index on the materialized view so we can use REFRESH CONCURRENTLY.
CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_summary_org_id
    ON dashboard_summary (organization_id);
