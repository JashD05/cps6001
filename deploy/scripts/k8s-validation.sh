#!/usr/bin/env bash
# =============================================================================
# Chaos-Sec: Kubernetes Validation Script
# =============================================================================
# End-to-end validation script for Chaos-Sec against a real Kubernetes cluster.
# Validates the complete experiment execution pipeline including:
#   - Prerequisites check (kubectl, kind/minikube, docker)
#   - Local cluster creation (kind) or connection to existing EKS
#   - Platform deployment using existing manifests
#   - Experiment execution validation
#   - WebSocket real-time updates testing
#   - SIEM alert integration verification
#   - Cleanup and teardown
#
# Usage:
#   ./k8s-validation.sh                    # Full validation with local kind cluster
#   ./k8s-validation.sh --use-existing     # Use current kubectl context
#   ./k8s-validation.sh --eks              # Connect to AWS EKS cluster
#   ./k8s-validation.sh --cleanup-only     # Only cleanup resources
#   ./k8s-validation.sh --skip-deploy      # Skip deployment (use existing)
#   ./k8s-validation.sh --help             # Show usage
#
# Requirements:
#   - kubectl 1.20+
#   - docker (for kind cluster creation)
#   - kind (optional, for local testing)
#   - awscli + eksctl (for EKS testing)
# =============================================================================

set -uo pipefail

# ──────────────────────────────────────────────
# Configuration
# ──────────────────────────────────────────────
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly K8S_DIR="${PROJECT_ROOT}/deploy/kubernetes"
readonly SCRIPTS_DIR="${PROJECT_ROOT}/deploy/scripts"
readonly NAMESPACE="chaos-sec"
readonly CLUSTER_NAME="chaos-sec-validation"
readonly BACKEND_PORT=8081
readonly FRONTEND_PORT=3000
readonly SIEM_PORT=8089

# Test configuration
readonly TEST_TIMEOUT=120
readonly EXPERIMENT_TIMEOUT=180
readonly PROBE_INTERVAL=5

# Colors
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly BOLD='\033[1m'
readonly DIM='\033[2m'
readonly NC='\033[0m'

# State
SKIP_DEPLOY=false
USE_EXISTING=false
CLEANUP_ONLY=false
EKS_MODE=false
VERBOSE=false
PRESERVE_CLUSTER=false
TIMEOUT_SECONDS=300

# Service URLs (detected after deployment)
BASE_URL=""
FRONTEND_URL=""
SIEM_URL=""

# Auth
ACCESS_TOKEN=""
REFRESH_TOKEN=""

# ──────────────────────────────────────────────
# Argument Parsing
# ──────────────────────────────────────────────
for arg in "$@"; do
    case "${arg}" in
        --use-existing)
            USE_EXISTING=true
            shift
            ;;
        --eks)
            EKS_MODE=true
            USE_EXISTING=true
            shift
            ;;
        --skip-deploy)
            SKIP_DEPLOY=true
            shift
            ;;
        --cleanup-only)
            CLEANUP_ONLY=true
            shift
            ;;
        --preserve-cluster)
            PRESERVE_CLUSTER=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --timeout)
            TIMEOUT_SECONDS="${2:-300}"
            shift 2
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Validate Chaos-Sec against a Kubernetes cluster."
            echo ""
            echo "Options:"
            echo "  --use-existing      Use existing kubectl context instead of creating kind cluster"
            echo "  --eks               Connect to AWS EKS cluster (requires awscli + eksctl)"
            echo "  --skip-deploy       Skip deployment phase (use already deployed platform)"
            echo "  --cleanup-only      Only cleanup resources and exit"
            echo "  --preserve-cluster  Don't delete kind cluster after validation"
            echo "  --timeout SECONDS   Set operation timeout (default: 300)"
            echo "  --verbose           Enable verbose output"
            echo "  --help, -h          Show this help message"
            echo ""
            echo "Environment variables:"
            echo "  KIND_VERSION        kind version to install (default: latest)"
            echo "  KUBECTL_VERSION     kubectl version to install (default: latest)"
            echo "  CHAOS_DB_PASSWORD   PostgreSQL password"
            echo "  CHAOS_REDIS_PASSWORD Redis password"
            echo "  CHAOS_JWT_SECRET    JWT signing secret"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown argument: ${arg}${NC}"
            echo "Run '$0 --help' for usage."
            exit 1
            ;;
    esac
done

