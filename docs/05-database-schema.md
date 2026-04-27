# Database Schema Design

## Overview

This document outlines the database schema for Chaos-Sec, detailing all data models, relationships, constraints, and indexing strategies. The database serves as the persistent storage layer for the orchestration platform, tracking experiments, results, configurations, and audit logs.

## Database Technology

**Recommended:** PostgreSQL 15+

**Rationale:**
- ACID compliance for critical security data
- JSONB support for flexible experiment configurations
- Strong concurrency handling for multiple simultaneous experiments
- Built-in encryption capabilities for sensitive data
- Excellent support for time-series data (experiment results)

---

## Entity Relationship Diagram (Conceptual)

```
┌─────────────────┐       ┌──────────────────┐       ┌─────────────────┐
│     User        │       │   Organization   │       │   Role          │
├─────────────────┤       ├──────────────────┤       ├─────────────────┤
│ id (PK)         │◄──────│ id (PK)          │       │ id (PK)         │
│ email           │       │ name             │◄──────│ name            │
│ password_hash   │       │ created_at       │       │ permissions     │
│ name            │       │ status           │       │ created_at      │
│ organization_id │       └──────────────────┘       └─────────────────┘
│ role_id         │
│ created_at      │
└─────────────────┘
         │
         ▼
┌─────────────────┐       ┌──────────────────┐       ┌─────────────────┐
│  API Key        │       │   Experiment     │       │ Attack Template │
├─────────────────┤       ├──────────────────┤       ├─────────────────┤
│ id (PK)         │       │ id (PK)          │       │ id (PK)         │
│ user_id (FK)    │       │ name             │       │ name            │
│ key_hash        │       │ description      │       │ category        │
│ name            │       │ organization_id  │       │ severity        │
│ created_at      │       │ status           │       │ k8s_manifest    │
│ last_used_at    │       │ created_by (FK)  │       │ parameters      │
│ expires_at      │       │ created_at       │       │ mitigation      │
└─────────────────┘       │ updated_at       │       │ created_at      │
                          └──────────────────┘       └─────────────────┘
                                   │                          │
                                   │                          │
                          ┌────────┴──────────┐              │
                          ▼                   ▼              │
┌─────────────────┐  ┌──────────────────┐  ┌─────────────────┴────────┐
│  Kubernetes     │  │  Experiment Run  │  │   Experiment Template    │
│    Cluster      │  ├──────────────────┤  ├──────────────────────────┤
├─────────────────┤  │ id (PK)          │  │ id (PK)                  │
│ id (PK)         │  │ experiment_id(FK)│  │ experiment_id (FK)       │
│ organization_id │  │ run_number       │  │ attack_template_id (FK)  │
│ name            │  │ status           │  │ configuration            │
│ api_endpoint    │  │ started_at       │  │ target_namespaces        │
│ ca_certificate  │  │ completed_at     │  │ duration_seconds         │
│ client_cert     │  │ duration_ms      │  │ cleanup_policy           │
│ client_key      │  │ result_summary   │  │ siem_validation          │
│ created_at      │  └──────────────────┘  │ created_at               │
└─────────────────┘           │            └──────────────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
┌─────────────────┐  ┌──────────────────┐  ┌─────────────────┐
│  Attack Pod     │  │  SIEM Validation │  │  Test Result    │
├─────────────────┤  ├──────────────────┤  ├─────────────────┤
│ id (PK)         │  │ id (PK)          │  │ id (PK)         │
│ run_id (FK)     │  │ run_id (FK)      │  │ run_id (FK)     │
│ pod_name        │  │ expected_alert   │  │ check_name      │
│ namespace       │  │ alert_received   │  │ status          │
│ status          │  │ received_at      │  │ details         │
│ ip_address      │  │ siem_response    │  │ timestamp       │
│ created_at      │  │ matched          │  │ error_message   │
│ terminated_at   │  └──────────────────┘  └─────────────────┘
└─────────────────┘
         │
         ▼
┌─────────────────┐
│  Audit Log      │
├─────────────────┤
│ id (PK)         │
│ user_id (FK)    │
│ action          │
│ resource_type   │
│ resource_id     │
│ details         │
│ ip_address      │
│ timestamp       │
└─────────────────┘
```

