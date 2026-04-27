-- ============================================================================
-- Chaos-Sec: Initial Database Schema
-- Migration: 001_initial_schema.sql
-- Description: Creates all core tables for the security orchestration platform
-- ============================================================================

-- Enable UUID extension
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- 1. organizations
-- ============================================================================
CREATE TABLE organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE,
    slug VARCHAR(100) NOT NULL UNIQUE,
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    settings JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_organizations_status CHECK (status IN ('active', 'suspended', 'deleted'))
);

CREATE INDEX idx_organizations_slug ON organizations(slug);
CREATE INDEX idx_organizations_status ON organizations(status);

-- ============================================================================
-- 2. roles
-- ============================================================================
CREATE TABLE roles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL UNIQUE,
    description TEXT NULL,
    permissions JSONB NOT NULL,
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_roles_name ON roles(name);

-- ============================================================================
-- 3. users
-- ============================================================================
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email VARCHAR(255) NOT NULL UNIQUE,
    password_hash VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    last_login_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_organization_id ON users(organization_id);
CREATE INDEX idx_users_role_id ON users(role_id);
CREATE INDEX idx_users_is_active ON users(is_active);

-- ============================================================================
-- 4. api_keys
-- ============================================================================
CREATE TABLE api_keys (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    key_hash VARCHAR(255) NOT NULL UNIQUE,
    key_prefix VARCHAR(8) NOT NULL,
    permissions JSONB NULL,
    expires_at TIMESTAMPTZ NULL,
    last_used_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    revoked_at TIMESTAMPTZ NULL
);

CREATE INDEX idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX idx_api_keys_expires_at ON api_keys(expires_at);

-- ============================================================================
-- 5. kubernetes_clusters
-- ============================================================================
CREATE TABLE kubernetes_clusters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    api_endpoint VARCHAR(500) NOT NULL,
    ca_certificate TEXT NOT NULL,
    client_certificate TEXT NOT NULL,
    client_key TEXT NOT NULL,
    default_namespace VARCHAR(255) NOT NULL DEFAULT 'chaos-sec',
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    last_connected_at TIMESTAMPTZ NULL,
    kubernetes_version VARCHAR(50) NULL,
    node_count INTEGER NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_clusters_status CHECK (status IN ('pending', 'connected', 'error', 'disabled'))
);

CREATE INDEX idx_clusters_organization_id ON kubernetes_clusters(organization_id);
CREATE INDEX idx_clusters_status ON kubernetes_clusters(status);
CREATE INDEX idx_clusters_name ON kubernetes_clusters(name);

-- ============================================================================
-- 6. attack_templates
-- ============================================================================
CREATE TABLE attack_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    category VARCHAR(100) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    description TEXT NOT NULL,
    mitre_attack_id VARCHAR(50) NULL,
    k8s_manifest JSONB NOT NULL,
    parameters JSONB NOT NULL,
    prerequisites JSONB DEFAULT '[]'::jsonb,
    expected_behavior TEXT NOT NULL,
    mitigation TEXT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    is_system BOOLEAN NOT NULL DEFAULT false,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_attack_templates_category CHECK (category IN ('network', 'privilege', 'data', 'availability')),
    CONSTRAINT chk_attack_templates_severity CHECK (severity IN ('low', 'medium', 'high', 'critical'))
);

CREATE INDEX idx_templates_slug ON attack_templates(slug);
CREATE INDEX idx_templates_category ON attack_templates(category);
CREATE INDEX idx_templates_severity ON attack_templates(severity);
CREATE INDEX idx_templates_mitre_attack_id ON attack_templates(mitre_attack_id);
CREATE INDEX idx_templates_is_active ON attack_templates(is_active);

-- ============================================================================
-- 7. experiments
-- ============================================================================
CREATE TABLE experiments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'draft',
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    schedule_cron VARCHAR(100) NULL,
    auto_cleanup BOOLEAN NOT NULL DEFAULT true,
    notification_config JSONB DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_experiments_status CHECK (status IN ('draft', 'active', 'archived'))
);

CREATE INDEX idx_experiments_organization_id ON experiments(organization_id);
CREATE INDEX idx_experiments_status ON experiments(status);
CREATE INDEX idx_experiments_created_by ON experiments(created_by);

-- ============================================================================
-- 8. experiment_templates
-- ============================================================================
CREATE TABLE experiment_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    attack_template_id UUID NOT NULL REFERENCES attack_templates(id) ON DELETE RESTRICT,
    order_index INTEGER NOT NULL DEFAULT 0,
    configuration JSONB NOT NULL,
    target_namespaces TEXT[] NULL,
    target_labels JSONB NULL,
    duration_seconds INTEGER NOT NULL DEFAULT 300,
    cleanup_policy VARCHAR(50) NOT NULL DEFAULT 'immediate',
    siem_validation JSONB NULL,
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_exp_templates_cleanup_policy CHECK (cleanup_policy IN ('immediate', 'delayed', 'manual'))
);

