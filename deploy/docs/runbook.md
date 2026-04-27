# Chaos-Sec: Deployment Runbook

> **Last Updated:** 2025-01-15  
> **Version:** 1.0.0  
> **Owner:** Platform Team  
> **Status:** Active

---

## Table of Contents

1. [Prerequisites](#1-prerequisites)
2. [Pre-Deployment Checklist](#2-pre-deployment-checklist)
3. [Deployment Steps](#3-deployment-steps)
4. [Post-Deployment Verification](#4-post-deployment-verification)
5. [Rollback Procedures](#5-rollback-procedures)
6. [Common Issues and Fixes](#6-common-issues-and-fixes)
7. [Emergency Contacts and Escalation](#7-emergency-contacts-and-escalation)

---

## 1. Prerequisites

### 1.1 Required Tools

| Tool | Minimum Version | Purpose | Install |
|------|-----------------|---------|---------|
| `kubectl` | v1.28+ | Kubernetes management | `brew install kubectl` |
| `docker` | 24+ | Container builds | [docker.com](https://docs.docker.com/get-docker/) |
| `helm` | v3.12+ | Chart deployments (optional) | `brew install helm` |
| `aws` | v2.13+ | S3 backup operations | `pip install awscli` |
| `jq` | v1.7+ | JSON processing | `brew install jq` |
| `curl` | v8+ | Health check probing | Pre-installed on most systems |
| `psql` | v15+ | Database operations | `brew install libpq` |
| `openssl` | v3+ | TLS certificate checks | Pre-installed on most systems |

Verify all tools are installed:

```bash
make deps  # Installs Go and Node dependencies
kubectl version --client
docker version --format '{{.Client.Version}}'
aws --version
jq --version
```

### 1.2 Access Requirements

- **Kubernetes cluster** — `kubectl` access to the production cluster
- **Docker Hub** — Push access to the `chaos-sec` Docker Hub organisation
- **GitHub Container Registry** — Write access to `ghcr.io/chaos-sec`
- **AWS S3** — Read/write access to the `chaos-sec-backups` bucket
- **Database** — Superuser access to PostgreSQL for connection termination during restores
- **Secrets** — Access to the following GitHub Secrets:
  - `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`
  - `KUBE_CONFIG` (base64-encoded kubeconfig)
  - `JWT_SECRET`, `DB_PASSWORD`, `REDIS_PASSWORD`, `RABBITMQ_PASSWORD`
  - `SLACK_WEBHOOK_URL`

### 1.3 Environment Configuration

| Variable | Description | Example |
|----------|-------------|---------|
| `KUBECONFIG` | Path to kubeconfig | `~/.kube/config` |
| `DOCKER_REGISTRY` | Docker image registry | `ghcr.io/chaos-sec` |
| `S3_BUCKET` | Backup S3 bucket | `s3://chaos-sec-backups` |
| `AWS_REGION` | AWS region for S3 | `eu-west-1` |
| `CHAOS_DB_HOST` | PostgreSQL host | `postgres.chaos-sec.svc.cluster.local` |
| `BASE_URL` | Backend API URL (for smoke tests) | `https://api.chaos-sec.example.com` |
| `FRONTEND_URL` | Frontend URL (for smoke tests) | `https://chaos-sec.example.com` |

---

## 2. Pre-Deployment Checklist

Complete **all** items before proceeding with deployment.

### 2.1 Code & Build

- [ ] All CI checks pass on the target branch (lint, test, security scan)
- [ ] Docker images build successfully locally:
  ```bash
  make docker-build-backend
  make docker-build-frontend
  docker build -t chaos-sec-mock-siem:latest -f backend/Dockerfile.mock-siem backend
  ```
- [ ] No critical or high-severity security findings in `gosec` report
- [ ] No known vulnerabilities in `npm audit` output
- [ ] Version tag created and pushed (e.g., `git tag v1.2.3`)

### 2.2 Database

- [ ] Database backup completed before deployment:
  ```bash
  ./deploy/scripts/backup.sh backup
  ```
- [ ] Backup verified:
  ```bash
  ./deploy/scripts/backup.sh verify s3://chaos-sec-backups/database/$(date +%Y-%m-%d)/
  ```
- [ ] Migrations tested against a copy of production data
- [ ] Rollback migration scripts available for all new migrations

### 2.3 Infrastructure

- [ ] Kubernetes cluster is healthy:
  ```bash
  kubectl get nodes
  kubectl top nodes
  ```
- [ ] Sufficient resources available (CPU, memory, disk)
- [ ] Kubernetes secrets are up to date
- [ ] Prometheus and Grafana are operational
- [ ] Alert rules loaded and validated:
  ```bash
  kubectl get prometheus -n chaos-sec -o yaml | grep ruleFiles
  ```

### 2.4 Communication

- [ ] Deployment window confirmed with team (avoid peak hours)
- [ ] Stakeholders notified of planned deployment
- [ ] Slack channel `#deployments` informed
- [ ] On-call engineer confirmed and available
- [ ] Rollback plan reviewed and acknowledged

---

## 3. Deployment Steps

### 3.1 Automated Deployment (Recommended)

The CD pipeline handles most steps automatically when a release tag is pushed:

```bash
# Create and push a release tag
git tag -a v1.2.3 -m "Release v1.2.3"
git push origin v1.2.3
```

The pipeline will:
1. Build Docker images for backend, frontend, and mock-siem
2. Push images to Docker Hub and GitHub Container Registry
3. Deploy to the Kubernetes cluster
4. Run smoke tests
5. Rollback automatically on failure

### 3.2 Manual Deployment

Use this procedure when the automated pipeline is unavailable or for emergency deployments.

#### Step 1: Build Docker Images

```bash
# Set version tag
export DOCKER_TAG="v1.2.3"
export DOCKER_REGISTRY="ghcr.io/chaos-sec"

# Build backend
docker build -t ${DOCKER_REGISTRY}/backend:${DOCKER_TAG} \
  -f backend/Dockerfile backend

# Build frontend (production target)
docker build -t ${DOCKER_REGISTRY}/frontend:${DOCKER_TAG} \
  -f frontend/Dockerfile \
  --target production frontend

# Build mock SIEM
docker build -t ${DOCKER_REGISTRY}/mock-siem:${DOCKER_TAG} \
  -f backend/Dockerfile.mock-siem backend
```

#### Step 2: Push Images to Registry

```bash
# Login to registries
docker login docker.io
echo "${GITHUB_TOKEN}" | docker login ghcr.io -u USERNAME --password-stdin

# Push all images
docker push ${DOCKER_REGISTRY}/backend:${DOCKER_TAG}
docker push ${DOCKER_REGISTRY}/frontend:${DOCKER_TAG}
docker push ${DOCKER_REGISTRY}/mock-siem:${DOCKER_TAG}
```

#### Step 3: Verify Kubernetes Connectivity

```bash
kubectl cluster-info
kubectl get nodes
kubectl get pods -n chaos-sec
```

#### Step 4: Update Kubernetes Secrets (if changed)

```bash
kubectl create secret generic chaos-sec-secrets \
  --namespace=chaos-sec \
  --from-literal=jwt-secret="${JWT_SECRET}" \
  --from-literal=db-password="${DB_PASSWORD}" \
  --from-literal=redis-password="${REDIS_PASSWORD}" \
  --from-literal=rabbitmq-password="${RABBITMQ_PASSWORD}" \
  --dry-run=client -o yaml | kubectl apply -f -
```

#### Step 5: Deploy Backend

```bash
# Record current state for rollback
kubectl rollout history deployment/chaos-sec-backend -n chaos-sec

# Update image
kubectl set image deployment/chaos-sec-backend \
  backend=${DOCKER_REGISTRY}/backend:${DOCKER_TAG} \
  --namespace=chaos-sec

# Wait for rollout
kubectl rollout status deployment/chaos-sec-backend \
  --namespace=chaos-sec \
  --timeout=300s
```

#### Step 6: Deploy Frontend

```bash
kubectl set image deployment/chaos-sec-frontend \
  frontend=${DOCKER_REGISTRY}/frontend:${DOCKER_TAG} \
  --namespace=chaos-sec

kubectl rollout status deployment/chaos-sec-frontend \
  --namespace=chaos-sec \
  --timeout=300s
```

#### Step 7: Deploy Mock SIEM

```bash
kubectl set image deployment/chaos-sec-mock-siem \
  mock-siem=${DOCKER_REGISTRY}/mock-siem:${DOCKER_TAG} \
  --namespace=chaos-sec

kubectl rollout status deployment/chaos-sec-mock-siem \
  --namespace=chaos-sec \
  --timeout=300s
```

#### Step 8: Run Database Migrations

```bash
# Apply pending migrations
./scripts/migrate.sh up

# Verify migration status
./scripts/migrate.sh status
```

#### Step 9: Run Smoke Tests

```bash
chmod +x deploy/scripts/smoke-tests.sh

BASE_URL=https://api.chaos-sec.example.com \
FRONTEND_URL=https://chaos-sec.example.com \
SIEM_URL=http://mock-siem.chaos-sec.svc.cluster.local:8089 \
AUTH_USER="${SMOKE_TEST_USER}" \
AUTH_PASS="${SMOKE_TEST_PASS}" \
./deploy/scripts/smoke-tests.sh
```

---

## 4. Post-Deployment Verification

### 4.1 Automated Checks (Smoke Tests)

Run the full smoke test suite:

```bash
./deploy/scripts/smoke-tests.sh
```

The smoke tests cover:
- Health check endpoints (`/health`, `/healthz`, `/readyz`, `/livez`)
- Authentication (login, token refresh, `/auth/me`)
- Experiment CRUD (create, list, get, delete)
- Cluster registration and health
- SIEM connector health
- Frontend serves correctly
- TLS certificate validity (if `TLS_CHECK_ENABLED=true`)
- Response time thresholds (API < 500ms, frontend < 2s)

### 4.2 Manual Checks

#### Service Health

```bash
# Check all pods are running
kubectl get pods -n chaos-sec -l app.kubernetes.io/part-of=chaos-sec

# Check pod logs for errors
kubectl logs -n chaos-sec -l app=chaos-sec-backend --tail=50 | grep -i error

# Check deployment readiness
kubectl get deployments -n chaos-sec
```

#### Database Connectivity

```bash
# Verify backend health endpoint includes DB status
curl -s https://api.chaos-sec.example.com/health | jq '.dependencies.postgres'
# Expected: {"status": "healthy", "latency_ms": <number>}
```

#### Redis Connectivity

```bash
curl -s https://api.chaos-sec.example.com/health | jq '.dependencies.redis'
# Expected: {"status": "healthy", "latency_ms": <number>}
```

#### SIEM Integration

```bash
curl -s https://api.chaos-sec.example.com/health | jq '.dependencies.siem'
# Expected: {"status": "healthy", "provider": "mock"}
```

#### Metrics Pipeline

```bash
# Verify Prometheus is scraping the backend
curl -s http://prometheus.chaos-sec.svc.cluster.local:9090/api/v1/targets | \
  jq '.data.activeTargets[] | select(.labels.job == "chaos-sec-backend") | .health'
# Expected: "up"

# Check for recent metrics
curl -s 'http://prometheus.chaos-sec.svc.cluster.local:9090/api/v1/query?query=up{job="chaos-sec-backend"}' | jq
```

#### Grafana Dashboards

1. Open Grafana at `http://grafana.chaos-sec.svc.cluster.local:3000`
2. Navigate to the **Chaos-Sec Operations** dashboard
3. Verify panels are loading data:
   - API request rate and latency
   - Error rate by endpoint
   - Experiment execution metrics
   - Kubernetes pod health
   - Database connections and queries
   - Redis metrics

#### Alert Rules

```bash
# Verify alert rules are loaded
kubectl exec -n chaos-sec prometheus-0 -- \
  wget -qO- http://localhost:9090/api/v1/rules | jq '.data.groups[].name'
```

### 4.3 Sign-Off

- [ ] All smoke tests pass
- [ ] No errors in pod logs
- [ ] Prometheus targets are UP
- [ ] Grafana dashboards display data
- [ ] Alert rules are active
- [ ] Stakeholders notified of successful deployment

---

## 5. Rollback Procedures

### 5.1 Automated Rollback (CD Pipeline)

If the CD pipeline detects a smoke test failure, it automatically triggers a rollback:

```yaml
# In .github/workflows/cd.yml, the rollback job runs on failure
rollback:
  if: failure()
  steps:
    - kubectl rollout undo deployment/chaos-sec-backend -n chaos-sec
    - kubectl rollout undo deployment/chaos-sec-frontend -n chaos-sec
    - kubectl rollout undo deployment/chaos-sec-mock-siem -n chaos-sec
```

### 5.2 Manual Rollback

#### Rollback to Previous Deployment

```bash
# Check rollout history
kubectl rollout history deployment/chaos-sec-backend -n chaos-sec

# Rollback to the previous revision
kubectl rollout undo deployment/chaos-sec-backend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-frontend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-mock-siem -n chaos-sec

# Wait for rollback to complete
kubectl rollout status deployment/chaos-sec-backend -n chaos-sec --timeout=180s
kubectl rollout status deployment/chaos-sec-frontend -n chaos-sec --timeout=180s
kubectl rollout status deployment/chaos-sec-mock-siem -n chaos-sec --timeout=180s
```

#### Rollback to Specific Version

```bash
# Find the revision number for the target version
kubectl rollout history deployment/chaos-sec-backend -n chaos-sec

# Rollback to a specific revision
kubectl rollout undo deployment/chaos-sec-backend --to-revision=3 -n chaos-sec
```

#### Rollback Database

```bash
# Restore from the pre-deployment backup
./deploy/scripts/restore.sh restore "$(date +%Y-%m-%d)" --confirm

# Or rollback specific migrations
./scripts/migrate.sh down
```

### 5.3 Full Disaster Recovery

If the entire deployment is unrecoverable:

```bash
# 1. Scale down all services
kubectl scale deployment --all --replicas=0 -n chaos-sec

# 2. Restore database from most recent backup
./deploy/scripts/restore.sh restore latest --confirm

# 3. Roll back all deployments to the last known good version
kubectl rollout undo deployment/chaos-sec-backend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-frontend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-mock-siem -n chaos-sec

# 4. Scale services back up
kubectl scale deployment --all --replicas=1 -n chaos-sec

# 5. Wait for pods to become ready
kubectl rollout status deployment --all -n chaos-sec --timeout=300s

# 6. Run smoke tests
./deploy/scripts/smoke-tests.sh
```

---

## 6. Common Issues and Fixes

### 6.1 Backend Pod CrashLoopBackOff

**Symptoms:** Pod restarts repeatedly, `kubectl get pods` shows `CrashLoopBackOff`

**Diagnosis:**
```bash
kubectl describe pod <pod-name> -n chaos-sec
kubectl logs <pod-name> -n chaos-sec --previous
```

**Common Causes and Fixes:**

| Cause | Error Message | Fix |
|-------|--------------|-----|
| Database unreachable | `connection refused` | Check `CHAOS_DB_HOST`, `CHAOS_DB_PORT`; verify PostgreSQL is running |
| Invalid JWT secret | `JWT secret too short` | Ensure `JWT_SECRET` is at least 32 characters |
| Redis auth failure | `NOAUTH Authentication required` | Verify `CHAOS_REDIS_PASSWORD` matches Redis config |
| Missing env vars | `required environment variable not set` | Check Kubernetes secret: `kubectl get secret chaos-sec-secrets -n chaos-sec -o yaml` |
| OOM killed | `OOMKilled` in pod events | Increase memory limit in deployment spec |

### 6.2 Frontend Shows Blank Page

**Symptoms:** Frontend loads but displays a blank white page

**Diagnosis:**
```bash
# Check browser console for errors
# Check nginx config
kubectl exec <frontend-pod> -n chaos-sec -- cat /etc/nginx/conf.d/default.conf
# Check that static files exist
kubectl exec <frontend-pod> -n chaos-sec -- ls -la /usr/share/nginx/html/
```

**Common Causes and Fixes:**

| Cause | Fix |
|-------|-----|
| Wrong `REACT_APP_API_URL` / `VITE_API_URL` | Update the environment variable and rebuild the frontend image |
| nginx misconfiguration | Verify `nginx.conf` has correct `try_files` and proxy pass rules |
| Build failure (empty `dist/`) | Rebuild frontend image: `docker build --target production -f frontend/Dockerfile frontend` |
| CORS errors | Ensure backend has CORS middleware enabled for the frontend origin |

### 6.3 Database Migration Failure

**Symptoms:** Backend fails to start after a migration

**Diagnosis:**
```bash
# Check migration status
./scripts/migrate.sh status

# Check for dirty migrations
psql -h $CHAOS_DB_HOST -U $CHAOS_DB_USER -d chaossec \
  -c "SELECT * FROM schema_migrations ORDER BY version;"
```

**Fixes:**

- **Dirty migration:** Force-set the migration version and manually fix the schema:
  ```bash
  psql -c "UPDATE schema_migrations SET dirty = false WHERE version = <version>;"
  ```
- **Migration conflict:** Rollback the failed migration and re-apply:
  ```bash
  ./scripts/migrate.sh down  # Roll back one step
  ./scripts/migrate.sh up    # Re-apply
  ```
- **Permission error:** Ensure the database user has `CREATE TABLE` and `ALTER TABLE` privileges

### 6.4 SIEM Connector Down

**Symptoms:** `chaos_sec_siem_connector_up == 0` alert fires

**Diagnosis:**
```bash
# Check mock SIEM health
curl http://mock-siem.chaos-sec.svc.cluster.local:8089/health

# Check backend SIEM config
kubectl exec <backend-pod> -n chaos-sec -- env | grep CHAOS_SIEM
```

**Fixes:**

| Cause | Fix |
|-------|-----|
| Mock SIEM pod not running | `kubectl get pods -n chaos-sec -l app=chaos-sec-mock-siem` |
| Wrong `CHAOS_SIEM_ENDPOINT` | Update the backend environment variable |
| Network policy blocking traffic | Add a NetworkPolicy allowing backend → mock-siem on port 8089 |
| Mock SIEM crash | Check logs: `kubectl logs <mock-siem-pod> -n chaos-sec` |

### 6.5 High Error Rate (5xx)

**Symptoms:** `HighErrorRate` alert fires (>5% 5xx rate)

**Diagnosis:**
```bash
# Check recent 5xx errors in backend logs
kubectl logs -n chaos-sec -l app=chaos-sec-backend --tail=200 | grep "500\|502\|503"

# Check Prometheus for error patterns
curl -s 'http://prometheus:9090/api/v1/query?query=http_requests_total{code=~"5.."}' | jq
```

**Common Causes:**

| Cause | Fix |
|-------|-----|
| Database connection pool exhausted | Increase `CHAOS_DB_MAX_OPEN_CONNS`; check for connection leaks |
| Redis timeout | Check Redis memory and connectivity |
| OOM in backend pod | Increase pod memory limit |
| Upstream service down | Check health of postgres, redis, rabbitmq |

### 6.6 Pod Not Ready

**Symptoms:** `PodNotReady` alert fires; pod is Running but not Ready

**Diagnosis:**
```bash
kubectl describe pod <pod-name> -n chaos-sec | grep -A10 "Readiness"
kubectl logs <pod-name> -n chaos-sec --tail=50
```

**Common Causes:**

| Cause | Fix |
|-------|-----|
| Readiness probe failing | Check probe path and port in deployment spec |
| Slow startup | Increase `initialDelaySeconds` or `timeoutSeconds` in the probe |
| Dependency not ready | Backend may need postgres/redis to pass readiness — ensure they start first |
| Image pull error | Check image tag and registry credentials |

### 6.7 Prometheus Not Scraping Metrics

**Symptoms:** Grafana dashboards show no data; `up{job="chaos-sec-backend"} == 0`

**Diagnosis:**
```bash
# Check Prometheus targets
kubectl port-forward -n chaos-sec svc/chaos-sec-prometheus 9091:9090
# Open http://localhost:9091/targets in browser

# Verify metrics endpoint is reachable
curl http://chaos-sec-backend.chaos-sec.svc.cluster.local:9090/metrics
```

**Fixes:**

| Cause | Fix |
|-------|-----|
| Wrong scrape target in `prometheus.yml` | Update the target address to match the backend service |
| Metrics port not exposed | Ensure the backend deployment exposes port 9090 |
| Network policy | Allow Prometheus → backend on port 9090 |
| Prometheus config not reloaded | Restart Prometheus or send SIGHUP |

---

## 7. Emergency Contacts and Escalation

### 7.1 Escalation Matrix

| Severity | Response Time | Contact | Channel |
|----------|--------------|---------|---------|
| **P0 — Critical** (service down) | < 15 min | On-call engineer | PagerDuty + Slack `#incidents` |
| **P1 — High** (degraded service) | < 1 hour | Platform team lead | Slack `#platform` |
| **P2 — Medium** (non-critical issue) | < 4 hours | Platform team | Slack `#platform` |
| **P3 — Low** (cosmetic/minor) | < 24 hours | Platform team | GitHub Issues |

### 7.2 Key Contacts

| Role | Name | Email | Availability |
|------|------|-------|-------------|
| Platform Lead | Jash Sharma | jash@example.com | Business hours + on-call |
| Backend Engineer | TBD | backend@example.com | Business hours |
| DevOps/SRE | TBD | devops@example.com | 24/7 on-call rotation |
| Security Engineer | TBD | security@example.com | Business hours |
| Product Owner | TBD | product@example.com | Business hours |

### 7.3 On-Call Rotation

- **Primary on-call:** Check PagerDuty schedule at `https://chaos-sec.pagerduty.com`
- **Secondary on-call:** Platform team lead
- **Escalation manager:** Platform Lead (for P0 incidents > 30 min)

### 7.4 Communication Channels

| Channel | Purpose | URL |
|---------|---------|-----|
| Slack `#incidents` | Active incident coordination | `https://slack.com/channels/incidents` |
| Slack `#deployments` | Deployment announcements | `https://slack.com/channels/deployments` |
| Slack `#platform` | General platform discussion | `https://slack.com/channels/platform` |
| PagerDuty | Alerting and on-call | `https://chaos-sec.pagerduty.com` |
| GitHub Actions | CI/CD status | `https://github.com/chaos-sec/backend/actions` |
| Grafana | Monitoring dashboards | `https://grafana.chaos-sec.example.com` |

### 7.5 Incident Response Procedure

1. **Acknowledge** the alert in PagerDuty within 15 minutes
2. **Join** the `#incidents` Slack channel and announce the incident:
   > 🔴 **INCIDENT**: [Brief description] — Severity: P[0-3] — IC: @your-name
3. **Diagnose** using the runbook sections above
4. **Communicate** status updates every 15 minutes for P0, every 30 min for P1
5. **Resolve** and verify with smoke tests
6. **Post-mortem** within 48 hours for P0/P1 incidents

### 7.6 Backup and Recovery Contacts

| Scenario | Command | Fallback |
|----------|---------|----------|
| Database backup fails | `./deploy/scripts/backup.sh backup` | Contact DBA; manual `pg_dump` |
| Database restore needed | `./deploy/scripts/restore.sh restore latest --confirm` | Contact DBA; S3 console access |
| S3 access issue | Check AWS IAM permissions | Contact DevOps for temporary credentials |
| Full disaster | Section 5.3 above | Contact all stakeholders; execute DR plan |

---

## Appendix A: Quick Reference Commands

```bash
# ── Check system health ───────────────────────────────────────────
kubectl get pods,svc,deploy -n chaos-sec
curl -s https://api.chaos-sec.example.com/health | jq

# ── View logs ─────────────────────────────────────────────────────
kubectl logs -n chaos-sec -l app=chaos-sec-backend --tail=100 -f
kubectl logs -n chaos-sec -l app=chaos-sec-frontend --tail=50

# ── Scale deployments ──────────────────────────────────────────────
kubectl scale deployment/chaos-sec-backend --replicas=3 -n chaos-sec

# ── Run backup ────────────────────────────────────────────────────
./deploy/scripts/backup.sh backup

# ── Run smoke tests ───────────────────────────────────────────────
./deploy/scripts/smoke-tests.sh

# ── Emergency rollback ─────────────────────────────────────────────
kubectl rollout undo deployment/chaos-sec-backend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-frontend -n chaos-sec
kubectl rollout undo deployment/chaos-sec-mock-siem -n chaos-sec

# ── Port-forward for local debugging ──────────────────────────────
kubectl port-forward -n chaos-sec svc/chaos-sec-backend 8080:8080
kubectl port-forward -n chaos-sec svc/chaos-sec-postgres 5432:5432
```

---

## Appendix B: Deployment History Template

| Date | Version | Deployer | Result | Notes |
|------|---------|----------|--------|--------|
| 2025-01-15 | v1.0.0 | @jash | ✅ Pass | Initial production deployment |
| | | | | |

---

*This runbook is a living document. Update it after every deployment incident or process change.*