---

## Table Definitions

### 1. organizations

Stores multi-tenant organization data for isolation.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `name` | VARCHAR(255) | NOT NULL, UNIQUE | Organization name |
| `slug` | VARCHAR(100) | NOT NULL, UNIQUE | URL-friendly identifier |
| `status` | VARCHAR(20) | NOT NULL, DEFAULT 'active' | active, suspended, deleted |
| `settings` | JSONB | DEFAULT '{}'::jsonb | Organization-level configurations |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes:**
- `idx_organizations_slug` ON organizations(slug)
- `idx_organizations_status` ON organizations(status)

---

### 2. users

Stores user account information and authentication credentials.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `email` | VARCHAR(255) | NOT NULL, UNIQUE | User email (login) |
| `password_hash` | VARCHAR(255) | NOT NULL | Bcrypt hashed password |
| `name` | VARCHAR(255) | NOT NULL | Display name |
| `organization_id` | UUID | NOT NULL, FK → organizations(id) | Parent organization |
| `role_id` | UUID | NOT NULL, FK → roles(id) | User role |
| `is_active` | BOOLEAN | NOT NULL, DEFAULT true | Account status |
| `last_login_at` | TIMESTAMPTZ | NULL | Last successful login |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes:**
- `idx_users_email` ON users(email)
- `idx_users_organization_id` ON users(organization_id)
- `idx_users_role_id` ON users(role_id)
- `idx_users_is_active` ON users(is_active)

---

### 3. roles