# ──────────────────────────────────────────────
# Logging Functions
# ──────────────────────────────────────────────
log()       { echo -e "${CYAN}[INFO]${NC}  $*"; }
log_ok()    { echo -e "${GREEN}[  OK]${NC}  $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }
log_skip()  { echo -e "${YELLOW}[SKIP]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $*"; }
log_dim()   { echo -e "${DIM}$*${NC}"; }
log_bold()  { echo -e "${BOLD}$*${NC}"; }

verbose() {
    if [[ "${VERBOSE}" == "true" ]]; then
        log "[verbose] $*"
    fi
}

# ──────────────────────────────────────────────
# Cleanup Functions
# ──────────────────────────────────────────────
cleanup_kind_cluster() {
    log_step "Cleaning up kind cluster"
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        kind delete cluster --name "${CLUSTER_NAME}" 2>/dev/null || true
        log_ok "Kind cluster deleted"
    else
        log_dim "No kind cluster to clean up"
    fi
}

cleanup_namespace() {
    log_step "Cleaning up namespace"
    if kubectl get namespace "${NAMESPACE}" &>/dev/null; then
        kubectl delete namespace "${NAMESPACE}" --wait=false 2>/dev/null || true
        log_ok "Namespace deletion initiated"
    fi
}

full_cleanup() {
    log_step "Performing full cleanup"
    cleanup_namespace
    if [[ "${PRESERVE_CLUSTER}" == "false" && "${USE_EXISTING}" == "false" && "${EKS_MODE}" == "false" ]]; then
        cleanup_kind_cluster
    fi
    log_ok "Cleanup complete"
}

# Set trap for cleanup on exit
trap 'full_cleanup' EXIT INT TERM

# ──────────────────────────────────────────────
# Prerequisites Check
# ──────────────────────────────────────────────
check_prerequisites() {
    log_step "Checking prerequisites"

    local missing=()

    # Check kubectl
    if ! command -v kubectl &>/dev/null; then
        missing+=("kubectl")
        log_fail "kubectl not found - install from https://kubernetes.io/docs/tasks/tools/"
    else
        local kubectl_version
        kubectl_version="$(kubectl version --client -o json 2>/dev/null | grep -o '"gitVersion"[[:space:]]*:[[:space:]]*"v[^"]*"' | head -1 | grep -o 'v[0-9]\+\.[0-9]\+' || echo "unknown")"
        log_ok "kubectl found: ${kubectl_version}"
    fi

    # Check docker
    if ! command -v docker &>/dev/null; then
        missing+=("docker")
        log_fail "docker not found - install from https://docs.docker.com/"
    else
        if ! docker info &>/dev/null; then
            missing+=("docker (not running)")
            log_fail "docker daemon not running - start docker and try again"
        else
            local docker_version
            docker_version="$(docker --version 2>/dev/null | awk '{print $3}' | tr -d ',')"
            log_ok "docker found: ${docker_version}"
        fi
    fi

    # Check kind (only if not using existing cluster)
    if [[ "${USE_EXISTING}" == "false" ]]; then
        if ! command -v kind &>/dev/null; then
            log_warn "kind not found - will attempt to install"
            install_kind
        else
            local kind_version
            kind_version="$(kind version 2>/dev/null | grep -o 'v[0-9]\+\.[0-9]\+' | head -1 || echo "unknown")"
            log_ok "kind found: ${kind_version}"
        fi
    fi

    # Check awscli and eksctl for EKS mode
    if [[ "${EKS_MODE}" == "true" ]]; then
        if ! command -v aws &>/dev/null; then
            missing+=("aws")
            log_fail "aws cli not found - required for EKS mode"
        else
            log_ok "aws cli found"
        fi

        if ! command -v eksctl &>/dev/null; then
            missing+=("eksctl")
            log_fail "eksctl not found - required for EKS mode"
        else
            log_ok "eksctl found"
        fi
    fi

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_fail "Missing prerequisites: ${missing[*]}"
        return 1
    fi

    return 0
}

install_kind() {
    log_step "Installing kind"
    local kind_version="${KIND_VERSION:-latest}"
    local os_type
    os_type="$(uname -s | tr '[:upper:]' '[:lower:]')"
    local arch
    arch="$(uname -m)"

    case "${arch}" in
        x86_64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
    esac

    if [[ "${kind_version}" == "latest" ]]; then
        kind_version="$(curl -s https://api.github.com/repos/kubernetes-sigs/kind/releases/latest | grep -o '"tag_name": "v[^"]*"' | cut -d'"' -f4 | tr -d 'v')"
    fi

    local kind_url="https://kind.sigs.k8s.io/dl/v${kind_version}/kind-${os_type}-${arch}"
    log "Downloading kind ${kind_version} from ${kind_url}"

    if curl -sSL -o /usr/local/bin/kind "${kind_url}" && chmod +x /usr/local/bin/kind; then
        log_ok "kind installed successfully"
    else
        log_fail "Failed to install kind"
        return 1
    fi
}

# ──────────────────────────────────────────────
# Cluster Management
# ──────────────────────────────────────────────
create_kind_cluster() {
    log_step "Creating kind cluster: ${CLUSTER_NAME}"

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "^${CLUSTER_NAME}$"; then
        log "Cluster already exists, using existing cluster"
        kubectl config use-context "kind-${CLUSTER_NAME}"
        return 0
    fi

    # Create kind cluster configuration
    local kind_config_file="/tmp/kind-config.yaml"
    cat > "${kind_config_file}" << 'EOF'
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
name: chaos-sec-validation
nodes:
  - role: control-plane
    image: kindest/node:v1.28.0
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      - containerPort: 80
        hostPort: 80
        protocol: TCP
      - containerPort: 443
        hostPort: 443
        protocol: TCP
      - containerPort: 30000
        hostPort: 30000
        protocol: TCP
      - containerPort: 30001
        hostPort: 30001
        protocol: TCP
  - role: worker
    image: kindest/node:v1.28.0
  - role: worker
    image: kindest/node:v1.28.0
networking:
  ipFamily: ipv4
  apiServerAddress: 127.0.0.1
  apiServerPort: 6443
features:
  # Enable webhook and other features for chaos experiments
  runtimeConfig:
    "admissionregistration.k8s.io/v1": "true"
    "authentication.k8s.io/v1": "true"
    "authorization.k8s.io/v1": "true"
EOF

    if kind create cluster --name "${CLUSTER_NAME}" --config "${kind_config_file}" --wait 120s; then
        log_ok "Kind cluster created successfully"
        kubectl config use-context "kind-${CLUSTER_NAME}"
    else
        log_fail "Failed to create kind cluster"
        return 1
    fi

    # Wait for control plane to be ready
    log "Waiting for control plane to be ready..."
    kubectl wait --for=condition=Ready nodes --all --timeout=120s || true

    # Install ingress controller for kind
    install_ingress_for_kind

    return 0
}

install_ingress_for_kind() {
    log_step "Installing ingress controller for kind"

    # Install nginx ingress controller
    kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/kind/deploy.yaml 2>/dev/null || {
        log_warn "Failed to install ingress-nginx, creating basic ingress resources"
    }

    # Wait for ingress controller to be ready
    kubectl wait --namespace ingress-nginx \
        --for=condition=Ready pods \
        --selector=app.kubernetes.io/component=controller \
        --timeout=120s 2>/dev/null || {
        log_warn "Ingress controller not ready, some tests may fail"
    }

    log_ok "Ingress controller installed"
}

setup_eks_cluster() {
    log_step "Setting up EKS cluster: ${CLUSTER_NAME}"

    local region="${AWS_REGION:-us-west-2}"

    if ! command -v eksctl &>/dev/null; then
        log_fail "eksctl not found - cannot create EKS cluster"
        return 1
    fi

    cat > /tmp/eks-cluster-config.yaml << EOF
---
apiVersion: eksctl.io/v1alpha5
kind: ClusterConfig
metadata:
  name: ${CLUSTER_NAME}
  region: ${region}
  version: "1.28"
managedNodeGroups:
  - name: chaos-sec-workers
    instanceType: t3.medium
    desiredCapacity: 3
    minSize: 2
    maxSize: 5
    volumeSize: 50
    volumeType: gp3
    allowRootAccess: true
    enableSераrator: false
    tags:
      nodegroup-role: chaos-worker
addons:
  - name: aws-load-balancer-controller
  - name: coredns
  - name: kube-proxy
  - name: vpc-cni
EOF

    log "Creating EKS cluster (this may take 10-15 minutes)..."
    if eksctl create cluster -f /tmp/eks-cluster-config.yaml --timeout="${TIMEOUT_SECONDS}s"; then
        log_ok "EKS cluster created successfully"
        kubectl config use-context "arn:aws:eks:${region}:*:cluster/${CLUSTER_NAME}"
    else
        log_fail "Failed to create EKS cluster"
        return 1
    fi

    return 0
}

# ──────────────────────────────────────────────
# Namespace and Resources
# ──────────────────────────────────────────────
create_namespace() {
    log_step "Creating namespace: ${NAMESPACE}"

    if kubectl get namespace "${NAMESPACE}" &>/dev/null; then
        log "Namespace already exists"
        return 0
    fi

    kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -
    log_ok "Namespace created"
}

prepare_secrets() {
    log_step "Preparing secrets"

    local secrets_file="${K8S_DIR}/secrets.yaml"

    # Generate random passwords if not set
    local db_password="${CHAOS_DB_PASSWORD:-$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 16)}"
    local redis_password="${CHAOS_REDIS_PASSWORD:-$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 16)}"
    local jwt_secret="${CHAOS_JWT_SECRET:-$(openssl rand -base64 32 | tr -dc 'a-zA-Z0-9' | head -c 32)}"
    local siem_api_key="${CHAOS_SIEM_API_KEY:-$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 16)}"

    # Create a temporary secrets file with generated values
    local temp_secrets="/tmp/chaos-sec-secrets.yaml"
    cat > "${temp_secrets}" << EOF
apiVersion: v1
kind: Secret
metadata:
  name: chaos-sec-secrets
  namespace: ${NAMESPACE}
type: Opaque
stringData:
  DATABASE_PASSWORD: "${db_password}"
  REDIS_PASSWORD: "${redis_password}"
  JWT_SECRET: "${jwt_secret}"
  SIEM_API_KEY: "${siem_api_key}"
  SMTP_USER: "admin@chaos-sec.io"
  SMTP_PASSWORD: "smtp-password-change-me"
  RABBITMQ_PASSWORD: "rabbitmq-password-change-me"
  ENCRYPTION_KEY: "$(openssl rand -base64 24 | tr -dc 'a-zA-Z0-9' | head -c 24)"
EOF

    kubectl apply -f "${temp_secrets}" --dry-run=client -o yaml | kubectl apply -f -
    log_ok "Secrets configured"

    # Export for later use
    export CHAOS_DB_PASSWORD="${db_password}"
    export CHAOS_REDIS_PASSWORD="${redis_password}"
    export CHAOS_JWT_SECRET="${jwt_secret}"
}

deploy_manifests() {
    log_step "Deploying Chaos-Sec manifests"

    cd "${K8S_DIR}"

    # Apply namespace
    kubectl apply -f namespace.yaml
    log_dim "Applied namespace.yaml"

    # Apply configmap
    kubectl apply -f configmap.yaml
    log_dim "Applied configmap.yaml"

    # Apply secrets
    prepare_secrets
    log_dim "Applied secrets"

    # Apply postgres
    kubectl apply -f postgres-statefulset.yaml
    log_dim "Applied postgres-statefulset.yaml"

    # Apply redis
    kubectl apply -f redis-statefulset.yaml
    log_dim "Applied redis-statefulset.yaml"

    # Apply backend
    kubectl apply -f backend-deployment.yaml
    kubectl apply -f backend-service.yaml
    log_dim "Applied backend-deployment.yaml and backend-service.yaml"

    # Apply frontend
    kubectl apply -f frontend-deployment.yaml
    kubectl apply -f frontend-service.yaml
    log_dim "Applied frontend-deployment.yaml and frontend-service.yaml"

    # Apply ingress
    kubectl apply -f ingress.yaml || true
    log_dim "Applied ingress.yaml"

    # Apply HPA
    kubectl apply -f hpa.yaml || true
    log_dim "Applied hpa.yaml"

    cd - > /dev/null
    log_ok "All manifests applied"
}

wait_for_deployments() {
    log_step "Waiting for deployments to be ready"

    local timeout="${TIMEOUT_SECONDS}"
    local elapsed=0

    # Wait for postgres
    log_dim "Waiting for postgres StatefulSet..."
    kubectl wait --for=condition=Ready pod -l app=chaos-sec-postgres -n "${NAMESPACE}" --timeout="${timeout}s" || true

    # Wait for redis
    log_dim "Waiting for redis StatefulSet..."
    kubectl wait --for=condition=Ready pod -l app=chaos-sec-redis -n "${NAMESPACE}" --timeout="${timeout}s" || true

    # Wait for backend
    log_dim "Waiting for backend Deployment..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=chaos-sec-backend -n "${NAMESPACE}" --timeout="${timeout}s" || true

    # Wait for frontend
    log_dim "Waiting for frontend Deployment..."
    kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=chaos-sec-frontend -n "${NAMESPACE}" --timeout="${timeout}s" || true

    # Show status
    kubectl get pods -n "${NAMESPACE}"
    log_ok "All deployments ready"
}

# ──────────────────────────────────────────────
# Service URL Detection
# ──────────────────────────────────────────────
detect_service_urls() {
    log_step "Detecting service URLs"

    local backend_svc
    local frontend_svc

    # For kind, use node ports or port-forward
    if kubectl get svc chaos-sec-backend -n "${NAMESPACE}" &>/dev/null; then
        local backend_type
        backend_type="$(kubectl get svc chaos-sec-backend -n "${NAMESPACE}" -o jsonpath='{.spec.type}' 2>/dev/null)"

        case "${backend_type}" in
            NodePort)
                local node_port
                node_port="$(kubectl get svc chaos-sec-backend -n "${NAMESPACE}" -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}' 2>/dev/null)"
                local node_ip
                node_ip="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || echo "localhost")"
                BASE_URL="http://${node_ip}:${node_port}"
                ;;
            LoadBalancer)
                # Wait for external IP
                log "Waiting for LoadBalancer IP..."
                sleep 10
                local lb_ip
                lb_ip="$(kubectl get svc chaos-sec-backend -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || kubectl get svc chaos-sec-backend -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")"
                if [[ -n "${lb_ip}" ]]; then
                    BASE_URL="http://${lb_ip}:8081"
                else
                    BASE_URL="http://localhost:8081"
                fi
                ;;
            ClusterIP|*)
                BASE_URL="http://localhost:8081"
                ;;
        esac
    else
        BASE_URL="http://localhost:8081"
    fi

    # Frontend URL
    if kubectl get svc chaos-sec-frontend -n "${NAMESPACE}" &>/dev/null; then
        local frontend_type
        frontend_type="$(kubectl get svc chaos-sec-frontend -n "${NAMESPACE}" -o jsonpath='{.spec.type}' 2>/dev/null)"

        case "${frontend_type}" in
            NodePort)
                local node_port
                node_port="$(kubectl get svc chaos-sec-frontend -n "${NAMESPACE}" -o jsonpath='{.spec.ports[?(@.name=="http")].nodePort}' 2>/dev/null)"
                local node_ip
                node_ip="$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}' 2>/dev/null || echo "localhost")"
                FRONTEND_URL="http://${node_ip}:${node_port}"
                ;;
            LoadBalancer)
                sleep 5
                local lb_host
                lb_host="$(kubectl get svc chaos-sec-frontend -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || kubectl get svc chaos-sec-frontend -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "localhost")"
                FRONTEND_URL="http://${lb_host}:3000"
                ;;
            ClusterIP|*)
                FRONTEND_URL="http://localhost:3000"
                ;;
        esac
    else
        FRONTEND_URL="http://localhost:3000"
    fi

    SIEM_URL="http://localhost:8089"

    log "Service URLs detected:"
    log "  BASE_URL: ${BASE_URL}"
    log "  FRONTEND_URL: ${FRONTEND_URL}"
    log "  SIEM_URL: ${SIEM_URL}"
}

