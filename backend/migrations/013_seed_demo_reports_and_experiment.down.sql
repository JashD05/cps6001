-- ============================================================================
-- Chaos-Sec: Demo Data Seed Migration Down
-- Migration: 013_seed_demo_reports_and_experiment.down.sql
-- Description: Removes the seeded demo experiment, report, and related records
-- ============================================================================

DELETE FROM reports
WHERE title = 'CPU Stress Test Demo Report';

DELETE FROM experiment_runs
WHERE experiment_id IN (
    SELECT id FROM experiments WHERE name = 'CPU Stress Test Demo'
);

DELETE FROM experiment_templates
WHERE experiment_id IN (
    SELECT id FROM experiments WHERE name = 'CPU Stress Test Demo'
);

DELETE FROM experiments
WHERE name = 'CPU Stress Test Demo';

DELETE FROM kubernetes_clusters
WHERE name = 'Demo Kubernetes Cluster';