Defines role-based access control (RBAC) roles and permissions.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `name` | VARCHAR(100) | NOT NULL, UNIQUE | Role name |
| `description` | TEXT | NULL | Role description |
| `permissions` | JSONB | NOT NULL | Array of permission strings |
| `is_system` | BOOLEAN | NOT NULL, DEFAULT false | System roles cannot be deleted |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_roles_name` ON roles(name)

**Sample Permissions:**
```json
[
  "experiments:read",
  "experiments:write",
  "experiments:execute",
  "experiments:delete",
  "clusters:read",
  "clusters:write",
  "templates:read",
  "templates:write",
  "users:manage",
  "audit:read",
  "admin:all"
]
```

---

### 4. api_keys

Stores API keys for programmatic access to the platform.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `user_id` | UUID | NOT NULL, FK → users(id) | Key owner |
| `name` | VARCHAR(255) | NOT NULL | Human-readable key name |
| `key_hash` | VARCHAR(255) | NOT NULL, UNIQUE | SHA-256 hashed key |
| `key_prefix` | VARCHAR(8) | NOT NULL | First 8 chars for identification |
| `permissions` | JSONB | NULL | Override permissions (NULL = inherit) |
| `expires_at` | TIMESTAMPTZ | NULL | Expiration (NULL = never) |
| `last_used_at` | TIMESTAMPTZ | NULL | Last usage timestamp |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| `revoked_at` | TIMESTAMPTZ | NULL | Revocation timestamp |

**Indexes:**
- `idx_api_keys_key_hash` ON api_keys(key_hash)
- `idx_api_keys_user_id` ON api_keys(user_id)
- `idx_api_keys_expires_at` ON api_keys(expires_at)

---

### 5. kubernetes_clusters

Stores connection details for target Kubernetes clusters.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `organization_id` | UUID | NOT NULL, FK → organizations(id) | Owner organization |
| `name` | VARCHAR(255) | NOT NULL | Cluster display name |
| `description` | TEXT | NULL | Cluster description |
| `api_endpoint` | VARCHAR(500) | NOT NULL | Kubernetes API server URL |
| `ca_certificate` | TEXT | NOT NULL | CA cert (encrypted at rest) |
| `client_certificate` | TEXT | NOT NULL | Client cert (encrypted at rest) |
| `client_key` | TEXT | NOT NULL | Client key (encrypted at rest) |
| `default_namespace` | VARCHAR(255) | NOT NULL, DEFAULT 'chaos-sec' | Default experiment namespace |
| `status` | VARCHAR(20) | NOT NULL, DEFAULT 'pending' | pending, connected, error, disabled |
| `last_connected_at` | TIMESTAMPTZ | NULL | Last successful connection |
| `kubernetes_version` | VARCHAR(50) | NULL | Detected K8s version |
| `node_count` | INTEGER | NULL | Number of nodes |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes:**
- `idx_clusters_organization_id` ON kubernetes_clusters(organization_id)
- `idx_clusters_status` ON kubernetes_clusters(status)
- `idx_clusters_name` ON kubernetes_clusters(name)

---

### 6. attack_templates

Predefined attack patterns that can be executed in experiments.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `name` | VARCHAR(255) | NOT NULL | Template name |
| `slug` | VARCHAR(255) | NOT NULL, UNIQUE | URL-friendly identifier |
| `category` | VARCHAR(100) | NOT NULL | network, privilege, data, availability |
| `severity` | VARCHAR(20) | NOT NULL | low, medium, high, critical |
| `description` | TEXT | NOT NULL | Detailed description |
| `mitre_attack_id` | VARCHAR(50) | NULL | MITRE ATT&CK technique ID |
| `k8s_manifest` | JSONB | NOT NULL | Kubernetes manifest template |
| `parameters` | JSONB | NOT NULL | Configurable parameters schema |
| `prerequisites` | JSONB | DEFAULT '[]'::jsonb | Required conditions |
| `expected_behavior` | TEXT | NOT NULL | What should happen during attack |
| `mitigation` | TEXT | NULL | Recommended mitigations |
| `is_active` | BOOLEAN | NOT NULL, DEFAULT true | Template availability |
| `is_system` | BOOLEAN | NOT NULL, DEFAULT false | System templates cannot be deleted |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes:**
- `idx_templates_slug` ON attack_templates(slug)
- `idx_templates_category` ON attack_templates(category)
- `idx_templates_severity` ON attack_templates(severity)
- `idx_templates_mitre_attack_id` ON attack_templates(mitre_attack_id)
- `idx_templates_is_active` ON attack_templates(is_active)

**Sample Parameters Schema:**
```json
{
  "type": "object",
  "properties": {
    "target_namespace": {
      "type": "string",
      "description": "Namespace to target"
    },
    "duration_seconds": {
      "type": "integer",
      "minimum": 10,
      "maximum": 3600,
      "default": 300
    },
    "target_pod_label": {
      "type": "string",
      "description": "Label selector for target pods"
    }
  },
  "required": ["target_namespace", "duration_seconds"]
}
```

---

### 7. experiments

Defines reusable experiment configurations.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `organization_id` | UUID | NOT NULL, FK → organizations(id) | Owner organization |
| `name` | VARCHAR(255) | NOT NULL | Experiment name |
| `description` | TEXT | NULL | Experiment description |
| `status` | VARCHAR(20) | NOT NULL, DEFAULT 'draft' | draft, active, archived |
| `created_by` | UUID | NOT NULL, FK → users(id) | Creator |
| `schedule_cron` | VARCHAR(100) | NULL | Cron expression for scheduled runs |
| `auto_cleanup` | BOOLEAN | NOT NULL, DEFAULT true | Auto-cleanup after execution |
| `notification_config` | JSONB | DEFAULT '{}'::jsonb | Alert/notification settings |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |
| `updated_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Last update timestamp |

**Indexes:**
- `idx_experiments_organization_id` ON experiments(organization_id)
- `idx_experiments_status` ON experiments(status)
- `idx_experiments_created_by` ON experiments(created_by)

---

### 8. experiment_templates