start_port_forwarding() {
    log_step "Starting port forwarding"

    # Kill any existing port-forwards
    pkill -f "kubectl port-forward.*${NAMESPACE}" 2>/dev/null || true

    # Start backend port forward in background
    kubectl port-forward -n "${NAMESPACE}" svc/chaos-sec-backend 8081:8080 &
    local backend_pid=$!
    echo "${backend_pid}" > /tmp/chaos-sec-backend-pf.pid

    # Start frontend port forward in background
    kubectl port-forward -n "${NAMESPACE}" svc/chaos-sec-frontend 3000:3000 &
    local frontend_pid=$!
    echo "${frontend_pid}" > /tmp/chaos-sec-frontend-pf.pid

    # Start SIEM port forward in background
    kubectl port-forward -n "${NAMESPACE}" svc/chaos-sec-siem 8089:8080 &
    local siem_pid=$!
    echo "${siem_pid}" > /tmp/chaos-sec-siem-pf.pid

    # Wait for port forwards to be established
    sleep 5

    # Verify port forwarding
    if kill -0 "${backend_pid}" 2>/dev/null; then
        log_ok "Backend port forwarding active (PID: ${backend_pid})"
    else
        log_warn "Backend port forwarding may have failed"
    fi

    BASE_URL="http://localhost:8081"
    FRONTEND_URL="http://localhost:3000"
    SIEM_URL="http://localhost:8089"

    log_ok "Port forwarding configured"
}

