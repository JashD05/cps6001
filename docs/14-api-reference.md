# Chaos-Sec API Reference

**Version:** 1.0.0
**Last Updated:** 2026-04-21
**Base URL:** `https://app.chaos-sec.io/api/v1`

---

## Table of Contents

1. [Overview](#1-overview)
2. [Authentication](#2-authentication)
3. [Common Patterns](#3-common-patterns)
4. [Authentication Endpoints](#4-authentication-endpoints)
5. [Experiment Endpoints](#5-experiment-endpoints)
6. [Template Endpoints](#6-template-endpoints)
7. [Attack Template Endpoints](#7-attack-template-endpoints)
8. [Cluster Endpoints](#8-cluster-endpoints)
9. [SIEM Endpoints](#9-siem-endpoints)
10. [Dashboard Endpoints](#10-dashboard-endpoints)
11. [Report Endpoints](#11-report-endpoints)
12. [Health Endpoints](#12-health-endpoints)
13. [User Management Endpoints](#13-user-management-endpoints)
14. [Settings & Configuration Endpoints](#14-settings--configuration-endpoints)
15. [WebSocket API](#15-websocket-api)
16. [Error Codes](#16-error-codes)
17. [Rate Limiting](#17-rate-limiting)

---

## 1. Overview

### Design Principles

| Principle | Description |
|-----------|-------------|
| **RESTful** | Follows REST architectural constraints |
| **Stateless** | Each request contains all necessary information |
| **Resource-Oriented** | Resources identified by URIs |
| **JSON-Based** | All request/response bodies use JSON |
| **Versioned** | API version included in URL path (`/api/v1`) |
| **Secure** | HTTPS required, authentication mandatory |

### Content Types

| Context | Content Type |
|---------|-------------|
| Request body | `application/json` |
| Response body | `application/json` |
| File download | `application/pdf`, `text/csv` |

### Standard Response Envelope

```json
{
  "success": true,
  "data": { ... },
  "message": "Operation completed",
  "metadata": {
    "request_id": "a6245574-911b-4215-87c2-b5fec2da2530",
    "timestamp": "2026-04-21T10:30:00Z",
    "version": "1.0.0"
  }
}
```

### Paginated Response

```json
{
  "success": true,
  "data": {
    "items": [ ... ],
    "total_count": 142,
    "page": 1,
    "page_size": 20,
    "total_pages": 8,
    "has_next_page": true,
    "has_previous_page": false
  }
}
```

---

## 2. Authentication

### JWT Authentication

All authenticated endpoints require a valid JWT access token in the `Authorization` header:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

### Token Types

| Token Type | Expiry | Purpose |
|------------|--------|---------|
| Access Token | 1 hour | API request authentication |
| Refresh Token | 7 days | Obtain new access tokens |

### Token Claims

| Claim | Type | Description |
|-------|------|-------------|
| `user_id` | UUID | User unique identifier |
| `email` | string | User email address |
| `role` | string | User role name |
| `organization_id` | UUID | User's organization |
| `permissions` | string[] | Granted permissions |
| `token_type` | string | `access` or `refresh` |
| `iss` | string | Issuer: `chaos-sec` |
| `sub` | string | Subject: user ID |
| `aud` | string[] | Audience: `["chaos-sec-api"]` |
| `exp` | number | Expiry timestamp |
| `iat` | number | Issued-at timestamp |
| `jti` | string | JWT ID (for revocation) |

### Permissions

| Permission | Description |
|-----------|-------------|
| `admin:all` | Full administrative access (bypasses RBAC) |
| `users:manage` | Create and manage user accounts |
| `experiments:read` | View experiments and results |
| `experiments:write` | Create and edit experiments |
| `experiments:execute` | Run and stop experiments |
| `experiments:delete` | Delete experiments |
| `templates:read` | View attack templates |
| `templates:write` | Create, edit, and delete templates |
| `clusters:read` | View cluster information |
| `clusters:write` | Register and manage clusters |

### Role-Permission Mapping

| Permission | Admin | Operator | Viewer |
|-----------|-------|----------|--------|
| `admin:all` | ✅ | — | — |
| `users:manage` | ✅ | — | — |
| `experiments:read` | ✅ | ✅ | ✅ |
| `experiments:write` | ✅ | ✅ | — |
| `experiments:execute` | ✅ | ✅ | — |
| `experiments:delete` | ✅ | ✅ | — |
| `templates:read` | ✅ | ✅ | ✅ |
| `templates:write` | ✅ | ✅ | — |
| `clusters:read` | ✅ | ✅ | ✅ |
| `clusters:write` | ✅ | ✅ | — |

---

## 3. Common Patterns

### Pagination

Query parameters for paginated endpoints:

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number (1-indexed) |
| `per_page` | integer | 20 | Items per page (max: 100) |
| `sort` | string | `created_at_desc` | Sort field and direction |

**Valid sort directions:** `_asc` (ascending), `_desc` (descending)

**Example:**
```
GET /api/v1/experiments?page=2&per_page=50&sort=name_asc
```

### Filtering

| Parameter | Type | Description |
|-----------|------|-------------|
| `search` | string | Free-text search |
| `status` | string | Filter by status |
| `template_id` | UUID | Filter by template |
| `cluster_id` | UUID | Filter by cluster |
| `date_from` | ISO 8601 | Filter from date |
| `date_to` | ISO 8601 | Filter to date |

### Error Response Format

```json
{
  "success": false,
  "error": "error_code",
  "message": "Human-readable description of the error",
  "code": 400,
  "errors": [
    {
      "code": "validation_error",
      "message": "Name is required",
      "field": "name"
    }
  ]
}
```

### Request ID

Every response includes an `X-Request-ID` header. If not provided by the client, the server generates a UUID. You can pass your own via the `X-Request-ID` request header for tracing.

---

## 4. Authentication Endpoints

### POST /api/v1/auth/login

Authenticate a user and obtain access/refresh tokens.

**Authentication:** None

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `username` | string | Yes | User username |
| `password` | string | Yes | User password (min 8 chars) |

**Request Example:**
```json
{
  "username": "admin",
  "password": "admin"
}
```

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 3600,
    "token_type": "Bearer"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_request` | Missing or invalid fields |
| 401 | `invalid_credentials` | Email/password combination incorrect |
| 403 | `account_disabled` | User account is deactivated |

---

### POST /api/v1/auth/refresh

Obtain new tokens using a valid refresh token.

**Authentication:** None

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `refresh_token` | string | Yes | Valid refresh token |

**Request Example:**
```json
{
  "refresh_token": "eyJhbGciOiJIUzI1NiIs..."
}
```

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "expires_in": 3600,
    "token_type": "Bearer"
  }
}
```

> **Note:** The old refresh token is blacklisted after use. You must store the new refresh token.

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_request` | Missing refresh token |
| 401 | `invalid_token` | Token is malformed or invalid |
| 401 | `expired_token` | Refresh token has expired |
| 401 | `token_revoked` | Refresh token has been blacklisted |
| 403 | `account_disabled` | User account is deactivated |

---

### POST /api/v1/auth/logout

Invalidate the current access token.

**Authentication:** JWT (any authenticated user)

**Behavior:** The access token is blacklisted in Redis with a TTL equal to the remaining time until expiry. If the token is already invalid or expired, the endpoint still returns success.

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "message": "logged out"
  }
}
```

---

### GET /api/v1/auth/me

Retrieve the authenticated user's profile with fresh role and permission data.

**Authentication:** JWT (any authenticated user)

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "f47ac10b-58cc-4372-a567-0e02b2c3d479",
    "email": "admin@chaos-sec.local",
    "username": "admin",
    "name": "Admin User",
    "is_active": true,
    "role": {
      "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "name": "admin",
      "description": "Full administrative access",
      "permissions": ["admin:all", "users:manage", "experiments:read", ...]
    },
    "organization_id": "7da9a065-2b3c-4d5e-8f9a-b1c2d3e4f5a6",
    "last_login_at": "2026-04-21T09:30:00Z",
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-04-21T09:30:00Z"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 401 | `unauthorized` | No valid token provided |
| 404 | `not_found` | User not found in database |

---

### POST /api/v1/auth/register

Create a new user account.

**Authentication:** JWT with `admin:all` or `users:manage` permission

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string | Yes | Unique email address |
| `password` | string | Yes | Password (min 8 chars) |
| `name` | string | Yes | Display name |
| `organization_id` | UUID | Yes | Organization UUID |
| `role_id` | UUID | Yes | Role UUID |

**Request Example:**
```json
{
  "email": "operator@company.com",
  "password": "SecureP@ss123",
  "name": "Jane Operator",
  "organization_id": "7da9a065-2b3c-4d5e-8f9a-b1c2d3e4f5a6",
  "role_id": "b2c3d4e5-f6a7-8901-bcde-f12345678901"
}
```

**Response `201 Created`:**
```json
{
  "success": true,
  "data": {
    "id": "c3d4e5f6-a7b8-9012-cdef-123456789012",
    "email": "operator@company.com",
    "name": "Jane Operator",
    "is_active": true,
    "role": { ... },
    "organization_id": "7da9a065-2b3c-4d5e-8f9a-b1c2d3e4f5a6",
    "created_at": "2026-04-21T10:30:00Z"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_request` | Missing or invalid fields |
| 403 | `forbidden` | Insufficient permissions or cross-org attempt |
| 409 | `conflict` | Email already exists |

---

## 5. Experiment Endpoints

### GET /api/v1/experiments

List experiments with filtering, pagination, and sorting.

**Authentication:** JWT with `experiments:read`

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number |
| `per_page` | integer | 20 | Items per page (max: 100) |
| `sort` | string | `created_at_desc` | Sort field and direction |
| `search` | string | — | Search experiment names |
| `status` | string | — | Filter by status |
| `template_id` | UUID | — | Filter by template |
| `cluster_id` | UUID | — | Filter by cluster |
| `date_from` | ISO 8601 | — | From date |
| `date_to` | ISO 8601 | — | To date |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "name": "Network Policy Egress Test",
        "description": "Validate egress network policies",
        "template_id": "t1t2t3t4-t5t6-t7t8-t9t0-t1t2t3t4t5t6",
        "template_name": "Egress Network Validation",
        "cluster_id": "c1c2c3c4-c5c6-c7c8-c9c0-c1c2c3c4c5c6",
        "cluster_name": "Production Cluster",
        "namespace": "chaos-test",
        "status": "completed",
        "progress": 100,
        "tags": ["network", "egress"],
        "created_by": "admin@chaos-sec.local",
        "created_at": "2026-04-20T14:00:00Z",
        "updated_at": "2026-04-20T14:05:30Z",
        "started_at": "2026-04-20T14:01:00Z",
        "completed_at": "2026-04-20T14:05:30Z",
        "duration": 270
      }
    ],
    "total_count": 42,
    "page": 1,
    "page_size": 20,
    "total_pages": 3,
    "has_next_page": true,
    "has_previous_page": false
  }
}
```

---

### POST /api/v1/experiments

Create a new experiment.

**Authentication:** JWT with `experiments:write`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Experiment name |
| `description` | string | No | Description |
| `template_id` | UUID | Yes | Attack template to use |
| `cluster_id` | UUID | Yes | Target cluster |
| `namespace` | string | Yes | Target namespace |
| `parameters` | object | No | Template-specific parameters |
| `validation` | object | No | SIEM validation settings |
| `tags` | string[] | No | Category labels |
| `schedule` | string | No | Cron expression for recurring runs |

**Validation Object:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `siem_alert_type` | string | No | Expected SIEM alert type |
| `time_window_seconds` | integer | No | Wait time for SIEM alert (default: 300) |
| `expected_alert_count` | integer | No | Minimum expected alerts (default: 1) |
| `custom_rules` | object | No | Custom SIEM query rules |

**Request Example:**
```json
{
  "name": "Weekly Egress Policy Check",
  "description": "Validate egress network policies on production",
  "template_id": "t1t2t3t4-t5t6-t7t8-t9t0-t1t2t3t4t5t6",
  "cluster_id": "c1c2c3c4-c5c6-c7c8-c9c0-c1c2c3c4c5c6",
  "namespace": "chaos-test",
  "parameters": {
    "target_url": "8.8.8.8",
    "protocol": "udp",
    "port": 53,
    "duration_seconds": 60
  },
  "validation": {
    "siem_alert_type": "network_flow",
    "time_window_seconds": 300,
    "expected_alert_count": 1
  },
  "tags": ["network", "egress", "weekly"],
  "schedule": "0 0 * * 1"
}
```

**Response `201 Created`:**
```json
{
  "success": true,
  "data": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "name": "Weekly Egress Policy Check",
    "status": "pending",
    "created_at": "2026-04-21T10:30:00Z"
  }
}
```

---

### GET /api/v1/experiments/:id

Get full experiment details including steps, current run, and recent runs.

**Authentication:** JWT with `experiments:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Experiment ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "name": "Network Policy Egress Test",
    "description": "Validate egress network policies",
    "template_id": "t1t2t3t4-t5t6-t7t8-t9t0-t1t2t3t4t5t6",
    "template_name": "Egress Network Validation",
    "cluster_id": "c1c2c3c4-c5c6-c7c8-c9c0-c1c2c3c4c5c6",
    "cluster_name": "Production Cluster",
    "namespace": "chaos-test",
    "status": "completed",
    "progress": 100,
    "parameters": { ... },
    "steps": [
      {
        "id": "s1s2s3s4-s5s6-s7s8-s9s0-s1s2s3s4s5s6",
        "name": "Spawn attacker pod",
        "description": "Create pod in target namespace",
        "status": "completed",
        "started_at": "2026-04-20T14:01:00Z",
        "completed_at": "2026-04-20T14:01:05Z",
        "order": 1
      }
    ],
    "tags": ["network", "egress"],
    "created_by": "admin@chaos-sec.local",
    "created_at": "2026-04-20T14:00:00Z",
    "updated_at": "2026-04-20T14:05:30Z",
    "started_at": "2026-04-20T14:01:00Z",
    "completed_at": "2026-04-20T14:05:30Z",
    "duration": 270,
    "result": {
      "success": true,
      "score": 100,
      "summary": "All expected alerts were detected",
      "details": [...],
      "siem_validation": { ... },
      "started_at": "2026-04-20T14:01:00Z",
      "completed_at": "2026-04-20T14:05:30Z",
      "duration": 270
    }
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 404 | `not_found` | Experiment ID does not exist |

---

### PUT /api/v1/experiments/:id

Update an existing experiment.

**Authentication:** JWT with `experiments:write`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Experiment ID |

**Request Body:** Same fields as `POST /api/v1/experiments` (all optional — only provided fields are updated).

**Response `200 OK`:** Returns the updated experiment object.

---

### DELETE /api/v1/experiments/:id

Soft-delete an experiment.

**Authentication:** JWT with `experiments:delete`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Experiment ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "message": "Experiment deleted"
  }
}
```

---

### POST /api/v1/experiments/:id/execute

Start running an experiment.

**Authentication:** JWT with `experiments:execute`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Experiment ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "queued",
    "message": "Experiment queued for execution"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_state` | Experiment is already running or completed |
| 404 | `not_found` | Experiment ID does not exist |

---

### POST /api/v1/experiments/:id/stop

Stop a running experiment.

**Authentication:** JWT with `experiments:execute`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Experiment ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "stopped",
    "message": "Experiment stopped"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_state` | Experiment is not running |

---

### POST /api/v1/experiments/:id/cancel

Cancel a running or scheduled experiment.

> **Note:** This endpoint is distinct from `POST /experiments/:id/stop`. While `stop` halts a running experiment and transitions it to `stopped`, `cancel` cancels a running or scheduled experiment and transitions it to `cancelled`, returning a `cancelled_at` timestamp. The API design document (`04-api-design.md`) defines both endpoints; `stop` is the primary action for halting execution, while `cancel` is used for cancelling scheduled or running experiments with a cancellation record.

**Authentication:** JWT with `experiments:execute`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Experiment ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "cancelled",
    "cancelled_at": "2026-04-21T10:30:00Z"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_state` | Experiment is already completed or cancelled |
| 404 | `not_found` | Experiment ID does not exist |

---

### POST /api/v1/experiments/:id/retry

Retry a failed or cancelled experiment by creating a new experiment instance based on the original's configuration.

**Authentication:** JWT with `experiments:execute`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Original experiment ID |

**Response `201 Created`:**
```json
{
  "success": true,
  "data": {
    "id": "b2c3d4e5-f6a7-8901-bcde-f12345678901",
    "original_experiment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "status": "pending"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `invalid_state` | Original experiment status is not `failed` or `cancelled` |
| 404 | `not_found` | Experiment ID does not exist |

---

## 6. Template Endpoints

### GET /api/v1/templates

List experiment templates.

**Authentication:** JWT with `templates:read`

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": "t1t2t3t4-t5t6-t7t8-t9t0-t1t2t3t4t5t6",
        "name": "Egress Network Validation",
        "description": "Tests egress network policies by attempting outbound connections",
        "category": "network",
        "severity": "high",
        "icon": "network_check",
        "version": "1.0.0",
        "author": "Chaos-Sec",
        "is_official": true,
        "usage_count": 42,
        "created_at": "2026-01-15T10:00:00Z",
        "updated_at": "2026-04-01T08:00:00Z"
      }
    ],
    "total_count": 5
  }
}
```

---

### POST /api/v1/templates

Create a new experiment template.

**Authentication:** JWT with `templates:write`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Template name |
| `description` | string | Yes | What the template tests |
| `category` | string | Yes | `network`, `application`, `infrastructure`, `data`, `identity`, `custom` |
| `severity` | string | Yes | `critical`, `high`, `medium`, `low`, `info` |
| `parameters` | array | Yes | Parameter definitions |
| `attack_phases` | array | Yes | Attack phase definitions |
| `expected_detections` | array | No | Expected SIEM detections |
| `tags` | string[] | No | Category labels |

**Parameter Definition:**

| Field | Type | Description |
|-------|------|-------------|
| `key` | string | Parameter identifier |
| `label` | string | Display label |
| `type` | string | `string`, `number`, `boolean`, `select`, `multi-select` |
| `default_value` | any | Default value |
| `required` | boolean | Whether the parameter is required |
| `description` | string | Help text |
| `options` | array | Select options (for select types) |
| `validation` | object | Validation rules (min, max, pattern, etc.) |

**Response `201 Created`:** Returns the created template.

---

### GET /api/v1/templates/:id

Get template details with full parameter and phase definitions.

**Authentication:** JWT with `templates:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Template ID |

**Response `200 OK`:** Returns the full template object with parameters, attack phases, and expected detections.

---

### PUT /api/v1/templates/:id

Update an existing experiment template.

**Authentication:** JWT with `templates:write`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Template ID |

**Request Body:** Same fields as `POST /api/v1/templates` (all optional — only provided fields are updated).

**Request Example:**
```json
{
  "name": "Updated Egress Network Validation",
  "description": "Updated description for egress network policy testing",
  "severity": "critical",
  "parameters": [
    {
      "key": "target_url",
      "label": "Target URL",
      "type": "string",
      "default_value": "8.8.8.8",
      "required": true,
      "description": "Target IP or hostname for egress test"
    }
  ]
}
```

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "t1t2t3t4-t5t6-t7t8-t9t0-t1t2t3t4t5t6",
    "name": "Updated Egress Network Validation",
    "description": "Updated description for egress network policy testing",
    "category": "network",
    "severity": "critical",
    "updated_at": "2026-04-21T10:30:00Z"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `validation_error` | Invalid request body fields |
| 404 | `not_found` | Template ID does not exist |

---

### DELETE /api/v1/templates/:id

Delete an experiment template.

**Authentication:** JWT with `templates:write` or `admin:all`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Template ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "message": "Template deleted"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 404 | `not_found` | Template ID does not exist |
| 409 | `conflict` | Template is currently in use by running experiments |

---

## 7. Attack Template Endpoints

### GET /api/v1/attack-templates

List attack templates.

**Authentication:** JWT with `templates:read`

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `category` | string | — | Filter by category |
| `severity` | string | — | Filter by severity |
| `search` | string | — | Search by name |

**Response `200 OK`:** Returns paginated list of attack templates.

---

### POST /api/v1/attack-templates

Create a new attack template.

**Authentication:** JWT with `templates:write`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Template name |
| `description` | string | Yes | What the template tests |
| `category` | string | Yes | Attack category |
| `severity` | string | Yes | Severity level |
| `parameters` | array | Yes | Configurable parameters |
| `attack_phases` | array | Yes | Ordered attack phases |
| `expected_detections` | array | No | Expected SIEM detections |

**Response `201 Created`:** Returns the created attack template.

---

### GET /api/v1/attack-templates/:id

Get attack template details.

**Authentication:** JWT with `templates:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Template ID |

**Response `200 OK`:** Returns the full attack template.

---

### PUT /api/v1/attack-templates/:id

Update an attack template.

**Authentication:** JWT with `templates:write`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Template ID |

**Request Body:** Same fields as POST (all optional — only provided fields are updated).

**Response `200 OK`:** Returns the updated template.

---

### DELETE /api/v1/attack-templates/:id

Delete an attack template.

**Authentication:** JWT with `templates:write` or `admin:all`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Template ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "message": "Attack template deleted"
  }
}
```

---

## 8. Cluster Endpoints

### GET /api/v1/clusters

List registered Kubernetes clusters.

**Authentication:** JWT with `clusters:read`

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": "c1c2c3c4-c5c6-c7c8-c9c0-c1c2c3c4c5c6",
        "name": "Production Cluster",
        "description": "Main production Kubernetes cluster",
        "api_endpoint": "https://k8s-api.prod.example.com:6443",
        "default_namespace": "chaos-sec",
        "status": "healthy",
        "kubernetes_version": "1.28.3",
        "node_count": 5,
        "last_connected_at": "2026-04-21T10:00:00Z",
        "created_at": "2026-01-15T10:00:00Z",
        "updated_at": "2026-04-21T10:00:00Z"
      }
    ],
    "total_count": 3
  }
}
```

---

### POST /api/v1/clusters

Register a new Kubernetes cluster.

**Authentication:** JWT with `clusters:write`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Cluster name |
| `description` | string | No | Cluster description |
| `api_endpoint` | string | Yes | Kubernetes API server URL |
| `ca_certificate` | string | Yes | Base64-encoded CA certificate |
| `client_certificate` | string | Yes | Base64-encoded client certificate |
| `client_key` | string | Yes | Base64-encoded client key |
| `default_namespace` | string | No | Default namespace (default: `chaos-sec`) |

**Response `201 Created`:** Returns the registered cluster with initial health status.

---

### GET /api/v1/clusters/:id

Get cluster details with live health information.

**Authentication:** JWT with `clusters:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Cluster ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "c1c2c3c4-c5c6-c7c8-c9c0-c1c2c3c4c5c6",
    "name": "Production Cluster",
    "description": "Main production Kubernetes cluster",
    "api_endpoint": "https://k8s-api.prod.example.com:6443",
    "default_namespace": "chaos-sec",
    "organization_id": "7da9a065-2b3c-4d5e-8f9a-b1c2d3e4f5a6",
    "status": "healthy",
    "kubernetes_version": "1.28.3",
    "node_count": 5,
    "live_info": {
      "healthy": true,
      "version": "1.28.3",
      "nodes": 5,
      "ready_nodes": 5,
      "error": null,
      "resources": {
        "cpu_capacity": "20",
        "memory_capacity": "64Gi"
      }
    },
    "last_connected_at": "2026-04-21T10:00:00Z",
    "created_at": "2026-01-15T10:00:00Z",
    "updated_at": "2026-04-21T10:00:00Z"
  }
}
```

---

### DELETE /api/v1/clusters/:id

Remove a registered cluster.

**Authentication:** JWT with `clusters:write`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Cluster ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "message": "Cluster deleted"
  }
}
```

---

### GET /api/v1/clusters/:id/namespaces

List namespaces in the cluster.

**Authentication:** JWT with `clusters:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Cluster ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "name": "default",
        "status": "Active",
        "labels": {},
        "created_at": "2026-01-15T10:00:00Z",
        "is_managed": false
      },
      {
        "name": "chaos-sec",
        "status": "Active",
        "labels": {"app.kubernetes.io/managed-by": "chaos-sec"},
        "created_at": "2026-01-15T10:05:00Z",
        "is_managed": true
      }
    ]
  }
}
```

---

### GET /api/v1/clusters/:id/network-policies

List network policies in the cluster's default namespace.

**Authentication:** JWT with `clusters:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Cluster ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "name": "deny-all-ingress",
        "namespace": "chaos-sec",
        "policy_types": ["Ingress"]
      },
      {
        "name": "allow-egress-dns",
        "namespace": "chaos-sec",
        "policy_types": ["Egress"]
      }
    ]
  }
}
```

---

### GET /api/v1/clusters/:id/health

Get detailed cluster health including per-node status and resource usage.

**Authentication:** JWT with `clusters:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | Cluster ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "cluster_id": "c1c2c3c4-c5c6-c7c8-c9c0-c1c2c3c4c5c6",
    "cluster_name": "Production Cluster",
    "status": "healthy",
    "healthy": true,
    "version": "1.28.3",
    "node_count": 5,
    "ready_nodes": 5,
    "nodes": [
      {
        "name": "node-1",
        "status": "Ready",
        "cpu": "1.2 cores",
        "memory": "8Gi / 16Gi"
      }
    ],
    "resources": {
      "cpu_capacity": "20",
      "cpu_usage_percent": 35,
      "memory_capacity": "64Gi",
      "memory_usage_percent": 42
    },
    "error": null,
    "checked_at": "2026-04-21T10:30:00Z"
  }
}
```

---

## 9. SIEM Endpoints

### GET /api/v1/siem/status

Get current SIEM connector status.

**Authentication:** JWT with `experiments:read`

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "connected": true,
    "provider": "splunk",
    "endpoint": "https://splunk.example.com:8089",
    "health": "healthy",
    "latency_ms": 42,
    "error": null,
    "timestamp": "2026-04-21T10:30:00Z",
    "metadata": {
      "version": "9.0.0",
      "index": "main"
    }
  }
}
```