Links experiments to attack templates with specific configurations.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `experiment_id` | UUID | NOT NULL, FK → experiments(id) | Parent experiment |
| `attack_template_id` | UUID | NOT NULL, FK → attack_templates(id) | Attack template |
| `order_index` | INTEGER | NOT NULL, DEFAULT 0 | Execution order |
| `configuration` | JSONB | NOT NULL | Template-specific configuration |
| `target_namespaces` | TEXT[] | NULL | Target namespaces (NULL = all) |
| `target_labels` | JSONB | NULL | Kubernetes label selectors |
| `duration_seconds` | INTEGER | NOT NULL, DEFAULT 300 | Attack duration |
| `cleanup_policy` | VARCHAR(50) | NOT NULL, DEFAULT 'immediate' | immediate, delayed, manual |
| `siem_validation` | JSONB | NULL | SIEM validation configuration |
| `enabled` | BOOLEAN | NOT NULL, DEFAULT true | Step enabled flag |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_exp_templates_experiment_id` ON experiment_templates(experiment_id)
- `idx_exp_templates_attack_template_id` ON experiment_templates(attack_template_id)
- `idx_exp_templates_order` ON experiment_templates(experiment_id, order_index)

---

### 9. experiment_runs

Tracks individual execution instances of experiments.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `experiment_id` | UUID | NOT NULL, FK → experiments(id) | Parent experiment |
| `cluster_id` | UUID | NOT NULL, FK → kubernetes_clusters(id) | Target cluster |
| `run_number` | INTEGER | NOT NULL | Sequential run number |
| `status` | VARCHAR(30) | NOT NULL, DEFAULT 'pending' | pending, running, completed, failed, cancelled |
| `triggered_by` | UUID | FK → users(id) | User who triggered (NULL = scheduled) |
| `trigger_type` | VARCHAR(30) | NOT NULL | manual, scheduled, api, webhook |
| `started_at` | TIMESTAMPTZ | NULL | Execution start time |
| `completed_at` | TIMESTAMPTZ | NULL | Execution end time |
| `duration_ms` | BIGINT | NULL | Total duration in milliseconds |
| `result_summary` | JSONB | NULL | Aggregated results |
| `error_message` | TEXT | NULL | Error details if failed |
| `cleanup_status` | VARCHAR(30) | DEFAULT 'pending' | pending, completed, failed, skipped |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_runs_experiment_id` ON experiment_runs(experiment_id)
- `idx_runs_cluster_id` ON experiment_runs(cluster_id)
- `idx_runs_status` ON experiment_runs(status)
- `idx_runs_started_at` ON experiment_runs(started_at)
- `idx_runs_created_at` ON experiment_runs(created_at)
- Composite: `idx_runs_exp_created` ON experiment_runs(experiment_id, created_at DESC)

---

### 10. attack_pods