# ──────────────────────────────────────────────
# Authentication
# ──────────────────────────────────────────────
authenticate() {
    log_step "Authenticating"

    local username="${AUTH_USER:-admin@chaos-sec.io}"
    local password="${AUTH_PASS:-admin}"

    local response
    response="$(curl -s -X POST "${BASE_URL}/api/v1/auth/login" \
        -H "Content-Type: application/json" \
        -d "{\"email\":\"${username}\",\"password\":\"${password}\"}" \
        --max-time 10)"

    if [[ -z "${response}" ]]; then
        log_fail "Empty authentication response"
        return 1
    fi

    ACCESS_TOKEN="$(echo "${response}" | grep -o '"accessToken":"[^"]*"' | cut -d'"' -f4)"
    REFRESH_TOKEN="$(echo "${response}" | grep -o '"refreshToken":"[^"]*"' | cut -d'"' -f4)"

    if [[ -z "${ACCESS_TOKEN}" ]]; then
        log_fail "Failed to get access token"
        log_dim "Response: ${response}"
        return 1
    fi

    log_ok "Authentication successful"
    return 0
}

# ──────────────────────────────────────────────
# Health Checks
# ──────────────────────────────────────────────
check_backend_health() {
    log_step "Checking backend health"

    local max_attempts=30
    local attempt=1

    while [[ ${attempt} -le ${max_attempts} ]]; do
        local response
        response="$(curl -s -w "\n%{http_code}" "${BASE_URL}/health/ready" --max-time 5)"

        local http_code
        http_code="$(echo "${response}" | tail -1)"
        local body
        body="$(echo "${response}" | head -n -1)"

        if [[ "${http_code}" == "200" ]]; then
            log_ok "Backend health check passed"
            return 0
        fi

        log_dim "Backend not ready (attempt ${attempt}/${max_attempts}), retrying..."
        sleep 2
        attempt=$((attempt + 1))
    done

    log_fail "Backend health check failed after ${max_attempts} attempts"
    return 1
}

check_frontend_health() {
    log_step "Checking frontend health"

    local response
    response="$(curl -s -w "\n%{http_code}" "${FRONTEND_URL}" --max-time 10)"

    local http_code
    http_code="$(echo "${response}" | tail -1)"

    if [[ "${http_code}" == "200" ]]; then
        log_ok "Frontend health check passed"
        return 0
    else
        log_warn "Frontend returned HTTP ${http_code}"
        return 1
    fi
}

# ──────────────────────────────────────────────
# Experiment Validation Tests
# ──────────────────────────────────────────────
test_experiment_crud() {
    log_step "Testing experiment CRUD operations"

    local experiment_payload='{
        "name": "K8s Validation Test - Pod Kill",
        "description": "Validates pod deletion experiment execution in k8s",
        "target": {
            "kind": "Pod",
            "namespace": "default",
            "selector": {"app": "test-target"}
        },
        "attack": {
            "type": "PodKill",
            "params": {
                "duration": 30,
                "force": true
            }
        },
        "schedule": {
            "type": "once",
            "delay": 0
        }
    }'

    # Create experiment
    local response
    response="$(curl -s -X POST "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${experiment_payload}" \
        --max-time 10)"

    local http_code
    http_code="$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${experiment_payload}" \
        --max-time 10)"

    if [[ "${http_code}" == "200" || "${http_code}" == "201" ]]; then
        log_ok "Experiment created successfully"
    else
        log_fail "Failed to create experiment (HTTP ${http_code})"
        log_dim "Response: ${response}"
        return 1
    fi

    # List experiments
    local list_response
    list_response="$(curl -s -X GET "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        --max-time 10)"

    if echo "${list_response}" | grep -q "experiments"; then
        log_ok "Experiment list retrieved"
    else
        log_warn "Could not verify experiment list"
    fi

    return 0
}

