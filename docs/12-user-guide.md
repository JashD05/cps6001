# Chaos-Sec User Guide

**Version:** 1.0.0
**Last Updated:** 2026-04-21

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Getting Started](#2-getting-started)
3. [Dashboard](#3-dashboard)
4. [Experiments](#4-experiments)
5. [Attack Templates](#5-attack-templates)
6. [Clusters](#6-clusters)
7. [SIEM Integration](#7-siem-integration)
8. [Reports](#8-reports)
9. [User Profile & Settings](#9-user-profile--settings)
10. [Keyboard Shortcuts](#10-keyboard-shortcuts)
11. [FAQ](#11-faq)

---

## 1. Introduction

### 1.1 What is Chaos-Sec?

Chaos-Sec is an orchestration platform that validates your security controls through controlled chaos engineering experiments. It simulates cyberattacks against your Kubernetes clusters and verifies that your firewalls, network policies, RBAC rules, and SIEM detection systems work as intended.

### 1.2 Who Should Use This Guide

This guide is for **end users** of Chaos-Sec — security engineers, DevOps operators, and compliance analysts who run experiments and interpret results. If you are installing or configuring the platform, see the [Administrator Guide](./12-administrator-guide.md).

### 1.3 Key Concepts

| Concept | Description |
|---------|-------------|
| **Experiment** | A security validation test that simulates an attack against a target cluster |
| **Template** | A reusable definition of an attack scenario with configurable parameters |
| **Run** | A single execution of an experiment, producing results and findings |
| **Finding** | A security issue discovered during an experiment run |
| **Cluster** | A registered Kubernetes cluster that experiments target |
| **SIEM Validation** | Verification that your SIEM system detected the simulated attack |

### 1.4 User Roles

Chaos-Sec uses role-based access control with three roles:

| Role | Permissions |
|------|-------------|
| **Admin** | Full access: manage users, all experiments, clusters, templates, and settings |
| **Operator** | Create and run experiments, manage clusters and templates, view reports |
| **Viewer** | Read-only access: view experiments, results, and reports |

### 1.5 Browser Requirements

| Browser | Minimum Version |
|---------|----------------|
| Chrome | 90+ |
| Firefox | 88+ |
| Safari | 14+ |
| Edge | 90+ |

---

## 2. Getting Started

### 2.1 Logging In

1. Open your browser and navigate to the Chaos-Sec dashboard URL (e.g., `https://app.chaos-sec.io`).
2. Enter your email address and password.
3. Click **Sign In**.

> **Note:** If your session expires, you will be redirected to the login page with a warning. Re-enter your credentials to continue.

### 2.2 First-Time Login

If this is your first time logging in, you may be prompted to change your password. Contact your administrator if you need initial credentials.

### 2.3 Navigating the Dashboard

The left sidebar contains the main navigation menu:

| Menu Item | Description |
|-----------|-------------|
| **Dashboard** | Overview of security posture, recent experiments, and cluster health |
| **Experiments** | List, create, and manage experiments |
| **Templates** | Browse and manage attack templates |
| **Clusters** | View and manage registered Kubernetes clusters |
| **Reports** | Generate and download experiment reports |
| **Settings** | Configure notifications, SIEM, and preferences |

---

## 3. Dashboard

### 3.1 Overview

The dashboard provides a high-level view of your security posture and recent activity.

#### Security Posture Score

A numerical score (0–100) representing your overall security control effectiveness, based on experiment results. The score trend arrow indicates whether your posture is improving, declining, or stable.

#### Experiment Summary

| Metric | Description |
|--------|-------------|
| Total | All experiments regardless of status |
| Running | Currently executing experiments |
| Completed | Experiments that finished successfully |
| Failed | Experiments that encountered errors |
| Pending | Experiments waiting to be executed |

#### Recent Experiments

A list of the 5 most recent experiments with their status, score, and timestamp. Click any experiment to view its details.

#### Cluster Health

Visual indicators for each registered cluster:

| Status | Meaning |
|--------|---------|
| 🟢 Healthy | Cluster is reachable, all nodes ready |
| 🟡 Degraded | Cluster is reachable but some nodes are not ready |
| 🔴 Unreachable | Cannot connect to the cluster API |
| ⚪ Unknown | Health check has not been performed |

### 3.2 Quick Actions

The dashboard includes quick-action buttons for common tasks:

- **New Experiment** — Jump to experiment creation
- **View Reports** — Open the reports section
- **Register Cluster** — Add a new Kubernetes cluster

---

## 4. Experiments

### 4.1 Understanding Experiments

An experiment is a security validation test that:

1. Spawns an attacker pod in the target Kubernetes cluster
2. Executes a simulated attack based on the selected template
3. Observes whether security controls block the attack
4. Queries the SIEM to verify the attack was detected
5. Produces a result with findings and recommendations

### 4.2 Experiment Lifecycle

```
Pending → Queued → Running → Completed
                   └──→ Failed
                   └──→ Timed Out
                   └──→ Stopped (manual)
```

| Status | Description |
|--------|-------------|
| **Pending** | Experiment is defined but not yet queued |
| **Queued** | Experiment is in the execution queue, waiting for resources |
| **Running** | Attacker pod is active and executing the attack |
| **Completed** | Experiment finished, results available |
| **Failed** | Experiment encountered an error during execution |
| **Timed Out** | Experiment exceeded the configured time limit |
| **Stopped** | Experiment was manually stopped by a user |

### 4.3 Creating an Experiment

1. Navigate to **Experiments** in the sidebar.
2. Click **New Experiment**.
3. Fill in the experiment details:

| Field | Required | Description |
|-------|----------|-------------|
| Name | Yes | A descriptive name for the experiment |
| Description | No | Additional context about the experiment's purpose |
| Template | Yes | The attack template to use |
| Cluster | Yes | The target Kubernetes cluster |
| Namespace | Yes | The target namespace within the cluster |
| Parameters | Varies | Template-specific parameters (see Section 5) |
| Tags | No | Labels for categorizing experiments |
| Schedule | No | Cron expression for recurring experiments |

4. Configure SIEM validation settings:

| Field | Description |
|-------|-------------|
| SIEM Alert Type | The type of alert expected from the SIEM |
| Time Window (seconds) | How long to wait for SIEM alert after the attack |
| Expected Alert Count | Minimum number of alerts expected |

5. Click **Create Experiment**.

### 4.4 Running an Experiment

1. Navigate to the experiment detail page.
2. Click **Execute** to start the experiment immediately.

The experiment will progress through the lifecycle stages. You can monitor its progress in real time on the experiment detail page.

### 4.5 Monitoring a Running Experiment

The experiment detail page shows:

- **Status indicator** — Current state of the experiment
- **Progress bar** — Visual representation of completion percentage
- **Steps** — Individual attack phases with their status (pending, in progress, completed, failed, skipped)
- **Live logs** — Real-time log output from the attacker pod
- **Pod statuses** — Status of Kubernetes pods involved in the experiment

### 4.6 Stopping an Experiment

To stop a running experiment:

1. Navigate to the experiment detail page.
2. Click **Stop**.
3. Confirm the action in the dialog.

> **Warning:** Stopping an experiment mid-execution may produce incomplete results. The attacker pod will be terminated, and partial findings will be recorded.

### 4.7 Viewing Experiment Results

After an experiment completes, the results page shows:

#### Summary Tab

| Metric | Description |
|--------|-------------|
| Total Pods Spawned | Number of attacker pods created |
| Successful Attacks | Attacks that reached their target |
| Blocked Attacks | Attacks stopped by security controls |
| Detection Rate | Percentage of attacks detected by the SIEM |
| Overall Status | `passed`, `partial`, or `failed` |

#### Runs Tab

A table of all execution runs for the experiment, including run number, status, trigger type, start time, and duration.

#### Results Tab

Detailed breakdown of each attack phase with:
- Pod status information
- SIEM validation results per phase
- Detection latency measurements

#### Findings Tab

Each finding includes:

| Field | Description |
|-------|-------------|
| Severity | `critical`, `high`, `medium`, `low`, or `info` |
| Category | The type of finding (e.g., network policy gap, RBAC misconfiguration) |
| Description | Human-readable explanation of the issue |
| Recommendation | Suggested remediation action |

### 4.8 Filtering Experiments

Use the filter bar on the experiments list page to narrow results:

| Filter | Options |
|--------|---------|
| Search | Free-text search on experiment name |
| Status | Any experiment status |
| Template | Filter by attack template |
| Cluster | Filter by target cluster |
| Date Range | From/To date pickers |

### 4.9 Scheduling Recurring Experiments

When creating or editing an experiment, you can set a schedule using a cron expression:

| Schedule | Cron Expression |
|----------|----------------|
| Every hour | `0 * * * *` |
| Daily at midnight | `0 0 * * *` |
| Weekly on Monday | `0 0 * * 1` |
| Monthly on the 1st | `0 0 1 * *` |

---

## 5. Attack Templates

### 5.1 Understanding Templates

Templates define the attack scenarios that experiments execute. Each template specifies:

- **Attack phases** — Ordered steps the attacker pod performs
- **Parameters** — Configurable values (target URL, duration, thresholds, etc.)
- **Expected detections** — SIEM alerts the experiment should trigger
- **Severity** — The risk level of the simulated attack

### 5.1 Template Categories

| Category | Description | Example Attacks |
|----------|-------------|-----------------|
| **Network** | Network policy and firewall validation | Egress traffic, ingress access, port scanning |
| **Application** | Application-layer security testing | SQL injection, API abuse |
| **Infrastructure** | Infrastructure security controls | Privilege escalation, container breakout |
| **Data** | Data protection validation | Secret access, data exfiltration |
| **Identity** | Identity and access management | RBAC escalation, service account abuse |

### 5.2 Template Severity Levels

| Severity | Color | Meaning |
|----------|-------|---------|
| Critical | Red | Exploitable vulnerability with high impact |
| High | Orange | Significant security weakness |
| Medium | Yellow | Moderate security concern |
| Low | Blue | Minor security observation |
| Info | Grey | Informational, no direct security impact |

### 5.3 Viewing Templates

1. Navigate to **Templates** in the sidebar.
2. Browse available templates. Use the category filter or search bar to narrow results.
3. Click a template to view its details, including:
   - Description and attack phases
   - Required parameters with types, defaults, and validation rules
   - Expected SIEM detections
   - Usage count (how many experiments use this template)

### 5.4 Template Parameters

Parameters are specific to each template. Common parameter types:

| Type | Description | Example |
|------|-------------|---------|
| `string` | Free-text input | Target namespace, pod name |
| `number` | Numeric input with optional min/max | Duration in seconds, port number |
| `boolean` | True/false toggle | Enable TLS verification |
| `select` | Choose from predefined options | Protocol (TCP/UDP/ICMP) |
| `multi-select` | Choose multiple options | Attack vectors to include |

### 5.5 Creating Custom Templates

> **Requires:** `templates:write` permission (Operator or Admin role)

1. Navigate to **Templates** → **Create Template**.
2. Define the template:

| Field | Required | Description |
|-------|----------|-------------|
| Name | Yes | Unique template name |
| Description | Yes | What the template tests |
| Category | Yes | Network, Application, Infrastructure, Data, Identity, or Custom |
| Severity | Yes | Critical, High, Medium, Low, or Info |
| Parameters | Yes | At least one configurable parameter |
| Attack Phases | Yes | At least one phase definition |
| Expected Detections | No | SIEM alerts expected during execution |

3. Click **Save Template**.

### 5.6 Attack Template API

Templates can also be managed via the API:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/attack-templates` | List all attack templates |
| `POST` | `/api/v1/attack-templates` | Create a new template |
| `GET` | `/api/v1/attack-templates/:id` | Get template details |
| `PUT` | `/api/v1/attack-templates/:id` | Update a template |
| `DELETE` | `/api/v1/attack-templates/:id` | Delete a template (Admin only) |

---

## 6. Clusters

### 6.1 Understanding Clusters

A **cluster** in Chaos-Sec represents a target Kubernetes environment where experiments run. Before you can execute any experiment, at least one cluster must be registered.

### 6.2 Viewing Clusters

1. Navigate to **Clusters** in the sidebar.
2. The list shows each cluster's name, status, provider, Kubernetes version, and node count.

### 6.3 Cluster Details

Click a cluster to view detailed information:

- **Status** — Current health (healthy, degraded, unreachable)
- **Kubernetes Version** — Cluster API server version
- **Nodes** — Total and ready node counts
- **Namespaces** — List of available namespaces
- **Network Policies** — Policies defined in the cluster namespace
- **Resource Usage** — CPU and memory utilization
- **Last Health Check** — Timestamp of the most recent health check

### 6.4 Cluster Health Indicators

| Metric | Description |
|--------|-------------|
| CPU Usage | Aggregate CPU utilization across nodes |
| Memory Usage | Aggregate memory utilization across nodes |
| Pod Count | Number of running pods |
| Node Count | Total nodes vs. ready nodes |
| Error Rate | Percentage of failed API requests |

### 6.5 Cluster API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/clusters` | List registered clusters |
| `POST` | `/api/v1/clusters` | Register a new cluster |
| `GET` | `/api/v1/clusters/:id` | Get cluster details with live info |
| `DELETE` | `/api/v1/clusters/:id` | Remove a cluster |
| `GET` | `/api/v1/clusters/:id/namespaces` | List namespaces |
| `GET` | `/api/v1/clusters/:id/network-policies` | List network policies |
| `GET` | `/api/v1/clusters/:id/health` | Get cluster health |

> **Note:** Cluster registration and deletion require the `clusters:write` permission.

---

## 7. SIEM Integration

### 7.1 Understanding SIEM Validation

Chaos-Sec performs **closed-loop validation** — after simulating an attack, it queries your SIEM system to confirm that security alerts were generated. This proves that your detection pipeline works end-to-end.

### 7.2 How SIEM Validation Works

1. The experiment executes the simulated attack.
2. Chaos-Sec waits for the configured propagation delay (typically 30 seconds).
3. It queries the SIEM for alerts matching the expected type, severity, and time window.
4. Each expected alert is correlated against received alerts.
5. A detection score is calculated: matched alerts / total expected alerts × 100.

### 7.3 Validation Status

| Status | Meaning |
|--------|---------|
| **Passed** | All expected alerts were detected (100% match) |
| **Partial** | Some expected alerts were detected, others were missed |
| **Failed** | No expected alerts were detected |

### 7.4 Checking SIEM Status

1. Navigate to **Settings** → **SIEM**.
2. The status page shows:
   - Whether the SIEM is connected
   - The provider name (e.g., Splunk, Elastic, Sentinel)
   - Connection endpoint
   - Current health status
   - Latency

### 7.5 Testing SIEM Connection

1. Navigate to **Settings** → **SIEM**.
2. Click **Test Connection**.
3. Chaos-Sec will attempt to connect to your SIEM and perform a health check.
4. The result shows success/failure, latency, and any error details.

### 7.6 Querying SIEM Alerts

Use the SIEM query API to search for alerts:

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/api/v1/siem/alerts/query` | Custom alert query with time range and filters |
| `GET` | `/api/v1/siem/alerts/:run_id` | Alerts for a specific experiment run |

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `from` | RFC3339 timestamp | Start of time range |
| `to` | RFC3339 timestamp | End of time range |
| `alert_type` | string | Filter by alert type |
| `severity` | string | Filter by severity (low, medium, high, critical) |
| `source` | string | Filter by alert source |
| `experiment_id` | UUID | Filter by experiment |
| `run_id` | UUID | Filter by experiment run |
| `offset` | integer | Pagination offset |
| `limit` | integer | Pagination limit (default 100, max 1000) |

### 7.7 Mock SIEM (Development)

For local development and testing, Chaos-Sec includes a mock SIEM service. It:

- Accepts alerts via `POST /api/alerts`
- Serves alerts via `GET /api/alerts` with filtering
- Reports healthy status at `/health`

The mock SIEM is started automatically with `docker compose up -d` and listens on port `9100`.

---

## 8. Reports

### 8.1 Report Types

| Type | Description |
|------|-------------|
| **Experiment Report** | Detailed results for a single experiment |
| **Compliance Report** | Aggregated results mapped to compliance frameworks |
| **Executive Report** | High-level summary for leadership |
| **Trend Report** | Security posture changes over time |

### 8.2 Generating a Report

1. Navigate to an experiment's detail page.
2. Click **Generate Report**.
3. Select the report type and format:

| Format | Description |
|--------|-------------|
| **PDF** | Formatted document suitable for printing and sharing |
| **CSV** | Tabular data for spreadsheet import |
| **JSON** | Machine-readable structured data |
| **HTML** | Web-viewable report |

4. Click **Generate**. The report will be prepared and a download link will appear.

### 8.3 Report Contents

A typical experiment report includes:

#### Experiment Summary
- Experiment name, ID, and description
- Status, creation date, and duration
- Template and cluster information

#### Runs Table
- Run number, status, trigger type, start time, and duration for each execution

#### Results Summary
- Total pods spawned
- Successful and blocked attacks
- Detection rate percentage
- Overall status (passed, partial, failed)

#### Findings
Each finding includes:
- **Severity** (critical, high, medium, low, info)
- **Description** of the security issue
- **Recommendation** for remediation

#### SIEM Validation
- Expected vs. received alert counts
- Detection latency
- Alert coverage percentage

### 8.4 Accessing Reports via API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/reports/:experimentId` | Get experiment report |

---

## 9. User Profile & Settings

### 9.1 Viewing Your Profile

1. Click your avatar/name in the top-right corner.
2. Select **Profile**.

Your profile shows:
- Email address
- Name
- Role and organization
- Last login time

### 9.2 Updating Your Profile

1. Navigate to **Profile**.
2. Edit your name or avatar URL.
3. Click **Save**.

### 9.3 Changing Your Password

1. Navigate to **Profile** → **Security**.
2. Enter your current password and new password.
3. Passwords must be at least 8 characters long.
4. Click **Change Password**.

### 9.4 Notification Settings

Configure how you receive experiment notifications:

| Channel | Settings |
|---------|----------|
| **Email** | SMTP host, port, username. Receive notifications on experiment completion, failure, and SIEM alerts |
| **Slack** | Webhook URL, channel, username. Real-time alerts to your Slack channel |
| **Webhook** | Generic webhook URL for custom integrations |

Toggle notifications per event type:

| Event | Description |
|-------|-------------|
| Experiment completed | An experiment finished successfully |
| Experiment failed | An experiment encountered an error |
| Cluster degraded | A cluster's health status changed |
| SIEM alert missed | An expected SIEM alert was not detected |

### 9.5 General Settings

| Setting | Description |
|---------|-------------|
| Theme | Light, Dark, or System |
| Auto-refresh interval | How often the dashboard updates (5s, 10s, 30s, 60s, off) |
| Default cluster | Pre-selected cluster when creating experiments |
| Default namespace | Pre-selected namespace when creating experiments |

---

## 10. Keyboard Shortcuts

| Shortcut | Action |
|----------|--------|
| `Ctrl+K` / `Cmd+K` | Open search/command palette |
| `Ctrl+N` / `Cmd+N` | New experiment |
| `Ctrl+/` | Toggle keyboard shortcut help |
| `Esc` | Close dialog or panel |
| `?` | Show keyboard shortcuts |

---

## 11. FAQ

### General

**Q: What happens when an experiment runs?**
A: Chaos-Sec creates a temporary attacker pod in your Kubernetes cluster. The pod executes the attack steps defined by the template, then Chaos-Sec checks whether your security controls blocked the attack and whether your SIEM detected it. The attacker pod is removed after the experiment completes.

**Q: Are experiments safe to run in production?**
A: Chaos-Sec is designed for controlled, isolated testing. However, you should always review template parameters and start with non-critical namespaces before testing production workloads. Use the `viewer` role for production dashboards and limit `operator` access.

**Q: How long do experiments take?**
A: Most experiments complete in 1–10 minutes, depending on the attack complexity and propagation delay settings. SIEM validation adds a configurable wait period (default 30 seconds) to allow alerts to propagate.

### Technical

**Q: What Kubernetes permissions does Chaos-Sec need?**
A: The platform's service account needs permissions to create, list, and delete pods, read namespaces and network policies, and view node information. See the Administrator Guide for the exact RBAC manifest.

**Q: Can I run multiple experiments simultaneously?**
A: Yes. Chaos-Sec supports concurrent experiment execution, limited by the `CHAOS_K8S_MAX_CONCURRENT` configuration (default: 10). Each experiment runs in its own attacker pod.

**Q: How are experiment results stored?**
A: Results are stored in PostgreSQL and cached in Redis. You can generate PDF/CSV/JSON/HTML reports at any time.

**Q: What SIEM systems are supported?**
A: Chaos-Sec supports any SIEM with a REST API. Built-in connectors are available for Splunk, Elastic, and Microsoft Sentinel. A mock SIEM is provided for development.

### Troubleshooting

**Q: My experiment is stuck in "Pending" status.**
A: Check that the target cluster is healthy and reachable. Verify that the namespace exists and the configured service account has the required permissions.

**Q: SIEM validation shows 0% detection rate.**
A: Verify the SIEM connection is healthy (Settings → SIEM → Test Connection). Check that the expected alert type and severity match what your SIEM produces. The propagation delay may be too short — try increasing the time window.

**Q: I can't see certain menus.**
A: Your role determines which features are accessible. Contact your administrator to adjust your permissions.

**Q: The dashboard shows stale data.**
A: Click the refresh button or check your auto-refresh interval in Settings. You can also force a refresh with `Ctrl+Shift+R`.

---

## Appendix

### A. Permission Reference

| Permission | Description |
|------------|-------------|
| `admin:all` | Full administrative access (bypasses all RBAC checks) |
| `experiments:read` | View experiments and results |
| `experiments:write` | Create and edit experiments |
| `experiments:execute` | Run and stop experiments |
| `experiments:delete` | Delete experiments |
| `templates:read` | View attack templates |
| `templates:write` | Create, edit, and delete templates |
| `clusters:read` | View cluster information |
| `clusters:write` | Register and manage clusters |
| `users:manage` | Create and manage user accounts |

### B. Rate Limits

| Context | Limit | Window |
|---------|-------|--------|
| Authenticated users | 100 requests | 60 seconds |
| Unauthenticated (by IP) | 100 requests | 60 seconds |

Rate limits are configurable by the administrator. When exceeded, the API returns `429 Too Many Requests` with the response body:

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests. Please slow down.",
  "code": 429
}
```

### C. API Authentication

All API requests (except login and refresh) require a valid JWT access token in the `Authorization` header:

```
Authorization: Bearer <access_token>
```

Access tokens expire after 1 hour. Use the refresh token to obtain a new access token:

```bash
curl -X POST https://app.chaos-sec.io/api/v1/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "<your_refresh_token>"}'
```

---

**Document Version:** 1.0.0
**Last Updated:** 2026-04-21