Tracks individual attacker pods spawned during experiment runs.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `run_id` | UUID | NOT NULL, FK → experiment_runs(id) | Parent run |
| `template_id` | UUID | NOT NULL, FK → attack_templates(id) | Attack template used |
| `pod_name` | VARCHAR(255) | NOT NULL | Kubernetes pod name |
| `namespace` | VARCHAR(255) | NOT NULL | Kubernetes namespace |
| `node_name` | VARCHAR(255) | NULL | Scheduled node |
| `ip_address` | VARCHAR(45) | NULL | Pod IP address |
| `status` | VARCHAR(30) | NOT NULL, DEFAULT 'pending' | pending, creating, running, completed, failed, terminated |
| `phase` | VARCHAR(30) | NULL | Kubernetes pod phase |
| `started_at` | TIMESTAMPTZ | NULL | Pod start time |
| `terminated_at` | TIMESTAMPTZ | NULL | Pod termination time |
| `exit_code` | INTEGER | NULL | Container exit code |
| `logs_summary` | TEXT | NULL | Truncated pod logs |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_pods_run_id` ON attack_pods(run_id)
- `idx_pods_template_id` ON attack_pods(template_id)
- `idx_pods_status` ON attack_pods(status)
- `idx_pods_namespace` ON attack_pods(namespace)

---

### 11. siem_validations

Tracks SIEM alert validation for each experiment run.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `run_id` | UUID | NOT NULL, FK → experiment_runs(id) | Parent run |
| `attack_pod_id` | UUID | FK → attack_pods(id) | Related attack pod |
| `expected_alert_type` | VARCHAR(255) | NOT NULL | Expected SIEM alert type |
| `expected_alert_severity` | VARCHAR(20) | NULL | Expected severity level |
| `alert_received` | BOOLEAN | NOT NULL, DEFAULT false | Whether alert was received |
| `received_at` | TIMESTAMPTZ | NULL | When alert was received |
| `siem_response` | JSONB | NULL | Full SIEM response data |
| `alert_id` | VARCHAR(255) | NULL | SIEM alert identifier |
| `matched` | BOOLEAN | NULL | Whether alert matched expectations |
| `match_details` | JSONB | NULL | Details of matching process |
| `validation_status` | VARCHAR(30) | NOT NULL, DEFAULT 'pending' | pending, validated, failed, timeout |
| `checked_at` | TIMESTAMPTZ | NULL | Last validation check time |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_siem_run_id` ON siem_validations(run_id)
- `idx_siem_attack_pod_id` ON siem_validations(attack_pod_id)
- `idx_siem_alert_received` ON siem_validations(alert_received)
- `idx_siem_validation_status` ON siem_validations(validation_status)

---

### 12. test_results

Stores individual test/check results within an experiment run.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `run_id` | UUID | NOT NULL, FK → experiment_runs(id) | Parent run |
| `attack_pod_id` | UUID | FK → attack_pods(id) | Related attack pod |
| `check_name` | VARCHAR(255) | NOT NULL | Name of the check |
| `check_type` | VARCHAR(100) | NOT NULL | network, privilege, detection, remediation |
| `status` | VARCHAR(30) | NOT NULL | passed, failed, skipped, error |
| `expected` | TEXT | NULL | Expected outcome |
| `actual` | TEXT | NULL | Actual outcome |
| `details` | JSONB | NULL | Additional result details |
| `error_message` | TEXT | NULL | Error details if failed |
| `timestamp` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Result timestamp |

**Indexes:**
- `idx_results_run_id` ON test_results(run_id)
- `idx_results_attack_pod_id` ON test_results(attack_pod_id)
- `idx_results_status` ON test_results(status)
- `idx_results_check_type` ON test_results(check_type)
- `idx_results_timestamp` ON test_results(timestamp)

---

### 13. audit_logs

Immutable log of all user actions and system events.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `organization_id` | UUID | NOT NULL, FK → organizations(id) | Organization context |
| `user_id` | UUID | FK → users(id) | Acting user (NULL = system) |
| `api_key_id` | UUID | FK → api_keys(id) | API key used (NULL = not API) |
| `action` | VARCHAR(100) | NOT NULL | Action performed |
| `resource_type` | VARCHAR(100) | NOT NULL | Type of resource affected |
| `resource_id` | UUID | NULL | Specific resource ID |
| `resource_name` | VARCHAR(255) | NULL | Resource name for readability |
| `details` | JSONB | NULL | Action details and changes |
| `ip_address` | VARCHAR(45) | NULL | Client IP address |
| `user_agent` | VARCHAR(500) | NULL | Client user agent |
| `status` | VARCHAR(30) | NOT NULL | success, failure, partial |
| `timestamp` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Event timestamp |

**Indexes:**
- `idx_audit_organization_id` ON audit_logs(organization_id)
- `idx_audit_user_id` ON audit_logs(user_id)
- `idx_audit_action` ON audit_logs(action)
- `idx_audit_resource_type` ON audit_logs(resource_type)
- `idx_audit_timestamp` ON audit_logs(timestamp)
- `idx_audit_status` ON audit_logs(status)

**Note:** This table should be append-only. Consider using database triggers to prevent UPDATE/DELETE operations.

---

### 14. notifications