---

### POST /api/v1/siem/test-connection

Test the SIEM connection by performing a health check.

> **Note:** The API design document (`04-api-design.md`) defines this endpoint as `POST /siem/test` with a request body containing `provider`, `endpoint`, and `credentials` for testing arbitrary SIEM connections. The implemented endpoint is `POST /siem/test-connection`, which tests the currently configured SIEM connection without requiring a request body. Both endpoints serve the same purpose; `test-connection` is the canonical endpoint name.

**Authentication:** JWT with `clusters:write`

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "success": true,
    "endpoint": "https://splunk.example.com:8089",
    "latency_ms": 42,
    "error": null
  }
}
```

**Error Response `502 Bad Gateway`:**
```json
{
  "success": false,
  "error": "siem_connection_failed",
  "message": "Failed to connect to SIEM",
  "code": 502
}
```

---

### POST /api/v1/siem/alerts/query

Query SIEM alerts with custom filters and time range.

**Authentication:** JWT with `experiments:read`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | RFC3339 | Yes | Start of time range |
| `to` | RFC3339 | Yes | End of time range |
| `alert_type` | string | No | Filter by alert type |
| `severity` | string | No | Filter by severity |
| `source` | string | No | Filter by source |
| `experiment_id` | UUID | No | Filter by experiment |
| `run_id` | UUID | No | Filter by run |
| `offset` | integer | No | Pagination offset (default: 0) |
| `limit` | integer | No | Pagination limit (default: 100, max: 1000) |

**Request Example:**
```json
{
  "from": "2026-04-21T09:00:00Z",
  "to": "2026-04-21T10:30:00Z",
  "alert_type": "network_flow",
  "severity": "high",
  "limit": 50
}
```

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "alerts": [
      {
        "id": "alert-001",
        "timestamp": "2026-04-21T10:01:30Z",
        "severity": "high",
        "type": "network_flow",
        "source": "chaos-engine",
        "description": "Egress traffic detected to 8.8.8.8:53",
        "metadata": {
          "namespace": "chaos-test",
          "pod_ip": "10.0.0.5",
          "protocol": "udp"
        },
        "experiment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "run_id": "r1r2r3r4-r5r6-r7r8-r9r0-r1r2r3r4r5r6"
      }
    ],
    "total": 3,
    "from": "2026-04-21T09:00:00Z",
    "to": "2026-04-21T10:30:00Z"
  }
}
```