CREATE INDEX idx_exp_templates_experiment_id ON experiment_templates(experiment_id);
CREATE INDEX idx_exp_templates_attack_template_id ON experiment_templates(attack_template_id);
CREATE INDEX idx_exp_templates_order ON experiment_templates(experiment_id, order_index);

-- ============================================================================
-- 9. experiment_runs
-- ============================================================================
CREATE TABLE experiment_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    experiment_id UUID NOT NULL REFERENCES experiments(id) ON DELETE CASCADE,
    cluster_id UUID NOT NULL REFERENCES kubernetes_clusters(id) ON DELETE RESTRICT,
    run_number INTEGER NOT NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    triggered_by UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    trigger_type VARCHAR(30) NOT NULL,
    started_at TIMESTAMPTZ NULL,
    completed_at TIMESTAMPTZ NULL,
    duration_ms BIGINT NULL,
    result_summary JSONB NULL,
    error_message TEXT NULL,
    cleanup_status VARCHAR(30) DEFAULT 'pending',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_experiment_runs_status CHECK (status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
    CONSTRAINT chk_experiment_runs_trigger_type CHECK (trigger_type IN ('manual', 'scheduled', 'api', 'webhook')),
    CONSTRAINT chk_experiment_runs_cleanup_status CHECK (cleanup_status IN ('pending', 'completed', 'failed', 'skipped'))
);

CREATE INDEX idx_runs_experiment_id ON experiment_runs(experiment_id);
CREATE INDEX idx_runs_cluster_id ON experiment_runs(cluster_id);
CREATE INDEX idx_runs_status ON experiment_runs(status);
CREATE INDEX idx_runs_started_at ON experiment_runs(started_at);
CREATE INDEX idx_runs_created_at ON experiment_runs(created_at);
CREATE INDEX idx_runs_exp_created ON experiment_runs(experiment_id, created_at DESC);

-- ============================================================================
-- 10. attack_pods
-- ============================================================================
CREATE TABLE attack_pods (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES experiment_runs(id) ON DELETE CASCADE,
    template_id UUID NOT NULL REFERENCES attack_templates(id) ON DELETE RESTRICT,
    pod_name VARCHAR(255) NOT NULL,
    namespace VARCHAR(255) NOT NULL,
    node_name VARCHAR(255) NULL,
    ip_address VARCHAR(45) NULL,
    status VARCHAR(30) NOT NULL DEFAULT 'pending',
    phase VARCHAR(30) NULL,
    started_at TIMESTAMPTZ NULL,
    terminated_at TIMESTAMPTZ NULL,
    exit_code INTEGER NULL,
    logs_summary TEXT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_attack_pods_status CHECK (status IN ('pending', 'creating', 'running', 'completed', 'failed', 'terminated'))
);

CREATE INDEX idx_pods_run_id ON attack_pods(run_id);
CREATE INDEX idx_pods_template_id ON attack_pods(template_id);
CREATE INDEX idx_pods_status ON attack_pods(status);
CREATE INDEX idx_pods_namespace ON attack_pods(namespace);

-- ============================================================================
-- 11. siem_validations
-- ============================================================================
CREATE TABLE siem_validations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES experiment_runs(id) ON DELETE CASCADE,
    attack_pod_id UUID NULL REFERENCES attack_pods(id) ON DELETE SET NULL,
    expected_alert_type VARCHAR(255) NOT NULL,
    expected_alert_severity VARCHAR(20) NULL,
    alert_received BOOLEAN NOT NULL DEFAULT false,
    received_at TIMESTAMPTZ NULL,
    siem_response JSONB NULL,
    alert_id VARCHAR(255) NULL,
    matched BOOLEAN NULL,
    match_details JSONB NULL,
    validation_status VARCHAR(30) NOT NULL DEFAULT 'pending',
    checked_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_siem_validation_status CHECK (validation_status IN ('pending', 'validated', 'failed', 'timeout'))
);

CREATE INDEX idx_siem_run_id ON siem_validations(run_id);
CREATE INDEX idx_siem_attack_pod_id ON siem_validations(attack_pod_id);
CREATE INDEX idx_siem_alert_received ON siem_validations(alert_received);
CREATE INDEX idx_siem_validation_status ON siem_validations(validation_status);

-- ============================================================================
-- 12. test_results
-- ============================================================================
CREATE TABLE test_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES experiment_runs(id) ON DELETE CASCADE,
    attack_pod_id UUID NULL REFERENCES attack_pods(id) ON DELETE SET NULL,
    check_name VARCHAR(255) NOT NULL,
    check_type VARCHAR(100) NOT NULL,
    status VARCHAR(30) NOT NULL,
    expected TEXT NULL,
    actual TEXT NULL,
    details JSONB NULL,
    error_message TEXT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_test_results_status CHECK (status IN ('passed', 'failed', 'skipped', 'error')),
    CONSTRAINT chk_test_results_check_type CHECK (check_type IN ('network', 'privilege', 'detection', 'remediation'))
);

