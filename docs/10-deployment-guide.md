# Deployment Guide

## Chaos-Sec: An Orchestration Platform for Security Control Validation

**Version:** 1.0.0  
**Last Updated:** 2026-01-15  
**Status:** Draft

---

## Table of Contents

1. [Overview](#overview)
2. [Deployment Prerequisites](#deployment-prerequisites)
3. [Deployment Architecture](#deployment-architecture)
4. [Environment Configuration](#environment-configuration)
5. [Kubernetes Deployment](#kubernetes-deployment)
6. [CI/CD Pipeline](#cicd-pipeline)
7. [Monitoring & Observability](#monitoring--observability)
8. [Backup & Disaster Recovery](#backup--disaster-recovery)
9. [Scaling & Performance](#scaling--performance)
10. [Troubleshooting](#troubleshooting)

---

## Overview

This guide provides comprehensive instructions for deploying Chaos-Sec to production environments. The platform is designed to be deployed on Kubernetes clusters with support for various cloud providers and on-premises installations.

### Deployment Objectives

| Objective | Description |
|-----------|-------------|
| **High Availability** | Ensure 99.5% uptime for the orchestration platform |
| **Security** | Implement defense-in-depth security controls |
| **Scalability** | Support horizontal scaling for increased load |
| **Observability** | Enable comprehensive monitoring and logging |
| **Maintainability** | Simplify updates and rollback procedures |

---

## Deployment Prerequisites

### Hardware Requirements

#### Minimum Production Environment

| Component | Specification | Notes |
|-----------|---------------|-------|
| CPU | 8 cores | 16 cores recommended |
| RAM | 16 GB | 32 GB recommended |
| Storage | 100 GB SSD | 500 GB for extended retention |
| Network | 1 Gbps | Low-latency connection to target clusters |

#### Kubernetes Cluster Requirements

| Component | Minimum | Recommended |
|-----------|---------|-------------|
| Control Plane Nodes | 1 (3 GB RAM) | 3 (8 GB RAM each) |
| Worker Nodes | 2 (4 GB RAM each) | 3 (8 GB RAM each) |
| Kubernetes Version | 1.26+ | 1.28+ |
| Storage Class | Available | SSD-backed preferred |

### Software Prerequisites

| Software | Version | Purpose |
|----------|---------|---------|
| kubectl | 1.28+ | Kubernetes CLI |
| Helm | 3.12+ | Package manager |
| Docker | 24.0+ | Container runtime |
| Terraform | 1.6+ | Infrastructure as Code (optional) |
| Git | 2.40+ | Version control |

### Required Permissions

#### Kubernetes RBAC Permissions

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: chaos-sec-deployer
rules:
- apiGroups: [""]
  resources: ["namespaces", "pods", "services", "configmaps", "secrets"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["apps"]
  resources: ["deployments", "statefulsets", "replicasets"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["networking.k8s.io"]
  resources: ["networkpolicies", "ingresses"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["roles", "rolebindings", "serviceaccounts"]
  verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
```

#### Cloud Provider Permissions (if applicable)

| Provider | Required Permissions |
|----------|---------------------|
| AWS | EKS access, IAM roles, S3 for backups |
| GCP | GKE access, IAM roles, GCS for backups |
| Azure | AKS access, IAM roles, Blob Storage for backups |

---

## Deployment Architecture

### Production Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CHAOS-SEC PRODUCTION DEPLOYMENT                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                        INGRESS LAYER                                 │    │
│  │  ┌─────────────────────────────────────────────────────────────┐    │    │
│  │  │              Load Balancer (Cloud Provider LB)               │    │    │
│  │  │                    TLS Termination                           │    │    │
│  │  └─────────────────────────────────────────────────────────────┘    │    │
│  │                              │                                        │    │
│  │                              ▼                                        │    │
│  │  ┌─────────────────────────────────────────────────────────────┐    │    │
│  │  │              Ingress Controller (nginx/traefik)              │    │    │
│  │  │         ┌──────────────┐  ┌──────────────┐                 │    │    │
│  │  │         │ TLS Secrets  │  │ Rate Limiting│                 │    │    │
│  │  │         └──────────────┘  └──────────────┘                 │    │    │
│  │  └─────────────────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                              │                                                │
│                              ▼                                                │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │                     CHAOS-SEC NAMESPACE                              │    │
│  │  ┌─────────────────────────────────────────────────────────────┐    │    │
│  │  │                    APPLICATION LAYER                         │    │    │
│  │  │                                                              │    │    │
│  │  │  ┌──────────────┐      ┌──────────────┐                     │    │    │
│  │  │  │  Web         │      │  API         │                     │    │    │
│  │  │  │  Frontend    │◀────▶│  Backend     │                     │    │    │
│  │  │  │  (2 replicas)│      │  (3 replicas)│                     │    │    │
│  │  │  └──────────────┘      └──────────────┘                     │    │    │
│  │  │                                                              │    │    │
│  │  │  ┌──────────────┐      ┌──────────────┐                     │    │    │
│  │  │  │  Scheduler   │      │  Worker      │                     │    │    │
│  │  │  │  Service     │      │  Pool        │                     │    │    │
│  │  │  │  (1 replica) │      │  (3 replicas)│                     │    │    │
│  │  │  └──────────────┘      └──────────────┘                     │    │    │
│  │  │                                                              │    │    │
│  │  └─────────────────────────────────────────────────────────────┘    │    │
│  │                                                                      │    │
│  │  ┌─────────────────────────────────────────────────────────────┐    │    │
│  │  │                     DATA LAYER                               │    │    │
│  │  │                                                              │    │    │
│  │  │  ┌──────────────┐      ┌──────────────┐                     │    │    │
│  │  │  │  PostgreSQL  │      │  Redis       │                     │    │    │
│  │  │  │  (Stateful)  │      │  (Cache)     │                     │    │    │
│  │  │  │  1 primary   │      │  1 replica   │                     │    │    │
│  │  │  │  1 replica   │      │              │                     │    │    │
│  │  │  └──────────────┘      └──────────────┘                     │    │    │
│  │  │                                                              │    │    │
│  │  │  ┌──────────────┐      ┌──────────────┐                     │    │    │
│  │  │  │  Message     │      │  Mock        │                     │    │    │
│  │  │  │  Queue       │      │  SIEM        │                     │    │    │
│  │  │  │  (RabbitMQ)  │      │              │                     │    │    │
│  │  │  └──────────────┘      └──────────────┘                     │    │    │
│  │  │                                                              │    │    │
│  │  └─────────────────────────────────────────────────────────────┘    │    │
│  │                                                                      │    │
│  │  ┌─────────────────────────────────────────────────────────────┐    │    │
│  │  │                   MONITORING LAYER                           │    │    │
│  │  │  ┌──────────────┐      ┌──────────────┐                     │    │    │
│  │  │  │  Prometheus  │      │  Grafana     │                     │    │    │
│  │  │  │  (Metrics)   │      │  (Dashboards)│                     │    │    │
│  │  │  └──────────────┘      └──────────────┘                     │    │    │
│  │  └─────────────────────────────────────────────────────────────┘    │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Namespace Structure

```yaml
# chaos-sec namespace
apiVersion: v1
kind: Namespace
metadata:
  name: chaos-sec
  labels:
    app.kubernetes.io/name: chaos-sec
    app.kubernetes.io/part-of: chaos-sec-platform
---
# chaos-sec-experiments namespace (isolated experiment execution)
apiVersion: v1
kind: Namespace
metadata:
  name: chaos-sec-experiments
  labels:
    app.kubernetes.io/name: chaos-sec-experiments
    pod-security.kubernetes.io/enforce: restricted
```

---

## Environment Configuration

### Configuration Files Structure

```
config/
├── base/
│   ├── kustomization.yaml
│   ├── namespace.yaml
│   ├── configmap.yaml
│   └── secrets.yaml
├── overlays/
│   ├── development/
│   │   ├── kustomization.yaml
│   │   ├── replicas.yaml
│   │   └── resources.yaml
│   ├── staging/
│   │   ├── kustomization.yaml
│   │   ├── replicas.yaml
│   │   └── resources.yaml
│   └── production/
│       ├── kustomization.yaml
│       ├── replicas.yaml
│       └── resources.yaml
```

### Base ConfigMap

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: chaos-sec-config
  namespace: chaos-sec
data:
  # Application Settings
  APP_ENV: "production"
  APP_NAME: "Chaos-Sec"
  APP_VERSION: "1.0.0"
  
  # Server Configuration
  SERVER_PORT: "8080"
  SERVER_READ_TIMEOUT: "30s"
  SERVER_WRITE_TIMEOUT: "30s"
  SERVER_SHUTDOWN_TIMEOUT: "60s"
  
  # Database Configuration
  DATABASE_HOST: "postgres-service"
  DATABASE_PORT: "5432"
  DATABASE_NAME: "chaossec"
  DATABASE_SSL_MODE: "require"
  DATABASE_MAX_CONNECTIONS: "100"
  DATABASE_CONNECTION_TIMEOUT: "10s"
  
  # Redis Configuration
  REDIS_HOST: "redis-service"
  REDIS_PORT: "6379"
  REDIS_DB: "0"
  
  # Kubernetes Integration
  KUBECONFIG_PATH: "/var/run/secrets/kubernetes.io/serviceaccount"
  EXPERIMENT_NAMESPACE: "chaos-sec-experiments"
  DEFAULT_POD_TIMEOUT: "600s"
  MAX_CONCURRENT_EXPERIMENTS: "10"
  
  # SIEM Configuration
  SIEM_ENABLED: "true"
  SIEM_PROVIDER: "mock"
  SIEM_ENDPOINT: "http://mock-siem-service:8080"
  SIEM_POLL_INTERVAL: "5s"
  SIEM_ALERT_WINDOW: "300s"
  
  # Security Settings
  JWT_EXPIRY: "3600s"
  JWT_REFRESH_EXPIRY: "604800s"
  PASSWORD_MIN_LENGTH: "12"
  SESSION_TIMEOUT: "28800s"
  
  # Rate Limiting
  RATE_LIMIT_ENABLED: "true"
  RATE_LIMIT_REQUESTS: "100"
  RATE_LIMIT_WINDOW: "60s"
  
  # Logging
  LOG_LEVEL: "info"
  LOG_FORMAT: "json"
  LOG_OUTPUT: "stdout"
  
  # Metrics
  METRICS_ENABLED: "true"
  METRICS_PORT: "9090"
  METRICS_PATH: "/metrics"
```

### Environment-Specific Secrets

```yaml
# DO NOT commit actual secrets to version control
# This is a template for generating secrets
apiVersion: v1
kind: Secret
metadata:
  name: chaos-sec-secrets
  namespace: chaos-sec
type: Opaque
stringData:
  # Database Credentials
  DATABASE_USER: "chaossec_admin"
  DATABASE_PASSWORD: "<GENERATE_SECURE_PASSWORD>"
  
  # JWT Signing Key
  JWT_SECRET: "<GENERATE_32_BYTE_SECRET>"
  
  # Redis Password
  REDIS_PASSWORD: "<GENERATE_SECURE_PASSWORD>"
  
  # API Encryption Key
  API_ENCRYPTION_KEY: "<GENERATE_32_BYTE_KEY>"
  
  # TLS Certificates (if not using cert-manager)
  tls.crt: |
    -----BEGIN CERTIFICATE-----
    <CERTIFICATE_CONTENT>
    -----END CERTIFICATE-----
  
  tls.key: |
    -----BEGIN PRIVATE KEY-----
    <PRIVATE_KEY_CONTENT>
    -----END PRIVATE KEY-----
```

### Secret Generation Script

```bash
#!/bin/bash
# scripts/generate-secrets.sh

set -e

echo "Generating secure secrets for Chaos-Sec deployment..."

# Generate random passwords
generate_password() {
    openssl rand -base64 32 | tr -dc 'a-zA-Z0-9' | head -c 32
}

# Generate secrets
DATABASE_PASSWORD=$(generate_password)
JWT_SECRET=$(openssl rand -base64 32)
REDIS_PASSWORD=$(generate_password)
API_ENCRYPTION_KEY=$(openssl rand -base64 32)

echo "DATABASE_PASSWORD: $DATABASE_PASSWORD"
echo "JWT_SECRET: $JWT_SECRET"
echo "REDIS_PASSWORD: $REDIS_PASSWORD"
echo "API_ENCRYPTION_KEY: $API_ENCRYPTION_KEY"

# Create Kubernetes secret
kubectl create secret generic chaos-sec-secrets \
    --from-literal=database-password="$DATABASE_PASSWORD" \
    --from-literal=jwt-secret="$JWT_SECRET" \
    --from-literal=redis-password="$REDIS_PASSWORD" \
    --from-literal=api-encryption-key="$API_ENCRYPTION_KEY" \
    --namespace=chaos-sec \
    --dry-run=client -o yaml | kubectl apply -f -

echo "Secrets generated and applied successfully!"
```

---

## Kubernetes Deployment

### Helm Chart Structure

```
charts/
└── chaos-sec/
    ├── Chart.yaml
    ├── values.yaml
    ├── values-dev.yaml
    ├── values-staging.yaml
    ├── values-prod.yaml
    └── templates/
        ├── _helpers.tpl
        ├── namespace.yaml
        ├── configmap.yaml
        ├── secrets.yaml
        ├── deployment-frontend.yaml
        ├── deployment-backend.yaml
        ├── deployment-scheduler.yaml
        ├── deployment-worker.yaml
        ├── deployment-mock-siem.yaml
        ├── statefulset-postgres.yaml
        ├── deployment-redis.yaml
        ├── deployment-rabbitmq.yaml
        ├── service-frontend.yaml
        ├── service-backend.yaml
        ├── service-postgres.yaml
        ├── service-redis.yaml
        ├── service-rabbitmq.yaml
        ├── service-mock-siem.yaml
        ├── ingress.yaml
        ├── hpa.yaml
        ├── pdb.yaml
        ├── serviceaccount.yaml
        ├── role.yaml
        ├── rolebinding.yaml
        ├── networkpolicy.yaml
        └── tests/
            └── test-connection.yaml
```

### Main Deployment Manifests

#### Backend Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chaos-sec-backend
  namespace: chaos-sec
  labels:
    app.kubernetes.io/name: chaos-sec
    app.kubernetes.io/component: backend
spec:
  replicas: 3
  selector:
    matchLabels:
      app.kubernetes.io/name: chaos-sec
      app.kubernetes.io/component: backend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: chaos-sec
        app.kubernetes.io/component: backend
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "9090"
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: chaos-sec
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        fsGroup: 1000
      containers:
      - name: backend
        image: chaos-sec/backend:1.0.0
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 8080
          protocol: TCP
        - name: metrics
          containerPort: 9090
          protocol: TCP
        env:
        - name: APP_ENV
          valueFrom:
            configMapKeyRef:
              name: chaos-sec-config
              key: APP_ENV
        - name: DATABASE_PASSWORD
          valueFrom:
            secretKeyRef:
              name: chaos-sec-secrets
              key: database-password
        - name: JWT_SECRET
          valueFrom:
            secretKeyRef:
              name: chaos-sec-secrets
              key: jwt-secret
        resources:
          requests:
            cpu: 250m
            memory: 256Mi
          limits:
            cpu: 1000m
            memory: 1Gi
        livenessProbe:
          httpGet:
            path: /health/live
            port: http
          initialDelaySeconds: 15
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /health/ready
            port: http
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 3
          failureThreshold: 3
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        volumeMounts:
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: tmp
        emptyDir: {}
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: chaos-sec
                  app.kubernetes.io/component: backend
              topologyKey: kubernetes.io/hostname
```

#### Frontend Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: chaos-sec-frontend
  namespace: chaos-sec
  labels:
    app.kubernetes.io/name: chaos-sec
    app.kubernetes.io/component: frontend
spec:
  replicas: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: chaos-sec
      app.kubernetes.io/component: frontend
  template:
    metadata:
      labels:
        app.kubernetes.io/name: chaos-sec
        app.kubernetes.io/component: frontend
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 101
        fsGroup: 101
      containers:
      - name: frontend
        image: chaos-sec/frontend:1.0.0
        imagePullPolicy: IfNotPresent
        ports:
        - name: http
          containerPort: 80
          protocol: TCP
        env:
        - name: REACT_APP_API_URL
          value: "https://chaos-sec.example.com/api"
        - name: REACT_APP_WS_URL
          value: "wss://chaos-sec.example.com/ws"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          httpGet:
            path: /
            port: http
          initialDelaySeconds: 10
          periodSeconds: 15
        readinessProbe:
          httpGet:
            path: /
            port: http
          initialDelaySeconds: 5
          periodSeconds: 10
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
```

#### PostgreSQL StatefulSet

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: chaos-sec
spec:
  serviceName: postgres
  replicas: 2
  selector:
    matchLabels:
      app: postgres
  template:
    metadata:
      labels:
        app: postgres
    spec:
      containers:
      - name: postgres
        image: postgres:15.4
        ports:
        - name: postgres
          containerPort: 5432
        env:
        - name: POSTGRES_DB
          value: "chaossec"
        - name: POSTGRES_USER
          valueFrom:
            secretKeyRef:
              name: chaos-sec-secrets
              key: database-user
        - name: POSTGRES_PASSWORD
          valueFrom:
            secretKeyRef:
              name: chaos-sec-secrets
              key: database-password
        - name: PGDATA
          value: "/var/lib/postgresql/data/pgdata"
        resources:
          requests:
            cpu: 500m
            memory: 1Gi
          limits:
            cpu: 2000m
            memory: 4Gi
        livenessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - $(POSTGRES_USER)
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command:
            - pg_isready
            - -U
            - $(POSTGRES_USER)
          initialDelaySeconds: 5
          periodSeconds: 5
        volumeMounts:
        - name: postgres-storage
          mountPath: /var/lib/postgresql/data
        - name: backup-config
          mountPath: /etc/postgresql/backup
      volumes:
      - name: backup-config
        configMap:
          name: postgres-backup-config
  volumeClaimTemplates:
  - metadata:
      name: postgres-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: "ssd"
      resources:
        requests:
          storage: 50Gi
```

### Ingress Configuration

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: chaos-sec-ingress
  namespace: chaos-sec
  annotations:
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/proxy-body-size: "10m"
    nginx.ingress.kubernetes.io/proxy-read-timeout: "60"
    nginx.ingress.kubernetes.io/proxy-send-timeout: "60"
    nginx.ingress.kubernetes.io/rate-limit: "100"
    nginx.ingress.kubernetes.io/rate-limit-window: "1m"
    cert-manager.io/cluster-issuer: "letsencrypt-prod"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - chaos-sec.example.com
    secretName: chaos-sec-tls
  rules:
  - host: chaos-sec.example.com
    http:
      paths:
      - path: /api
        pathType: Prefix
        backend:
          service:
            name: chaos-sec-backend
            port:
              number: 8080
      - path: /ws
        pathType: Prefix
        backend:
          service:
            name: chaos-sec-backend
            port:
              number: 8080
      - path: /
        pathType: Prefix
        backend:
          service:
            name: chaos-sec-frontend
            port:
              number: 80
```

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: chaos-sec-network-policy
  namespace: chaos-sec
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  # Allow ingress from within the cluster
  - from:
    - namespaceSelector:
        matchLabels:
          name: chaos-sec
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
  egress:
  # Allow DNS resolution
  - to:
    - namespaceSelector: {}
    ports:
    - protocol: UDP
      port: 53
  # Allow communication within chaos-sec namespace
  - to:
    - namespaceSelector:
        matchLabels:
          name: chaos-sec
  # Allow egress to Kubernetes API
  - to:
    - ipBlock:
        cidr: 0.0.0.0/0
    ports:
    - protocol: TCP
      port: 443
```

### Horizontal Pod Autoscaler

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: chaos-sec-backend-hpa
  namespace: chaos-sec
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: chaos-sec-backend
  minReplicas: 3
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 10
        periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
      - type: Percent
        value: 100
        periodSeconds: 15
      - type: Pods
        value: 4
        periodSeconds: 15
      selectPolicy: Max
```

### Pod Disruption Budget

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: chaos-sec-backend-pdb
  namespace: chaos-sec
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app.kubernetes.io/name: chaos-sec
      app.kubernetes.io/component: backend
---
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: chaos-sec-frontend-pdb
  namespace: chaos-sec
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: chaos-sec
      app.kubernetes.io/component: frontend
```

---

## CI/CD Pipeline

### GitHub Actions Workflow

```yaml
# .github/workflows/ci-cd.yaml
name: Chaos-Sec CI/CD

on:
  push:
    branches: [main, develop]
    tags: ['v*']
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  # ========== BUILD ==========
  build-backend:
    runs-on: ubuntu-latest
    steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.21'

    - name: Cache Go modules
      uses: actions/cache@v3
      with:
        path: ~/go/pkg/mod
        key: ${{ runner.os }}-go-${{ hashFiles('**/go.sum') }}

    - name: Install dependencies
      run: go mod download

    - name: Run linter
      run: |
        go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
        golangci-lint run

    - name: Run tests
      run: go test -v -race -coverprofile=coverage.out ./...

    - name: Upload coverage
      uses: codecov/codecov-action@v3
      with:
        files: ./coverage.out

    - name: Build binary
      run: go build -o chaos-sec-backend ./cmd/backend

    - name: Upload artifact
      uses: actions/upload-artifact@v3
      with:
        name: backend-binary
        path: chaos-sec-backend

  build-frontend:
    runs-on: ubuntu-latest
    steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up Node.js
      uses: actions/setup-node@v4
      with:
        node-version: '18'
        cache: 'npm'
        cache-dependency-path: frontend/package-lock.json

    - name: Install dependencies
      working-directory: ./frontend
      run: npm ci

    - name: Run linter
      working-directory: ./frontend
      run: npm run lint

    - name: Run tests
      working-directory: ./frontend
      run: npm test -- --coverage

    - name: Build
      working-directory: ./frontend
      run: npm run build

    - name: Upload artifact
      uses: actions/upload-artifact@v3
      with:
        name: frontend-build
        path: frontend/build

  # ========== SECURITY SCAN ==========
  security-scan:
    runs-on: ubuntu-latest
    needs: [build-backend, build-frontend]
    steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Run Trivy vulnerability scanner
      uses: aquasecurity/trivy-action@master
      with:
        scan-type: 'fs'
        scan-ref: '.'
        format: 'sarif'
        output: 'trivy-results.sarif'

    - name: Upload Trivy results to GitHub Security
      uses: github/codeql-action/upload-sarif@v2
      with:
        sarif_file: 'trivy-results.sarif'

    - name: Run SAST
      uses: securecodewarrior/github-action-add-sarif@v1
      with:
        input-sarif-path: 'semgrep.sarif'
        output-sarif-path: 'semgrep-out.sarif'

  # ========== CONTAINER BUILD ==========
  build-containers:
    runs-on: ubuntu-latest
    needs: [build-backend, build-frontend, security-scan]
    if: github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/')
    permissions:
      contents: read
      packages: write

    steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up Docker Buildx
      uses: docker/setup-buildx-action@v3

    - name: Login to Container Registry
      uses: docker/login-action@v3
      with:
        registry: ${{ env.REGISTRY }}
        username: ${{ github.actor }}
        password: ${{ secrets.GITHUB_TOKEN }}

    - name: Extract metadata (tags, labels)
      id: meta
      uses: docker/metadata-action@v5
      with:
        images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}

    - name: Build and push backend
      uses: docker/build-push-action@v5
      with:
        context: .
        file: ./Dockerfile.backend
        push: true
        tags: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}/backend:${{ github.sha }}
        labels: ${{ steps.meta.outputs.labels }}
        cache-from: type=gha
        cache-to: type=gha,mode=max

    - name: Build and push frontend
      uses: docker/build-push-action@v5
      with:
        context: ./frontend
        file: ./frontend/Dockerfile
        push: true
        tags: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}/frontend:${{ github.sha }}
        labels: ${{ steps.meta.outputs.labels }}
        cache-from: type=gha
        cache-to: type=gha,mode=max

  # ========== DEPLOY ==========
  deploy-staging:
    runs-on: ubuntu-latest
    needs: [build-containers]
    if: github.ref == 'refs/heads/develop'
    environment: staging

    steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up kubectl
      uses: azure/setup-kubectl@v3
      with:
        version: 'v1.28.0'

    - name: Configure kubectl
      run: |
        echo "${{ secrets.STAGING_KUBECONFIG }}" | base64 -d > kubeconfig
        export KUBECONFIG=kubeconfig

    - name: Deploy to staging
      run: |
        kubectl apply -f k8s/overlays/staging/
        kubectl rollout status deployment/chaos-sec-backend -n chaos-sec
        kubectl rollout status deployment/chaos-sec-frontend -n chaos-sec

    - name: Run smoke tests
      run: |
        ./scripts/smoke-tests.sh --environment=staging

  deploy-production:
    runs-on: ubuntu-latest
    needs: [deploy-staging]
    if: startsWith(github.ref, 'refs/tags/')
    environment: production

    steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Set up kubectl
      uses: azure/setup-kubectl@v3
      with:
        version: 'v1.28.0'

    - name: Configure kubectl
      run: |
        echo "${{ secrets.PRODUCTION_KUBECONFIG }}" | base64 -d > kubeconfig
        export KUBECONFIG=kubeconfig

    - name: Deploy to production
      run: |
        kubectl apply -f k8s/overlays/production/
        kubectl rollout status deployment/chaos-sec-backend -n chaos-sec
        kubectl rollout status deployment/chaos-sec-frontend -n chaos-sec

    - name: Run production smoke tests
      run: |
        ./scripts/smoke-tests.sh --environment=production

    - name: Create GitHub Release
      uses: softprops/action-gh-release@v1
      if: startsWith(github.ref, 'refs/tags/')
      with:
        generate_release_notes: true
```

### Deployment Scripts

```bash
#!/bin/bash
# scripts/deploy.sh

set -e

ENVIRONMENT=${1:-staging}
NAMESPACE="chaos-sec"

echo "Deploying Chaos-Sec to ${ENVIRONMENT} environment..."

# Validate environment
case $ENVIRONMENT in
    development|staging|production)
        echo "Environment validated: ${ENVIRONMENT}"
        ;;
    *)
        echo "Invalid environment: ${ENVIRONMENT}"
        echo "Valid options: development, staging, production"
        exit 1
        ;;
esac

# Set context based on environment
case $ENVIRONMENT in
    development)
        KUBE_CONTEXT="kind-chaos-sec-dev"
        ;;
    staging)
        KUBE_CONTEXT="gke-project-staging"
        ;;
    production)
        KUBE_CONTEXT="gke-project-production"
        ;;
esac

echo "Using Kubernetes context: ${KUBE_CONTEXT}"
kubectl config use-context ${KUBE_CONTEXT}

# Apply namespace
echo "Creating namespace..."
kubectl apply -f k8s/overlays/${ENVIRONMENT}/namespace.yaml

# Apply secrets (if not already present)
echo "Applying secrets..."
kubectl apply -f k8s/overlays/${ENVIRONMENT}/secrets.yaml

# Apply ConfigMap
echo "Applying ConfigMap..."
kubectl apply -f k8s/overlays/${ENVIRONMENT}/configmap.yaml

# Apply all resources
echo "Applying resources..."
kubectl apply -k k8s/overlays/${ENVIRONMENT}/

# Wait for deployments
echo "Waiting for deployments to be ready..."
kubectl rollout status deployment/chaos-sec-backend -n ${NAMESPACE} --timeout=300s
kubectl rollout status deployment/chaos-sec-frontend -n ${NAMESPACE} --timeout=300s
kubectl rollout status statefulset/postgres -n ${NAMESPACE} --timeout=600s

# Run health checks
echo "Running health checks..."
./scripts/health-check.sh --environment=${ENVIRONMENT}

echo "Deployment to ${ENVIRONMENT} completed successfully!"
```

---

## Monitoring & Observability

### Prometheus Configuration

```yaml
# prometheus-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: prometheus-config
  namespace: chaos-sec
data:
  prometheus.yml: |
    global:
      scrape_interval: 15s
      evaluation_interval: 15s

    alerting:
      alertmanagers:
      - static_configs:
        - targets:
          - alertmanager:9093

    rule_files:
    - "alert_rules.yml"

    scrape_configs:
    - job_name: 'prometheus'
      static_configs:
      - targets: ['localhost:9090']

    - job_name: 'chaos-sec-backend'
      kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
          - chaos-sec
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_component]
        action: keep
        regex: backend
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: true
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_path]
        action: replace
        target_label: __metrics_path__
        regex: (.+)
      - source_labels: [__address__, __meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
        target_label: __address__

    - job_name: 'chaos-sec-experiments'
      kubernetes_sd_configs:
      - role: pod
        namespaces:
          names:
          - chaos-sec-experiments
      relabel_configs:
      - source_labels: [__meta_kubernetes_pod_label_app_kubernetes_io_managed_by]
        action: keep
        regex: chaos-sec
```

### Alert Rules

```yaml
# alert_rules.yml
groups:
- name: chaos-sec-alerts
  rules:
  - alert: BackendHighErrorRate
    expr: |
      sum(rate(http_requests_total{job="chaos-sec-backend",status=~"5.."}[5m])) 
      / sum(rate(http_requests_total{job="chaos-sec-backend"}[5m])) > 0.05
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "High error rate in Chaos-Sec backend"
      description: "Error rate is {{ $value | humanizePercentage }} over the last 5 minutes"

  - alert: BackendHighLatency
    expr: |
      histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket{job="chaos-sec-backend"}[5m])) by (le)) > 0.5
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "High latency in Chaos-Sec backend"
      description: "95th percentile latency is {{ $value | humanizeDuration }}"

  - alert: ExperimentExecutionFailure
    expr: |
      sum(rate(experiment_executions_total{status="failed"}[1h])) > 5
    for: 10m
    labels:
      severity: warning
    annotations:
      summary: "Multiple experiment failures detected"
      description: "{{ $value }} experiments failed in the last hour"

  - alert: DatabaseConnectionPoolExhausted
    expr: |
      database_connection_pool_available{job="chaos-sec-backend"} < 10
    for: 5m
    labels:
      severity: critical
    annotations:
      summary: "Database connection pool nearly exhausted"
      description: "Only {{ $value }} connections available"

  - alert: PodRestarting
    expr: |
      increase(kube_pod_container_status_restarts_total{namespace="chaos-sec"}[1h]) > 3
    for: 5m
    labels:
      severity: warning
    annotations:
      summary: "Pod restarting frequently"
      description: "Pod {{ $labels.pod }} has restarted {{ $value }} times in the last hour"
```

### Grafana Dashboards

```json
{
  "dashboard": {
    "title": "Chaos-Sec Platform Overview",
    "panels": [
      {
        "title": "Experiment Execution Rate",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(rate(experiment_executions_total[5m])) by (status)",
            "legendFormat": "{{status}}"
          }
        ]
      },
      {
        "title": "API Request Latency (p95)",
        "type": "graph",
        "targets": [
          {
            "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, endpoint))",
            "legendFormat": "{{endpoint}}"
          }
        ]
      },
      {
        "title": "SIEM Alert Correlation Rate",
        "type": "gauge",
        "targets": [
          {
            "expr": "sum(siem_alerts_correlated_total) / sum(siem_alerts_expected_total) * 100"
          }
        ]
      },
      {
        "title": "Backend Pod Resource Usage",
        "type": "graph",
        "targets": [
          {
            "expr": "sum(container_cpu_usage_seconds_total{namespace=\"chaos-sec\", container=\"backend\"}) by (pod)",
            "legendFormat": "{{pod}} CPU"
          },
          {
            "expr": "sum(container_memory_usage_bytes{namespace=\"chaos-sec\", container=\"backend\"}) by (pod)",
            "legendFormat": "{{pod}} Memory"
          }
        ]
      }
    ]
  }
}
```

---

## Backup & Disaster Recovery

### Backup Strategy

```yaml
# postgres-backup-cronjob.yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-backup
  namespace: chaos-sec
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:15.4
            command:
            - /bin/sh
            - -c
            - |
              pg_dump -h postgres-primary -U $DATABASE_USER $DATABASE_NAME | gzip > /backups/backup-$(date +%Y%m%d-%H%M%S).sql.gz
              # Upload to cloud storage
              aws s3 cp /backups/ s3://chaos-sec-backups/postgres/ --recursive
            env:
            - name: DATABASE_USER
              valueFrom:
                secretKeyRef:
                  name: chaos-sec-secrets
                  key: database-user
            - name: DATABASE_NAME
              value: "chaossec"
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: chaos-sec-secrets
                  key: database-password
            volumeMounts:
            - name: backup-volume
              mountPath: /backups
          volumes:
          - name: backup-volume
            persistentVolumeClaim:
              claimName: backup-pvc
          restartPolicy: OnFailure