---

### GET /api/v1/siem/alerts/:run_id

Get SIEM alerts for a specific experiment run.

> **Note:** The API design document (`04-api-design.md`) defines this endpoint as `GET /experiments/:id/siem-alerts`, which retrieves alerts scoped to an experiment ID with optional `from`/`to` query parameters and returns a `validation` object. The implemented endpoint is `GET /siem/alerts/:run_id`, which retrieves alerts scoped to a specific run ID and supports additional query parameters (`experiment_id`, `offset`, `limit`). Both serve the same purpose; `/siem/alerts/:run_id` is the canonical endpoint.

**Authentication:** JWT with `experiments:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `run_id` | UUID | Experiment run ID |

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `experiment_id` | UUID | — | Filter by experiment |
| `from` | RFC3339 | 24h ago | Start of time range |
| `to` | RFC3339 | now | End of time range |
| `offset` | integer | 0 | Pagination offset |
| `limit` | integer | 100 | Pagination limit |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "experiment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "run_id": "r1r2r3r4-r5r6-r7r8-r9r0-r1r2r3r4r5r6",
    "alerts": [ ... ],
    "total": 3
  }
}
```

---

## 10. Dashboard Endpoints

### GET /api/v1/dashboard/summary

Get dashboard summary data.

**Authentication:** JWT with `experiments:read`

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "security_posture_score": 82,
    "posture_trend": {
      "direction": "up",
      "percentage": 5.2,
      "period": "last_7_days"
    },
    "experiment_summary": {
      "total": 42,
      "running": 2,
      "completed": 35,
      "failed": 3,
      "pending": 2
    },
    "recent_experiments": [ ... ],
    "cluster_health": [ ... ],
    "threat_coverage": {
      "total_controls": 15,
      "validated": 12,
      "passed": 10,
      "failed": 2,
      "untested": 3,
      "coverage": 80
    }
  }
}
```

---

### GET /api/v1/dashboard/experiments/chart

Get experiment trend data for charts.

**Authentication:** JWT with `experiments:read`

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `period` | string | `7d` | Time period: `7d`, `30d`, `90d` |
| `granularity` | string | `day` | Data granularity: `hour`, `day`, `week` |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "labels": ["2026-04-15", "2026-04-16", "2026-04-17", "2026-04-18", "2026-04-19", "2026-04-20", "2026-04-21"],
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

---

### GET /api/v1/dashboard/security-controls

Get security control validation status.

**Authentication:** JWT with `experiments:read`

**Response `200 OK`:**
```json
{
  "success": true,
  "data": [
    {
      "control_id": "ctrl_001",
      "name": "Pod Egress Restriction",
      "type": "network_policy",
      "status": "validated",
      "last_tested": "2026-04-21T09:00:00Z",
      "test_count": 45,
      "pass_rate": 100
    },
    {
      "control_id": "ctrl_002",
      "name": "Ingress Traffic Filtering",
      "type": "network_policy",
      "status": "failed",
      "last_tested": "2026-04-21T08:00:00Z",
      "test_count": 30,
      "pass_rate": 73.3,
      "failure_reason": "Unexpected ingress traffic allowed from 10.0.0.0/8"
    }
  ]
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 500 | `internal_error` | Failed to retrieve security control data |

---

## 11. Report Endpoints

### GET /api/v1/reports/:experimentId

Get a generated experiment report.

**Authentication:** JWT with `experiments:read`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `experimentId` | UUID | Experiment ID |

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `format` | string | `json` | Report format: `pdf`, `csv`, `json`, `html` |

**Response `200 OK` (JSON format):**
```json
{
  "success": true,
  "data": {
    "experiment": { ... },
    "runs": [ ... ],
    "summary": {
      "total_pods_spawned": 5,
      "successful_attacks": 3,
      "blocked_attacks": 2,
      "siem_alerts_expected": 3,
      "siem_alerts_received": 3,
      "detection_rate": 100.0,
      "overall_status": "passed",
      "findings": [
        {
          "severity": "high",
          "description": "Network policy gap: egress to 8.8.8.8 not blocked",
          "recommendation": "Add network policy to deny egress to external DNS"
        }
      ]
    }
  }
}
```

**Response `200 OK` (PDF format):** Returns `application/pdf` binary content with `Content-Disposition: attachment; filename=report_<experiment_id>.pdf`.

---

## 12. Health Endpoints

### GET /health/live

Liveness probe — verifies the process is alive.

**Authentication:** None

**Response `200 OK`:**
```json
{
  "status": "alive",
  "timestamp": "2026-04-21T10:30:00Z"
}
```

---

### GET /health/ready

Readiness probe — verifies the process is ready to serve traffic (checks database and Redis connectivity).

**Authentication:** None

**Response `200 OK`:**
```json
{
  "status": "ready",
  "checks": {
    "database": "healthy",
    "redis": "healthy"
  },
  "timestamp": "2026-04-21T10:30:00Z"
}
```

**Response `503 Service Unavailable`:**
```json
{
  "status": "not_ready",
  "checks": {
    "database": "healthy",
    "redis": "unhealthy: connection refused"
  },
  "timestamp": "2026-04-21T10:30:00Z"
}
```

---

## 13. User Management Endpoints

### GET /api/v1/users

List all users in the organization.

**Authentication:** JWT with `admin:all` or `users:manage`

**Query Parameters:**

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `page` | integer | 1 | Page number |
| `per_page` | integer | 20 | Items per page (max: 100) |
| `sort` | string | `created_at_desc` | Sort field and direction |
| `search` | string | — | Search by name or email |
| `role` | string | — | Filter by role (`admin`, `operator`, `viewer`) |
| `is_active` | boolean | — | Filter by active status |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "items": [
      {
        "id": "u1u2u3u4-u5u6-u7u8-u9u0-u1u2u3u4u5u6",
        "email": "admin@chaos-sec.local",
        "name": "Admin User",
        "is_active": true,
        "role": {
          "id": "r1r2r3r4-r5r6-r7r8-r9r0-r1r2r3r4r5r6",
          "name": "admin",
          "description": "Full administrative access",
          "permissions": ["admin:all", "users:manage", "experiments:read", "experiments:write", "experiments:execute", "experiments:delete", "templates:read", "templates:write", "clusters:read", "clusters:write"]
        },
        "organization_id": "o1o2o3o4-o5o6-o7o8-o9o0-o1o2o3o4o5o6",
        "last_login_at": "2026-04-21T08:30:00Z",
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-04-21T08:30:00Z"
      }
    ],
    "total_count": 5,
    "page": 1,
    "page_size": 20,
    "total_pages": 1,
    "has_next_page": false,
    "has_previous_page": false
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 403 | `forbidden` | User does not have `admin:all` or `users:manage` permission |

---

### POST /api/v1/users

Create a new user.

**Authentication:** JWT with `admin:all` or `users:manage`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string | Yes | User email address |
| `password` | string | Yes | Initial password (min 8 characters) |
| `name` | string | Yes | Display name |
| `role_id` | UUID | Yes | Role ID to assign |
| `organization_id` | UUID | No | Organization ID (defaults to caller's org) |

**Request Example:**
```json
{
  "email": "newuser@chaos-sec.local",
  "password": "secure_password_123",
  "name": "New Operator",
  "role_id": "r2r3r4r5-r6r7-r8r9-r0r1-r2r3r4r5r6r7",
  "organization_id": "o1o2o3o4-o5o6-o7o8-o9o0-o1o2o3o4o5o6"
}
```

**Response `201 Created`:**
```json
{
  "success": true,
  "data": {
    "id": "u2u3u4u5-u6u7-u8u9-u0u1-u2u3u4u5u6u7",
    "email": "newuser@chaos-sec.local",
    "name": "New Operator",
    "is_active": true,
    "role": {
      "id": "r2r3r4r5-r6r7-r8r9-r0r1-r2r3r4r5r6r7",
      "name": "operator",
      "description": "Standard operator access"
    },
    "organization_id": "o1o2o3o4-o5o6-o7o8-o9o0-o1o2o3o4o5o6",
    "created_at": "2026-04-21T10:30:00Z"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `validation_error` | Missing or invalid request body fields |
| 403 | `forbidden` | User does not have `admin:all` or `users:manage` permission |
| 409 | `conflict` | Email already exists |

---

### PUT /api/v1/users/:id

Update user details.

**Authentication:** JWT with `admin:all` or `users:manage`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | User ID |

**Request Body:** All fields optional — only provided fields are updated.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string | No | New email address |
| `name` | string | No | New display name |
| `role_id` | UUID | No | New role to assign |
| `is_active` | boolean | No | Activate or deactivate user |

**Request Example:**
```json
{
  "email": "updated@chaos-sec.local",
  "role_id": "r3r4r5r6-r7r8-r9r0-r1r2-r3r4r5r6r7r8",
  "is_active": true
}
```

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "id": "u2u3u4u5-u6u7-u8u9-u0u1-u2u3u4u5u6u7",
    "email": "updated@chaos-sec.local",
    "name": "New Operator",
    "is_active": true,
    "role": {
      "id": "r3r4r5r6-r7r8-r9r0-r1r2-r3r4r5r6r7r8",
      "name": "viewer",
      "description": "Read-only access"
    },
    "organization_id": "o1o2o3o4-o5o6-o7o8-o9o0-o1o2o3o4o5o6",
    "updated_at": "2026-04-21T11:00:00Z"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `validation_error` | Invalid request body fields |
| 403 | `forbidden` | User does not have `admin:all` or `users:manage` permission |
| 404 | `not_found` | User ID does not exist |
| 409 | `conflict` | Email already in use by another user |

---

### DELETE /api/v1/users/:id

Delete a user.

**Authentication:** JWT with `admin:all` or `users:manage`

**Path Parameters:**

| Parameter | Type | Description |
|-----------|------|-------------|
| `id` | UUID | User ID |

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "message": "User deleted"
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 403 | `forbidden` | User does not have `admin:all` or `users:manage` permission |
| 404 | `not_found` | User ID does not exist |
| 409 | `conflict` | Cannot delete the last admin user |

---

## 14. Settings & Configuration Endpoints

### GET /api/v1/settings

Get current system settings.

**Authentication:** JWT with `admin:all`

**Response `200 OK`:**
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

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 403 | `forbidden` | User does not have `admin:all` permission |

---

### PUT /api/v1/settings

Update system settings. Only provided fields are updated; omitted fields remain unchanged.

**Authentication:** JWT with `admin:all`

**Request Body:**

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `general` | object | No | General platform settings |
| `general.platform_name` | string | No | Platform display name |
| `general.default_timeout_seconds` | integer | No | Default experiment timeout (default: 600) |
| `general.max_concurrent_experiments` | integer | No | Maximum concurrent experiments (default: 5) |
| `kubernetes` | object | No | Kubernetes connection settings |
| `kubernetes.default_namespace` | string | No | Default namespace for experiments |
| `kubernetes.cleanup_on_failure` | boolean | No | Clean up resources on failure |
| `kubernetes.pod_security_context` | object | No | Pod security context settings |
| `siem` | object | No | SIEM integration settings |
| `siem.enabled` | boolean | No | Enable SIEM integration |
| `siem.provider` | string | No | SIEM provider (`splunk`, `elastic`, `mock`) |
| `siem.sync_interval_seconds` | integer | No | SIEM sync interval |
| `notifications` | object | No | Notification settings |
| `notifications.email_enabled` | boolean | No | Enable email notifications |
| `notifications.slack_enabled` | boolean | No | Enable Slack notifications |
| `notifications.slack_webhook_url` | string | No | Slack webhook URL |

**Request Example:**
```json
{
  "general": {
    "default_timeout_seconds": 900,
    "max_concurrent_experiments": 10
  },
  "siem": {
    "provider": "elastic",
    "sync_interval_seconds": 30
  },
  "notifications": {
    "email_enabled": true,
    "email_recipients": ["admin@example.com", "security@example.com"]
  }
}
```

**Response `200 OK`:**
```json
{
  "success": true,
  "data": {
    "general": {
      "platform_name": "Chaos-Sec",
      "default_timeout_seconds": 900,
      "max_concurrent_experiments": 10
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
      "provider": "elastic",
      "sync_interval_seconds": 30
    },
    "notifications": {
      "email_enabled": true,
      "slack_enabled": true,
      "slack_webhook_url": "https://hooks.slack.com/..."
    }
  }
}
```

**Error Responses:**

| Status | Code | When |
|--------|------|------|
| 400 | `validation_error` | Invalid setting values |
| 403 | `forbidden` | User does not have `admin:all` permission |

---

## 15. WebSocket API

### Connection

Connect to the WebSocket endpoint:

```
ws://localhost:8080/ws
```

Or with TLS:
```
wss://app.chaos-sec.io/ws
```

### Authentication

Send a connection message with the JWT token after connecting:

```json
{
  "type": "auth",
  "payload": {
    "token": "eyJhbGciOiJIUzI1NiIs..."
  }
}
```

### Event Types

| Event Type | Direction | Description |
|------------|-----------|-------------|
| `experiment:started` | Server → Client | An experiment has started running |
| `experiment:progress` | Server → Client | Progress update for a running experiment |
| `experiment:step_completed` | Server → Client | An attack step has completed |
| `experiment:step_failed` | Server → Client | An attack step has failed |
| `experiment:completed` | Server → Client | An experiment has completed |
| `experiment:failed` | Server → Client | An experiment has failed |
| `experiment:cancelled` | Server → Client | An experiment was cancelled |
| `experiment:notifications` | Server → Client | A notification event |
| `experiment:logs` | Server → Client | Log output from an experiment |
| `cluster:health` | Server → Client | Cluster health update |
| `cluster:status` | Server → Client | Cluster status change |
| `cluster:resource_update` | Server → Client | Cluster resource usage update |
| `siem:alert` | Server → Client | New SIEM alert received |
| `siem:validation` | Server → Client | SIEM validation result |
| `system:notification` | Server → Client | System-wide notification |

### Event Message Format

```json
{
  "type": "experiment:progress",
  "payload": {
    "experiment_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
    "progress": 65,
    "current_step": "Executing egress test",
    "status": "running"
  },
  "timestamp": "2026-04-21T10:30:00Z",
  "id": "msg-001"
}
```

### Client-to-Server Messages

| Message Type | Description |
|--------------|-------------|
| `auth` | Authenticate the WebSocket connection |
| `ping` | Client heartbeat (server responds with `pong`) |

### Heartbeat

The server sends `system:ping` events periodically. The client should respond with a `pong` message. If no response is received, the server may close the connection.

### Reconnection

The WebSocket client implements automatic reconnection with exponential backoff (1s, 2s, 4s, 8s, capped at 30s). Maximum reconnection attempts are configurable (default: 10).

---

## 16. Error Codes

### Standard Error Codes

| HTTP Status | Error Code | Description |
|-------------|-----------|-------------|
| 400 | `invalid_request` | Malformed or missing request parameters |
| 401 | `unauthorized` | No valid authentication token provided |
| 401 | `invalid_token` | JWT token is invalid or malformed |
| 401 | `expired_token` | JWT token has expired |
| 401 | `token_revoked` | JWT token has been blacklisted |
| 403 | `forbidden` | Insufficient permissions for the requested resource |
| 404 | `not_found` | Requested resource does not exist |
| 409 | `conflict` | Resource already exists (e.g., duplicate email) |
| 429 | `rate_limit_exceeded` | Too many requests; slow down |
| 500 | `internal_error` | Unexpected server error |
| 502 | `siem_connection_failed` | SIEM backend unreachable |
| 503 | `service_unavailable` | Dependency (DB, Redis) unavailable |

### Validation Error Format

```json
{
  "success": false,
  "error": "invalid_request",
  "message": "Validation failed",
  "code": 400,
  "errors": [
    {
      "code": "validation_error",
      "message": "Name is required",
      "field": "name"
    },
    {
      "code": "validation_error",
      "message": "Password must be at least 8 characters",
      "field": "password"
    }
  ]
}
```

---

## 17. Rate Limiting

### Configuration

| Setting | Default |
|---------|---------|
| Enabled | Yes |
| Max requests per window | 100 |
| Window duration | 60 seconds |

### Rate Limit Identification

| Context | Key Format |
|---------|------------|
| Authenticated user | `rl:user:<user-uuid>` |
| Unauthenticated (by IP) | `rl:ip:<client-ip>` |

### Response Headers

On allowed requests:

| Header | Description |
|--------|-------------|
| `X-RateLimit-Limit` | Maximum requests per window |
| `X-RateLimit-Window` | Time window duration |

On rate-limited requests:

**Status:** `429 Too Many Requests`

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests. Please slow down.",
  "code": 429
}
```

### Rate Limiting Behavior

- **Redis available:** Uses a sliding window counter with `INCR` and `EXPIRE` for distributed rate limiting across backend replicas.
- **Redis unavailable:** Falls back to an in-memory rate limiter. Fails open (allows the request) if the in-memory limiter encounters an error.
- **Admin users:** Subject to the same rate limits as other users. There is no rate limit bypass for admin accounts.

---

## Appendix

### A. Request/Response Headers

#### Request Headers

| Header | Required | Description |
|--------|----------|-------------|
| `Authorization` | Yes* | `Bearer <access_token>` |
| `Content-Type` | Yes | `application/json` |
| `X-Request-ID` | No | Client-provided request ID (UUID recommended) |
| `X-API-Key` | No | Alternative to Bearer token (future) |

\* Not required for `POST /auth/login`, `POST /auth/refresh`, and health endpoints.

#### Response Headers

| Header | Description |
|--------|-------------|
| `X-Request-ID` | Request ID (echoed or server-generated) |
| `X-Frame-Options` | `DENY` |
| `X-Content-Type-Options` | `nosniff` |
| `X-XSS-Protection` | `1; mode=block` |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` (production) |
| `Referrer-Policy` | `strict-origin-when-cross-origin` |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` |
| `X-RateLimit-Limit` | Rate limit max (on allowed requests) |
| `X-RateLimit-Window` | Rate limit window (on allowed requests) |

### B. Data Types

| Type | Format | Example |
|------|--------|---------|
| UUID | Lowercase, hyphenated | `a1b2c3d4-e5f6-7890-abcd-ef1234567890` |
| Timestamp | ISO 8601 / RFC 3339 | `2026-04-21T10:30:00Z` |
| Duration | Go duration string | `5m`, `300s` |
| Severity | Enumerated | `low`, `medium`, `high`, `critical` |
| Experiment Status | Enumerated | `pending`, `queued`, `running`, `completed`, `failed`, `stopped`, `timed_out` |
| Cluster Status | Enumerated | `healthy`, `degraded`, `unreachable`, `unknown` |
| Report Format | Enumerated | `pdf`, `csv`, `json`, `html` |

### C. cURL Examples

#### Login and List Experiments

```bash
# Step 1: Login
TOKEN=$(curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@chaos-sec.local","password":"admin"}' \
  | jq -r '.data.access_token')

# Step 2: List experiments
curl -s http://localhost:8080/api/v1/experiments \
  -H "Authorization: Bearer $TOKEN" \
  | jq '.data.items[] | {name, status}'
```

#### Create and Execute an Experiment

```bash
# Create
EXP_ID=$(curl -s -X POST http://localhost:8080/api/v1/experiments \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Quick Egress Test",
    "template_id": "<template-uuid>",
    "cluster_id": "<cluster-uuid>",
    "namespace": "chaos-test",
    "parameters": {"target_url": "8.8.8.8", "duration_seconds": 30},
    "validation": {"siem_alert_type": "network_flow", "time_window_seconds": 300}
  }' \
  | jq -r '.data.id')

# Execute
curl -s -X POST "http://localhost:8080/api/v1/experiments/$EXP_ID/execute" \
  -H "Authorization: Bearer $TOKEN"
```

#### Generate a PDF Report

```bash
curl -s "http://localhost:8080/api/v1/reports/$EXP_ID?format=pdf" \
  -H "Authorization: Bearer $TOKEN" \
  -o report.pdf
```

---

**Document Version:** 1.0.0
**Last Updated:** 2026-04-21