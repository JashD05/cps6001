# Chaos-Sec Administrator Guide

**Version:** 1.0.0
**Last Updated:** 2026-04-21

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [System Requirements](#2-system-requirements)
3. [Installation](#3-installation)
4. [Configuration Reference](#4-configuration-reference)
5. [Database Administration](#5-database-administration)
6. [User Management](#6-user-management)
7. [Kubernetes Cluster Management](#7-kubernetes-cluster-management)
8. [SIEM Integration](#8-siem-integration)
9. [Monitoring & Observability](#9-monitoring--observability)
10. [Backup & Recovery](#10-backup--recovery)
11. [Security Administration](#11-security-administration)
12. [Troubleshooting](#12-troubleshooting)
13. [Upgrading](#13-upgrading)
14. [API Quick Reference](#14-api-quick-reference)

---

## 1. Introduction

### 1.1 Purpose

This guide is for **system administrators, DevOps engineers, and platform operators** responsible for installing, configuring, and maintaining Chaos-Sec. It covers everything from initial deployment to day-to-day operations and disaster recovery.

For end-user documentation (running experiments, viewing reports), see the [User Guide](./12-user-guide.md).

### 1.2 Architecture Overview

Chaos-Sec consists of the following components:

| Component | Technology | Purpose |
|-----------|------------|---------|
| **Backend API** | Go 1.21+ / Gin | REST API server, experiment orchestration |
| **Frontend Dashboard** | React 18 / TypeScript | Web-based control panel |
| **Database** | PostgreSQL 15+ | Persistent storage for experiments, users, results |
| **Cache** | Redis 7+ | Session management, rate limiting, token blacklisting |
| **Mock SIEM** | Go / Docker | Simulated SIEM for development and testing |
| **Target Cluster** | Kubernetes 1.28+ | Target environment for attack simulation |

### 1.3 Network Topology

```
┌──────────────┐     ┌──────────────────────────────────────────┐
│   Users      │────▶│          Ingress (nginx)                  │
│   (Browser)  │     │   /api/*  ──▶  Backend :8080             │
└──────────────┘     │   /ws/*   ──▶  Backend :8080 (WebSocket) │
                     │   /       ──▶  Frontend :80 (Nginx)       │
                     └──────────────────────────────────────────┘
                                       │
                    ┌──────────────────┼──────────────────┐
                    ▼                  ▼                  ▼
             ┌──────────┐      ┌──────────┐      ┌──────────────┐
             │ PostgreSQL│      │  Redis   │      │  K8s Cluster │
             │  :5432    │      │  :6379   │      │  (Target)     │
             └──────────┘      └──────────┘      └──────────────┘
```

---

## 2. System Requirements

### 2.1 Hardware Requirements

#### Minimum (Development)

| Resource | Specification |
|----------|---------------|
| CPU | 4 cores |
| RAM | 8 GB |
| Storage | 50 GB SSD |
| Network | Local development |

#### Recommended (Production)

| Resource | Specification |
|----------|---------------|
| CPU | 8 cores (16 recommended) |
| RAM | 16 GB (32 recommended) |
| Storage | 100 GB SSD (500 GB for extended retention) |
| Network | 1 Gbps low-latency connection to target clusters |

#### Kubernetes Worker Node Sizing

| Component | CPU Request | Memory Request | CPU Limit | Memory Limit |
|-----------|-------------|----------------|-----------|--------------|
| Backend | 250m | 256Mi | 500m | 512Mi |
| Frontend | 100m | 128Mi | 200m | 256Mi |
| PostgreSQL | 250m | 256Mi | 500m | 1Gi |
| Redis | 100m | 128Mi | 250m | 512Mi |

### 2.2 Software Requirements

| Software | Version | Purpose |
|----------|---------|---------|
| Go | 1.21+ | Backend development |
| Node.js | 18.x LTS | Frontend development |
| Docker | 24.0+ | Container runtime |
| kubectl | 1.28+ | Kubernetes CLI |
| kind | 0.20+ | Local Kubernetes cluster (dev) |
| PostgreSQL | 15+ | Primary database |
| Redis | 7+ | Cache and rate limiting |
| Terraform | 1.6+ | Infrastructure provisioning (optional) |
| Helm | 3.12+ | Package management (optional) |
| Git | 2.40+ | Version control |

### 2.3 Kubernetes Cluster Requirements

| Requirement | Minimum | Recommended |
|-------------|---------|-------------|
| Kubernetes Version | 1.26+ | 1.28+ |
| Control Plane Nodes | 1 | 3 |
| Worker Nodes | 2 | 3+ |
| Storage Class | Available | SSD-backed |
| Pod Security Standards | — | `restricted` profile |

---

## 3. Installation

### 3.1 Quick Start (Development)

The fastest way to get Chaos-Sec running locally:

```bash
# 1. Clone the repository
git clone <repository-url>
cd Individual-Project

# 2. Start infrastructure services (PostgreSQL, Redis, Mock SIEM)
docker compose up -d

# 3. Set up the backend
cd backend
go mod download
go run cmd/backend/main.go migrate    # Run database migrations
go run cmd/backend/main.go serve      # Start API server

# 4. Set up the frontend (in a new terminal)
cd frontend
npm install
npm run dev

# 5. Create a local Kubernetes cluster (optional)
kind create cluster --name chaos-sec-dev --config scripts/kind-config.yaml
```

Or use the automated setup script:

```bash
./scripts/setup-dev.sh
```

Options: `--skip-kind` (skip Kubernetes cluster), `--skip-infra` (skip Docker Compose).

### 3.2 Production Deployment (Kubernetes)

#### Step 1: Prepare Secrets

Create Kubernetes secrets with production values:

```bash
# Generate strong secrets
JWT_SECRET=$(openssl rand -base64 32)
DB_PASSWORD=$(openssl rand -base64 24)
REDIS_PASSWORD=$(openssl rand -base64 24)
ENCRYPTION_KEY=$(openssl rand -base64 32)

# Apply secrets (interactive — prompts for each value)
./deploy/scripts/deploy.sh
```

Or manually:

```bash
kubectl create namespace chaos-sec --dry-run=client -o yaml | kubectl apply -f -

kubectl create secret generic chaos-sec-secrets \
  --namespace chaos-sec \
  --from-literal=DATABASE_PASSWORD="$DB_PASSWORD" \
  --from-literal=REDIS_PASSWORD="$REDIS_PASSWORD" \
  --from-literal=JWT_SECRET="$JWT_SECRET" \
  --from-literal=SIEM_API_KEY="" \
  --from-literal=ENCRYPTION_KEY="$ENCRYPTION_KEY"
```

> **Warning:** Never commit secrets to version control. Use a secrets management solution (HashiCorp Vault, AWS Secrets Manager, etc.) in production.

#### Step 2: Apply Configuration

Edit `deploy/kubernetes/configmap.yaml` with your environment values, then apply:

```bash
kubectl apply -f deploy/kubernetes/configmap.yaml
kubectl apply -f deploy/kubernetes/secrets.yaml
```

#### Step 3: Deploy Infrastructure

```bash
kubectl apply -f deploy/kubernetes/postgres-statefulset.yaml
kubectl apply -f deploy/kubernetes/redis-statefulset.yaml

# Wait for databases to be ready
kubectl rollout status statefulset/chaos-sec-postgres -n chaos-sec
kubectl rollout status statefulset/chaos-sec-redis -n chaos-sec
```

#### Step 4: Deploy Application

```bash
kubectl apply -f deploy/kubernetes/backend-deployment.yaml
kubectl apply -f deploy/kubernetes/frontend-deployment.yaml
kubectl apply -f deploy/kubernetes/backend-service.yaml
kubectl apply -f deploy/kubernetes/frontend-service.yaml

# Wait for rollouts
kubectl rollout status deployment/chaos-sec-backend -n chaos-sec
kubectl rollout status deployment/chaos-sec-frontend -n chaos-sec
```

#### Step 5: Configure Ingress

```bash
kubectl apply -f deploy/kubernetes/ingress.yaml
```

#### Step 6: Set Up Monitoring

```bash
kubectl apply -f deploy/monitoring/prometheus-alerts.yaml
```

#### Step 7: Verify Deployment

```bash
# Run smoke tests
./deploy/scripts/smoke-tests.sh
```

Or manually verify:

```bash
# Health checks
curl http://localhost:8080/health/live
curl http://localhost:8080/health/ready

# Authentication
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@chaos-sec.local","password":"admin"}'
```

### 3.3 Docker Compose (Staging)

For staging environments that don't use Kubernetes:

```bash
# Start all services
docker compose -f docker-compose.yml up -d

# Run migrations
docker compose exec backend sh -c "go run cmd/backend/main.go migrate"

# View logs
docker compose logs -f backend
docker compose logs -f frontend
```

### 3.4 Default Credentials

| Service | Username | Password | Notes |
|---------|----------|----------|-------|
| PostgreSQL | `chaossec` | `chaossec_dev` | Change in production! |
| Backend Admin | `admin@chaos-sec.local` | `admin` | Change on first login! |
| Mock SIEM | — | — | No authentication |

> **Critical:** Change all default credentials before deploying to any shared or production environment.

---

## 4. Configuration Reference

All configuration is loaded from **environment variables** with the `CHAOS_` prefix. Defaults are applied automatically when env vars are not set.

### 4.1 Server Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_SERVER_HOST` | `0.0.0.0` | Server bind address |
| `CHAOS_SERVER_PORT` | `8080` | Server listen port |
| `CHAOS_SERVER_READ_TIMEOUT` | `15s` | HTTP read timeout |
| `CHAOS_SERVER_WRITE_TIMEOUT` | `15s` | HTTP write timeout |
| `CHAOS_SERVER_IDLE_TIMEOUT` | `60s` | HTTP idle timeout |

### 4.2 Database Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_DB_HOST` | `localhost` | PostgreSQL host |
| `CHAOS_DB_PORT` | `5432` | PostgreSQL port |
| `CHAOS_DB_NAME` | `chaos_sec` | Database name |
| `CHAOS_DB_USER` | `chaos_sec` | Database user |
| `CHAOS_DB_PASSWORD` | `chaos_sec_dev` | Database password (**secret**) |
| `CHAOS_DB_SSLMODE` | `disable` | SSL mode |
| `CHAOS_DB_MAX_OPEN_CONNS` | `25` | Max open connections |
| `CHAOS_DB_MAX_IDLE_CONNS` | `5` | Max idle connections |
| `CHAOS_DB_CONN_MAX_LIFETIME` | `5m` | Connection max lifetime |
| `CHAOS_DB_CONN_MAX_IDLE_TIME` | `1m` | Connection max idle time |
| `CHAOS_DB_MIGRATIONS_PATH` | `file://migrations` | Migration files path |

**Valid SSL modes:** `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full`

**Connection string format:**
```
postgres://chaos_sec:<password>@localhost:5432/chaos_sec?sslmode=disable
```

### 4.3 Redis Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_REDIS_HOST` | `localhost` | Redis host |
| `CHAOS_REDIS_PORT` | `6379` | Redis port |
| `CHAOS_REDIS_PASSWORD` | *(empty)* | Redis password (**secret**) |
| `CHAOS_REDIS_DB` | `0` | Redis database number |

### 4.4 JWT Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_JWT_SECRET` | *(empty)* | Signing key (**secret**) |
| `CHAOS_JWT_EXPIRY` | `1h` | Access token expiry |
| `CHAOS_JWT_REFRESH_EXPIRY` | `168h` (7 days) | Refresh token expiry |
| `CHAOS_JWT_ISSUER` | `chaos-sec` | JWT issuer claim |

> **Security Warning:** If `CHAOS_JWT_SECRET` is empty, the system uses an insecure development secret with a warning log. **Never run production without a strong JWT secret.** Generate one with: `openssl rand -base64 64`

**Token validation rules:**
- Access token expiry must be ≥ 1 minute
- Refresh token expiry must be ≥ 1 hour
- Signing algorithm is enforced as HMAC SHA-256 (`HS256`)
- Tokens with `algorithm: none` are rejected

### 4.5 SIEM Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_SIEM_ENABLED` | `false` | Enable SIEM integration |
| `CHAOS_SIEM_PROVIDER` | *(empty)* | SIEM provider name |
| `CHAOS_SIEM_ENDPOINT` | *(empty)* | SIEM API endpoint URL |
| `CHAOS_SIEM_API_KEY` | *(empty)* | SIEM API key (**secret**) |

**Supported providers:** `splunk`, `elastic`, `sentinel`, `other`

### 4.6 Kubernetes Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_K8S_NAMESPACE` | `chaos-sec` | Default namespace |
| `CHAOS_K8S_POD_TIMEOUT` | `5m` | Pod operation timeout |
| `CHAOS_K8S_MAX_CONCURRENT` | `10` | Max concurrent K8s operations |
| `CHAOS_K8S_IN_CLUSTER` | `false` | Running inside a cluster? |
| `CHAOS_K8S_KUBECONFIG` | *(empty)* | Path to kubeconfig file |

When `CHAOS_K8S_IN_CLUSTER=true`, the service account token is used instead of a kubeconfig file. In the Kubernetes deployment, this is set automatically via the downward API.

### 4.7 Rate Limiting Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_RATE_LIMIT_ENABLED` | `true` | Enable rate limiting |
| `CHAOS_RATE_LIMIT_REQUESTS` | `100` | Max requests per window |
| `CHAOS_RATE_LIMIT_WINDOW` | `60s` | Rate limit time window |

Rate limiting uses Redis when available (sliding window counter), falling back to an in-memory limiter when Redis is unavailable. The in-memory limiter fails open (allows the request) if there are errors.

**Rate limit keys:**
- Authenticated users: `rl:user:<uuid>`
- Unauthenticated requests: `rl:ip:<client-ip>`

### 4.8 Logging Configuration

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `CHAOS_LOG_LEVEL` | `info` | Log level |
| `CHAOS_LOG_FORMAT` | `json` | Log format |

**Valid log levels:** `debug`, `info`, `warn`, `error`, `fatal`

**Valid log formats:** `json` (machine-readable), `console` (human-readable)

> **Tip:** Use `json` format in production for log aggregation tools (ELK, CloudWatch, Datadog). Use `console` in development.

### 4.9 Configuration Validation

The application validates all configuration on startup. Invalid values cause the server to exit with an error message. Validation rules include:

- Server port is required
- Database host, name, and user are required
- SSL mode must be one of the valid values
- Max open/idle connections must be ≥ 1
- JWT expiry must be ≥ 1 minute; refresh expiry must be ≥ 1 hour
- Kubernetes MaxConcurrent must be ≥ 1
- Log level must be a valid level
- Log format must be `json` or `console`

### 4.10 Environment File

Create a `.env` file in the project root for local development:

```bash
# Backend
APP_ENV=development
DATABASE_URL=postgres://chaossec:chaossec_dev@localhost:5432/chaossec?sslmode=disable
REDIS_URL=redis://localhost:6379
KUBECONFIG=~/.kube/config
JWT_SECRET=dev-secret-change-in-production
JWT_REFRESH_SECRET=dev-refresh-secret-change-in-production
SERVER_PORT=8080

# Frontend
VITE_API_URL=http://localhost:8080/api/v1
VITE_WS_URL=ws://localhost:8080/ws
```

---

## 5. Database Administration

### 5.1 PostgreSQL Management

#### Connection

```bash
# Via psql
psql -h localhost -U chaossec -d chaos_sec

# Via Docker
docker compose exec postgres psql -U chaossec -d chaos_sec
```

#### Connection Pooling

Chaos-Sec configures connection pooling via these settings:

| Setting | Default | Description |
|---------|---------|-------------|
| `MaxOpenConns` | 25 | Maximum open connections |
| `MaxIdleConns` | 5 | Maximum idle connections |
| `ConnMaxLifetime` | 5m | Maximum connection lifetime |
| `ConnMaxIdleTime` | 1m | Maximum idle connection lifetime |

Monitor pool usage with the query:

```sql
SELECT count(*) AS total,
       count(*) FILTER (WHERE state = 'active') AS active,
       count(*) FILTER (WHERE state = 'idle') AS idle
FROM pg_stat_activity
WHERE datname = 'chaos_sec';
```

### 5.2 Database Migrations

Migrations are managed using `golang-migrate`. The helper script is at `scripts/migrate.sh`.

#### Commands

```bash
# Run all pending migrations
./scripts/migrate.sh up

# Roll back the most recent migration
./scripts/migrate.sh down

# Create a new migration
./scripts/migrate.sh create add_experiment_templates

# Check migration status
./scripts/migrate.sh status

# Force-set migration version (recovery only!)
./scripts/migrate.sh force 42
```

The migration files are stored in `backend/migrations/` as pairs:
- `000001_init.up.sql` — Forward migration
- `000001_init.down.sql` — Rollback migration

#### Connection String Resolution

The script resolves the database URL in this order:
1. `DATABASE_URL` environment variable
2. `.env` file in the project root
3. Default: `postgres://chaossec:chaossec_dev@localhost:5432/chaossec?sslmode=disable`

### 5.3 Database Optimization

Use the optimization script for diagnostics:

```bash
# Generate a diagnostic report
./scripts/db-optimize.sh --report

# Run ANALYZE to update statistics
./scripts/db-optimize.sh --analyze

# Full vacuum (locks tables!)
./scripts/db-optimize.sh --full-vacuum
```

The script reports:
- Unused indexes
- VACUUM candidates
- Slow queries
- Table sizes

### 5.4 Common SQL Queries

```sql
-- List all experiments with status
SELECT id, name, status, created_at FROM experiments ORDER BY created_at DESC LIMIT 10;

-- Count experiments by status
SELECT status, count(*) FROM experiments GROUP BY status;

-- Recent failed runs
SELECT r.id, r.experiment_id, r.status, r.error_message, r.started_at
FROM experiment_runs r
WHERE r.status = 'failed'
ORDER BY r.started_at DESC LIMIT 10;

-- User activity
SELECT u.email, u.last_login_at, r.name AS role_name
FROM users u JOIN roles r ON u.role_id = r.id
ORDER BY u.last_login_at DESC;
```

---

## 6. User Management

### 6.1 Roles and Permissions

Chaos-Sec implements role-based access control with three predefined roles:

| Role | Description | Key Permissions |
|------|-------------|-----------------|
| **Admin** | Full system access | All permissions + `admin:all` + `users:manage` |
| **Operator** | Day-to-day operations | `experiments:*`, `templates:*`, `clusters:*` |
| **Viewer** | Read-only access | `experiments:read`, `templates:read`, `clusters:read` |

### 6.2 Complete Permission Reference

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

> **Note:** The `admin:all` permission bypasses all RBAC checks. Users with this permission can access everything regardless of other permission checks.

### 6.3 Creating Users

Users are created via the API. Registration requires an authenticated user with `admin:all` or `users:manage` permission.

```bash
# Create a new user
curl -X POST https://app.chaos-sec.io/api/v1/auth/register \
  -H "Authorization: Bearer <admin_token>" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "operator@company.com",
    "password": "SecureP@ss123",
    "name": "Jane Operator",
    "organization_id": "<org-uuid>",
    "role_id": "<role-uuid>"
  }'
```

**Password requirements:**
- Minimum 8 characters
- bcrypt cost factor: 12 (~250ms per hash)

**Important constraints:**
- Non-admin users can only create users within their own organization
- Email addresses must be unique across the system
- The organization must exist and be active
- The role must exist in the system

### 6.4 Managing User Sessions

#### Token Lifecycle

| Token Type | Default Expiry | Purpose |
|------------|----------------|---------|
| Access Token | 1 hour | API authentication |
| Refresh Token | 7 days | Obtain new access tokens |

#### Token Blacklisting

When a user logs out, their access token is blacklisted in Redis:

- **Key format:** `token:blacklist:<token>`
- **TTL:** Remaining time until token expiry
- **Check:** Performed on every authenticated request

If Redis is unavailable, blacklisting is skipped (tokens remain valid until expiry). This is a trade-off — for strict session management, ensure Redis is highly available.

#### Revoking All Sessions

To revoke all sessions for a user (e.g., after a security incident):

1. Change the user's password (this invalidates existing tokens at next validation)
2. Flush the Redis blacklist database: `redis-cli -n 0 FLUSHDB` (affects all users — use with caution)

### 6.5 User Profile Information

Retrieve current user profile:

```bash
curl https://app.chaos-sec.io/api/v1/auth/me \
  -H "Authorization: Bearer <token>"
```

Response includes: user ID, email, name, role name, role description, permissions list, organization info.

---

## 7. Kubernetes Cluster Management

### 7.1 Registering a Cluster

Clusters are registered via the API:

```bash
curl -X POST https://app.chaos-sec.io/api/v1/clusters \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Production Cluster",
    "description": "Main production Kubernetes cluster",
    "api_endpoint": "https://k8s-api.prod.example.com:6443",
    "ca_certificate": "<base64-encoded-ca-cert>",
    "client_certificate": "<base64-encoded-client-cert>",
    "client_key": "<base64-encoded-client-key>",
    "default_namespace": "chaos-sec"
  }'
```

### 7.2 Required RBAC Permissions

The Chaos-Sec service account in the target cluster needs these permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: chaos-sec-role
rules:
  - apiGroups: [""]
    resources: ["pods", "pods/log", "namespaces", "nodes"]
    verbs: ["create", "delete", "get", "list", "watch"]
  - apiGroups: [""]
    resources: ["secrets", "configmaps"]
    verbs: ["get", "list"]
  - apiGroups: ["networking.k8s.io"]
    resources: ["networkpolicies"]
    verbs: ["get", "list"]
  - apiGroups: ["rbac.authorization.k8s.io"]
    resources: ["roles", "rolebindings"]
    verbs: ["get", "list"]
```

Create the service account and binding:

```bash
kubectl create serviceaccount chaos-sec -n chaos-sec
kubectl create clusterrolebinding chaos-sec \
  --clusterrole=chaos-sec-role \
  --serviceaccount=chaos-sec:chaos-sec
```

### 7.3 Cluster Health Monitoring

Chaos-Sec continuously monitors registered clusters. The health endpoint returns detailed information:

```bash
curl https://app.chaos-sec.io/api/v1/clusters/<id>/health \
  -H "Authorization: Bearer <token>"
```

Response includes:
- Overall status (healthy, degraded, unreachable)
- Kubernetes version
- Node count (total vs. ready)
- Per-node CPU and memory usage
- Aggregate resource utilization
- Error details if unreachable

### 7.4 Cluster Operations

| Operation | API | Method |
|----------|-----|--------|
| List clusters | `/api/v1/clusters` | GET |
| Register cluster | `/api/v1/clusters` | POST |
| Get cluster details | `/api/v1/clusters/:id` | GET |
| Delete cluster | `/api/v1/clusters/:id` | DELETE |
| List namespaces | `/api/v1/clusters/:id/namespaces` | GET |
| List network policies | `/api/v1/clusters/:id/network-policies` | GET |
| Get cluster health | `/api/v1/clusters/:id/health` | GET |

### 7.5 Local Development with kind

For local testing, use the provided kind configuration:

```bash
kind create cluster --name chaos-sec-dev --config scripts/kind-config.yaml
```

This creates:
- 1 control-plane node
- 2 worker nodes labeled with `chaos-sec.io/chaos-target: "true"`
- Port mappings: 80, 443, 30000, 30001
- Pod subnet: `10.244.0.0/16`

---

## 8. SIEM Integration

### 8.1 Configuration

Enable SIEM integration by setting the required environment variables:

```bash
CHAOS_SIEM_ENABLED=true
CHAOS_SIEM_PROVIDER=splunk       # splunk, elastic, sentinel, other
CHAOS_SIEM_ENDPOINT=https://splunk.example.com:8089
CHAOS_SIEM_API_KEY=your-api-key  # Store as a secret!
```

### 8.2 Supported SIEM Providers

| Provider | Endpoint Format | Authentication |
|----------|----------------|----------------|
| **Splunk** | `https://<host>:<port>` | API key (header) or basic auth |
| **Elastic** | `https://<host>:<port>` | API key |
| **Microsoft Sentinel** | `https://<tenant>.ods.opinsights.azure.com` | API key |
| **Other** | Any REST API endpoint | API key or basic auth |

### 8.3 Testing SIEM Connection

Use the API to test connectivity before running experiments:

```bash
curl -X POST https://app.chaos-sec.io/api/v1/siem/test-connection \
  -H "Authorization: Bearer <token>" \
  -H "Content-Type: application/json"
```

Response:
```json
{
  "success": true,
  "endpoint": "https://splunk.example.com:8089",
  "latency_ms": 42,
  "error": null
}
```

If the connection fails, the API returns `502 Bad Gateway` with error details.

### 8.4 SIEM Validation Flow

When an experiment runs with SIEM validation enabled:

1. Experiment executes the simulated attack
2. The validator waits for a **propagation delay** (default: 30 seconds) to allow the SIEM to ingest and correlate events
3. It queries the SIEM for alerts matching the expected type and severity within the time window
4. Each expected alert is correlated against received alerts using type matching and severity ranking
5. A detection score is calculated: `matched alerts / total expected alerts × 100`
6. The result status is set:
   - **passed** — All expected alerts detected (100%)
   - **partial** — Some alerts detected (>0% and <100%)
   - **failed** — No alerts detected (0%)

### 8.5 SIEM Alert Severity Ranking

| Severity | Rank |
|----------|------|
| `low` | 1 |
| `medium` | 2 |
| `high` | 3 |
| `critical` | 4 |

For an alert to match, the received severity must **meet or exceed** the expected severity. For example, if you expect `medium` and the SIEM returns `high`, it counts as a match.

### 8.6 Mock SIEM (Development)

Chaos-Sec includes a mock SIEM service for development and testing. It:

- Accepts alerts via `POST /api/alerts`
- Serves alerts via `GET /api/alerts` with query parameter filtering (`alert_type`, `severity`, `source`)
- Reports healthy at `/health`
- Supports basic authentication

Start it with Docker Compose:

```bash
docker compose up -d mock-siem
```

The mock SIEM is available at `http://localhost:9100`.

### 8.7 SIEM Connector Configuration

The SIEM connector (`MockSIEM`) has these settings:

| Setting | Type | Description |
|---------|------|-------------|
| `Endpoint` | string | Base URL of the SIEM API |
| `APIKey` | string | Authentication key |
| `Username` | string | Basic auth username |
| `Password` | string | Basic auth password |
| `Index` | string | SIEM index/database to query |
| `Timeout` | duration | HTTP client timeout |
| `MaxRetries` | int | Maximum retry attempts |

The connector uses exponential backoff for retries (1s, 2s, 4s, 8s, capped at 30s). It fails open — if the SIEM is unavailable, the operation returns an error rather than hanging.

---

## 9. Monitoring & Observability

### 9.1 Health Endpoints

| Endpoint | Purpose | Checks |
|----------|---------|--------|
| `GET /health/live` | Liveness probe | Process is alive |
| `GET /health/ready` | Readiness probe | Database + Redis connectivity |

Use these for Kubernetes probes:

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 30

readinessProbe:
  httpGet:
    path: /health/ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 10
```

### 9.2 Metrics

The backend exposes Prometheus metrics on port **9090** (separate from the API port).

Key metrics to monitor:
- `http_requests_total` — Total HTTP requests by method, path, status
- `http_request_duration_seconds` — Request latency histogram
- `go_goroutines` — Current goroutine count
- `go_memstats_alloc_bytes` — Allocated memory

### 9.3 Prometheus Alerting Rules

The platform includes 22 alerting rules across 8 groups:

#### Backend Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `BackendDown` | critical | No metrics for 2 minutes |
| `HighErrorRate` | warning | 5xx rate > 5% for 5 minutes |
| `HighLatency` | warning | P99 latency > 5s for 5 minutes |
| `CriticalErrorRate` | critical | 5xx rate > 20% for 2 minutes |
| `ElevatedClientErrorRate` | warning | 4xx rate > 30% for 10 minutes |

#### Database Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `DatabaseConnectionFailed` | critical | Cannot connect for 1 minute |
| `DatabasePoolExhaustion` | warning | Open connections ≥ 90% of max |
| `SlowDatabaseQueries` | warning | Average query time > 500ms for 5 minutes |
| `DatabaseReplicationLag` | warning | Replication lag > 30s |

#### Redis Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `RedisConnectionFailed` | critical | Cannot connect for 1 minute |
| `RedisMemoryHigh` | warning | Memory usage > 80% of max |
| `RedisKeyEvictions` | warning | Key evictions occurring |

#### Kubernetes Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `PodCrashLooping` | warning | Pod restart count > 5 in 15 minutes |
| `PodNotReady` | warning | Pod not ready for 5 minutes |
| `DeploymentReplicasMismatch` | warning | Available replicas < desired for 5 minutes |
| `PodPending` | warning | Pod pending for 5 minutes |
| `ExperimentJobFailed` | warning | Experiment job failed |

#### Resource Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `HighMemoryUsage` | warning | Node memory > 85% |
| `HighCPUUsage` | warning | Node CPU > 85% |
| `DiskSpaceLow` | warning | Disk usage > 80% |
| `DiskSpaceCritical` | critical | Disk usage > 95% |
| `NodeNotReady` | critical | Node not ready for 3 minutes |

#### Security Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `SSLExpirySoon` | warning | SSL cert expires in < 30 days |
| `SSLExpiryCritical` | critical | SSL cert expires in < 7 days |
| `SSLCertificateExpired` | critical | SSL cert has expired |

#### Experiment Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `ExperimentStuckRunning` | warning | Experiment running > 2 hours |
| `HighExperimentFailureRate` | warning | Failure rate > 50% over 24 hours |
| `TooManyConcurrentExperiments` | warning | > 10 concurrent experiments |

#### SIEM Alerts

| Alert | Severity | Condition |
|-------|----------|-----------|
| `SIEMConnectorDown` | critical | SIEM health check failing for 5 minutes |
| `SIEMIngestionLatencyHigh` | warning | SIEM ingestion latency > 60s |
| `SIEMIngestionErrors` | warning | SIEM ingestion error rate > 10% |

### 9.4 Structured Logging

All requests are logged with structured fields:

```json
{
  "level": "info",
  "ts": 1709654321.123,
  "logger": "middleware",
  "msg": "request",
  "method": "POST",
  "path": "/api/v1/experiments/42/execute",
  "query": "",
  "status": 200,
  "latency": "21.801ms",
  "client_ip": "10.0.1.5",
  "user_agent": "Mozilla/5.0",
  "body_size": 19,
  "request_id": "a6245574-911b-4215-87c2-b5fec2da2530",
  "user_id": "f47ac10b-58cc",
  "org_id": "7da9a065"
}
```

Log levels by status code:
- **500+** → `error`
- **400–499** → `warn`
- **Otherwise** → `info`

### 9.5 Horizontal Pod Autoscaling

The HPA configuration scales the backend based on resource usage:

| Setting | Value |
|---------|-------|
| Min replicas | 2 |
| Max replicas | 10 |
| CPU target | 70% |
| Memory target | 80% |
| Scale-up period | 60s (max 3 pods or +50%) |
| Scale-down period | 300s (max 1 pod or -25%) |
| Stabilization window (scale-up) | 60s |
| Stabilization window (scale-down) | 300s |

---

## 10. Backup & Recovery

### 10.1 Database Backup

Use the provided backup script:

```bash
./deploy/scripts/backup.sh
```

The script:
1. Creates a PostgreSQL dump with `pg_dump`
2. Compresses the backup with `gzip`
3. Uploads to S3 (if configured)
4. Applies a retention policy (default: 30 days)
5. Verifies the backup integrity

**S3 configuration** (via environment variables):

| Variable | Description |
|----------|-------------|
| `AWS_ACCESS_KEY_ID` | AWS credentials |
| `AWS_SECRET_ACCESS_KEY` | AWS credentials |
| `BACKUP_S3_BUCKET` | S3 bucket name |
| `BACKUP_RETENTION_DAYS` | Days to retain backups (default: 30) |

**Manual backup:**

```bash
# Create a manual backup
docker compose exec postgres pg_dump -U chaossec chaos_sec | gzip > backup_$(date +%Y%m%d_%H%M%S).sql.gz

# Restore from backup
gunzip -c backup_20260421_120000.sql.gz | docker compose exec -T postgres psql -U chaossec chaos_sec
```

### 10.2 Automated Restore

```bash
./deploy/scripts/restore.sh
```

The restore script:
1. Downloads the specified backup from S3 (or uses a local file)
2. Verifies the backup integrity
3. Stops application traffic (optional)
4. Restores the database
5. Verifies the restore was successful
6. Resumes application traffic

### 10.3 Disaster Recovery Plan

| Scenario | RPO | RTO | Recovery Steps |
|----------|-----|-----|----------------|
| Database corruption | < 1 hour | < 2 hours | Restore from latest backup |
| Node failure | 0 | < 5 minutes | Kubernetes reschedules pods |
| Full cluster failure | < 1 hour | < 4 hours | Rebuild cluster, apply manifests, restore DB |
| Accidental data deletion | < 1 hour | < 2 hours | Point-in-time recovery from backup |

### 10.4 Redis Persistence

Redis is configured with:
- **AOF persistence** (`appendonly yes`) — Every write is logged
- **AOF sync** — Every second (good balance of performance and durability)

Redis data is ephemeral (rate limit counters, token blacklist). Loss of Redis data only causes:
- Temporary rate limit reset (users may exceed limits briefly)
- Active sessions remain valid until token expiry (max 1 hour)

---

## 11. Security Administration

### 11.1 TLS Configuration

In production, all communication must use TLS:

- **Ingress** — Terminate TLS at the nginx ingress with valid certificates
- **Backend** — Served behind the ingress; internal communication is HTTP
- **PostgreSQL** — Set `CHAOS_DB_SSLMODE=require` (or `verify-full` for maximum security)
- **Redis** — Configure TLS in Redis and the application

The deployment includes TLS secrets:

```bash
kubectl create secret tls chaos-sec-tls \
  --namespace chaos-sec \
  --key tls.key \
  --cert tls.crt
```

### 11.2 Security Headers

The middleware automatically sets these security headers on all responses:

| Header | Value | Purpose |
|--------|-------|---------|
| `X-Frame-Options` | `DENY` | Prevent clickjacking |
| `X-Content-Type-Options` | `nosniff` | Prevent MIME sniffing |
| `X-XSS-Protection` | `1; mode=block` | Browser XSS filter |
| `Strict-Transport-Security` | `max-age=31536000; includeSubDomains` | Force HTTPS (production only) |
| `Referrer-Policy` | `strict-origin-when-cross-origin` | Limit referrer leakage |
| `Permissions-Policy` | `camera=(), microphone=(), geolocation=()` | Disable unnecessary APIs |
| `X-Request-ID` | `<uuid>` | Request tracing |

### 11.3 CORS Configuration

| Environment | Allowed Origins | Notes |
|-------------|----------------|-------|
| Development | `*` (all origins) | For local development convenience |
| Production | Must be restricted | Log warning if open CORS detected |

Allowed methods: `GET, POST, PUT, PATCH, DELETE, OPTIONS`
Allowed headers: `Origin, Content-Type, Accept, Authorization, X-Request-ID, X-API-Key`
Preflight cache: 24 hours

### 11.4 Rate Limiting

Rate limiting is enabled by default with these settings:

- **Authenticated users:** Identified by JWT user ID
- **Unauthenticated requests:** Identified by client IP
- **Default limit:** 100 requests per 60 seconds
- **Response when exceeded:** `429 Too Many Requests`

```json
{
  "error": "rate_limit_exceeded",
  "message": "Too many requests. Please slow down.",
  "code": 429
}
```

Rate limit headers on allowed responses:
- `X-RateLimit-Limit` — Maximum requests per window
- `X-RateLimit-Window` — Time window duration

### 11.5 JWT Token Security

| Aspect | Setting |
|--------|---------|
| Signing algorithm | HMAC SHA-256 (`HS256`) — enforced, rejects `none` |
| Access token expiry | 1 hour (configurable) |
| Refresh token expiry | 7 days (configurable) |
| Token blacklisting | Redis-backed, TTL matches token expiry |
| bcrypt cost | 12 (~250ms per hash) |
| Password minimum | 8 characters |

### 11.6 Pod Security

The Kubernetes deployment enforces the **restricted** Pod Security Standard:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: chaos-sec
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

All containers run with:
- Non-root user (UID 1000 for backend, 101 for frontend)
- Read-only root filesystem
- All Linux capabilities dropped
- Seccomp profile: `RuntimeDefault`
- No privileged access

### 11.7 Network Policies

In production, restrict network traffic with NetworkPolicies:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: chaos-sec-backend
  namespace: chaos-sec
spec:
  podSelector:
    matchLabels:
      app: chaos-sec-backend
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
      ports:
        - port: 8080
  egress:
    - to:
        - podSelector:
            matchLabels:
              app: chaos-sec-postgres
      ports:
        - port: 5432
    - to:
        - podSelector:
            matchLabels:
              app: chaos-sec-redis
      ports:
        - port: 6379
```

---

## 12. Troubleshooting

### 12.1 Common Issues

#### Database Connection Failed

**Symptoms:** `GET /health/ready` returns 503; logs show `connection refused` or `password authentication failed`.

**Solutions:**
1. Verify PostgreSQL is running: `docker compose ps postgres` or `kubectl get pods -n chaos-sec -l app=chaos-sec-postgres`
2. Check connection parameters: `CHAOS_DB_HOST`, `CHAOS_DB_PORT`, `CHAOS_DB_USER`, `CHAOS_DB_PASSWORD`, `CHAOS_DB_NAME`
3. Test connection manually: `psql -h <host> -U <user> -d <db>`
4. Check PostgreSQL logs: `docker compose logs postgres` or `kubectl logs -n chaos-sec -l app=chaos-sec-postgres`

#### Redis Connection Failed

**Symptoms:** Rate limiting falls back to in-memory; token blacklisting is skipped on logout.

**Solutions:**
1. Verify Redis is running: `redis-cli -h <host> -p <port> ping`
2. Check `CHAOS_REDIS_HOST`, `CHAOS_REDIS_PORT`, `CHAOS_REDIS_PASSWORD`
3. The application continues to function without Redis but with reduced features

#### Kubernetes API Unreachable

**Symptoms:** Cluster health shows "unreachable"; experiments fail to create pods.

**Solutions:**
1. Verify the cluster is accessible: `kubectl cluster-info`
2. Check `CHAOS_K8S_IN_CLUSTER` is set correctly
3. Verify the service account has the required RBAC permissions
4. Check `CHAOS_K8S_KUBECONFIG` path when running outside the cluster
5. Verify network connectivity between Chaos-Sec and the target cluster

#### SIEM Connection Failed

**Symptoms:** SIEM status shows "disconnected"; validation always reports 0% detection.

**Solutions:**
1. Use the test connection endpoint: `POST /api/v1/siem/test-connection`
2. Verify `CHAOS_SIEM_ENDPOINT` is correct and reachable
3. Check `CHAOS_SIEM_API_KEY` is valid
4. If using the mock SIEM, verify it's running: `curl http://localhost:9100/health`
5. Check SIEM firewall rules allow traffic from the Chaos-Sec backend

#### High Memory Usage

**Symptoms:** Backend pod OOM killed; HPA scaling frequently.

**Solutions:**
1. Increase memory limits in the deployment
2. Check for goroutine leaks: `curl http://localhost:9090/metrics | grep go_goroutines`
3. Reduce `CHAOS_K8S_MAX_CONCURRENT` to limit concurrent operations
4. Reduce `CHAOS_DB_MAX_OPEN_CONNS` to reduce connection memory overhead
5. Review experiment execution patterns — large result sets consume memory

#### Experiment Stuck in "Pending"

**Symptoms:** Experiment never transitions to "running" status.

**Solutions:**
1. Check the target cluster is healthy
2. Verify the namespace exists in the target cluster
3. Check the worker pool is processing: review backend logs for queue processing errors
4. Verify the service account can create pods in the target namespace

### 12.2 Log Locations

| Component | Log Command |
|-----------|-------------|
| Backend (Docker) | `docker compose logs -f backend` |
| Backend (K8s) | `kubectl logs -n chaos-sec -l app=chaos-sec-backend -c chaos-sec-backend` |
| Frontend (Docker) | `docker compose logs -f frontend` |
| Frontend (K8s) | `kubectl logs -n chaos-sec -l app=chaos-sec-frontend` |
| PostgreSQL | `docker compose logs -f postgres` |
| Redis | `docker compose logs -f redis` |

### 12.3 Health Check Interpretation

| Health Endpoint | Status | Meaning |
|-----------------|--------|---------|
| `/health/live` | 200 | Process is alive and responsive |
| `/health/live` | 500 | Process is in a bad state (panic recovery) |
| `/health/ready` | 200 | Database and Redis are connected; ready for traffic |
| `/health/ready` | 503 | Database or Redis is unreachable; do not send traffic |

### 12.4 Debug Mode

For detailed troubleshooting, enable debug logging:

```bash
CHAOS_LOG_LEVEL=debug
CHAOS_LOG_FORMAT=console
```

Debug logging includes:
- Full request/response details
- Database query execution
- Redis operations
- Kubernetes API calls
- SIEM query details

> **Warning:** Debug logging may expose sensitive data. Never use in production for extended periods.

---

## 13. Upgrading

### 13.1 Upgrade Procedure

```bash
# 1. Back up the database
./deploy/scripts/backup.sh

# 2. Pull the latest code
git pull origin main

# 3. Review configuration changes
git diff HEAD~1 -- backend/internal/config/

# 4. Run database migrations
./scripts/migrate.sh up

# 5. Rebuild and deploy
docker compose build backend frontend
docker compose up -d

# For Kubernetes:
kubectl set image deployment/chaos-sec-backend \
  backend=chaos-sec/backend:latest -n chaos-sec
kubectl set image deployment/chaos-sec-frontend \
  frontend=chaos-sec/frontend:latest -n chaos-sec

# 6. Verify the upgrade
kubectl rollout status deployment/chaos-sec-backend -n chaos-sec
kubectl rollout status deployment/chaos-sec-frontend -n chaos-sec
./deploy/scripts/smoke-tests.sh
```

### 13.2 Rollback Procedure

If the upgrade causes issues:

```bash
# Kubernetes rollback
kubectl rollout undo deployment/chaos-sec-backend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-frontend -n chaos-sec

# Database rollback (if migrations ran)
./scripts/migrate.sh down

# Full database restore (if needed)
./deploy/scripts/restore.sh --file backup_before_upgrade.sql.gz
```

### 13.3 Configuration Changes

When upgrading, check for:
- New environment variables (added features)
- Changed default values
- Deprecated settings
- Required database migrations

Review the release notes and `git diff` on config files before deploying.

---

## 14. API Quick Reference

### Authentication

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `POST` | `/api/v1/auth/login` | None | None |
| `POST` | `/api/v1/auth/refresh` | None | None |
| `POST` | `/api/v1/auth/logout` | JWT | Any |
| `GET` | `/api/v1/auth/me` | JWT | Any |
| `POST` | `/api/v1/auth/register` | JWT | `admin:all` or `users:manage` |

### Experiments

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/experiments` | JWT | `experiments:read` |
| `POST` | `/api/v1/experiments` | JWT | `experiments:write` |
| `GET` | `/api/v1/experiments/:id` | JWT | `experiments:read` |
| `PUT` | `/api/v1/experiments/:id` | JWT | `experiments:write` |
| `DELETE` | `/api/v1/experiments/:id` | JWT | `experiments:delete` |
| `POST` | `/api/v1/experiments/:id/execute` | JWT | `experiments:execute` |
| `POST` | `/api/v1/experiments/:id/stop` | JWT | `experiments:execute` |

### Templates

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/templates` | JWT | `templates:read` |
| `POST` | `/api/v1/templates` | JWT | `templates:write` |
| `GET` | `/api/v1/templates/:id` | JWT | `templates:read` |

### Attack Templates

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/attack-templates` | JWT | `templates:read` |
| `POST` | `/api/v1/attack-templates` | JWT | `templates:write` |
| `GET` | `/api/v1/attack-templates/:id` | JWT | `templates:read` |
| `PUT` | `/api/v1/attack-templates/:id` | JWT | `templates:write` |
| `DELETE` | `/api/v1/attack-templates/:id` | JWT | `templates:write` or `admin:all` |

### Clusters

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/clusters` | JWT | `clusters:read` |
| `POST` | `/api/v1/clusters` | JWT | `clusters:write` |
| `GET` | `/api/v1/clusters/:id` | JWT | `clusters:read` |
| `DELETE` | `/api/v1/clusters/:id` | JWT | `clusters:write` |
| `GET` | `/api/v1/clusters/:id/namespaces` | JWT | `clusters:read` |
| `GET` | `/api/v1/clusters/:id/network-policies` | JWT | `clusters:read` |
| `GET` | `/api/v1/clusters/:id/health` | JWT | `clusters:read` |

### SIEM

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/siem/status` | JWT | `experiments:read` |
| `POST` | `/api/v1/siem/test-connection` | JWT | `clusters:write` |
| `POST` | `/api/v1/siem/alerts/query` | JWT | `experiments:read` |
| `GET` | `/api/v1/siem/alerts/:run_id` | JWT | `experiments:read` |

### Reports

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/reports/:experimentId` | JWT | `experiments:read` |

### Dashboard

| Method | Endpoint | Auth | Permissions |
|--------|----------|------|-------------|
| `GET` | `/api/v1/dashboard/summary` | JWT | `experiments:read` |

### Health

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/health/live` | None | Liveness probe |
| `GET` | `/health/ready` | None | Readiness probe (checks DB + Redis) |

### Common Request/Response Patterns

**Authentication header:**
```
Authorization: Bearer <access_token>
```

**Pagination query parameters:**
```
?page=1&per_page=20&sort=created_at_desc
```

**Error response format:**
```json
{
  "error": "error_code",
  "message": "Human-readable error description",
  "code": 400
}
```

---

**Document Version:** 1.0.0
**Last Updated:** 2026-04-21