Stores notification configurations and delivery history.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `organization_id` | UUID | NOT NULL, FK → organizations(id) | Owner organization |
| `name` | VARCHAR(255) | NOT NULL | Notification channel name |
| `type` | VARCHAR(50) | NOT NULL | email, slack, webhook, pagerduty |
| `configuration` | JSONB | NOT NULL | Channel-specific config |
| `is_active` | BOOLEAN | NOT NULL, DEFAULT true | Channel status |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_notifications_organization_id` ON notifications(organization_id)
- `idx_notifications_type` ON notifications(type)
- `idx_notifications_is_active` ON notifications(is_active)

---

### 15. notification_events

Tracks notification delivery attempts and results.

| Column | Type | Constraints | Description |
|--------|------|-------------|-------------|
| `id` | UUID | PRIMARY KEY, DEFAULT gen_random_uuid() | Unique identifier |
| `notification_id` | UUID | NOT NULL, FK → notifications(id) | Target notification channel |
| `event_type` | VARCHAR(100) | NOT NULL | Type of event triggering notification |
| `related_run_id` | UUID | FK → experiment_runs(id) | Related experiment run |
| `subject` | VARCHAR(500) | NULL | Notification subject |
| `content` | TEXT | NULL | Notification body |
| `status` | VARCHAR(30) | NOT NULL | pending, sent, delivered, failed |
| `sent_at` | TIMESTAMPTZ | NULL | When notification was sent |
| `delivered_at` | TIMESTAMPTZ | NULL | When notification was delivered |
| `error_message` | TEXT | NULL | Error details if failed |
| `retry_count` | INTEGER | NOT NULL, DEFAULT 0 | Number of retry attempts |
| `created_at` | TIMESTAMPTZ | NOT NULL, DEFAULT NOW() | Creation timestamp |

**Indexes:**
- `idx_notif_events_notification_id` ON notification_events(notification_id)
- `idx_notif_events_event_type` ON notification_events(event_type)
- `idx_notif_events_status` ON notification_events(status)
- `idx_notif_events_created_at` ON notification_events(created_at)

---

## Relationships Summary

| Relationship | Type | Description |
|--------------|------|-------------|
| organizations → users | 1:N | One organization has many users |
| organizations → clusters | 1:N | One organization has many clusters |
| organizations → experiments | 1:N | One organization has many experiments |
| users → api_keys | 1:N | One user has many API keys |
| users → experiments | 1:N | One user creates many experiments |
| roles → users | 1:N | One role assigned to many users |
| experiments → experiment_templates | 1:N | One experiment has many template steps |
| experiments → experiment_runs | 1:N | One experiment has many runs |
| experiment_runs → attack_pods | 1:N | One run spawns many attack pods |
| experiment_runs → siem_validations | 1:N | One run has many SIEM validations |
| experiment_runs → test_results | 1:N | One run produces many test results |
| attack_templates → experiment_templates | 1:N | One template used in many experiment configs |
| attack_pods → siem_validations | 1:1 | One pod has one SIEM validation |
| attack_pods → test_results | 1:N | One pod produces many test results |
| notifications → notification_events | 1:N | One channel sends many events |

---

## Data Retention Policies

### Automatic Cleanup Rules

| Table | Retention Period | Cleanup Strategy |
|-------|------------------|------------------|
| experiment_runs | 90 days | Archive summary, delete details |
| attack_pods | 90 days | Delete with parent run |
| test_results | 90 days | Aggregate into run summary, delete |
| siem_validations | 90 days | Delete with parent run |
| notification_events | 30 days | Delete old records |
| audit_logs | 7 years | Keep for compliance |

### Partitioning Strategy

For high-volume tables, consider time-based partitioning:

```sql
-- Example: Partition experiment_runs by month
CREATE TABLE experiment_runs (
    -- columns...
) PARTITION BY RANGE (created_at);

CREATE TABLE experiment_runs_y2024m01 
    PARTITION OF experiment_runs
    FOR VALUES FROM ('2024-01-01') TO ('2024-02-01');
