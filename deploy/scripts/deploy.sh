#!/usr/bin/env bash
# =============================================================================
# Chaos-Sec Platform Deployment Script
# =============================================================================
# Deploys the Chaos-Sec platform to a Kubernetes cluster.
#
# Features:
#   - Creates namespace
#   - Applies secrets (interactive prompts for values)
#   - Applies ConfigMap and all K8s manifests
#   - Waits for rollouts to complete
#   - Runs health checks against deployed services
#   - Supports --dry-run flag for validation without changes
#
# Usage:
#   ./deploy.sh                    # Full interactive deployment
#   ./deploy.sh --dry-run          # Validate manifests without applying
#   ./deploy.sh --skip-secrets     # Skip secret creation (use existing)
#   ./deploy.sh --skip-health     # Skip health checks after deploy
#   ./deploy.sh --non-interactive  # Use environment variables for secrets
#   ./deploy.sh --help             # Show usage information
#
# Environment variables (for non-interactive mode):
#   CHAOS_DB_PASSWORD       - PostgreSQL database password
#   CHAOS_REDIS_PASSWORD    - Redis authentication password
#   CHAOS_JWT_SECRET        - JWT signing secret
#   CHAOS_SIEM_API_KEY      - SIEM integration API key
#   CHAOS_SMTP_USER         - SMTP username
#   CHAOS_SMTP_PASSWORD     - SMTP password
#   CHAOS_RABBITMQ_PASSWORD - RabbitMQ password
#   CHAOS_ENCRYPTION_KEY    - AES-256 encryption key
# =============================================================================

set -euo pipefail

# ──────────────────────────────────────────────
# Script Configuration
# ──────────────────────────────────────────────
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
readonly K8S_DIR="${PROJECT_ROOT}/deploy/kubernetes"
readonly NAMESPACE="chaos-sec"
readonly TIMEOUT_SECONDS=300

# Colors for output
readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[1;33m'
readonly BLUE='\033[0;34m'
readonly CYAN='\033[0;36m'
readonly BOLD='\033[1m'
readonly NC='\033[0m' # No Color

# ──────────────────────────────────────────────
# Parse Command Line Arguments
# ──────────────────────────────────────────────
DRY_RUN=false
SKIP_SECRETS=false
SKIP_HEALTH=false
NON_INTERACTIVE=false
VERBOSE=false

for arg in "$@"; do
    case "${arg}" in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --skip-secrets)
            SKIP_SECRETS=true
            shift
            ;;
        --skip-health)
            SKIP_HEALTH=true
            shift
            ;;
        --non-interactive)
            NON_INTERACTIVE=true
            shift
            ;;
        --verbose)
            VERBOSE=true
            shift
            ;;
        --help|-h)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Deploy the Chaos-Sec platform to Kubernetes."
            echo ""
            echo "Options:"
            echo "  --dry-run          Validate manifests without applying changes"
            echo "  --skip-secrets     Skip secret creation (use existing secrets)"
            echo "  --skip-health      Skip health checks after deployment"
            echo "  --non-interactive  Use environment variables instead of prompts"
            echo "  --verbose          Enable verbose output"
            echo "  --help, -h         Show this help message"
            echo ""
            echo "Environment variables (for --non-interactive):"
            echo "  CHAOS_DB_PASSWORD, CHAOS_REDIS_PASSWORD, CHAOS_JWT_SECRET,"
            echo "  CHAOS_SIEM_API_KEY, CHAOS_SMTP_USER, CHAOS_SMTP_PASSWORD,"
            echo "  CHAOS_RABBITMQ_PASSWORD, CHAOS_ENCRYPTION_KEY"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown argument: ${arg}${NC}"
            echo "Run '$0 --help' for usage information."
            exit 1
            ;;
    esac
done

# ──────────────────────────────────────────────
# Utility Functions
# ──────────────────────────────────────────────

log() {
    local level="$1"
    shift
    local message="$*"
    local timestamp
    timestamp="$(date '+%Y-%m-%d %H:%M:%S')"

    case "${level}" in
        info)
            echo -e "${timestamp} ${BLUE}[INFO]${NC}  ${message}"
            ;;
        success)
            echo -e "${timestamp} ${GREEN}[OK]${NC}    ${message}"
            ;;
        warn)
            echo -e "${timestamp} ${YELLOW}[WARN]${NC}  ${message}"
            ;;
        error)
            echo -e "${timestamp} ${RED}[ERROR]${NC} ${message}"
            ;;
        step)
            echo -e ""
            echo -e "${timestamp} ${CYAN}${BOLD}[STEP]${NC}  ${BOLD}${message}${NC}"
            echo -e "${CYAN}────────────────────────────────────────────────────────────${NC}"
            ;;
        dry)
            echo -e "${timestamp} ${YELLOW}[DRY]${NC}   ${message}"
            ;;
    esac
}