```

### Disaster Recovery Procedures

| Scenario | Recovery Time Objective (RTO) | Recovery Point Objective (RPO) | Procedure |
|----------|------------------------------|-------------------------------|-----------|
| Single Pod Failure | < 5 minutes | 0 | Automatic restart by Kubernetes |
| Node Failure | < 10 minutes | 0 | Pod rescheduling to healthy node |
| Database Failure | < 30 minutes | < 5 minutes | Failover to replica, restore from backup |
| Complete Cluster Failure | < 4 hours | < 1 hour | Restore from backups to alternate cluster |
| Data Corruption | < 4 hours | < 24 hours | Point-in-time recovery from backups |

### Backup Verification Script

```bash
#!/bin/bash
# scripts/verify-backup.sh

set -e

BACKUP_FILE=$1
S3_BUCKET="chaos-sec-backups"

echo "Verifying backup: ${BACKUP_FILE}"

# Download backup
aws s3 cp s3://${S3_BUCKET}/postgres/${BACKUP_FILE} /tmp/

# Verify gzip integrity
gzip -t /tmp/${BACKUP_FILE}

# Restore to test database
gunzip -c /tmp/${BACKUP_FILE} | psql -h test-db -U test_user test_chaossec

# Run verification queries
psql -h test-db -U test_user test_chaossec <<EOF
SELECT COUNT(*) FROM users;
SELECT COUNT(*) FROM experiments;
SELECT COUNT(*) FROM experiment_runs;
EOF