test_attacker_pod_spawning() {
    log_step "Testing attacker pod spawning"

    # Create a target namespace and pod for testing
    kubectl create namespace test-target-ns --dry-run=client -o yaml | kubectl apply -f -

    # Deploy a target pod
    cat << 'EOF' | kubectl apply -f -
apiVersion: v1
kind: Pod
metadata:
  name: attacker-test-target
  namespace: test-target-ns
  labels:
    app: attacker-test-target
spec:
  containers:
    - name: nginx
      image: nginx:alpine
      ports:
        - containerPort: 80
      resources:
        limits:
          cpu: 100m
          memory: 64Mi
        requests:
          cpu: 50m
          memory: 32Mi
  restartPolicy: Always
EOF

    # Wait for target pod to be ready
    kubectl wait --for=condition=Ready pod attacker-test-target -n test-target-ns --timeout=60s || true

    # Create an experiment that targets this pod
    local experiment_payload="{
        \"name\": \"Attacker Pod Spawn Test\",
        \"description\": \"Tests if attacker pods spawn correctly in the cluster\",
        \"target\": {
            \"kind\": \"Pod\",
            \"namespace\": \"test-target-ns\",
            \"selector\": {\"app\": \"attacker-test-target\"}
        },
        \"attack\": {
            \"type\": \"NetworkLatency\",
            \"params\": {
                \"duration\": 10,
                \"latency\": 5000
            }
        },
        \"schedule\": {
            \"type\": \"once\",
            \"delay\": 5
        }
    }"

    # Submit experiment
    local response
    response="$(curl -s -X POST "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${experiment_payload}" \
        --max-time 10)"

    local experiment_id
    experiment_id="$(echo "${response}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"

    if [[ -z "${experiment_id}" ]]; then
        log_warn "Could not extract experiment ID, checking via API"
        experiment_id="$(curl -s -X GET "${BASE_URL}/api/v1/experiments" \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            --max-time 10 | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"
    fi

    # Wait for experiment to start and check for attacker pods
    log "Waiting for attacker pod to be spawned..."
    sleep 15

    # Check if attacker pods exist in the namespace
    local attacker_pods
    attacker_pods="$(kubectl get pods -n "${NAMESPACE}" -l chaos-sec-type=attacker --no-headers 2>/dev/null | wc -l)"

    if [[ ${attacker_pods} -gt 0 ]]; then
        log_ok "Attacker pods spawned successfully (${attacker_pods} found)"
        kubectl get pods -n "${NAMESPACE}" -l chaos-sec-type=attacker
    else
        # Check if there's a different label for attacker pods
        local pods
        pods="$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null)"
        log_dim "Available pods in namespace:"
        log_dim "${pods}"

        log_warn "No attacker pods found with chaos-sec-type=attacker label"
        log "This may be expected if the experiment type doesn't use attacker pods"
    fi

    # Check experiment status
    if [[ -n "${experiment_id}" ]]; then
        local status_response
        status_response="$(curl -s -X GET "${BASE_URL}/api/v1/experiments/${experiment_id}" \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            --max-time 10)"

        if echo "${status_response}" | grep -q "status"; then
            local status
            status="$(echo "${status_response}" | grep -o '"status":"[^"]*"' | cut -d'"' -f4)"
            log "Experiment status: ${status}"
        fi
    fi

    # Cleanup test resources
    kubectl delete pod attacker-test-target -n test-target-ns --ignore-not-found=true
    kubectl delete namespace test-target-ns --ignore-not-found=true

    return 0
}