verbose() {
    if [[ "${VERBOSE}" == "true" ]]; then
        log info "[verbose] $*"
    fi
}

check_dependency() {
    local cmd="$1"
    local description="${2:-${cmd}}"

    if ! command -v "${cmd}" &>/dev/null; then
        log error "Required dependency '${cmd}' (${description}) is not installed."
        log error "Please install it before running this script."
        exit 1
    fi
    verbose "Found dependency: ${cmd}"
}

wait_for_resource() {
    local resource_type="$1"
    local resource_name="$2"
    local namespace="$3"
    local condition="${4:-}"
    local max_attempts=30
    local attempt=1
    local wait_seconds=10

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would wait for ${resource_type}/${resource_name} in ${namespace} to be ready"
        return 0
    fi

    log info "Waiting for ${resource_type}/${resource_name} in ${namespace} to be ready..."

    while [[ ${attempt} -le ${max_attempts} ]]; do
        if kubectl get "${resource_type}" "${resource_name}" \
            --namespace "${namespace}" &>/dev/null; then

            if [[ -n "${condition}" ]]; then
                local condition_met
                condition_met=$(kubectl get "${resource_type}" "${resource_name}" \
                    --namespace "${namespace}" \
                    -o jsonpath="{.status.conditions[?(@.type=='${condition}')].status}" 2>/dev/null || echo "Unknown")

                if [[ "${condition_met}" == "True" ]]; then
                    log success "${resource_type}/${resource_name} is ready (${condition}=True)"
                    return 0
                fi
            else
                log success "${resource_type}/${resource_name} exists"
                return 0
            fi
        fi

        log info "Attempt ${attempt}/${max_attempts}: ${resource_type}/${resource_name} not ready yet, waiting ${wait_seconds}s..."
        sleep "${wait_seconds}"
        attempt=$((attempt + 1))
    done

    log error "Timed out waiting for ${resource_type}/${resource_name} in ${namespace} after $((max_attempts * wait_seconds))s"
    return 1
}

wait_for_rollout() {
    local resource_type="$1"
    local resource_name="$2"
    local namespace="$3"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would wait for rollout of ${resource_type}/${resource_name} in ${namespace}"
        return 0
    fi

    log info "Waiting for rollout of ${resource_type}/${resource_name} in ${namespace} (timeout: ${TIMEOUT_SECONDS}s)..."

    if kubectl rollout status "${resource_type}" "${resource_name}" \
        --namespace "${namespace}" \
        --timeout="${TIMEOUT_SECONDS}s" 2>/dev/null; then
        log success "Rollout of ${resource_type}/${resource_name} completed successfully"
        return 0
    else
        log error "Rollout of ${resource_type}/${resource_name} failed or timed out"
        log error "Check with: kubectl rollout status ${resource_type}/${resource_name} -n ${namespace}"
        return 1
    fi
}

run_health_check() {
    local name="$1"
    local url="$2"
    local expected_status="${3:-200}"
    local max_attempts=15
    local attempt=1
    local wait_seconds=5

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would run health check: ${name} -> ${url} (expecting HTTP ${expected_status})"
        return 0
    fi

    log info "Running health check: ${name} (expecting HTTP ${expected_status})..."

    while [[ ${attempt} -le ${max_attempts} ]]; do
        local http_code
        http_code=$(kubectl exec -n "${NAMESPACE}" deployment/chaos-sec-backend -- \
            curl -sk -o /dev/null -w "%{http_code}" "${url}" 2>/dev/null || echo "000")

        if [[ "${http_code}" == "${expected_status}" ]]; then
            log success "Health check passed: ${name} (HTTP ${http_code})"
            return 0
        fi

        log info "Attempt ${attempt}/${max_attempts}: ${name} returned HTTP ${http_code} (expected ${expected_status}), retrying in ${wait_seconds}s..."
        sleep "${wait_seconds}"
        attempt=$((attempt + 1))
    done

    log error "Health check failed: ${name} did not return HTTP ${expected_status} after $((max_attempts * wait_seconds))s"
    return 1
}

# ──────────────────────────────────────────────
# Pre-flight Checks
# ──────────────────────────────────────────────