echo "Backup verification completed successfully!"
```

---

## Scaling & Performance

### Scaling Guidelines

| Metric | Threshold | Action |
|--------|-----------|--------|
| CPU Utilization | > 70% for 5 minutes | Scale up backend replicas |
| Memory Utilization | > 80% for 5 minutes | Scale up or increase limits |
| Request Queue Depth | > 100 | Scale up backend replicas |
| Database Connections | > 80% of pool | Increase pool size or add read replicas |
| Response Time (p95) | > 500ms | Investigate and scale |

### Performance Tuning

#### Database Optimization

```sql
-- Create indexes for common queries
CREATE INDEX CONCURRENTLY idx_experiment_runs_status_created 
ON experiment_runs(status, created_at DESC);

CREATE INDEX CONCURRENTLY idx_siem_validations_run_id 
ON siem_validations(run_id);

-- Analyze tables for query optimization
ANALYZE experiment_runs;
ANALYZE siem_validations;
ANALYZE attack_pods;

-- Set up connection pooling (PgBouncer)
-- Recommended settings:
-- pool_mode = transaction
-- max_client_conn = 1000
-- default_pool_size = 20
```

#### Application Tuning

```yaml
# Backend performance settings
server:
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 120s
  max_header_bytes: 1MB

database:
  max_open_conns: 100
  max_idle_conns: 25
  conn_max_lifetime: 5m