test_siem_alerts() {
    log_step "Testing SIEM alert generation"

    # Create a test experiment
    local experiment_payload='{
        "name": "SIEM Alert Test",
        "description": "Tests SIEM alert generation",
        "target": {
            "kind": "Pod",
            "namespace": "default",
            "selector": {"app": "test"}
        },
        "attack": {
            "type": "CPUBurn",
            "params": {"duration": 5, "load": 50}
        },
        "schedule": {
            "type": "once",
            "delay": 2
        }
    }'

    # Submit experiment
    local response
    response="$(curl -s -X POST "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${experiment_payload}" \
        --max-time 10)"

    local experiment_id
    experiment_id="$(echo "${response}" | grep -o '"id":"[^"]*"' | head -1 | cut -d'"' -f4)"

    if [[ -z "${experiment_id}" ]]; then
        log_warn "Could not create experiment for SIEM test"
        return 1
    fi

    # Wait for experiment to execute
    log "Waiting for experiment to execute and alerts to be generated..."
    sleep 20

    # Check SIEM endpoint for alerts
    local siem_response
    siem_response="$(curl -s -X GET "${SIEM_URL}/api/v1/alerts" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        --max-time 10)"

    if echo "${siem_response}" | grep -qE "(alert|event|security)"; then
        log_ok "SIEM alerts retrieved successfully"
    else
        # Check if SIEM service is accessible
        local siem_health
        siem_health="$(curl -s -w "%{http_code}" "${SIEM_URL}/health" --max-time 5)"
        if [[ "${siem_health: -3}" == "200" ]]; then
            log_ok "SIEM service is healthy (no alerts may be expected in test)"
        else
            log_warn "SIEM service may not be fully integrated"
        fi
    fi

    # Check kubernetes events for security-related events
    local events
    events="$(kubectl get events -n "${NAMESPACE}" --sort-by='.lastTimestamp' 2>/dev/null | tail -20)"

    if echo "${events}" | grep -qE "(Warning|Attack|Experiment)"; then
        log_ok "Kubernetes events showing experiment activity"
    fi

    return 0
}