preflight_checks() {
    log step "Running pre-flight checks"

    check_dependency kubectl "Kubernetes CLI"
    check_dependency base64 "Base64 encoder"
    check_dependency openssl "OpenSSL (for secret generation)"

    # Check kubectl can connect to a cluster
    if ! kubectl cluster-info &>/dev/null; then
        log error "Cannot connect to a Kubernetes cluster. Is kubeconfig configured?"
        log error "Run: kubectl cluster-info"
        exit 1
    fi
    log success "Kubernetes cluster connection verified"

    # Check kubectl version compatibility
    local k8s_version
    k8s_version=$(kubectl version --output=json 2>/dev/null | grep -o '"gitVersion": "[^"]*"' | head -1 | grep -o 'v[0-9]*\.[0-9]*' || echo "unknown")
    log info "Kubernetes server version: ${k8s_version}"

    # Verify manifest directory exists
    if [[ ! -d "${K8S_DIR}" ]]; then
        log error "Kubernetes manifest directory not found: ${K8S_DIR}"
        log error "Ensure the deploy/kubernetes/ directory exists with all manifests."
        exit 1
    fi
    log success "Manifest directory found: ${K8S_DIR}"

    # List all manifest files for verification
    local manifest_count
    manifest_count=$(find "${K8S_DIR}" -name "*.yaml" -not -name "kustomization.yaml" | wc -l)
    log info "Found ${manifest_count} Kubernetes manifest files"

    if [[ "${VERBOSE}" == "true" ]]; then
        find "${K8S_DIR}" -name "*.yaml" | sort | while read -r f; do
            verbose "  Manifest: $(basename "${f}")"
        done
    fi

    log success "All pre-flight checks passed"
}

# ──────────────────────────────────────────────
# Step 1: Create Namespace
# ──────────────────────────────────────────────

create_namespace() {
    log step "Creating namespace: ${NAMESPACE}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would apply: ${K8S_DIR}/namespace.yaml"
        kubectl apply --dry-run=client -f "${K8S_DIR}/namespace.yaml"
        log dry "Namespace ${NAMESPACE} would be created"
        return 0
    fi

    # Check if namespace already exists
    if kubectl get namespace "${NAMESPACE}" &>/dev/null; then
        log warn "Namespace ${NAMESPACE} already exists, updating..."
        kubectl apply -f "${K8S_DIR}/namespace.yaml"
    else
        kubectl apply -f "${K8S_DIR}/namespace.yaml"
        log success "Namespace ${NAMESPACE} created"
    fi

    # Wait for namespace to be active
    wait_for_resource "namespace" "${NAMESPACE}" "" "Active"
}

# ──────────────────────────────────────────────
# Step 2: Apply Secrets
# ──────────────────────────────────────────────

generate_secret_value() {
    local length="${1:-32}"
    openssl rand -base64 "${length}" | tr -d '=\n' | head -c "${length}"
}

prompt_secret() {
    local secret_name="$1"
    local description="$2"
    local env_var="$3"
    local is_optional="${4:-false}"
    local value=""

    # If non-interactive, try environment variable
    if [[ "${NON_INTERACTIVE}" == "true" ]]; then
        value="${!env_var:-}"
        if [[ -z "${value}" ]]; then
            if [[ "${is_optional}" == "true" ]]; then
                log warn "Non-interactive mode: ${secret_name} not set via ${env_var}, using generated value"
                value="$(generate_secret_value)"
            else
                log error "Non-interactive mode: ${secret_name} must be set via ${env_var} environment variable"
                exit 1
            fi
        else
            log info "Using ${secret_name} from environment variable ${env_var}"
        fi
    else
        # Interactive prompt
        echo -e "${CYAN}${BOLD}${description}${NC}"
        read -r -p "  Enter value (or press Enter to auto-generate): " value

        if [[ -z "${value}" ]]; then
            value="$(generate_secret_value)"
            log info "  Auto-generated ${secret_name}"
        fi
    fi

    echo "${value}"
}