CREATE INDEX idx_results_run_id ON test_results(run_id);
CREATE INDEX idx_results_attack_pod_id ON test_results(attack_pod_id);
CREATE INDEX idx_results_status ON test_results(status);
CREATE INDEX idx_results_check_type ON test_results(check_type);
CREATE INDEX idx_results_timestamp ON test_results(timestamp);

-- ============================================================================
-- 13. audit_logs
-- ============================================================================
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NULL REFERENCES users(id) ON DELETE SET NULL,
    api_key_id UUID NULL REFERENCES api_keys(id) ON DELETE SET NULL,
    action VARCHAR(100) NOT NULL,
    resource_type VARCHAR(100) NOT NULL,
    resource_id UUID NULL,
    resource_name VARCHAR(255) NULL,
    details JSONB NULL,
    ip_address VARCHAR(45) NULL,
    user_agent VARCHAR(500) NULL,
    status VARCHAR(30) NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_audit_logs_status CHECK (status IN ('success', 'failure', 'partial'))
);

CREATE INDEX idx_audit_organization_id ON audit_logs(organization_id);
CREATE INDEX idx_audit_user_id ON audit_logs(user_id);
CREATE INDEX idx_audit_action ON audit_logs(action);
CREATE INDEX idx_audit_resource_type ON audit_logs(resource_type);
CREATE INDEX idx_audit_timestamp ON audit_logs(timestamp);
CREATE INDEX idx_audit_status ON audit_logs(status);

-- Make audit_logs append-only: prevent UPDATE and DELETE
CREATE OR REPLACE FUNCTION prevent_audit_log_modification()
RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'audit_logs table is append-only: UPDATEs are not allowed';
    ELSIF TG_OP = 'DELETE' THEN
        RAISE EXCEPTION 'audit_logs table is append-only: DELETEs are not allowed';
    END IF;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_audit_log_no_update
    BEFORE UPDATE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_modification();

CREATE TRIGGER trg_audit_log_no_delete
    BEFORE DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_modification();

-- ============================================================================
-- 14. notifications
-- ============================================================================
CREATE TABLE notifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    type VARCHAR(50) NOT NULL,
    configuration JSONB NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_notifications_type CHECK (type IN ('email', 'slack', 'webhook', 'pagerduty'))
);

CREATE INDEX idx_notifications_organization_id ON notifications(organization_id);
CREATE INDEX idx_notifications_type ON notifications(type);
CREATE INDEX idx_notifications_is_active ON notifications(is_active);

-- ============================================================================
-- 15. notification_events
-- ============================================================================
CREATE TABLE notification_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id UUID NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    related_run_id UUID NULL REFERENCES experiment_runs(id) ON DELETE SET NULL,
    subject VARCHAR(500) NULL,
    content TEXT NULL,
    status VARCHAR(30) NOT NULL,
    sent_at TIMESTAMPTZ NULL,
    delivered_at TIMESTAMPTZ NULL,
    error_message TEXT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT chk_notification_events_status CHECK (status IN ('pending', 'sent', 'delivered', 'failed'))
);

CREATE INDEX idx_notif_events_notification_id ON notification_events(notification_id);
CREATE INDEX idx_notif_events_event_type ON notification_events(event_type);
CREATE INDEX idx_notif_events_status ON notification_events(status);
CREATE INDEX idx_notif_events_created_at ON notification_events(created_at);

-- ============================================================================
-- Updated_at Trigger
-- Automatically updates the updated_at column on row modification
-- ============================================================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Apply updated_at trigger to tables that have the column
CREATE TRIGGER trg_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_kubernetes_clusters_updated_at
    BEFORE UPDATE ON kubernetes_clusters
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_attack_templates_updated_at
    BEFORE UPDATE ON attack_templates
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER trg_experiments_updated_at
    BEFORE UPDATE ON experiments
    FOR EACH ROW EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Seed Data: Default System Roles
-- ============================================================================
INSERT INTO roles (name, description, permissions, is_system) VALUES
    ('admin', 'Full system administrator with all permissions', '["admin:all"]'::jsonb, true),
    ('security_engineer', 'Can create and execute experiments, manage templates and clusters', '["experiments:read","experiments:write","experiments:execute","experiments:delete","clusters:read","clusters:write","templates:read","templates:write","audit:read"]'::jsonb, true),
    ('analyst', 'Can view experiments, reports, and audit logs', '["experiments:read","clusters:read","templates:read","audit:read"]'::jsonb, true),
    ('viewer', 'Read-only access to experiments and reports', '["experiments:read","templates:read"]'::jsonb, true);

-- ============================================================================
-- Seed Data: Default Organization
-- ============================================================================
INSERT INTO organizations (name, slug, status, settings) VALUES
    ('Default Organization', 'default', 'active', '{"allow_self_signup": true}'::jsonb);