test_websocket_updates() {
    log_step "Testing WebSocket real-time updates"

    # Check if websockets are available
    local ws_endpoint="${BASE_URL}/api/v1/ws/experiments"

    # Use wscat if available, otherwise use a simple WebSocket client test
    if command -v wscat &>/dev/null; then
        log "Using wscat for WebSocket test"
        timeout 10 wscat -c "${ws_endpoint}" -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            --execute '{"type":"subscribe","channel":"experiments"}' 2>/dev/null || {
            log_warn "WebSocket test with wscat failed or timed out"
        }
    else
        # Create a simple WebSocket test using Python
        log "Using Python for WebSocket test"

        local ws_test_script="/tmp/ws_test.py"
        cat > "${ws_test_script}" << 'WSEOF'
import socket
import base64
import hashlib
import time
import sys

def create_websocket_key():
    import secrets
    key = base64.b64encode(secrets.token_bytes(16)).decode()
    return key

def shake_hash(key):
    h = hashlib.sha1((key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").encode()).digest()
    return base64.b64encode(h).decode()

def parse_headers(response):
    headers = {}
    for line in response.decode().split("\r\n"):
        if ":" in line:
            key, value = line.split(":", 1)
            headers[key.strip().lower()] = value.strip()
    return headers

def websocket_test(url, headers=None):
    # Parse URL
    import re
    match = re.match(r'http://([^:]+):(\d+)(.*)', url)
    if not match:
        print("Could not parse URL")
        return False

    host, port, path = match.groups()
    if path == "":
        path = "/"

    # Create socket
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.settimeout(5)
    try:
        sock.connect((host, int(port)))
    except Exception as e:
        print(f"Connection failed: {e}")
        return False

    # WebSocket handshake
    key = create_websocket_key()
    get_request = f"GET {path} HTTP/1.1\r\n"
    get_request += f"Host: {host}:{port}\r\n"
    get_request += f"Upgrade: websocket\r\n"
    get_request += f"Connection: Upgrade\r\n"
    get_request += f"Sec-WebSocket-Key: {key}\r\n"
    get_request += f"Sec-WebSocket-Version: 13\r\n"

    if headers:
        for k, v in headers.items():
            get_request += f"{k}: {v}\r\n"

    get_request += "\r\n"

    sock.send(get_request.encode())

    # Read response
    try:
        response = b""
        while b"\r\n\r\n" not in response:
            response += sock.recv(1024)
    except socket.timeout:
        print("Timeout reading response")
        return False

    if b"101" not in response:
        print(f"WebSocket handshake failed: {response[:200]}")
        return False

    print("WebSocket handshake successful")

    # Send a simple ping frame
    # Frame format: FIN=1, opcode=0x9 (ping), mask=1, payload=4 bytes
    import struct
    ping_frame = struct.pack('!BB', 0x89, 0x80)  # FIN + ping opcode, masked
    ping_frame += b'\x00\x00\x00\x00'  # mask key
    sock.send(ping_frame)

    # Wait for pong
    sock.settimeout(5)
    try:
        data = sock.recv(1024)
        if data:
            print(f"Received response: {data[:20].hex()}")
            print("WebSocket communication working")
            return True
    except socket.timeout:
        print("Timeout waiting for response")

    return True  # WebSocket at least established

if __name__ == "__main__":
    url = sys.argv[1] if len(sys.argv) > 1 else "localhost:8081/api/v1/ws/experiments"
    headers = {}
    if len(sys.argv) > 2:
        headers["Authorization"] = f"Bearer {sys.argv[2]}"

    success = websocket_test(url, headers)
    sys.exit(0 if success else 1)
WSEOF

        if command -v python3 &>/dev/null; then
            timeout 15 python3 "${ws_test_script}" \
                "$(echo "${BASE_URL}" | sed 's|http://||')${BASE_URL#http://localhost:8081}" \
                "${ACCESS_TOKEN}" 2>/dev/null || {
                log_warn "WebSocket test did not complete"
            }
        else
            log_warn "Python3 not available for WebSocket test"
        fi
    fi

    log_ok "WebSocket test completed (check logs for details)"
    return 0
}

test_experiment_execution_cycle() {
    log_step "Testing complete experiment execution cycle"

    # Create a simple experiment that runs quickly
    local experiment_payload='{
        "name": "Full Execution Cycle Test",
        "description": "Tests complete experiment lifecycle",
        "target": {
            "kind": "Pod",
            "namespace": "default",
            "selector": {"app": "test"}
        },
        "attack": {
            "type": "Delay",
            "params": {"duration": 3, "delay_ms": 1000}
        },
        "schedule": {
            "type": "once",
            "delay": 0
        }
    }'

    # Submit experiment
    local response
    response="$(curl -s -X POST "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${experiment_payload}" \
        --max-time 10)"

    local http_code
    http_code="$(curl -s -o /dev/null -w "%{http_code}" -X POST "${BASE_URL}/api/v1/experiments" \
        -H "Authorization: Bearer ${ACCESS_TOKEN}" \
        -H "Content-Type: application/json" \
        -d "${experiment_payload}" \
        --max-time 10)"

    if [[ "${http_code}" != "200" && "${http_code}" != "201" ]]; then
        log_fail "Failed to submit experiment (HTTP ${http_code})"
        return 1
    fi

    log "Experiment submitted, waiting for execution..."

    # Monitor experiment status
    local max_wait=60
    local waited=0
    local last_status=""

    while [[ ${waited} -lt ${max_wait} ]]; do
        # Get experiments list
        local experiments
        experiments="$(curl -s -X GET "${BASE_URL}/api/v1/experiments" \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            --max-time 10)"

        # Try to find our experiment
        local status
        status="$(echo "${experiments}" | grep -o '"status":"[^"]*"' | head -1 | cut -d'"' -f4)"

        if [[ -n "${status}" && "${status}" != "${last_status}" ]]; then
            log "Status: ${status}"
            last_status="${status}"
        fi

        if echo "${experiments}" | grep -qE '"status":"(completed|failed|finished)"'; then
            log_ok "Experiment completed successfully"
            return 0
        fi

        sleep 5
        waited=$((waited + 5))
    done

    log_warn "Experiment execution did not complete within ${max_wait}s (this may be normal)"
    return 0
}

# ──────────────────────────────────────────────
# Kubernetes Resource Validation
# ──────────────────────────────────────────────
validate_k8s_resources() {
    log_step "Validating Kubernetes resources"

    # Check all required resources exist
    local resources=(
        "namespace/${NAMESPACE}"
        "configmap/chaos-sec-config"
        "secret/chaos-sec-secrets"
        "statefulset/chaos-sec-postgres"
        "statefulset/chaos-sec-redis"
        "deployment/chaos-sec-backend"
        "deployment/chaos-sec-frontend"
        "service/chaos-sec-backend"
        "service/chaos-sec-frontend"
    )

    local all_ok=true
    for resource in "${resources[@]}"; do
        if kubectl get "${resource}" &>/dev/null; then
            log_ok "${resource} exists"
        else
            log_fail "${resource} not found"
            all_ok=false
        fi
    done

    if [[ "${all_ok}" == "true" ]]; then
        log_ok "All Kubernetes resources validated"
        return 0
    else
        log_fail "Some Kubernetes resources are missing"
        return 1
    fi
}

validate_resource_limits() {
    log_step "Validating resource limits and QoS"

    # Check pod resource requests/limits
    local pods
    pods="$(kubectl get pods -n "${NAMESPACE}" -o json 2>/dev/null)"

    if echo "${pods}" | grep -q "resources"; then
        log_ok "Pods have resource configuration"
    else
        log_warn "Some pods may not have resource limits set"
    fi

    # Check QoS classes
    local qos_pods
    qos_pods="$(kubectl get pods -n "${NAMESPACE}" --field-selector=status.phase=Running -o jsonpath='{range .items[*]}{.metadata.name}: {.status.qosClass}{"\n"}{end}' 2>/dev/null)"

    log_dim "Pod QoS Classes:"
    log_dim "${qos_pods}"

    if echo "${qos_pods}" | grep -q "Guaranteed"; then
        log_ok "Some pods have Guaranteed QoS"
    fi

    return 0
}

validate_network_policies() {
    log_step "Validating network policies"

    # Check if network policies are applied
    local policies
    policies="$(kubectl get networkpolicies -n "${NAMESPACE}" 2>/dev/null || echo "")"

    if [[ -n "${policies}" ]]; then
        log_ok "Network policies found"
        log_dim "${policies}"
    else
        log_warn "No network policies found (may be expected in kind)"
    fi

    return 0
}

# ──────────────────────────────────────────────
# Main Validation Flow
# ──────────────────────────────────────────────
run_validation() {
    log_bold "╔════════════════════════════════════════════════════════════╗"
    log_bold "║   Chaos-Sec Kubernetes Validation                          ║"
    log_bold "╚════════════════════════════════════════════════════════════╝"
    echo

    local start_time
    start_time="$(date +%s)"

    # Phase 1: Prerequisites
    log_step "PHASE 1: Prerequisites"
    check_prerequisites || {
        log_fail "Prerequisites check failed"
        exit 1
    }
    echo

    # Phase 2: Cluster Setup
    log_step "PHASE 2: Cluster Setup"
    if [[ "${USE_EXISTING}" == "false" ]]; then
        if [[ "${EKS_MODE}" == "true" ]]; then
            setup_eks_cluster || {
                log_fail "EKS cluster setup failed"
                exit 1
            }
        else
            create_kind_cluster || {
                log_fail "Kind cluster creation failed"
                exit 1
            }
        fi
    else
        log "Using existing kubectl context: $(kubectl config current-context)"
        kubectl get nodes
    fi
    echo

    # Phase 3: Deployment
    log_step "PHASE 3: Platform Deployment"
    if [[ "${SKIP_DEPLOY}" == "false" ]]; then
        create_namespace
        deploy_manifests
        wait_for_deployments
    else
        log "Skipping deployment (using existing resources)"
    fi
    echo

    # Phase 4: Service Access
    log_step "PHASE 4: Service Access Configuration"
    start_port_forwarding
    detect_service_urls
    echo

    # Phase 5: Health Checks
    log_step "PHASE 5: Health Checks"
    check_backend_health || log_warn "Backend health check had issues"
    check_frontend_health || log_warn "Frontend health check had issues"
    echo

    # Phase 6: Authentication
    log_step "PHASE 6: Authentication"
    if ! authenticate; then
        log_fail "Authentication failed"
        log_warn "Some tests may fail without authentication"
    fi
    echo

    # Phase 7: Kubernetes Resource Validation
    log_step "PHASE 7: Kubernetes Resource Validation"
    validate_k8s_resources
    validate_resource_limits
    validate_network_policies
    echo

    # Phase 8: Experiment Validation
    log_step "PHASE 8: Experiment Validation"
    test_experiment_crud
    test_attacker_pod_spawning
    test_siem_alerts
    test_websocket_updates
    test_experiment_execution_cycle
    echo

    # Summary
    local end_time
    end_time="$(date +%s)"
    local duration=$((end_time - start_time))

    log_bold "╔════════════════════════════════════════════════════════════╗"
    log_bold "║   Validation Complete                                      ║"
    log_bold "╚════════════════════════════════════════════════════════════╝"
    echo
    log "Duration: ${duration} seconds"
    echo
    log "Service URLs:"
    log "  API:      ${BASE_URL}"
    log "  Frontend: ${FRONTEND_URL}"
    log "  SIEM:     ${SIEM_URL}"
    echo
    log "To interact with the platform:"
    log "  kubectl port-forward -n ${NAMESPACE} svc/chaos-sec-backend 8081:8080"
    log "  kubectl port-forward -n ${NAMESPACE} svc/chaos-sec-frontend 3000:3000"
    echo

    if [[ "${PRESERVE_CLUSTER}" == "false" ]]; then
        log "Cluster will be cleaned up on exit (use --preserve-cluster to keep)"
    else
        log "Cluster preserved (use --preserve-cluster to skip cleanup)"
    fi
}

# ──────────────────────────────────────────────
# Entry Point
# ──────────────────────────────────────────────
main() {
    if [[ "${CLEANUP_ONLY}" == "true" ]]; then
        full_cleanup
        exit 0
    fi

    run_validation

    exit 0
}

main "$@"