apply_secrets() {
    log step "Applying secrets to namespace ${NAMESPACE}"

    if [[ "${SKIP_SECRETS}" == "true" ]]; then
        log warn "Skipping secret creation (--skip-secrets flag provided)"
        log warn "Ensure secrets already exist in namespace ${NAMESPACE}"

        # Verify secrets exist
        if kubectl get secret chaos-sec-secrets --namespace "${NAMESPACE}" &>/dev/null; then
            log success "Existing secret chaos-sec-secrets found"
        else
            log error "Secret chaos-sec-secrets not found! Remove --skip-secrets or create secrets manually."
            exit 1
        fi
        return 0
    fi

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would prompt for secret values and create chaos-sec-secrets Secret"
        log dry "Would create chaos-sec-tls Secret (requires cert-manager or manual TLS cert)"
        return 0
    fi

    # ──────────────────────────────────────────
    # Collect Secret Values
    # ──────────────────────────────────────────
    echo -e ""
    echo -e "${BOLD}${CYAN}Chaos-Sec Secret Configuration${NC}"
    echo -e "${CYAN}Enter values for the following secrets.${NC}"
    echo -e "${CYAN}Press Enter to auto-generate a secure random value.${NC}"
    echo -e ""

    local db_password redis_password jwt_secret siem_api_key
    local smtp_user smtp_password rabbitmq_password encryption_key

    db_password=$(prompt_secret \
        "DATABASE_PASSWORD" \
        "1/8: PostgreSQL Database Password" \
        "CHAOS_DB_PASSWORD" \
        "false")

    redis_password=$(prompt_secret \
        "REDIS_PASSWORD" \
        "2/8: Redis Authentication Password" \
        "CHAOS_REDIS_PASSWORD" \
        "false")

    jwt_secret=$(prompt_secret \
        "JWT_SECRET" \
        "3/8: JWT Signing Secret (min 32 chars)" \
        "CHAOS_JWT_SECRET" \
        "false")

    siem_api_key=$(prompt_secret \
        "SIEM_API_KEY" \
        "4/8: SIEM Integration API Key" \
        "CHAOS_SIEM_API_KEY" \
        "true")

    smtp_user=$(prompt_secret \
        "SMTP_USER" \
        "5/8: SMTP Username (email address)" \
        "CHAOS_SMTP_USER" \
        "true")

    smtp_password=$(prompt_secret \
        "SMTP_PASSWORD" \
        "6/8: SMTP Password / App Password" \
        "CHAOS_SMTP_PASSWORD" \
        "true")

    rabbitmq_password=$(prompt_secret \
        "RABBITMQ_PASSWORD" \
        "7/8: RabbitMQ Password" \
        "CHAOS_RABBITMQ_PASSWORD" \
        "true")

    encryption_key=$(prompt_secret \
        "ENCRYPTION_KEY" \
        "8/8: AES-256 Encryption Key (32 bytes)" \
        "CHAOS_ENCRYPTION_KEY" \
        "true")

    echo -e ""

    # ──────────────────────────────────────────
    # Create the Opaque Secret
    # ──────────────────────────────────────────
    log info "Creating chaos-sec-secrets Secret..."

    # Delete existing secret if present (to allow re-creation with new values)
    if kubectl get secret chaos-sec-secrets --namespace "${NAMESPACE}" &>/dev/null; then
        log warn "Replacing existing chaos-sec-secrets Secret"
        kubectl delete secret chaos-sec-secrets --namespace "${NAMESPACE}" --wait=true
    fi

    kubectl create secret generic chaos-sec-secrets \
        --namespace "${NAMESPACE}" \
        --from-literal=DATABASE_PASSWORD="${db_password}" \
        --from-literal=REDIS_PASSWORD="${redis_password}" \
        --from-literal=JWT_SECRET="${jwt_secret}" \
        --from-literal=SIEM_API_KEY="${siem_api_key}" \
        --from-literal=SMTP_USER="${smtp_user}" \
        --from-literal=SMTP_PASSWORD="${smtp_password}" \
        --from-literal=SMTP_HOST="$(echo -n 'smtp.example.com' | base64)" \
        --from-literal=SMTP_PORT="$(echo -n '587' | base64)" \
        --from-literal=RABBITMQ_USER="$(echo -n 'chaossec' | base64)" \
        --from-literal=RABBITMQ_PASSWORD="${rabbitmq_password}" \
        --from-literal=ENCRYPTION_KEY="${encryption_key}" \
        --save-config --dry-run=client -o yaml | kubectl apply -f -

    log success "Secret chaos-sec-secrets created with 12 keys"

    # ──────────────────────────────────────────
    # Create TLS Secret (self-signed for development)
    # ──────────────────────────────────────────
    log info "Setting up TLS certificate..."

    if kubectl get secret chaos-sec-tls --namespace "${NAMESPACE}" &>/dev/null; then
        log warn "TLS secret chaos-sec-tls already exists, preserving it"
        log info "To regenerate, delete it first: kubectl delete secret chaos-sec-tls -n ${NAMESPACE}"
    else
        # Check if cert-manager is available
        if kubectl get clusterissuer &>/dev/null 2>&1; then
            log info "cert-manager detected – using secrets.yaml template for cert-manager provisioning"
            kubectl apply -f "${K8S_DIR}/secrets.yaml"
            log warn "TLS secret created with placeholder values. Configure cert-manager for automatic provisioning."
        else
            # Generate self-signed certificate for initial deployment
            log warn "cert-manager not found – generating self-signed TLS certificate"
            log warn "For production, use cert-manager with Let's Encrypt or your organization's CA"

            local tmp_dir
            tmp_dir=$(mktemp -d)
            trap 'rm -rf "${tmp_dir}"' EXIT

            openssl req -x509 -nodes -days 365 \
                -newkey rsa:2048 \
                -keyout "${tmp_dir}/tls.key" \
                -out "${tmp_dir}/tls.crt" \
                -subj "/CN=chaos-sec.io/O=Chaos-Sec/C=GB" \
                -addext "subjectAltName=DNS:chaos-sec.io,DNS:app.chaos-sec.io,DNS:localhost,IP:127.0.0.1" \
                2>/dev/null

            kubectl create secret tls chaos-sec-tls \
                --namespace "${NAMESPACE}" \
                --key "${tmp_dir}/tls.key" \
                --cert "${tmp_dir}/tls.crt" \
                --save-config --dry-run=client -o yaml | kubectl apply -f -

            log success "Self-signed TLS secret chaos-sec-tls created (expires in 365 days)"
        fi
    fi

    # ──────────────────────────────────────────
    # Create Service Account Token Secret
    # ──────────────────────────────────────────
    log info "Applying service account token secret..."
    kubectl apply -f "${K8S_DIR}/secrets.yaml" 2>/dev/null || true

    log success "All secrets applied to namespace ${NAMESPACE}"
}