```

---

## Security Considerations

### Encryption at Rest

1. **Sensitive Fields** (encrypt using application-layer encryption before storage):
   - `kubernetes_clusters.client_key`
   - `kubernetes_clusters.client_certificate`
   - `users.password_hash` (already hashed, but consider additional encryption)
   - `api_keys.key_hash`

2. **Database-Level Encryption:**
   - Enable Transparent Data Encryption (TDE)
   - Use pgcrypto extension for field-level encryption

### Access Control

1. **Row-Level Security (RLS):**
   - Implement RLS policies to ensure organizations can only access their own data
   - Example policy:
   ```sql
   CREATE POLICY organization_isolation ON experiments
       USING (organization_id = current_setting('app.current_organization_id')::uuid);
   ```

2. **Audit Logging:**
   - All data access should be logged in audit_logs
   - Consider database triggers for automatic audit logging

### Data Integrity

1. **Soft Deletes:**
   - Use `deleted_at` timestamp instead of hard deletes for critical tables
   - Implement unique indexes that include NULL values for soft-deleted records

2. **Immutable Records:**
   - `audit_logs` should never be updated or deleted
   - `experiment_runs` should not be modified after completion

---

## Migration Strategy

### Version Control

All schema changes should be version-controlled using a migration tool:

**Recommended Tools:**
- golang-migrate
- Flyway
- Liquibase

### Migration File Naming Convention

```
V001__initial_schema.sql
V002__add_notification_tables.sql
V003__add_siem_validation_fields.sql
```

### Rollback Strategy

- Each migration should have a corresponding rollback script
- Test rollbacks in staging before production deployment
- Maintain backup before major schema changes

---

## Performance Optimization

### Indexing Strategy

1. **Foreign Keys:** Always index foreign key columns
2. **Timestamps:** Index columns used for time-range queries
3. **Status Columns:** Index columns used for filtering by status
4. **Composite Indexes:** Create for common query patterns

### Query Optimization

1. Use EXPLAIN ANALYZE for slow queries
2. Consider materialized views for dashboard aggregations
3. Implement connection pooling (PgBouncer recommended)

### Caching Layer

Consider Redis for:
- Session storage
- API rate limiting
- Frequently accessed configuration data
- Real-time experiment status updates

---

## Backup and Recovery

### Backup Strategy

1. **Full Backups:** Daily
2. **WAL Archiving:** Continuous for point-in-time recovery
3. **Off-site Replicas:** Maintain in different geographic region

### Recovery Time Objective (RTO)

- Target: < 1 hour for full recovery
- Test recovery procedures quarterly

### Backup Verification

- Automated backup integrity checks
- Monthly restore tests to staging environment

---

## Appendix: Sample Data

### Sample Attack Template

```json
{
  "name": "Pod Egress Test",
  "slug": "pod-egress-test",
  "category": "network",
  "severity": "medium",
  "description": "Tests whether pod egress restrictions are properly enforced",
  "mitre_attack_id": "T1041",
  "k8s_manifest": {
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
      "name": "egress-test-{{.RunID}}",
      "namespace": "{{.Namespace}}"
    },
    "spec": {
      "containers": [{
        "name": "tester",
        "image": "curlimages/curl:latest",
        "command": ["sh", "-c", "curl -v {{.TargetURL}}"]
      }]
    }
  },
  "parameters": {
    "target_namespace": "default",
    "target_url": "http://external-service",
    "duration_seconds": 300
  }
}
```

### Sample Experiment Run Result Summary

```json
{
  "total_pods_spawned": 3,
  "successful_attacks": 2,
  "blocked_attacks": 1,
  "siem_alerts_expected": 3,
  "siem_alerts_received": 2,
  "detection_rate": 0.67,
  "overall_status": "partial_success",
  "findings": [
    {
      "severity": "high",
      "description": "Egress traffic was not blocked as expected",
      "recommendation": "Review NetworkPolicy configuration"
    }
  ]
}
```

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | Initial | Project Team | Initial schema design |