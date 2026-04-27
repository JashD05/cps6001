# Chaos-Sec Troubleshooting Guide

**Version:** 1.0.0
**Last Updated:** 2026-04-21

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Quick Diagnostic Checklist](#2-quick-diagnostic-checklist)
3. [Installation & Startup Issues](#3-installation--startup-issues)
4. [Database Issues](#4-database-issues)
5. [Redis Issues](#5-redis-issues)
6. [Kubernetes Cluster Issues](#6-kubernetes-cluster-issues)
7. [Experiment Execution Issues](#7-experiment-execution-issues)
8. [SIEM Integration Issues](#8-siem-integration-issues)
9. [Authentication & Authorization Issues](#9-authentication--authorization-issues)
10. [Frontend & Dashboard Issues](#10-frontend--dashboard-issues)
11. [Networking & Connectivity Issues](#11-networking--connectivity-issues)
12. [Performance Issues](#12-performance-issues)
13. [Deployment Issues](#13-deployment-issues)
14. [Log Reference](#14-log-reference)
15. [Recovery Procedures](#15-recovery-procedures)

---

## 1. Introduction

### 1.1 Purpose

This guide helps operators diagnose and resolve common issues with the Chaos-Sec platform. It covers problems from installation failures to runtime errors, with step-by-step resolution procedures.

### 1.2 When to Use This Guide

- The platform won't start or becomes unresponsive
- Experiments fail to execute or produce unexpected results
- SIEM integration is not working
- Users cannot log in or access features
- Performance is degraded
- Deployment or upgrade procedures fail

### 1.3 Before You Begin

Gather the following information before troubleshooting:

| Information | How to Obtain |
|-------------|---------------|
| Backend logs | `docker compose logs backend` or `kubectl logs -n chaos-sec -l app=chaos-sec-backend` |
| Frontend logs | Browser developer console (F12) |
| Database logs | `docker compose logs postgres` or `kubectl logs -n chaos-sec -l app=chaos-sec-postgres` |
| Redis logs | `docker compose logs redis` or `kubectl logs -n chaos-sec -l app=chaos-sec-redis` |
| Health status | `curl http://localhost:8080/health/ready` |
| Configuration | Check environment variables and `.env` file |
| Version | `git log --oneline -1` |

### 1.4 Support Escalation

If you cannot resolve an issue using this guide:

1. Collect the relevant logs and diagnostic information
2. Note the exact steps to reproduce the problem
3. Check the FAQ in the [User Guide](./12-user-guide.md)
4. Contact the platform maintainer with the collected information

---

## 2. Quick Diagnostic Checklist

Run through this checklist to quickly identify common problems:

### 2.1 Health Check

```bash
# Liveness — should return 200
curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/health/live

# Readiness — should return 200
curl -s http://localhost:8080/health/ready
```

**Expected result:**
```json
{
  "status": "ready",
  "checks": {
    "database": "healthy",
    "redis": "healthy"
  }
}
```

**If readiness returns 503**, the response body tells you which component is unhealthy:
- `"database": "unhealthy"` → See [Section 4](#4-database-issues)
- `"redis": "unhealthy"` → See [Section 5](#5-redis-issues)

### 2.2 Component Status

| Check | Command | Expected |
|-------|---------|----------|
| Backend running | `curl -s http://localhost:8080/health/live` | `200 OK` |
| PostgreSQL reachable | `psql -h localhost -U chaossec -d chaos_sec -c "SELECT 1"` | `1` row |
| Redis reachable | `redis-cli -h localhost ping` | `PONG` |
| Frontend serving | `curl -s -o /dev/null -w "%{http_code}" http://localhost:3000` | `200` |
| Docker services up | `docker compose ps` | All services `Up` |
| K8s pods running | `kubectl get pods -n chaos-sec` | All `Running` |

### 2.3 Common Configuration Mistakes

| Mistake | Symptom | Fix |
|---------|---------|-----|
| Wrong `CHAOS_DB_PASSWORD` | `password authentication failed` | Match password in PostgreSQL and env var |
| Missing `CHAOS_JWT_SECRET` | Warning log at startup; insecure tokens | Set a strong secret |
| `CHAOS_K8S_IN_CLUSTER=true` outside cluster | Cannot connect to K8s API | Set to `false` and provide `CHAOS_K8S_KUBECONFIG` |
| `CHAOS_DB_SSLMODE=require` without TLS | Connection refused | Use `disable` for local dev, `require` only with TLS |
| Port already in use | `bind: address already in use` | Change `CHAOS_SERVER_PORT` or stop conflicting process |

---

## 3. Installation & Startup Issues

### 3.1 Backend Won't Start

#### Symptom: `go run cmd/backend/main.go serve` exits immediately

**Step 1: Check configuration validation errors**

```bash
CHAOS_LOG_LEVEL=debug go run cmd/backend/main.go serve 2>&1 | head -30
```

Common validation errors:
- `server port is required` → Set `CHAOS_SERVER_PORT=8080`
- `database host is required` → Set `CHAOS_DB_HOST=localhost`
- `jwt expiry must be at least 1 minute` → Fix `CHAOS_JWT_EXPIRY`

**Step 2: Check port availability**

```bash
# Check if port 8080 is in use
lsof -i :8080
# Or
ss -tlnp | grep 8080
```

Kill the conflicting process or change the port:
```bash
CHAOS_SERVER_PORT=8081 go run cmd/backend/main.go serve
```

**Step 3: Check database connectivity**

```bash
psql -h localhost -U chaossec -d chaos_sec -c "SELECT 1"
```

If this fails, PostgreSQL is not running or credentials are wrong. See [Section 4](#4-database-issues).

#### Symptom: Migration errors on startup

```
ERROR: relation "experiments" already exists
```

The database was partially migrated. Check migration status:
```bash
./scripts/migrate.sh status
```

If the schema is corrupt, you can force the version:
```bash
# CAREFUL: This does not run the migration, just sets the version number
./scripts/migrate.sh force <version>
```

Or start fresh for development:
```bash
# Drop and recreate the database
psql -h localhost -U chaossec -c "DROP DATABASE chaos_sec;"
psql -h localhost -U chaossec -c "CREATE DATABASE chaos_sec;"
./scripts/migrate.sh up
```

### 3.2 Frontend Won't Start

#### Symptom: `npm run dev` fails

**Step 1: Check Node.js version**
```bash
node --version  # Should be 18.x LTS
```

**Step 2: Clear node_modules and reinstall**
```bash
rm -rf node_modules package-lock.json
npm install
```

**Step 3: Check port availability**
```bash
lsof -i :3000
```

#### Symptom: Frontend loads but shows API errors

Check the `VITE_API_URL` environment variable:
```bash
# In frontend/.env or your shell
VITE_API_URL=http://localhost:8080/api/v1
VITE_WS_URL=ws://localhost:8080/ws
```

Verify the backend is reachable from the frontend:
```bash
curl http://localhost:8080/health/live
```

#### Symptom: `npm install` fails with peer dependency errors

```bash
# Use legacy peer deps resolution
npm install --legacy-peer-deps
```

### 3.3 Docker Compose Issues

#### Symptom: `docker compose up -d` fails

**Step 1: Check Docker is running**
```bash
docker info
```

**Step 2: Check for port conflicts**
```bash
# Default ports used: 5432 (Postgres), 6379 (Redis), 8080 (Backend), 9100 (Mock SIEM)
ss -tlnp | grep -E '5432|6379|8080|9100'
```

**Step 3: Pull fresh images**
```bash
docker compose pull
docker compose up -d
```

**Step 4: Check container logs**
```bash
docker compose logs postgres
docker compose logs redis
docker compose logs mock-siem
```

#### Symptom: Containers start but immediately exit

```bash
docker compose ps  # Look for "Exit" status
docker compose logs <service-name>  # Check the exit reason
```

Common causes:
- Out of disk space: `df -h`
- Out of memory: `free -h`
- Invalid configuration in `docker-compose.yml`

### 3.4 Kind Cluster Issues

#### Symptom: `kind create cluster` fails

**Step 1: Check Docker is running** (kind uses Docker containers)

**Step 2: Check for conflicting clusters**
```bash
kind get clusters
# Delete old cluster if needed
kind delete cluster --name chaos-sec-dev
```

**Step 3: Check the kind config**
```bash
kind create cluster --name chaos-sec-dev --config scripts/kind-config.yaml --retain
# --retain keeps the cluster even on failure for debugging
```

#### Symptom: kind cluster created but kubectl won't connect

```bash
# Set the kubeconfig
kind get kubeconfig --name chaos-sec-dev > ~/.kube/config

# Verify
kubectl cluster-info
kubectl get nodes
```

---

## 4. Database Issues

### 4.1 Connection Refused

**Symptoms:** Backend logs show `connection refused` or `dial tcp [::1]:5432: connect: connection refused`

**Diagnosis:**

```bash
# Is PostgreSQL running?
docker compose ps postgres
# Or for K8s:
kubectl get pods -n chaos-sec -l app=chaos-sec-postgres

# Can you connect manually?
psql -h localhost -U chaossec -d chaos_sec
```

**Solutions:**

1. **PostgreSQL not running:** Start it
   ```bash
   docker compose up -d postgres
   ```

2. **Wrong host/port:** Check `CHAOS_DB_HOST` and `CHAOS_DB_PORT`
   ```bash
   echo $CHAOS_DB_HOST  # Should match the PostgreSQL host
   echo $CHAOS_DB_PORT  # Should be 5432
   ```

3. **PostgreSQL still starting up:** Wait for it
   ```bash
   docker compose logs -f postgres  # Watch for "database system is ready to accept connections"
   ```

### 4.2 Authentication Failed

**Symptoms:** `password authentication failed for user chaossec`

**Solutions:**

1. **Verify credentials match:**
   ```bash
   # Check the env var
   echo $CHAOS_DB_PASSWORD

   # Try connecting with the same credentials
   PGPASSWORD=chaossec_dev psql -h localhost -U chaossec -d chaos_sec
   ```

2. **Reset the password in PostgreSQL:**
   ```bash
   docker compose exec postgres psql -U postgres -c \
     "ALTER USER chaossec WITH PASSWORD 'new_password';"
   ```
   Then update `CHAOS_DB_PASSWORD=new_password`.

3. **Check `pg_hba.conf`:** For custom PostgreSQL installations, ensure `md5` or `scram-sha-256` authentication is enabled.

### 4.3 Database Does Not Exist

**Symptoms:** `database "chaos_sec" does not exist`

**Solutions:**

1. **Create the database:**
   ```bash
   docker compose exec postgres psql -U postgres -c "CREATE DATABASE chaos_sec OWNER chaossec;"
   ```

2. **Run migrations:**
   ```bash
   ./scripts/migrate.sh up
   ```

### 4.4 Migration Errors

#### Symptom: `dirty database` error

This happens when a migration was interrupted mid-execution.

**Solutions:**

1. **Check the current version:**
   ```bash
   ./scripts/migrate.sh status
   ```

2. **Force to the last known good version:**
   ```bash
   ./scripts/migrate.sh force <last_good_version>
   ```

3. **Re-run migrations:**
   ```bash
   ./scripts/migrate.sh up
   ```

#### Symptom: `no such table` errors after migration

The migrations may have been run out of order.

**Solutions:**

1. For development, reset the database:
   ```bash
   psql -h localhost -U chaossec -c "DROP SCHEMA public CASCADE; CREATE SCHEMA public;"
   ./scripts/migrate.sh up
   ```

2. For production, restore from backup:
   ```bash
   ./deploy/scripts/restore.sh
   ```

### 4.5 Connection Pool Exhaustion

**Symptoms:** Slow responses, timeouts, logs show `sorry, too many clients already`

**Diagnosis:**

```sql
SELECT count(*) AS total,
       count(*) FILTER (WHERE state = 'active') AS active,
       count(*) FILTER (WHERE state = 'idle') AS idle,
       count(*) FILTER (WHERE state = 'idle in transaction') AS idle_in_transaction
FROM pg_stat_activity
WHERE datname = 'chaos_sec';
```

**Solutions:**

1. **Increase max connections:**
   ```bash
   CHAOS_DB_MAX_OPEN_CONNS=50
   ```

2. **Reduce idle connection lifetime:**
   ```bash
   CHAOS_DB_CONN_MAX_IDLE_TIME=30s
   ```

3. **Kill stuck idle connections:**
   ```sql
   SELECT pg_terminate_backend(pid)
   FROM pg_stat_activity
   WHERE datname = 'chaos_sec'
     AND state = 'idle in transaction'
     AND state_change < now() - interval '5 minutes';
   ```

4. **Run database optimization:**
   ```bash
   ./scripts/db-optimize.sh --analyze
   ```

### 4.6 Slow Queries

**Diagnosis:**

```sql
-- Find queries running longer than 5 seconds
SELECT query, state, now() - query_start AS duration
FROM pg_stat_activity
WHERE state = 'active' AND now() - query_start > interval '5 seconds';
```

**Solutions:**

1. Run `ANALYZE` to update statistics:
   ```bash
   ./scripts/db-optimize.sh --analyze
   ```

2. Check for missing indexes:
   ```bash
   ./scripts/db-optimize.sh --report
   ```

3. Check for long-running transactions:
   ```sql
   SELECT pid, now() - xact_start AS duration, query
   FROM pg_stat_activity
   WHERE xact_start IS NOT NULL
   ORDER BY duration DESC;
   ```

---

## 5. Redis Issues

### 5.1 Connection Refused

**Symptoms:** Rate limiting falls back to in-memory; token blacklisting doesn't work on logout.

**Diagnosis:**

```bash
# Is Redis running?
docker compose ps redis
# Or:
kubectl get pods -n chaos-sec -l app=chaos-sec-redis

# Can you connect?
redis-cli -h localhost -p 6379 ping
```

**Solutions:**

1. **Start Redis:**
   ```bash
   docker compose up -d redis
   ```

2. **Check credentials:**
   ```bash
   # If password-protected
   redis-cli -h localhost -p 6379 -a <password> ping
   ```

3. **Check `CHAOS_REDIS_HOST` and `CHAOS_REDIS_PORT`:**
   ```bash
   echo $CHAOS_REDIS_HOST  # Should be localhost or the Redis service name
   echo $CHAOS_REDIS_PORT  # Should be 6379
   ```

### 5.2 Redis Memory Issues

**Symptoms:** `OOM command not allowed when used memory > maxmemory`; key evictions occurring.

**Diagnosis:**

```bash
redis-cli -h localhost info memory
```

**Solutions:**

1. **Increase max memory** in Redis configuration
2. **Set eviction policy** in `redis.conf`:
   ```
   maxmemory-policy allkeys-lru
   ```
3. **Check for large keys:**
   ```bash
   redis-cli -h localhost --bigkeys
   ```

### 5.3 Rate Limit Not Working with Redis

**Symptoms:** Rate limiting behaves inconsistently across backend replicas.

**Solutions:**

1. Verify Redis is reachable from all backend instances:
   ```bash
   # From inside the backend container
   redis-cli -h <redis-host> -p 6379 ping
   ```

2. Check that all backends use the same Redis instance (same `CHAOS_REDIS_HOST`).

3. The in-memory fallback is per-process — each backend has its own limiter. Without Redis, rate limits are not shared.

---

## 6. Kubernetes Cluster Issues

### 6.1 Cluster Registration Fails

**Symptoms:** `POST /api/v1/clusters` returns 400 or 500 error.

**Diagnosis:**

1. **Verify the API endpoint is reachable:**
   ```bash
   curl -k https://<k8s-api-endpoint>:6443/healthz
   ```

2. **Verify certificates are valid:**
   ```bash
   # Decode and inspect the CA certificate
   echo "<base64-ca-cert>" | base64 -d | openssl x509 -text -noout

   # Check expiry
   echo "<base64-ca-cert>" | base64 -d | openssl x509 -noout -enddate
   ```

3. **Verify the service account permissions:**
   ```bash
   kubectl auth can-i create pods --as=system:serviceaccount:chaos-sec:chaos-sec -n chaos-sec
   kubectl auth can-i list namespaces --as=system:serviceaccount:chaos-sec:chaos-sec
   ```

**Solutions:**

1. **Regenerate certificates** if expired
2. **Create the required RBAC resources:**
   ```bash
   kubectl apply -f - <<EOF
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
   EOF

   kubectl create serviceaccount chaos-sec -n chaos-sec
   kubectl create clusterrolebinding chaos-sec \
     --clusterrole=chaos-sec-role \
     --serviceaccount=chaos-sec:chaos-sec
   ```

### 6.2 Cluster Shows "Unreachable"

**Symptoms:** Cluster health endpoint returns `status: "unreachable"`.

**Step-by-step diagnosis:**

```bash
# 1. Is the cluster API reachable from the Chaos-Sec backend?
#    (Run from inside the backend pod/container)
curl -k https://<k8s-api-endpoint>:6443/healthz

# 2. Is the cluster itself healthy?
kubectl get nodes

# 3. Are there network policies blocking the connection?
kubectl get networkpolicies -n chaos-sec

# 4. Check the backend logs for connection errors
docker compose logs backend | grep -i "kubernetes\|cluster\|unreachable"
```

**Solutions:**

1. **Fix network connectivity** between the backend and the cluster
2. **Update certificates** if they have expired
3. **Re-register the cluster** with updated credentials

### 6.3 Namespaces Not Listed

**Symptoms:** `GET /api/v1/clusters/:id/namespaces` returns empty list or error.

**Diagnosis:**

```bash
# Verify the service account can list namespaces
kubectl auth can-i list namespaces --as=system:serviceaccount:chaos-sec:chaos-sec

# Check if the cluster role binding exists
kubectl get clusterrolebinding chaos-sec
```

**Solutions:**

1. Add the `list` verb for `namespaces` to the ClusterRole
2. Recreate the ClusterRoleBinding if missing

### 6.4 Kind Cluster Not Working

**Symptoms:** kind cluster was created but experiments can't create pods.

**Diagnosis:**

```bash
# Check nodes are ready
kubectl get nodes

# Check the chaos-sec namespace exists
kubectl get namespace chaos-sec

# Try creating a test pod
kubectl run test --image=busybox --restart=Never -n chaos-sec -- echo hello
```

**Solutions:**

1. If nodes are `NotReady`, restart Docker and recreate the cluster:
   ```bash
   kind delete cluster --name chaos-sec-dev
   kind create cluster --name chaos-sec-dev --config scripts/kind-config.yaml
   ```

2. If the namespace doesn't exist:
   ```bash
   kubectl create namespace chaos-sec
   ```

3. If Docker is resource-constrained, increase Docker's memory limit (Docker Desktop → Settings → Resources)

---

## 7. Experiment Execution Issues

### 7.1 Experiment Stuck in "Pending"

**Symptoms:** Experiment status never changes from `pending` to `running`.

**Diagnosis:**

1. Check the target cluster health:
   ```bash
   curl http://localhost:8080/api/v1/clusters/<id>/health \
     -H "Authorization: Bearer <token>"
   ```

2. Check the namespace exists in the cluster:
   ```bash
   kubectl get namespace <target-namespace>
   ```

3. Check backend worker pool logs:
   ```bash
   docker compose logs backend | grep -i "worker\|queue\|experiment"
   ```

**Solutions:**

1. **Cluster unreachable:** See [Section 6.2](#62-cluster-shows-unreachable)
2. **Namespace missing:** Create it
   ```bash
   kubectl create namespace <target-namespace>
   ```
3. **Worker pool full:** Increase `CHAOS_K8S_MAX_CONCURRENT` or wait for running experiments to complete

### 7.2 Experiment Fails Immediately

**Symptoms:** Experiment transitions from `running` to `failed` within seconds.

**Diagnosis:**

1. Check the experiment's error message:
   ```bash
   curl http://localhost:8080/api/v1/experiments/<id> \
     -H "Authorization: Bearer <token>" | jq '.data.result'
   ```

2. Check if the attacker pod was created:
   ```bash
   kubectl get pods -n <target-namespace> -l app=chaos-sec-attacker
   ```

3. Check pod events:
   ```bash
   kubectl describe pod <pod-name> -n <target-namespace>
   ```

**Common causes and solutions:**

| Cause | Error Message | Solution |
|-------|---------------|----------|
| Image pull error | `ImagePullBackOff` | Check image exists and is accessible |
| Insufficient resources | `Insufficient cpu/memory` | Free up resources or increase node capacity |
| RBAC denied | `forbidden: User cannot create pods` | Fix ClusterRole permissions |
| Namespace not found | `namespaces "<ns>" not found` | Create the namespace |
| Network policy blocks pod creation | Pod stays `Pending` | Check network policies |

### 7.3 Attacker Pod Not Created

**Symptoms:** Experiment shows as "running" but no pod appears in the cluster.

**Diagnosis:**

```bash
# Check for the attacker pod (named chaos-sec-<experiment-id>-<run-id>)
kubectl get pods -A | grep chaos-sec

# Check events in the namespace
kubectl get events -n <namespace> --sort-by='.lastTimestamp'

# Check the Kubernetes client logs
docker compose logs backend | grep -i "kubernetes\|pod\|create"
```

**Solutions:**

1. **RBAC issue:** Verify the service account can create pods
2. **Resource quota:** Check for `ResourceQuota` limits in the namespace
3. **Admission controller:** Check if a `LimitRange` or `PodSecurityPolicy` is blocking the pod

### 7.4 Experiment Times Out

**Symptoms:** Experiment status changes to `timed_out` after the configured duration.

**Diagnosis:**

1. Check the attacker pod logs:
   ```bash
   kubectl logs <pod-name> -n <namespace>
   ```

2. Check if the pod is stuck waiting for a response:
   ```bash
   kubectl describe pod <pod-name> -n <namespace>
   ```

**Solutions:**

1. **Increase the timeout:** Set a longer `CHAOS_K8S_POD_TIMEOUT`
2. **Check network connectivity:** The pod may be unable to reach its target
3. **Check for deadlocks:** If the attack script waits for user input, it will hang

### 7.5 Experiment Results Show 0% Detection Rate

**Symptoms:** SIEM validation reports 0% detection even though attacks ran.

This is a SIEM integration issue. See [Section 8](#8-siem-integration-issues).

---

## 8. SIEM Integration Issues

### 8.1 SIEM Connection Fails

**Symptoms:** SIEM status shows "disconnected"; test connection returns error.

**Diagnosis:**

```bash
# Test the connection via API
curl -X POST http://localhost:8080/api/v1/siem/test-connection \
  -H "Authorization: Bearer <token>"

# Test directly
curl -k https://<siem-endpoint>/health
```

**Common causes and solutions:**

| Cause | Error | Solution |
|-------|-------|----------|
| Wrong endpoint URL | `connection refused` | Verify `CHAOS_SIEM_ENDPOINT` |
| Invalid API key | `401 Unauthorized` | Verify `CHAOS_SIEM_API_KEY` |
| TLS certificate issue | `certificate verify failed` | Check CA certs; use `CHAOS_SIEM_ENDPOINT=http://...` for non-TLS |
| Firewall blocking | `i/o timeout` | Open the required port |
| SIEM service down | `connection reset` | Check SIEM server health |

### 8.2 SIEM Enabled but Not Used

**Symptoms:** `CHAOS_SIEM_ENABLED=true` but experiments skip SIEM validation.

**Diagnosis:**

1. Check the experiment's validation settings:
   ```json
   {
     "validation": {
       "siem_alert_type": "network_flow",
       "time_window_seconds": 300,
       "expected_alert_count": 1
     }
   }
   ```

2. Check if the SIEM provider is configured:
   ```bash
   echo $CHAOS_SIEM_PROVIDER  # Should not be empty
   echo $CHAOS_SIEM_ENDPOINT  # Should not be empty
   ```

**Solutions:**

1. Ensure `validation` is configured in the experiment
2. Set `CHAOS_SIEM_PROVIDER` to a valid value (`splunk`, `elastic`, `sentinel`, `other`)

### 8.3 Mock SIEM Issues

**Symptoms:** Mock SIEM returns errors or is unreachable.

**Diagnosis:**

```bash
# Is the mock SIEM running?
curl http://localhost:9100/health

# Check Docker logs
docker compose logs mock-siem
```

**Solutions:**

1. **Restart the mock SIEM:**
   ```bash
   docker compose restart mock-siem
   ```

2. **Port conflict:**
   ```bash
   lsof -i :9100
   # Change port in docker-compose.yml if needed
   ```

3. **Rebuild the mock SIEM container:**
   ```bash
   docker compose up -d --build mock-siem
   ```

### 8.4 Detection Rate Lower Than Expected

**Symptoms:** SIEM validation reports partial or failed status even though you expected alerts.

**Diagnosis:**

1. Query the SIEM directly for the experiment's alerts:
   ```bash
   curl -X POST http://localhost:8080/api/v1/siem/alerts/query \
     -H "Authorization: Bearer <token>" \
     -H "Content-Type: application/json" \
     -d '{
       "from": "2026-04-21T09:00:00Z",
       "to": "2026-04-21T10:30:00Z",
       "alert_type": "network_flow"
     }'
   ```

2. Check the alert type matches exactly (case-sensitive)
3. Check the severity meets or exceeds expectations

**Common causes:**

| Cause | Detection Result | Solution |
|-------|-----------------|----------|
| Alert type mismatch | 0% | Ensure `siem_alert_type` matches the SIEM's alert type exactly |
| Severity mismatch | Partial | Expected `critical` but SIEM produced `high` — lower expected severity |
| Propagation delay too short | 0% | Increase `time_window_seconds` (default: 300) |
| Time window too narrow | Partial | Extend `from`/`to` range |
| SIEM not ingesting | 0% | Check SIEM data pipeline health |

### 8.5 SIEM Alert Query Returns No Results

**Diagnosis:**

1. Verify alerts exist in the SIEM by querying the SIEM directly (not through Chaos-Sec)
2. Check the time range — alerts older than the SIEM retention period may be purged
3. Verify the query filter values match the actual alert fields

---

## 9. Authentication & Authorization Issues

### 9.1 Login Fails with 401

**Symptoms:** `POST /api/v1/auth/login` returns 401.

**Diagnosis:**

```bash
curl -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@chaos-sec.local","password":"admin"}' -v
```

**Common causes:**

| Cause | Response | Solution |
|-------|----------|----------|
| Wrong email/password | `invalid_credentials` | Verify credentials; reset if needed |
| Account disabled | `account_disabled` | Re-enable the user in the database |
| User not found | `invalid_credentials` | Create the user first |
| Database unreachable | 500 error | See [Section 4](#4-database-issues) |

**Reset admin password (emergency):**

```sql
-- Connect to PostgreSQL
psql -h localhost -U chaossec -d chaos_sec

-- Generate a new bcrypt hash (requires Go or an online tool)
-- For "admin" with cost 12:
-- UPDATE users SET password_hash = '<bcrypt_hash>' WHERE email = 'admin@chaos-sec.local';
```

Or use the backend's password utility:
```bash
go run -tags=tools cmd/tools/reset-password/main.go --email admin@chaos-sec.local --password newpassword
```

### 9.2 Token Expired Errors

**Symptoms:** API returns `expired_token` even with a recent login.

**Causes:**

1. **Clock skew:** The server's clock is significantly different from the client's
2. **Very short token expiry:** `CHAOS_JWT_EXPIRY` is set too low
3. **Token was issued by a different instance** (if JWT secrets differ between replicas)

**Solutions:**

1. Synchronize clocks: `ntpdate pool.ntp.org`
2. Increase expiry: `CHAOS_JWT_EXPIRY=2h`
3. Ensure all backend replicas use the same `CHAOS_JWT_SECRET`

### 9.3 403 Forbidden on Permitted Actions

**Symptoms:** User gets 403 for actions their role should allow.

**Diagnosis:**

1. Check the user's current permissions:
   ```bash
   curl http://localhost:8080/api/v1/auth/me \
     -H "Authorization: Bearer <token>" | jq '.data.role.permissions'
   ```

2. Check the required permission for the endpoint (see [API Reference](./14-api-reference.md))

3. Verify the user's role assignment in the database:
   ```sql
   SELECT r.name, p.name AS permission
   FROM users u
   JOIN roles r ON u.role_id = r.id
   JOIN role_permissions rp ON r.id = rp.role_id
   JOIN permissions p ON rp.permission_id = p.id
   WHERE u.email = '<user-email>';
   ```

**Solutions:**

1. Update the user's role if they need more permissions
2. Check if the `admin:all` permission is missing from the admin role
3. Verify organization scope — non-admins can only access their own org's resources

### 9.4 Registration Fails

**Symptoms:** `POST /api/v1/auth/register` returns 403.

**Diagnosis:**

- The caller must have `admin:all` or `users:manage` permission
- Non-admins can only create users within their own organization
- The target organization must be active
- The email must be unique

**Solutions:**

1. Use an admin token for the registration
2. Ensure the `organization_id` matches the caller's organization (unless admin)
3. Verify the `role_id` exists and is valid

---

## 10. Frontend & Dashboard Issues

### 10.1 Blank Page on Load

**Symptoms:** Browser shows a white/blank page.

**Diagnosis:**

1. Open browser developer console (F12) — check for JavaScript errors
2. Check the network tab for failed API requests
3. Verify the frontend was built correctly:
   ```bash
   cd frontend && npm run build 2>&1 | tail -5
   ```

**Common causes:**

| Cause | Console Error | Solution |
|-------|---------------|----------|
| API URL wrong | `ERR_CONNECTION_REFUSED` | Fix `VITE_API_URL` |
| Build failed | `Unexpected token` | Rebuild: `npm run build` |
| Browser cache | Stale JavaScript | Hard refresh: `Ctrl+Shift+R` |
| Missing env vars | `undefined is not an object` | Set `VITE_API_URL` and `VITE_WS_URL` before building |

### 10.2 Dashboard Data Not Updating

**Symptoms:** Stale data on the dashboard despite recent experiments.

**Solutions:**

1. **Check auto-refresh interval** — Settings → General → Auto-refresh
2. **Manual refresh** — Click the refresh icon or press `Ctrl+Shift+R`
3. **WebSocket disconnected** — Check browser console for WebSocket errors; verify `VITE_WS_URL`
4. **API rate limiting** — If you see 429 responses, the auto-refresh may be too frequent

### 10.3 Login Page Loop

**Symptoms:** User is redirected back to login after successful authentication.

**Diagnosis:**

1. Check browser localStorage for the access token
2. Check the token expiry — if tokens expire immediately, the JWT secret may have changed
3. Verify the Redux state in the browser's React DevTools

**Solutions:**

1. Clear browser localStorage and cookies, then re-login
2. If the JWT secret was rotated, all existing tokens are invalid — all users must re-login
3. Check `CHAOS_JWT_EXPIRY` — a very short value causes immediate expiry

### 10.4 CORS Errors in Browser

**Symptoms:** Browser console shows `Access-Control-Allow-Origin` errors.

**Diagnosis:**

In development, CORS should allow all origins. In production, it must be properly configured.

**Solutions:**

1. **Development:** Verify the backend is running with `CHAOS_LOG_LEVEL=debug` — CORS middleware logs warnings for restricted origins
2. **Production:** Configure the allowed origin in the nginx ingress or backend CORS settings
3. **Proxy approach:** Use the nginx reverse proxy (as configured in `deploy/kubernetes/configmap.yaml`) to serve both frontend and API from the same origin

---

## 11. Networking & Connectivity Issues

### 11.1 Backend Cannot Reach Kubernetes API

**Symptoms:** Cluster health check fails; experiments can't create pods.

**Diagnosis:**

```bash
# From the backend container/pod
curl -k https://<k8s-api-endpoint>:6443/healthz

# Check DNS resolution
nslookup <k8s-api-hostname>
```

**Solutions:**

1. **Firewall rules:** Ensure the backend can reach the K8s API port (typically 6443)
2. **DNS resolution:** If using a hostname, ensure it resolves correctly
3. **In-cluster networking:** If `CHAOS_K8S_IN_CLUSTER=true`, the K8s API is available at `https://kubernetes.default.svc`
4. **Kubeconfig path:** If running outside the cluster, verify `CHAOS_K8S_KUBECONFIG` points to a valid file

### 11.2 Backend Cannot Reach SIEM

**Symptoms:** SIEM test connection fails with timeout.

**Diagnosis:**

```bash
# Test connectivity from the backend's network context
curl -v https://<siem-endpoint>/health

# Check DNS
nslookup <siem-hostname>
```

**Solutions:**

1. Open the required port in firewalls
2. Verify the SIEM endpoint URL is correct (including scheme and port)
3. For TLS issues, check certificate chain:
   ```bash
   openssl s_client -connect <siem-host>:<port>
   ```

### 11.3 WebSocket Connection Drops

**Symptoms:** Real-time updates stop appearing in the dashboard.

**Diagnosis:**

1. Check browser console for WebSocket errors
2. Check if the backend WebSocket endpoint is reachable:
   ```bash
   wscat -c ws://localhost:8080/ws
   ```

**Solutions:**

1. **Nginx/Ingress configuration:** Ensure WebSocket upgrade headers are set:
   ```nginx
   proxy_set_header Upgrade $http_upgrade;
   proxy_set_header Connection "upgrade";
   ```
2. **Idle timeout:** Increase the WebSocket idle timeout in the ingress/proxy
3. **Network interruption:** The client auto-reconnects with exponential backoff (1s→30s, max 10 retries)

### 11.4 DNS Resolution Failures

**Symptoms:** Services can't reach each other by hostname.

**In Kubernetes:** Check CoreDNS:
```bash
kubectl get pods -n kube-system -l k8s-app=kube-dns
kubectl logs -n kube-system -l k8s-app=kube-dns
```

**In Docker Compose:** Services use the service name as hostname (e.g., `postgres`, `redis`). Ensure services are on the same Docker network.

---

## 12. Performance Issues

### 12.1 Slow API Responses

**Symptoms:** API requests take > 5 seconds.

**Diagnosis:**

1. Check backend resource usage:
   ```bash
   # Docker
   docker stats chaos-sec-backend

   # Kubernetes
   kubectl top pods -n chaos-sec
   ```

2. Check database query times:
   ```sql
   SELECT mean_exec_time, calls, query
   FROM pg_stat_statements
   ORDER BY mean_exec_time DESC
   LIMIT 10;
   ```

3. Check for rate limiting delays (429 responses)

**Solutions:**

1. **Scale horizontally:** Increase backend replicas
2. **Optimize database:** See [Section 4.6](#46-slow-queries)
3. **Reduce logging overhead:** Set `CHAOS_LOG_LEVEL=info` (not `debug`)
4. **Increase resources:** Adjust CPU/memory limits in the deployment

### 12.2 High Memory Usage

**Symptoms:** OOM kills, frequent HPA scaling.

**Diagnosis:**

```bash
# Check Go runtime metrics
curl http://localhost:9090/metrics | grep go_memstats
# Check goroutine count
curl http://localhost:9090/metrics | grep go_goroutines
```

**Solutions:**

1. **Reduce concurrent operations:** Lower `CHAOS_K8S_MAX_CONCURRENT`
2. **Reduce DB pool size:** Lower `CHAOS_DB_MAX_OPEN_CONNS`
3. **Check for goroutine leaks:** If goroutine count keeps growing, there may be a leak — restart the backend and monitor

### 12.3 High CPU Usage

**Symptoms:** Backend consuming > 80% CPU consistently.

**Diagnosis:**

1. Enable profiling:
   ```bash
   curl http://localhost:9090/debug/pprof/profile?seconds=30 > profile.pb.gz
   go tool pprof profile.pb.gz
   ```

2. Check for infinite loops in experiment processing

**Solutions:**

1. Scale horizontally with HPA
2. Reduce `CHAOS_K8S_MAX_CONCURRENT`
3. Increase `CHAOS_SERVER_READ_TIMEOUT` and `CHAOS_SERVER_WRITE_TIMEOUT`

### 12.4 Database Growing Large

**Diagnosis:**

```sql
SELECT pg_database_size('chaos_sec') / 1024 / 1024 AS size_mb;

SELECT relname, pg_relation_size(C.oid) / 1024 / 1024 AS size_mb
FROM pg_class C
LEFT JOIN pg_namespace N ON (N.oid = C.relnamespace)
WHERE N.nspname = 'public'
ORDER BY pg_relation_size(C.oid) DESC
LIMIT 10;
```

**Solutions:**

1. **VACUUM and ANALYZE:**
   ```bash
   ./scripts/db-optimize.sh --analyze
   ```

2. **Archive old experiment data** (implement a data retention policy)
3. **Add disk space** or use a larger PVC

---

## 13. Deployment Issues

### 13.1 Pods Not Starting

**Symptoms:** Kubernetes pods stay in `Pending`, `CrashLoopBackOff`, or `ImagePullBackOff`.

**Pending:**
```bash
kubectl describe pod <pod-name> -n chaos-sec
# Look at Events section for scheduling failures
```

| Cause | Solution |
|-------|----------|
| Insufficient resources | Add nodes or reduce resource requests |
| PVC not bound | Check StorageClass and available PVs |
| Node selector mismatch | Check node labels |

**CrashLoopBackOff:**
```bash
kubectl logs <pod-name> -n chaos-sec --previous
```

| Cause | Solution |
|-------|----------|
| Config error | Check env vars from ConfigMap/Secret |
| DB not ready | Add init container to wait for DB |
| Missing secret | Create the required Secret |

**ImagePullBackOff:**
```bash
kubectl describe pod <pod-name> -n chaos-sec | grep -A5 Events
```

| Cause | Solution |
|-------|----------|
| Image not found | Build and push the image first |
| Private registry | Create imagePullSecret |
| Wrong tag | Check image tag in deployment |

### 13.2 Database Migration Init Container Fails

**Symptoms:** Backend pod restarts because the init container (`db-migrate`) fails.

**Diagnosis:**
```bash
kubectl logs <pod-name> -n chaos-sec -c db-migrate
```

**Solutions:**

1. Check database connectivity from the cluster
2. Verify `DATABASE_URL` secret is correct
3. If migration is dirty, force the version (see [Section 4.4](#44-migration-errors))

### 13.3 Ingress Not Working

**Symptoms:** Cannot reach the application via the ingress URL.

**Diagnosis:**
```bash
kubectl get ingress -n chaos-sec
kubectl describe ingress -n chaos-sec
```

**Solutions:**

1. Check the ingress controller is running:
   ```bash
   kubectl get pods -n ingress-nginx
   ```

2. Check DNS records point to the ingress controller's external IP

3. Check TLS certificate:
   ```bash
   openssl s_client -connect app.chaos-sec.io:443
   ```

4. Check ingress annotations for WebSocket support

### 13.4 Helm/Terraform Issues

**Symptoms:** Infrastructure provisioning fails.

**Terraform:**
```bash
terraform plan  # Review changes before applying
terraform apply -target=<specific_resource>  # Apply one resource at a time
```

**Common issues:**
- AWS credentials not configured
- S3 bucket for state backend doesn't exist
- IAM permissions insufficient

---

## 14. Log Reference

### 14.1 Log Locations

| Component | Command |
|-----------|---------|
| Backend (Docker) | `docker compose logs -f backend` |
| Backend (K8s) | `kubectl logs -f -n chaos-sec -l app=chaos-sec-backend -c chaos-sec-backend` |
| Frontend (Docker) | `docker compose logs -f frontend` |
| Frontend (K8s) | `kubectl logs -f -n chaos-sec -l app=chaos-sec-frontend` |
| PostgreSQL | `docker compose logs -f postgres` |
| Redis | `docker compose logs -f redis` |
| Mock SIEM | `docker compose logs -f mock-siem` |

### 14.2 Structured Log Fields

Backend logs use structured JSON format with these fields:

| Field | Description |
|-------|-------------|
| `level` | Log level: `debug`, `info`, `warn`, `error`, `fatal` |
| `ts` | Unix timestamp |
| `logger` | Component name (e.g., `middleware`, `auth`, `experiment`) |
| `msg` | Human-readable message |
| `method` | HTTP method |
| `path` | Request path |
| `status` | HTTP response status code |
| `latency` | Request duration |
| `client_ip` | Client IP address |
| `request_id` | Unique request identifier |
| `user_id` | Authenticated user ID (if logged in) |

### 14.3 Common Log Messages

| Log Message | Level | Meaning | Action |
|-------------|-------|---------|--------|
| `request` | info | Normal request processed | None |
| `rate limit exceeded` | warn | Rate limit triggered | Consider increasing limits |
| `failed to connect to SMTP server` | error | Email notifications failing | Check SMTP config |
| `rate limit exceeded (local)` | warn | Using in-memory rate limiter (no Redis) | Check Redis connectivity |
| `SMTP auth failed` | error | Email notifications failing | Check SMTP credentials |
| `failed to query SIEM alerts` | error | SIEM query failed | Check SIEM connectivity |
| `mock SIEM health check failed` | warn | Mock SIEM unreachable | Restart mock SIEM |
| `using insecure JWT secret` | warn | `CHAOS_JWT_SECRET` not set | Set a strong secret |

### 14.4 Enabling Debug Logging

```bash
# Temporary (process-level)
CHAOS_LOG_LEVEL=debug CHAOS_LOG_FORMAT=console go run cmd/backend/main.go serve

# In Docker Compose (add to docker-compose.yml environment)
CHAOS_LOG_LEVEL=debug

# In Kubernetes (update ConfigMap)
kubectl edit configmap chaos-sec-config -n chaos-sec
# Change CHAOS_LOG_LEVEL to "debug"
# Restart the backend
kubectl rollout restart deployment/chaos-sec-backend -n chaos-sec
```

> **Warning:** Debug logging produces significant output and may expose sensitive data (tokens, query parameters). Disable it as soon as troubleshooting is complete.

---

## 15. Recovery Procedures

### 15.1 Backend Crash Recovery

If the backend process crashes:

```bash
# Docker Compose — automatically restarts (restart: unless-stopped)
docker compose logs backend --tail 50  # Check crash reason
docker compose up -d backend          # Restart if not auto-restarted

# Kubernetes — automatically restarts via deployment controller
kubectl get pods -n chaos-sec -l app=chaos-sec-backend
kubectl logs -n chaos-sec <pod-name> --previous  # Check crash reason
```

### 15.2 Database Recovery

```bash
# 1. Stop the backend to prevent writes
docker compose stop backend
# Or: kubectl scale deployment chaos-sec-backend --replicas=0 -n chaos-sec

# 2. Restore from backup
./deploy/scripts/restore.sh --file <backup-file>

# 3. Run any pending migrations
./scripts/migrate.sh up

# 4. Restart the backend
docker compose start backend
# Or: kubectl scale deployment chaos-sec-backend --replicas=2 -n chaos-sec

# 5. Verify
curl http://localhost:8080/health/ready
```

### 15.3 Redis Data Loss Recovery

Redis data loss is non-critical. The system continues to function with these temporary effects:

| Lost Data | Effect | Duration |
|-----------|--------|----------|
| Rate limit counters | Limits reset for all users | Until next window |
| Token blacklist | Revoked tokens valid until expiry | Max 1 hour (access token lifetime) |
| Session data | Users may need to re-login | Until tokens expire |

No recovery action is needed — data rebuilds automatically as requests come in.

### 15.4 Full System Recovery

If the entire system is down:

```bash
# 1. Start infrastructure
docker compose up -d postgres redis
# Wait for healthy
docker compose exec postgres pg_isready

# 2. Restore database (if needed)
./deploy/scripts/restore.sh

# 3. Run migrations
./scripts/migrate.sh up

# 4. Start application
docker compose up -d backend frontend mock-siem

# 5. Verify
curl http://localhost:8080/health/ready
curl http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@chaos-sec.local","password":"admin"}'
```

### 15.5 Kubernetes Full Recovery

```bash
# 1. Check cluster health
kubectl get nodes
kubectl get pods -n chaos-sec

# 2. If nodes are down, wait for them to recover or add new nodes

# 3. If pods are stuck, delete them and let the controller recreate
kubectl delete pods -n chaos-sec --all

# 4. If the database is corrupted, restore from backup
kubectl exec -n chaos-sec chaos-sec-postgres-0 -- \
  pg_restore -U chaossec -d chaos_sec < /backup/latest.dump

# 5. Verify all services
./deploy/scripts/smoke-tests.sh
```

### 15.6 Emergency Access

If you are locked out of the system:

1. **Reset admin password** directly in the database:
   ```sql
   -- Connect to PostgreSQL
   psql -h localhost -U chaossec -d chaos_sec

   -- Check admin user exists
   SELECT id, email, is_active FROM users WHERE email = 'admin@chaos-sec.local';

   -- Re-enable if disabled
   UPDATE users SET is_active = true WHERE email = 'admin@chaos-sec.local';
   ```

2. **Bypass rate limiting:** Set `CHAOS_RATE_LIMIT_ENABLED=false` temporarily

3. **Reset all tokens:** Flush Redis
   ```bash
   redis-cli FLUSHALL
   ```

---

## Appendix

### A. Smoke Test Checklist

After any recovery or restart, verify these endpoints:

| Check | URL | Expected |
|-------|-----|----------|
| Liveness | `GET /health/live` | `200` |
| Readiness | `GET /health/ready` | `200` with `database: healthy, redis: healthy` |
| Login | `POST /api/v1/auth/login` | `200` with tokens |
| Profile | `GET /api/v1/auth/me` | `200` with user data |
| Experiments list | `GET /api/v1/experiments` | `200` with list |
| Clusters list | `GET /api/v1/clusters` | `200` with list |
| SIEM status | `GET /api/v1/siem/status` | `200` |
| Frontend | `GET /` | `200` HTML |

### B. Useful Commands Quick Reference

```bash
# Check all service health
docker compose ps

# View real-time backend logs
docker compose logs -f backend

# View real-time database logs
docker compose logs -f postgres

# Check PostgreSQL connections
psql -h localhost -U chaossec -d chaos_sec -c "SELECT count(*) FROM pg_stat_activity;"

# Check Redis memory
redis-cli info memory

# Check Kubernetes pod status
kubectl get pods -n chaos-sec -o wide

# Check Kubernetes events
kubectl get events -n chaos-sec --sort-by='.lastTimestamp' | tail -20

# Run database optimization
./scripts/db-optimize.sh --report

# Run security scan
./scripts/security-scan.sh

# Run smoke tests
./deploy/scripts/smoke-tests.sh
```

### C. Configuration Reset

To reset all configuration to defaults for a clean start:

```bash
# 1. Stop all services
docker compose down -v  # -v removes volumes (database data!)

# 2. Remove local state
rm -f .env backend/.env

# 3. Start fresh
docker compose up -d
cd backend && go run cmd/backend/main.go migrate
cd backend && go run cmd/backend/main.go serve
cd frontend && npm install && npm run dev
```

> **Warning:** `docker compose down -v` permanently deletes all database data. Use only when you want a completely fresh start.

---

**Document Version:** 1.0.0
**Last Updated:** 2026-04-21