# ──────────────────────────────────────────────
# Step 3: Apply ConfigMap
# ──────────────────────────────────────────────

apply_configmap() {
    log step "Applying ConfigMap configuration"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would apply: ${K8S_DIR}/configmap.yaml"
        kubectl apply --dry-run=client -f "${K8S_DIR}/configmap.yaml"
        log dry "ConfigMap chaos-sec-config would be created"
        return 0
    fi

    kubectl apply -f "${K8S_DIR}/configmap.yaml"
    log success "ConfigMap chaos-sec-config and chaos-sec-nginx-config applied"

    # Verify the ConfigMap was created with expected keys
    local key_count
    key_count=$(kubectl get configmap chaos-sec-config \
        --namespace "${NAMESPACE}" \
        -o jsonpath='{.data}' 2>/dev/null | jq 'keys | length' 2>/dev/null || echo "unknown")
    log info "ConfigMap chaos-sec-config contains ${key_count} keys"
}

# ──────────────────────────────────────────────
# Step 4: Apply All Kubernetes Manifests
# ──────────────────────────────────────────────

apply_manifests() {
    log step "Applying Kubernetes manifests"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would apply all manifests from: ${K8S_DIR}"
        log dry ""

        # Dry-run each manifest file
        local files_applied=0
        for manifest in "${K8S_DIR}"/*.yaml; do
            local filename
            filename=$(basename "${manifest}")

            # Skip kustomization.yaml (applied separately via -k)
            if [[ "${filename}" == "kustomization.yaml" ]]; then
                continue
            fi

            # Skip namespace.yaml and secrets.yaml (already applied in earlier steps)
            if [[ "${filename}" == "namespace.yaml" || "${filename}" == "secrets.yaml" ]]; then
                log dry "  [skip] ${filename} (applied in earlier step)"
                continue
            fi

            log dry "  [apply] ${filename}"
            if ! kubectl apply --dry-run=client -f "${manifest}" 2>/dev/null; then
                log warn "  [warn] ${filename} dry-run validation failed"
            fi
            files_applied=$((files_applied + 1))
        done

        log dry ""
        log dry "Total manifests to apply: ${files_applied}"
        return 0
    fi

    # Apply data layer first (StatefulSets for PostgreSQL and Redis)
    log info "Applying data layer (PostgreSQL, Redis)..."

    for manifest in \
        "${K8S_DIR}/postgres-statefulset.yaml" \
        "${K8S_DIR}/redis-statefulset.yaml"; do

        if [[ -f "${manifest}" ]]; then
            local filename
            filename=$(basename "${manifest}")
            kubectl apply -f "${manifest}"
            log success "Applied: ${filename}"
        else
            log warn "Manifest not found: ${manifest}"
        fi
    done

    # Wait for data layer to be ready before starting the application layer
    log info "Waiting for data layer to become ready..."
    wait_for_rollout "statefulset" "chaos-sec-postgres" "${NAMESPACE}" || true
    wait_for_rollout "statefulset" "chaos-sec-redis" "${NAMESPACE}" || true

    # Apply application layer (Backend and Frontend Deployments + Services)
    log info "Applying application layer (Backend, Frontend)..."

    for manifest in \
        "${K8S_DIR}/backend-deployment.yaml" \
        "${K8S_DIR}/backend-service.yaml" \
        "${K8S_DIR}/frontend-deployment.yaml" \
        "${K8S_DIR}/frontend-service.yaml"; do

        if [[ -f "${manifest}" ]]; then
            local filename
            filename=$(basename "${manifest}")
            kubectl apply -f "${manifest}"
            log success "Applied: ${filename}"
        else
            log warn "Manifest not found: ${manifest}"
        fi
    done

    # Apply networking layer (Ingress)
    log info "Applying networking layer (Ingress)..."

    if [[ -f "${K8S_DIR}/ingress.yaml" ]]; then
        kubectl apply -f "${K8S_DIR}/ingress.yaml"
        log success "Applied: ingress.yaml"
    else
        log warn "Ingress manifest not found: ${K8S_DIR}/ingress.yaml"
    fi

    # Apply autoscaling (HPA)
    log info "Applying autoscaling (HPA)..."

    if [[ -f "${K8S_DIR}/hpa.yaml" ]]; then
        kubectl apply -f "${K8S_DIR}/hpa.yaml"
        log success "Applied: hpa.yaml"
    else
        log warn "HPA manifest not found: ${K8S_DIR}/hpa.yaml"
    fi

    log success "All Kubernetes manifests applied"
}

# ──────────────────────────────────────────────
# Step 5: Wait for Rollouts
# ──────────────────────────────────────────────

wait_for_rollouts() {
    log step "Waiting for all rollouts to complete"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would wait for the following rollouts:"
        log dry "  - StatefulSet/chaos-sec-postgres"
        log dry "  - StatefulSet/chaos-sec-redis"
        log dry "  - Deployment/chaos-sec-backend"
        log dry "  - Deployment/chaos-sec-frontend"
        return 0
    fi

    local failures=0

    # Wait for data layer rollouts
    log info "Waiting for data layer rollouts..."

    if ! wait_for_rollout "statefulset" "chaos-sec-postgres" "${NAMESPACE}"; then
        failures=$((failures + 1))
        log error "PostgreSQL StatefulSet rollout failed"
    fi

    if ! wait_for_rollout "statefulset" "chaos-sec-redis" "${NAMESPACE}"; then
        failures=$((failures + 1))
        log error "Redis StatefulSet rollout failed"
    fi

    # Wait for application layer rollouts
    log info "Waiting for application layer rollouts..."

    if ! wait_for_rollout "deployment" "chaos-sec-backend" "${NAMESPACE}"; then
        failures=$((failures + 1))
        log error "Backend Deployment rollout failed"
    fi

    if ! wait_for_rollout "deployment" "chaos-sec-frontend" "${NAMESPACE}"; then
        failures=$((failures + 1))
        log error "Frontend Deployment rollout failed"
    fi

    if [[ ${failures} -gt 0 ]]; then
        log error "${failures} rollout(s) failed. Check pod events for details."
        log info "Useful commands:"
        log info "  kubectl get pods -n ${NAMESPACE}"
        log info "  kubectl describe pod <pod-name> -n ${NAMESPACE}"
        log info "  kubectl logs <pod-name> -n ${NAMESPACE}"
        return 1
    fi

    log success "All rollouts completed successfully"
}

# ──────────────────────────────────────────────
# Step 6: Health Checks
# ──────────────────────────────────────────────

run_health_checks() {
    log step "Running health checks"

    if [[ "${SKIP_HEALTH}" == "true" ]]; then
        log warn "Skipping health checks (--skip-health flag provided)"
        return 0
    fi

    if [[ "${DRY_RUN}" == "true" ]]; then
        log dry "Would run the following health checks:"
        log dry "  - Backend liveness: http://localhost:8080/health/live"
        log dry "  - Backend readiness: http://localhost:8080/health/ready"
        log dry "  - Frontend nginx: http://localhost:80/nginx-health"
        log dry "  - PostgreSQL: pg_isready via exec"
        log dry "  - Redis: PING via exec"
        return 0
    fi

    local failures=0

    # ──────────────────────────────────────────
    # Backend Health Checks
    # ──────────────────────────────────────────
    log info "Checking backend health endpoints..."

    # Liveness probe check via port-forward
    log info "Starting port-forward for backend health check..."
    local backend_pid
    kubectl port-forward -n "${NAMESPACE}" "deployment/chaos-sec-backend" 18080:8080 &>/dev/null &
    backend_pid=$!
    sleep 3

    # Liveness check
    local liveness_code
    liveness_code=$(curl -sk -o /dev/null -w "%{http_code}" "http://localhost:18080/health/live" 2>/dev/null || echo "000")
    if [[ "${liveness_code}" == "200" ]]; then
        log success "Backend liveness probe: HTTP ${liveness_code} (healthy)"
    else
        log error "Backend liveness probe: HTTP ${liveness_code} (expected 200)"
        failures=$((failures + 1))
    fi

    # Readiness check
    local readiness_code
    readiness_code=$(curl -sk -o /dev/null -w "%{http_code}" "http://localhost:18080/health/ready" 2>/dev/null || echo "000")
    if [[ "${readiness_code}" == "200" ]]; then
        log success "Backend readiness probe: HTTP ${readiness_code} (ready)"
    else
        log error "Backend readiness probe: HTTP ${readiness_code} (expected 200)"
        failures=$((failures + 1))
    fi

    # Kill the port-forward
    kill "${backend_pid}" 2>/dev/null || true
    wait "${backend_pid}" 2>/dev/null || true

    # ──────────────────────────────────────────
    # Frontend Health Check
    # ──────────────────────────────────────────
    log info "Checking frontend health endpoint..."

    local frontend_pid
    kubectl port-forward -n "${NAMESPACE}" "deployment/chaos-sec-frontend" 180:80 &>/dev/null &
    frontend_pid=$!
    sleep 3

    local frontend_code
    frontend_code=$(curl -sk -o /dev/null -w "%{http_code}" "http://localhost:180/nginx-health" 2>/dev/null || echo "000")
    if [[ "${frontend_code}" == "200" ]]; then
        log success "Frontend nginx health: HTTP ${frontend_code} (healthy)"
    else
        log error "Frontend nginx health: HTTP ${frontend_code} (expected 200)"
        failures=$((failures + 1))
    fi

    # Kill the port-forward
    kill "${frontend_pid}" 2>/dev/null || true
    wait "${frontend_pid}" 2>/dev/null || true

    # ──────────────────────────────────────────
    # PostgreSQL Health Check
    # ──────────────────────────────────────────
    log info "Checking PostgreSQL health..."

    local pg_pod
    pg_pod=$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=chaos-sec-postgres \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    if [[ -n "${pg_pod}" ]]; then
        local pg_result
        pg_result=$(kubectl exec -n "${NAMESPACE}" "${pg_pod}" -- \
            pg_isready -h 127.0.0.1 -p 5432 -U chaossec_admin -d chaossec 2>/dev/null || echo "failed")

        if echo "${pg_result}" | grep -q "accepting connections"; then
            log success "PostgreSQL is healthy and accepting connections"
        else
            log error "PostgreSQL health check failed: ${pg_result}"
            failures=$((failures + 1))
        fi
    else
        log warn "Could not find PostgreSQL pod for health check"
        failures=$((failures + 1))
    fi

    # ──────────────────────────────────────────
    # Redis Health Check
    # ──────────────────────────────────────────
    log info "Checking Redis health..."

    local redis_pod
    redis_pod=$(kubectl get pods -n "${NAMESPACE}" -l app.kubernetes.io/name=chaos-sec-redis \
        -o jsonpath='{.items[0].metadata.name}' 2>/dev/null || echo "")

    if [[ -n "${redis_pod}" ]]; then
        local redis_result
        redis_result=$(kubectl exec -n "${NAMESPACE}" "${redis_pod}" -- \
            redis-cli -a "$(kubectl get secret chaos-sec-secrets -n "${NAMESPACE}" \
                -o jsonpath='{.data.REDIS_PASSWORD}' 2>/dev/null | base64 -d 2>/dev/null)" \
            --no-auth-warning ping 2>/dev/null || echo "failed")

        if echo "${redis_result}" | grep -q "PONG"; then
            log success "Redis is healthy (PONG received)"
        else
            log error "Redis health check failed: ${redis_result}"
            failures=$((failures + 1))
        fi
    else
        log warn "Could not find Redis pod for health check"
        failures=$((failures + 1))
    fi

    # ──────────────────────────────────────────
    # Ingress Health Check
    # ──────────────────────────────────────────
    log info "Checking ingress status..."

    local ingress_ip
    ingress_ip=$(kubectl get ingress -n "${NAMESPACE}" chaos-sec-ingress \
        -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || \
        kubectl get ingress -n "${NAMESPACE}" chaos-sec-ingress \
        -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null || echo "")

    if [[ -n "${ingress_ip}" ]]; then
        log success "Ingress is provisioned: ${ingress_ip}"
    else
        log warn "Ingress external IP/hostname not yet assigned (may take a few minutes)"
    fi

    # ──────────────────────────────────────────
    # Summary
    # ──────────────────────────────────────────
    if [[ ${failures} -gt 0 ]]; then
        log error "Health checks completed with ${failures} failure(s)"
        log info "Services may still be initializing. Re-run health checks in a minute."
        log info "Useful commands:"
        log info "  kubectl get pods -n ${NAMESPACE}"
        log info "  kubectl get events -n ${NAMESPACE} --sort-by='.lastTimestamp'"
        return 1
    fi

    log success "All health checks passed"
}

# ──────────────────────────────────────────────
# Print Deployment Summary
# ──────────────────────────────────────────────

print_summary() {
    log step "Deployment summary"

    echo -e ""
    echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}║              Chaos-Sec Platform Deployment Summary           ║${NC}"
    echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo -e ""

    if [[ "${DRY_RUN}" == "true" ]]; then
        echo -e "  ${YELLOW}Mode:${NC}             Dry Run (no changes applied)"
    else
        echo -e "  ${GREEN}Mode:${NC}             Live Deployment"
    fi

    echo -e "  Namespace:         ${NAMESPACE}"
    echo -e "  Manifest Dir:      ${K8S_DIR}"
    echo -e ""

    if [[ "${DRY_RUN}" != "true" ]]; then
        # Gather deployed resource counts
        local deployments statefulsets services pods
        deployments=$(kubectl get deployments -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l || echo "0")
        statefulsets=$(kubectl get statefulsets -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l || echo "0")
        services=$(kubectl get services -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l || echo "0")
        pods=$(kubectl get pods -n "${NAMESPACE}" --no-headers 2>/dev/null | wc -l || echo "0")

        echo -e "  ${BOLD}Resources:${NC}"
        echo -e "    Deployments:     ${deployments}"
        echo -e "    StatefulSets:    ${statefulsets}"
        echo -e "    Services:        ${services}"
        echo -e "    Pods:            ${pods}"
        echo -e ""

        # Show pod status
        echo -e "  ${BOLD}Pod Status:${NC}"
        kubectl get pods -n "${NAMESPACE}" \
            -o custom-columns=NAME:.metadata.name,STATUS:.status.phase,RESTARTS:.status.containerStatuses[0].restartCount,AGE:.metadata.creationTimestamp 2>/dev/null || true
        echo -e ""

        # Show ingress
        echo -e "  ${BOLD}Ingress:${NC}"
        kubectl get ingress -n "${NAMESPACE}" 2>/dev/null || echo "    No ingress found"
        echo -e ""

        # Show HPA
        echo -e "  ${BOLD}Horizontal Pod Autoscaler:${NC}"
        kubectl get hpa -n "${NAMESPACE}" 2>/dev/null || echo "    No HPA found"
    fi

    echo -e ""
    echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}║                    Next Steps                               ║${NC}"
    echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo -e ""
    echo -e "  1. Check deployment status:"
    echo -e "     ${CYAN}kubectl get all -n ${NAMESPACE}${NC}"
    echo -e ""
    echo -e "  2. View backend logs:"
    echo -e "     ${CYAN}kubectl logs -f deployment/chaos-sec-backend -n ${NAMESPACE}${NC}"
    echo -e ""
    echo -e "  3. View frontend logs:"
    echo -e "     ${CYAN}kubectl logs -f deployment/chaos-sec-frontend -n ${NAMESPACE}${NC}"
    echo -e ""
    echo -e "  4. Port-forward to access locally:"
    echo -e "     ${CYAN}kubectl port-forward -n ${NAMESPACE} deployment/chaos-sec-backend 8080:8080${NC}"
    echo -e "     ${CYAN}kubectl port-forward -n ${NAMESPACE} deployment/chaos-sec-frontend 3000:80${NC}"
    echo -e ""
    echo -e "  5. Check ingress for external access:"
    echo -e "     ${CYAN}kubectl get ingress -n ${NAMESPACE}${NC}"
    echo -e ""
    echo -e "  6. Run database migrations manually (if needed):"
    echo -e "     ${CYAN}kubectl exec -n ${NAMESPACE} deployment/chaos-sec-backend -- /app/chaos-sec-backend --migrate${NC}"
    echo -e ""
    echo -e "  7. Monitor HPA scaling:"
    echo -e "     ${CYAN}kubectl get hpa -n ${NAMESPACE} -w${NC}"
    echo -e ""
}

# ──────────────────────────────────────────────
# Main Execution
# ──────────────────────────────────────────────

main() {
    local start_time
    start_time=$(date +%s)

    echo -e ""
    echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${CYAN}         Chaos-Sec Platform – Production Deployment           ${NC}"
    echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════════════════${NC}"
    echo -e ""

    if [[ "${DRY_RUN}" == "true" ]]; then
        echo -e "${YELLOW}${BOLD}  ⚠ DRY RUN MODE – No changes will be applied to the cluster${NC}"
        echo -e ""
    fi

    # Step 0: Pre-flight checks
    preflight_checks

    # Step 1: Create namespace
    create_namespace

    # Step 2: Apply secrets
    apply_secrets

    # Step 3: Apply ConfigMap
    apply_configmap

    # Step 4: Apply all Kubernetes manifests
    apply_manifests

    # Step 5: Wait for rollouts
    wait_for_rollouts

    # Step 6: Run health checks
    run_health_checks

    # Print deployment summary
    print_summary

    local end_time
    end_time=$(date +%s)
    local duration=$((end_time - start_time))

    echo -e ""
    if [[ "${DRY_RUN}" == "true" ]]; then
        log success "Dry run completed in ${duration}s"
    else
        log success "Deployment completed in ${duration}s"
    fi
    echo -e ""
}

# Run main function
main "$@"