cache:
  enabled: true
  default_ttl: 5m
  max_entries: 10000
```

---

## Troubleshooting

### Common Issues

#### Issue: Pods failing to start

```bash
# Check pod status
kubectl get pods -n chaos-sec

# Describe failing pod
kubectl describe pod <pod-name> -n chaos-sec

# Check logs
kubectl logs <pod-name> -n chaos-sec

# Check events
kubectl get events -n chaos-sec --sort-by='.lastTimestamp'
```

#### Issue: Database connection failures

```bash
# Check database pod
kubectl get pods -n chaos-sec -l app=postgres

# Test database connectivity
kubectl run test-db --rm -it --image=postgres:15.4 -- psql -h postgres-service -U chaossec_admin chaossec

# Check database logs
kubectl logs postgres-0 -n chaos-sec
```

#### Issue: High API latency

```bash
# Check backend metrics
kubectl port-forward svc/chaos-sec-backend 9090:9090 -n chaos-sec
# Visit http://localhost:9090/metrics

# Check for slow queries in database
kubectl exec postgres-0 -n chaos-sec -- psql -c "SELECT * FROM pg_stat_activity WHERE state = 'active';"

# Check resource usage
kubectl top pods -n chaos-sec
```

### Health Check Endpoints

| Endpoint | Purpose | Response |
|----------|---------|----------|
| `/health/live` | Liveness probe | 200 OK if service is alive |
| `/health/ready` | Readiness probe | 200 OK if service is ready |
| `/health/ready` | Readiness probe | 200 OK if service is ready |
| `/health/deep` | Deep health check | 200 OK with dependency status |
| `/metrics` | Prometheus metrics | Prometheus format metrics |

### Support Contacts

| Issue Type | Contact | Escalation Path |
|------------|---------|-----------------|
| Technical Issues | devops@example.com | Platform Team Lead |
| Security Issues | security@example.com | CISO |
| Performance Issues | performance@example.com | Engineering Manager |

---

## Document History

| Version | Date | Author | Changes |
|---------|------|--------|---------|
| 1.0 | 2026-01-15 | Chaos-Sec Team | Initial deployment guide |

---

## Appendix

### A. Deployment Checklist

- [ ] Prerequisites validated
- [ ] Kubernetes cluster accessible
- [ ] Secrets generated and applied
- [ ] ConfigMaps configured
- [ ] Storage classes available
- [ ] Ingress controller deployed
- [ ] TLS certificates configured
- [ ] Monitoring stack deployed
- [ ] Backup procedures tested
- [ ] Smoke tests passed
- [ ] Documentation updated

### B. Rollback Procedure

```bash
#!/bin/bash
# scripts/rollback.sh

ENVIRONMENT=$1
PREVIOUS_VERSION=$2

echo "Rolling back ${ENVIRONMENT} to version ${PREVIOUS_VERSION}..."

# Rollback deployment
kubectl rollout undo deployment/chaos-sec-backend -n chaos-sec --to-revision=${PREVIOUS_VERSION}
kubectl rollout undo deployment/chaos-sec-frontend -n chaos-sec --to-revision=${PREVIOUS_VERSION}

# Wait for rollback
kubectl rollout status deployment/chaos-sec-backend -n chaos-sec
kubectl rollout status deployment/chaos-sec-frontend -n chaos-sec

# Verify rollback
./scripts/health-check.sh --environment=${ENVIRONMENT}

echo "Rollback completed!"
```
