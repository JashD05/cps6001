# API Design Document

## Chaos-Sec: An Orchestration Platform for Security Control Validation

**Version:** 1.0.0  
**Last Updated:** 2026-01-15  
**Status:** Draft

---

## Table of Contents

1. [Overview](#overview)
2. [API Architecture](#api-architecture)
3. [Authentication & Authorization](#authentication--authorization)
4. [Base URL & Versioning](#base-url--versioning)
5. [Common Response Formats](#common-response-formats)
6. [Error Handling](#error-handling)
7. [Rate Limiting](#rate-limiting)
8. [REST API Endpoints](#rest-api-endpoints)
9. [WebSocket API](#websocket-api)
10. [API Examples](#api-examples)

---

## Overview

This document defines the RESTful API design for the Chaos-Sec orchestration platform. The API enables users to:

- Create, manage, and execute security experiments
- Monitor experiment status in real-time
- Query SIEM integration results
- Manage users and permissions
- Retrieve analytics and reports

### Design Principles

| Principle | Description |
|-----------|-------------|
| RESTful | Follows REST architectural constraints |
| Stateless | Each request contains all necessary information |
| Resource-Oriented | Resources identified by URIs |
| JSON-Based | All request/response bodies use JSON |
| Versioned | API version included in URL path |
| Secure | HTTPS required, authentication mandatory |

---

## API Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         Client Layer                            │
│                    (Web Dashboard / CLI)                        │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      API Gateway Layer                          │
│              (Authentication, Rate Limiting, Routing)           │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Application Layer                          │
│                 (Controllers, Business Logic)                   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Service Layer                             │
│         (Experiment, Kubernetes, SIEM, Notification Services)   │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────────┐
│                        Data Layer                               │
│              (Database, Kubernetes API, SIEM API)               │
└─────────────────────────────────────────────────────────────────┘
```

---

## Authentication & Authorization

### Authentication Scheme

Chaos-Sec uses **JWT (JSON Web Tokens)** for authentication.

```
Authorization: Bearer <jwt_token>
```

### Token Endpoint

```http
POST /api/v1/auth/login
```

**Request Body:**
```json
{
  "username": "admin",
  "password": "secure_password"
}
```

**Response:**
```json
{
  "access_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "token_type": "Bearer",
  "expires_in": 3600
}
```

### Token Refresh

```http
POST /api/v1/auth/refresh
```

**Request Body:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

### Role-Based Access Control (RBAC)

| Role | Permissions |
|------|-------------|
| `admin` | Full access to all resources |
| `operator` | Create/execute experiments, view reports |
| `viewer` | Read-only access to experiments and reports |
| `service` | Service-to-service communication only |

---

## Base URL & Versioning

### Base URL

```
https://chaos-sec.example.com/api/v1
```

### Versioning Strategy

- URL path versioning: `/api/v1/`, `/api/v2/`
- Backward compatibility maintained for minor versions
- Deprecation notices provided 90 days before removal

---

## Common Response Formats

### Success Response

```json
{
  "success": true,
  "data": {
    // Resource-specific data
  },
  "meta": {
    "request_id": "req_abc123xyz",
    "timestamp": "2026-01-15T10:30:00Z"
  }
}
```

### Paginated Response

```json
{
  "success": true,
  "data": [
    // Array of resources
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_pages": 5,
    "total_items": 100
  },
  "meta": {
    "request_id": "req_abc123xyz",
    "timestamp": "2026-01-15T10:30:00Z"
  }
}
```

### Error Response

```json
{
  "success": false,
  "error": {
    "code": "EXPERIMENT_NOT_FOUND",
    "message": "The requested experiment does not exist",
    "details": {
      "experiment_id": "exp_12345"
    }
  },
  "meta": {
    "request_id": "req_abc123xyz",
    "timestamp": "2026-01-15T10:30:00Z"
  }
}
```

---

## Error Handling

### HTTP Status Codes

| Status Code | Description |
|-------------|-------------|
| 200 OK | Request successful |
| 201 Created | Resource created successfully |
| 204 No Content | Request successful, no content to return |
| 400 Bad Request | Invalid request parameters |
| 401 Unauthorized | Missing or invalid authentication |
| 403 Forbidden | Insufficient permissions |
| 404 Not Found | Resource not found |
| 409 Conflict | Resource conflict (e.g., duplicate name) |
| 422 Unprocessable Entity | Validation error |
| 429 Too Many Requests | Rate limit exceeded |
| 500 Internal Server Error | Server error |
| 503 Service Unavailable | Service temporarily unavailable |

### Error Codes

| Error Code | HTTP Status | Description |
|------------|-------------|-------------|
| `INVALID_REQUEST` | 400 | Request body is malformed |
| `VALIDATION_ERROR` | 422 | Field validation failed |
| `UNAUTHORIZED` | 401 | Authentication required |
| `TOKEN_EXPIRED` | 401 | JWT token has expired |
| `FORBIDDEN` | 403 | Insufficient permissions |
| `NOT_FOUND` | 404 | Resource not found |
| `CONFLICT` | 409 | Resource already exists |
| `EXPERIMENT_ALREADY_RUNNING` | 409 | Experiment is already executing |
| `KUBERNETES_ERROR` | 500 | Kubernetes API error |
| `SIEM_CONNECTION_ERROR` | 503 | Cannot connect to SIEM |
| `RATE_LIMIT_EXCEEDED` | 429 | Too many requests |

---

## Rate Limiting

### Limits

| Endpoint Category | Limit | Window |
|-------------------|-------|--------|
| Authentication | 5 requests | 1 minute |
| Experiment Execution | 10 requests | 1 minute |
| Read Operations | 100 requests | 1 minute |
| Write Operations | 50 requests | 1 minute |

### Rate Limit Headers

```http
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1642234567
```

---

## REST API Endpoints

### Authentication Endpoints

#### POST /auth/login

Authenticate user and obtain access token.

```http
POST /api/v1/auth/login
Content-Type: application/json

{
  "username": "string",
  "password": "string"
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "access_token": "string",
    "refresh_token": "string",
    "token_type": "Bearer",
    "expires_in": 3600,
    "user": {
      "id": "usr_123",
      "username": "admin",
      "role": "admin"
    }
  }
}
```

#### POST /auth/refresh

Refresh access token using refresh token.

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "string"
}
```

#### POST /auth/logout

Invalidate current session.

```http
POST /api/v1/auth/logout
Authorization: Bearer <token>
```

#### GET /auth/me

Get current user information.

```http
GET /api/v1/auth/me
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "usr_123",
    "username": "admin",
    "email": "admin@example.com",
    "role": "admin",
    "created_at": "2026-01-01T00:00:00Z",
    "last_login": "2026-01-15T10:30:00Z"
  }
}
```

---

### Experiment Template Endpoints

#### GET /templates

List all experiment templates.

```http
GET /api/v1/templates
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number (default: 1) |
| `per_page` | integer | Items per page (default: 20, max: 100) |
| `category` | string | Filter by category (network, pod, security) |
| `search` | string | Search by name or description |

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "id": "tpl_001",
      "name": "Pod Egress Test",
      "description": "Tests egress network policies by attempting outbound connections",
      "category": "network",
      "severity": "medium",
      "duration_seconds": 300,
      "parameters": [
        {
          "name": "target_namespace",
          "type": "string",
          "required": true,
          "default": "default"
        },
        {
          "name": "destination_ip",
          "type": "string",
          "required": false,
          "default": "8.8.8.8"
        }
      ],
      "created_at": "2026-01-01T00:00:00Z",
      "updated_at": "2026-01-15T00:00:00Z"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_pages": 3,
    "total_items": 52
  }
}
```

#### GET /templates/:id

Get a specific experiment template.

```http
GET /api/v1/templates/tpl_001
Authorization: Bearer <token>
```

#### POST /templates

Create a new experiment template.

```http
POST /api/v1/templates
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Custom Network Test",
  "description": "Custom network policy validation",
  "category": "network",
  "severity": "medium",
  "duration_seconds": 300,
  "kubernetes_manifest": {
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": { ... },
    "spec": { ... }
  },
  "parameters": [
    {
      "name": "target_namespace",
      "type": "string",
      "required": true,
      "default": "default"
    }
  ],
  "siem_query": "SELECT * FROM alerts WHERE source='chaos-sec'",
  "expected_outcome": {
    "success_criteria": "No outbound connections allowed",
    "expected_alerts": ["NetworkPolicyViolation"]
  }
}
```

**Response:** `201 Created`
```json
{
  "success": true,
  "data": {
    "id": "tpl_053",
    "name": "Custom Network Test",
    ...
  }
}
```

#### PUT /templates/:id

Update an existing experiment template.

```http
PUT /api/v1/templates/tpl_001
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "Updated Network Test",
  "description": "Updated description",
  ...
}
```

#### DELETE /templates/:id

Delete an experiment template.

```http
DELETE /api/v1/templates/tpl_001
Authorization: Bearer <token>
```

**Response:** `204 No Content`

---

### Experiment Execution Endpoints

#### GET /experiments

List all experiments (scheduled, running, completed).

```http
GET /api/v1/experiments
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | integer | Page number |
| `per_page` | integer | Items per page |
| `status` | string | Filter by status (pending, running, completed, failed, cancelled) |
| `template_id` | string | Filter by template |
| `namespace` | string | Filter by Kubernetes namespace |
| `from_date` | string | Filter by start date (ISO 8601) |
| `to_date` | string | Filter by end date (ISO 8601) |

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "id": "exp_12345",
      "template_id": "tpl_001",
      "template_name": "Pod Egress Test",
      "name": "Production Egress Validation",
      "status": "running",
      "namespace": "production",
      "scheduled_at": "2026-01-15T10:00:00Z",
      "started_at": "2026-01-15T10:00:05Z",
      "estimated_end_at": "2026-01-15T10:05:05Z",
      "parameters": {
        "target_namespace": "production",
        "destination_ip": "8.8.8.8"
      },
      "progress": {
        "current_step": "executing_attack",
        "percentage": 60
      },
      "created_by": "usr_123"
    }
  ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total_pages": 5,
    "total_items": 98
  }
}
```

#### GET /experiments/:id

Get details of a specific experiment.

```http
GET /api/v1/experiments/exp_12345
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "exp_12345",
    "template_id": "tpl_001",
    "template_name": "Pod Egress Test",
    "name": "Production Egress Validation",
    "description": "Validating egress policies in production namespace",
    "status": "running",
    "namespace": "production",
    "scheduled_at": "2026-01-15T10:00:00Z",
    "started_at": "2026-01-15T10:00:05Z",
    "estimated_end_at": "2026-01-15T10:05:05Z",
    "completed_at": null,
    "parameters": {
      "target_namespace": "production",
      "destination_ip": "8.8.8.8"
    },
    "progress": {
      "current_step": "executing_attack",
      "percentage": 60,
      "steps": [
        {
          "name": "pod_creation",
          "status": "completed",
          "started_at": "2026-01-15T10:00:05Z",
          "completed_at": "2026-01-15T10:00:10Z"
        },
        {
          "name": "attack_execution",
          "status": "running",
          "started_at": "2026-01-15T10:00:10Z",
          "completed_at": null
        },
        {
          "name": "siem_validation",
          "status": "pending",
          "started_at": null,
          "completed_at": null
        },
        {
          "name": "cleanup",
          "status": "pending",
          "started_at": null,
          "completed_at": null
        }
      ]
    },
    "results": {
      "attack_successful": null,
      "siem_alerts_detected": null,
      "validation_passed": null,
      "findings": []
    },
    "logs": [
      {
        "timestamp": "2026-01-15T10:00:05Z",
        "level": "info",
        "message": "Experiment started"
      },
      {
        "timestamp": "2026-01-15T10:00:10Z",
        "level": "info",
        "message": "Attacker pod created: chaos-sec-attacker-abc123"
      }
    ],
    "created_by": "usr_123",
    "created_at": "2026-01-15T09:55:00Z",
    "updated_at": "2026-01-15T10:00:10Z"
  }
}
```

#### POST /experiments

Create and schedule a new experiment.

```http
POST /api/v1/experiments
Authorization: Bearer <token>
Content-Type: application/json

{
  "template_id": "tpl_001",
  "name": "Production Egress Validation",
  "description": "Optional description",
  "namespace": "production",
  "scheduled_at": "2026-01-15T10:00:00Z",
  "parameters": {
    "target_namespace": "production",
    "destination_ip": "8.8.8.8"
  },
  "options": {
    "auto_cleanup": true,
    "timeout_seconds": 600,
    "notify_on_completion": true,
    "dry_run": false
  }
}
```

**Response:** `201 Created`
```json
{
  "success": true,
  "data": {
    "id": "exp_12345",
    "template_id": "tpl_001",
    "name": "Production Egress Validation",
    "status": "pending",
    "scheduled_at": "2026-01-15T10:00:00Z"
  }
}
```

#### POST /experiments/:id/execute

Execute an experiment immediately (if not already running).

```http
POST /api/v1/experiments/exp_12345/execute
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "exp_12345",
    "status": "running",
    "started_at": "2026-01-15T10:30:00Z"
  }
}
```

#### POST /experiments/:id/cancel

Cancel a running or scheduled experiment.

```http
POST /api/v1/experiments/exp_12345/cancel
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "id": "exp_12345",
    "status": "cancelled",
    "cancelled_at": "2026-01-15T10:30:00Z"
  }
}
```

#### POST /experiments/:id/retry

Retry a failed experiment.

```http
POST /api/v1/experiments/exp_12345/retry
Authorization: Bearer <token>
```

**Response:** `201 Created`
```json
{
  "success": true,
  "data": {
    "id": "exp_12346",
    "original_experiment_id": "exp_12345",
    "status": "pending"
  }
}
```

---

### SIEM Integration Endpoints

#### GET /siem/status

Get SIEM connection status.

```http
GET /api/v1/siem/status
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "connected": true,
    "provider": "splunk",
    "endpoint": "https://siem.example.com:8089",
    "last_sync": "2026-01-15T10:25:00Z",
    "health": {
      "status": "healthy",
      "latency_ms": 45,
      "last_error": null
    }
  }
}
```

#### POST /siem/test

Test SIEM connection.

```http
POST /api/v1/siem/test
Authorization: Bearer <token>
Content-Type: application/json

{
  "provider": "splunk",
  "endpoint": "https://siem.example.com:8089",
  "credentials": {
    "username": "chaos_sec_user",
    "password": "secure_password"
  }
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "connected": true,
    "latency_ms": 45,
    "message": "SIEM connection successful"
  }
}
```

#### GET /experiments/:id/siem-alerts

Get SIEM alerts related to a specific experiment.

```http
GET /api/v1/experiments/exp_12345/siem-alerts
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `from` | string | Start time (ISO 8601) |
| `to` | string | End time (ISO 8601) |

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "experiment_id": "exp_12345",
    "query_executed_at": "2026-01-15T10:05:30Z",
    "alerts": [
      {
        "id": "alert_001",
        "timestamp": "2026-01-15T10:00:15Z",
        "severity": "high",
        "type": "NetworkPolicyViolation",
        "source": "calico",
        "message": "Outbound connection attempt blocked",
        "raw_data": {
          "src_pod": "chaos-sec-attacker-abc123",
          "src_namespace": "production",
          "dst_ip": "8.8.8.8",
          "dst_port": 53
        }
      }
    ],
    "validation": {
      "expected_alerts": ["NetworkPolicyViolation"],
      "detected_alerts": ["NetworkPolicyViolation"],
      "missing_alerts": [],
      "unexpected_alerts": [],
      "validation_passed": true
    }
  }
}
```

#### POST /siem/query

Execute a custom SIEM query.

```http
POST /api/v1/siem/query
Authorization: Bearer <token>
Content-Type: application/json

{
  "query": "SELECT * FROM alerts WHERE severity='high' AND time > now()-1h",
  "time_range": {
    "from": "2026-01-15T09:00:00Z",
    "to": "2026-01-15T10:00:00Z"
  }
}
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "query_id": "qry_123",
    "results": [
      // Alert objects
    ],
    "total_count": 15,
    "execution_time_ms": 234
  }
}
```

---

### Kubernetes Cluster Endpoints

#### GET /clusters

List registered Kubernetes clusters.

```http
GET /api/v1/clusters
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "id": "cls_001",
      "name": "production-cluster",
      "context": "gke_project_us-central1_prod",
      "version": "1.28.3",
      "status": "connected",
      "namespaces": ["default", "production", "staging"],
      "node_count": 5,
      "last_heartbeat": "2026-01-15T10:29:00Z",
      "health": {
        "status": "healthy",
        "api_server_latency_ms": 25
      }
    }
  ]
}
```

#### POST /clusters

Register a new Kubernetes cluster.

```http
POST /api/v1/clusters
Authorization: Bearer <token>
Content-Type: application/json

{
  "name": "staging-cluster",
  "kubeconfig": "base64_encoded_kubeconfig",
  "context": "gke_project_us-central1_staging"
}
```

#### GET /clusters/:id

Get cluster details.

```http
GET /api/v1/clusters/cls_001
Authorization: Bearer <token>
```

#### DELETE /clusters/:id

Remove a cluster registration.

```http
DELETE /api/v1/clusters/cls_001
Authorization: Bearer <token>
```

#### GET /clusters/:id/namespaces

List namespaces in a cluster.

```http
GET /api/v1/clusters/cls_001/namespaces
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "name": "default",
      "status": "active",
      "pod_count": 12,
      "network_policies": 3
    },
    {
      "name": "production",
      "status": "active",
      "pod_count": 45,
      "network_policies": 8
    }
  ]
}
```

#### GET /clusters/:id/network-policies

List network policies in a cluster/namespace.

```http
GET /api/v1/clusters/cls_001/network-policies?namespace=production
Authorization: Bearer <token>
```

---

### Dashboard & Analytics Endpoints

#### GET /dashboard/summary

Get dashboard summary statistics.

```http
GET /api/v1/dashboard/summary
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "experiments": {
      "total": 1250,
      "running": 3,
      "pending": 5,
      "completed_today": 45,
      "failed_today": 2
    },
    "validation": {
      "passed": 1180,
      "failed": 65,
      "pass_rate": 94.4
    },
    "security_controls": {
      "total_tested": 25,
      "validated": 22,
      "failed": 3
    },
    "recent_alerts": 156,
    "clusters": {
      "total": 3,
      "healthy": 3
    }
  }
}
```

#### GET /dashboard/experiments/chart

Get experiment trend data for charts.

```http
GET /api/v1/dashboard/experiments/chart
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `period` | string | Time period (7d, 30d, 90d) |
| `granularity` | string | Data granularity (hour, day, week) |

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "labels": ["2026-01-09", "2026-01-10", "2026-01-11", "2026-01-12", "2026-01-13", "2026-01-14", "2026-01-15"],
    "datasets": [
      {
        "label": "Passed",
        "data": [42, 38, 45, 50, 48, 52, 45]
      },
      {
        "label": "Failed",
        "data": [3, 5, 2, 4, 3, 2, 2]
      }
    ]
  }
}
```

#### GET /dashboard/security-controls

Get security control validation status.

```http
GET /api/v1/dashboard/security-controls
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": [
    {
      "control_id": "ctrl_001",
      "name": "Pod Egress Restriction",
      "type": "network_policy",
      "status": "validated",
      "last_tested": "2026-01-15T09:00:00Z",
      "test_count": 45,
      "pass_rate": 100
    },
    {
      "control_id": "ctrl_002",
      "name": "Ingress Traffic Filtering",
      "type": "network_policy",
      "status": "failed",
      "last_tested": "2026-01-15T08:00:00Z",
      "test_count": 30,
      "pass_rate": 73.3,
      "failure_reason": "Unexpected ingress traffic allowed from 10.0.0.0/8"
    }
  ]
}
```

#### GET /reports/:experiment-id

Generate detailed report for an experiment.

```http
GET /api/v1/reports/exp_12345
Authorization: Bearer <token>
```

**Query Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `format` | string | Report format (json, pdf, html) |

**Response:** `200 OK` (JSON format)
```json
{
  "success": true,
  "data": {
    "report_id": "rpt_12345",
    "experiment_id": "exp_12345",
    "generated_at": "2026-01-15T10:10:00Z",
    "summary": {
      "status": "completed",
      "validation_passed": true,
      "duration_seconds": 305
    },
    "findings": [
      {
        "severity": "info",
        "title": "Network Policy Effective",
        "description": "Egress traffic was successfully blocked as expected",
        "evidence": {
          "siem_alert": "alert_001",
          "pod_logs": "Connection refused to 8.8.8.8:53"
        },
        "recommendation": null
      }
    ],
    "timeline": [
      {
        "timestamp": "2026-01-15T10:00:05Z",
        "event": "Experiment started"
      },
      {
        "timestamp": "2026-01-15T10:00:10Z",
        "event": "Attacker pod created"
      },
      {
        "timestamp": "2026-01-15T10:00:15Z",
        "event": "Attack executed - egress attempt to 8.8.8.8:53"
      },
      {
        "timestamp": "2026-01-15T10:00:16Z",
        "event": "SIEM alert detected - NetworkPolicyViolation"
      },
      {
        "timestamp": "2026-01-15T10:05:10Z",
        "event": "Experiment completed successfully"
      }
    ]
  }
}
```

---

### User Management Endpoints (Admin Only)

#### GET /users

List all users.

```http
GET /api/v1/users
Authorization: Bearer <token>
```

#### POST /users

Create a new user.

```http
POST /api/v1/users
Authorization: Bearer <token>
Content-Type: application/json

{
  "username": "newuser",
  "email": "user@example.com",
  "password": "secure_password",
  "role": "operator"
}
```

#### PUT /users/:id

Update user details.

```http
PUT /api/v1/users/usr_456
Authorization: Bearer <token>
Content-Type: application/json

{
  "email": "newemail@example.com",
  "role": "admin"
}
```

#### DELETE /users/:id

Delete a user.

```http
DELETE /api/v1/users/usr_456
Authorization: Bearer <token>
```

---

### Settings & Configuration Endpoints

#### GET /settings

Get system settings.

```http
GET /api/v1/settings
Authorization: Bearer <token>
```

**Response:** `200 OK`
```json
{
  "success": true,
  "data": {
    "general": {
      "platform_name": "Chaos-Sec",
      "default_timeout_seconds": 600,
      "max_concurrent_experiments": 5
    },
    "kubernetes": {
      "default_namespace": "chaos-sec",
      "cleanup_on_failure": true,
      "pod_security_context": {
        "run_as_non_root": true,
        "allow_privilege_escalation": false
      }
    },
    "siem": {
      "enabled": true,
      "provider": "splunk",
      "sync_interval_seconds": 60
    },
    "notifications": {
      "email_enabled": false,
      "slack_enabled": true,
      "slack_webhook_url": "https://hooks.slack.com/..."
    }
  }
}
```

#### PUT /settings

Update system settings.

```http
PUT /api/v1/settings
Authorization: Bearer <token>
Content-Type: application/json

{
  "general": {
    "default_timeout_seconds": 900
  },
  "notifications": {
    "email_enabled": true,
    "email_recipients": ["admin@example.com"]
  }
}
```

---

## WebSocket API

### Real-time Experiment Updates

For real-time experiment status updates, Chaos-Sec provides WebSocket connections.

### Connection

```
wss://chaos-sec.example.com/api/v1/ws/experiments
Authorization: Bearer <token>
```

### Subscription Messages

**Subscribe to specific experiment:**
```json
{
  "action": "subscribe",
  "experiment_id": "exp_12345"
}
```

**Subscribe to all experiments:**
```json
{
  "action": "subscribe",
  "scope": "all"
}
```

### Server Push Messages

**Experiment status update:**
```json
{
  "type": "experiment_update",
  "data": {
    "experiment_id": "exp_12345",
    "status": "running",
    "progress": {
      "current_step": "executing_attack",
      "percentage": 60
    },
    "timestamp": "2026-01-15T10:00:30Z"
  }
}
```

**New log entry:**
```json
{
  "type": "log_entry",
  "data": {
    "experiment_id": "exp_12345",
    "timestamp": "2026-01-15T10:00:35Z",
    "level": "info",
    "message": "Attack payload executed successfully"
  }
}
```

**SIEM alert detected:**
```json
{
  "type": "siem_alert",
  "data": {
    "experiment_id": "exp_12345",
    "alert": {
      "id": "alert_001",
      "type": "NetworkPolicyViolation",
      "severity": "high"
    },
    "timestamp": "2026-01-15T10:00:40Z"
  }
}
```

**Experiment completed:**
```json
{
  "type": "experiment_complete",
  "data": {
    "experiment_id": "exp_12345",
    "status": "completed",
    "validation_passed": true,
    "completed_at": "2026-01-15T10:05:10Z"
  }
}
```

---

## API Examples

### Complete Experiment Flow

#### Step 1: Authenticate

```bash
curl -X POST https://chaos-sec.example.com/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{
    "username": "admin",
    "password": "secure_password"
  }'
```

#### Step 2: List Available Templates

```bash
curl -X GET https://chaos-sec.example.com/api/v1/templates \
  -H "Authorization: Bearer <access_token>"
```

#### Step 3: Create an Experiment

```bash
curl -X POST https://chaos-sec.example.com/api/v1/experiments \
  -H "Authorization: Bearer <access_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "template_id": "tpl_001",
    "name": "Production Egress Test",
    "namespace": "production",
    "parameters": {
      "target_namespace": "production",
      "destination_ip": "8.8.8.8"
    }
  }'
```

#### Step 4: Monitor Experiment Status

```bash
curl -X GET https://chaos-sec.example.com/api/v1/experiments/exp_12345 \
  -H "Authorization: Bearer <access_token>"
```

#### Step 5: Get SIEM Validation Results

```bash
curl -X GET https://chaos-sec.example.com/api/v1/experiments/exp_12345/siem-alerts \
  -H "Authorization: Bearer <access_token>"
```

#### Step 6: Generate Report

```bash
curl -X GET https://chaos-sec.example.com/api/v1/reports/exp_12345?format=json \
  -H "Authorization: Bearer <access_token>"
```

---

## Appendix

### A. Experiment Templates Reference

| Template ID | Name | Category | Description |
|-------------|------|----------|-------------|
| `tpl_001` | Pod Egress Test | Network | Tests egress network policies |
| `tpl_002` | Pod Ingress Test | Network | Tests ingress network policies |
| `tpl_003` | Inter-Namespace Test | Network | Tests namespace isolation |
| `tpl_004` | Service Account Privilege Test | RBAC | Tests service account permissions |
| `tpl_005` | Secret Access Test | Security | Tests secret access controls |
| `tpl_006` | Pod Security Policy Test | Security | Tests pod security policies |

### B. Webhook Payload Format

For notification integrations, webhooks use the following format:

```json
{
  "event_type": "experiment.completed",
  "timestamp": "2026-01-15T10:05:10Z",
  "data": {
    "experiment_id": "exp_12345",
    "name": "Production Egress Test",
    "status": "completed",
    "validation_passed": true,
    "url": "https://chaos-sec.example.com/experiments/exp_12345"
  }
}
```

### C. OpenAPI Specification

The complete OpenAPI 3.0 specification is available at:

```
https://chaos-sec.example.com/api/v1/openapi.json
```

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0.0 | 2026-01-15 | Chaos-Sec Team | Initial draft |
