#!/usr/bin/env bash
# ============================================================================
# Chaos-Sec: Development Setup Script
# ============================================================================
# Sets up the complete local development environment:
#   1. Checks prerequisites (go, node, docker, kubectl, kind)
#   2. Starts Docker Compose infrastructure services
#   3. Waits for services to become healthy
#   4. Runs database migrations
#   5. Installs frontend dependencies
#   6. Optionally creates a Kind Kubernetes cluster
#   7. Prints next steps
#
# Usage:
#   ./scripts/setup-dev.sh              # Full setup
#   ./scripts/setup-dev.sh --skip-kind  # Skip Kind cluster creation
#   ./scripts/setup-dev.sh --skip-infra # Skip infrastructure startup
#   ./scripts/setup-dev.sh --help       # Show help
# ============================================================================

set -euo pipefail

# ----------------------------------------------------------------------------
# Constants & Colors
# ----------------------------------------------------------------------------
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
readonly COMPOSE_FILE="${PROJECT_ROOT}/docker-compose.yml"
readonly ENV_EXAMPLE="${PROJECT_ROOT}/.env.example"
readonly ENV_FILE="${PROJECT_ROOT}/.env"

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly YELLOW='\033[0;33m'
readonly CYAN='\033[0;36m'
readonly BOLD='\033[1m'
readonly RESET='\033[0m'

# ----------------------------------------------------------------------------
# Flags
# ----------------------------------------------------------------------------
SKIP_KIND=false
SKIP_INFRA=false
SKIP_MIGRATIONS=false
SKIP_FRONTEND_DEPS=false
VERBOSE=false

# ----------------------------------------------------------------------------
# Helper Functions
# ----------------------------------------------------------------------------

print_banner() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${CYAN}║          Chaos-Sec Development Environment Setup           ║${RESET}"
    echo -e "${CYAN}╚══════════════════════════════════════════════════════════════╝${RESET}"
    echo ""
}

log_info() {
    echo -e "${CYAN}▶${RESET} $1"
}

log_success() {
    echo -e "${GREEN}✔${RESET} $1"
}

log_warn() {
    echo -e "${YELLOW}⚠${RESET} $1"
}

log_error() {
    echo -e "${RED}✘${RESET} $1"
}

log_step() {
    echo ""
    echo -e "${BOLD}${CYAN}── $1 ──${RESET}"
}

# Check if a command exists
check_command() {
    local cmd="$1"
    local name="${2:-$cmd}"
    local required="${3:-true}"

    if command -v "$cmd" &>/dev/null; then
        local version
        version=$("$cmd" --version 2>&1 | head -n1)
        log_success "$name is installed: $version"
        return 0
    elif [ "$required" = "true" ]; then
        log_error "$name is NOT installed (required)"
        return 1
    else
        log_warn "$name is NOT installed (optional — some features won't work)"
        return 0
    fi
}

# Wait for a TCP endpoint to become reachable
wait_for_tcp() {
    local host="$1"
    local port="$2"
    local description="$3"
    local max_attempts="${4:-30}"
    local attempt=1

    log_info "Waiting for ${description} (${host}:${port})..."

    while ! (echo > /dev/tcp/"${host}"/"${port}") 2>/dev/null; do
        if [ "$attempt" -ge "$max_attempts" ]; then
            log_error "Timed out waiting for ${description} after ${max_attempts}s"
            return 1
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    log_success "${description} is reachable (${host}:${port})"
    return 0
}

# Wait for an HTTP endpoint to return 200
wait_for_http() {
    local url="$1"
    local description="$2"
    local max_attempts="${3:-30}"
    local attempt=1

    log_info "Waiting for ${description} (${url})..."

    while ! curl -sfS -o /dev/null "$url" 2>/dev/null; do
        if [ "$attempt" -ge "$max_attempts" ]; then
            log_error "Timed out waiting for ${description} after ${max_attempts}s"
            return 1
        fi
        attempt=$((attempt + 1))
        sleep 2
    done

    log_success "${description} is healthy"
    return 0
}

# ----------------------------------------------------------------------------
# Prerequisite Checks
# ----------------------------------------------------------------------------
check_prerequisites() {
    log_step "Checking Prerequisites"

    local missing=0

    check_command go "Go" "true" || missing=1
    check_command node "Node.js" "true" || missing=1
    check_command npm "npm" "true" || missing=1
    check_command docker "Docker" "true" || missing=1
    check_command kubectl "kubectl" "false" || true
    check_command kind "Kind" "false" || true
    check_command migrate "golang-migrate" "false" || true

    # Verify Docker daemon is running
    if command -v docker &>/dev/null; then
        if docker info &>/dev/null; then
            log_success "Docker daemon is running"
        else
            log_error "Docker daemon is NOT running — please start Docker"
            missing=1
        fi
    fi

    # Verify docker compose is available
    if docker compose version &>/dev/null; then
        log_success "Docker Compose v2 is available"
    elif command -v docker-compose &>/dev/null; then
        log_warn "Docker Compose v1 detected — v2 is recommended (docker compose)"
        missing=1
    else
        log_error "Docker Compose is NOT available"
        missing=1
    fi

    # Go version check (1.21+)
    if command -v go &>/dev/null; then
        local go_version
        go_version=$(go version | grep -oP 'go\K[0-9]+\.[0-9]+' | head -1)
        local major minor
        IFS='.' read -r major minor <<< "$go_version"
        if [ "$major" -lt 1 ] || { [ "$major" -eq 1 ] && [ "$minor" -lt 21 ]; }; then
            log_error "Go version 1.21+ is required (found go${go_version})"
            missing=1
        fi
    fi

    # Node version check (18+)
    if command -v node &>/dev/null; then
        local node_version
        node_version=$(node --version | sed 's/v//' | cut -d. -f1)
        if [ "$node_version" -lt 18 ]; then
            log_error "Node.js version 18+ is required (found v${node_version})"
            missing=1
        fi
    fi

    if [ "$missing" -eq 1 ]; then
        echo ""
        log_error "Some required prerequisites are missing. Please install them and re-run."
        echo ""
        echo "  Install guides:"
        echo "    Go:          https://go.dev/dl/"
        echo "    Node.js:     https://nodejs.org/en/download/"
        echo "    Docker:      https://docs.docker.com/get-docker/"
        echo "    kubectl:     https://kubernetes.io/docs/tasks/tools/"
        echo "    Kind:        https://kind.sigs.k8s.io/docs/user/quick-start/"
        echo "    golang-migrate: https://github.com/golang-migrate/migrate/tree/master/cmd/migrate"
        exit 1
    fi

    log_success "All required prerequisites are satisfied"
}

# ----------------------------------------------------------------------------
# Environment File Setup
# ----------------------------------------------------------------------------
setup_env() {
    log_step "Setting Up Environment"

    if [ ! -f "$ENV_FILE" ]; then
        if [ -f "$ENV_EXAMPLE" ]; then
            cp "$ENV_EXAMPLE" "$ENV_FILE"
            log_success "Created .env from .env.example"
        else
            log_warn "No .env.example found — creating a minimal .env"
            cat > "$ENV_FILE" <<'ENVEOF'
# Chaos-Sec Local Development Environment
APP_ENV=development
DATABASE_URL=postgres://chaossec_admin:chaossec_local_dev_password@localhost:5432/chaossec?sslmode=disable
JWT_SECRET=local-dev-jwt-secret-change-in-production-32chars!!
ENVEOF
            log_success "Created minimal .env file"
        fi
    else
        log_success ".env file already exists"
    fi
}

# ----------------------------------------------------------------------------
# Start Infrastructure Services
# ----------------------------------------------------------------------------
start_infra() {
    log_step "Starting Infrastructure Services"

    if [ "$SKIP_INFRA" = "true" ]; then
        log_warn "Skipping infrastructure startup (--skip-infra)"
        return 0
    fi

    cd "$PROJECT_ROOT"

    # Check if services are already running
    if docker compose -f "$COMPOSE_FILE" ps --status running 2>/dev/null | grep -q "chaos-sec"; then
        log_warn "Some services are already running. Restarting..."
        docker compose -f "$COMPOSE_FILE" down
    fi

    log_info "Starting Docker Compose services (postgres, redis, mock-siem)..."
    docker compose -f "$COMPOSE_FILE" up -d postgres redis mock-siem

    log_success "Docker Compose services started"
}

# ----------------------------------------------------------------------------
# Wait for Services to Be Healthy
# ----------------------------------------------------------------------------
wait_for_services() {
    log_step "Waiting for Services to Become Healthy"

    if [ "$SKIP_INFRA" = "true" ]; then
        log_warn "Skipping service health checks (--skip-infra)"
        return 0
    fi

    # Wait for PostgreSQL
    wait_for_tcp "localhost" "5432" "PostgreSQL" 60 || {
        log_error "PostgreSQL did not become reachable"
        log_info "Check logs: docker compose logs postgres"
        exit 1
    }

    # Wait for Redis
    wait_for_tcp "localhost" "6379" "Redis" 30 || {
        log_error "Redis did not become reachable"
        log_info "Check logs: docker compose logs redis"
        exit 1
    }

    # Wait for Mock SIEM
    wait_for_http "http://localhost:8089/health" "Mock SIEM" 30 || {
        log_error "Mock SIEM did not become healthy"
        log_info "Check logs: docker compose logs mock-siem"
        exit 1
    }

    # Give PostgreSQL extra time to fully initialize
    log_info "Giving PostgreSQL a moment to fully initialize..."
    sleep 3

    log_success "All infrastructure services are healthy"
}

# ----------------------------------------------------------------------------
# Run Database Migrations
# ----------------------------------------------------------------------------
run_migrations() {
    log_step "Running Database Migrations"

    if [ "$SKIP_MIGRATIONS" = "true" ]; then
        log_warn "Skipping database migrations (--skip-migrations)"
        return 0
    fi

    local migrate_cmd=""

    # Prefer the migrate.sh helper if it exists
    if [ -f "${SCRIPT_DIR}/migrate.sh" ]; then
        migrate_cmd="bash ${SCRIPT_DIR}/migrate.sh up"
    elif command -v migrate &>/dev/null; then
        local db_url
        db_url="${DATABASE_URL:-postgres://chaossec_admin:chaossec_local_dev_password@localhost:5432/chaossec?sslmode=disable}"
        migrate_cmd="migrate -path ${PROJECT_ROOT}/backend/migrations -database \"${db_url}\" up"
    else
        log_warn "golang-migrate not found — skipping automatic migration"
        log_info "Install it: https://github.com/golang-migrate/migrate"
        log_info "Or run manually: cd backend && go run ./cmd/backend migrate"
        return 0
    fi

    log_info "Running: $migrate_cmd"
    if eval "$migrate_cmd"; then
        log_success "Database migrations completed"
    else
        log_error "Database migrations failed"
        log_info "You can retry with: make migrate-up"
        log_info "Or check status with: make migrate-status"
        # Non-fatal — the database may already be up to date
        log_warn "Continuing setup (migrations may already be applied)"
    fi
}

# ----------------------------------------------------------------------------
# Install Frontend Dependencies
# ----------------------------------------------------------------------------
install_frontend_deps() {
    log_step "Installing Frontend Dependencies"

    if [ "$SKIP_FRONTEND_DEPS" = "true" ]; then
        log_warn "Skipping frontend dependencies (--skip-frontend-deps)"
        return 0
    fi

    cd "${PROJECT_ROOT}/frontend"

    if [ -d "node_modules" ]; then
        log_info "node_modules exists — running npm ci for a clean install"
    else
        log_info "Installing frontend dependencies..."
    fi

    npm ci

    log_success "Frontend dependencies installed"
}

# ----------------------------------------------------------------------------
# Create Kind Cluster (Optional)
# ----------------------------------------------------------------------------
create_kind_cluster() {
    log_step "Kind Kubernetes Cluster"

    if [ "$SKIP_KIND" = "true" ]; then
        log_warn "Skipping Kind cluster creation (--skip-kind)"
        return 0
    fi

    if ! command -v kind &>/dev/null; then
        log_warn "Kind is not installed — skipping cluster creation"
        log_info "Install Kind: https://kind.sigs.k8s.io/docs/user/quick-start/"
        return 0
    fi

    # Check if cluster already exists
    if kind get clusters 2>/dev/null | grep -q "chaos-sec"; then
        log_warn "Kind cluster 'chaos-sec' already exists"
        log_info "To recreate: make kind-delete && make kind-create"
        return 0
    fi

    local kind_config="${SCRIPT_DIR}/kind-config.yaml"
    if [ ! -f "$kind_config" ]; then
        log_warn "Kind config not found at ${kind_config} — creating cluster without custom config"
        kind create cluster --name chaos-sec
    else
        log_info "Creating Kind cluster with custom config..."
        kind create cluster --name chaos-sec --config "$kind_config"
    fi

    log_success "Kind cluster 'chaos-sec' created"

    # Set kubeconfig
    export KUBECONFIG="$(kind get kubeconfig --name chaos-sec)"
    log_info "KUBECONFIG set for chaos-sec cluster"

    # Verify nodes are ready
    log_info "Waiting for cluster nodes to be ready..."
    local attempt=0
    local max_attempts=30
    while [ "$attempt" -lt "$max_attempts" ]; do
        if kubectl get nodes 2>/dev/null | grep -q "Ready"; then
            log_success "Cluster nodes are ready"
            kubectl get nodes
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 2
    done

    log_warn "Cluster nodes not ready yet — they may need more time"
}

# ----------------------------------------------------------------------------
# Print Next Steps
# ----------------------------------------------------------------------------
print_next_steps() {
    log_step "Next Steps"

    echo ""
    echo -e "${GREEN}╔══════════════════════════════════════════════════════════════╗${RESET}"
    echo -e "${GREEN}║       🎉  Chaos-Sec Development Environment Ready!  🎉      ║${RESET}"
    echo -e "${GREEN}╚══════════════════════════════════════════════════════════════╝${RESET}"
    echo ""
    echo -e "${BOLD}Quick Start:${RESET}"
    echo ""
    echo -e "  ${CYAN}Start both servers:${RESET}"
    echo "    make dev"
    echo ""
    echo -e "  ${CYAN}Start backend only:${RESET}"
    echo "    make backend-run"
    echo ""
    echo -e "  ${CYAN}Start frontend only:${RESET}"
    echo "    make frontend-run"
    echo ""
    echo -e "  ${CYAN}Start all infra services (full stack):${RESET}"
    echo "    make infra-up"
    echo ""
    echo -e "${BOLD}Services:${RESET}"
    echo ""
    echo "    Backend API:       http://localhost:8080"
    echo "    Backend Health:    http://localhost:8080/health"
    echo "    Backend Metrics:   http://localhost:9090/metrics"
    echo "    Frontend:          http://localhost:3000"
    echo "    PostgreSQL:        localhost:5432"
    echo "    Redis:             localhost:6379"
    echo "    Mock SIEM:         http://localhost:8089"
    echo "    Prometheus:        http://localhost:9091"
    echo "    Grafana:           http://localhost:3001  (admin / admin)"
    echo ""
    echo -e "${BOLD}Useful Commands:${RESET}"
    echo ""
    echo "    make test              # Run all tests"
    echo "    make lint              # Run all linters"
    echo "    make migrate-status    # Check migration status"
    echo "    make migrate-create name=desc  # Create a new migration"
    echo "    make infra-logs        # Tail infrastructure logs"
    echo "    make infra-logs svc=postgres    # Tail specific service"
    echo "    make kind-create       # Create Kind cluster"
    echo "    make help              # Show all available commands"
    echo ""
    echo -e "${BOLD}Troubleshooting:${RESET}"
    echo ""
    echo "    make infra-ps          # Check service status"
    echo "    make infra-logs        # View logs"
    echo "    docker compose ps      # Docker service status"
    echo "    docker compose logs    # All service logs"
    echo ""
    echo -e "${YELLOW}⚠  Remember to update .env with production values before deploying!${RESET}"
    echo ""
}

# ----------------------------------------------------------------------------
# Parse Arguments
# ----------------------------------------------------------------------------
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --skip-kind)
                SKIP_KIND=true
                shift
                ;;
            --skip-infra)
                SKIP_INFRA=true
                shift
                ;;
            --skip-migrations)
                SKIP_MIGRATIONS=true
                shift
                ;;
            --skip-frontend-deps)
                SKIP_FRONTEND_DEPS=true
                shift
                ;;
            --verbose|-v)
                VERBOSE=true
                shift
                ;;
            --help|-h)
                echo "Usage: $0 [OPTIONS]"
                echo ""
                echo "Options:"
                echo "  --skip-kind            Skip Kind cluster creation"
                echo "  --skip-infra           Skip Docker Compose infrastructure startup"
                echo "  --skip-migrations      Skip database migrations"
                echo "  --skip-frontend-deps   Skip frontend npm install"
                echo "  --verbose, -v          Enable verbose output"
                echo "  --help, -h             Show this help message"
                echo ""
                echo "Examples:"
                echo "  $0                     # Full setup"
                echo "  $0 --skip-kind         # Skip Kind cluster"
                echo "  $0 --skip-infra        # Skip infra (if already running)"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                echo "Run '$0 --help' for usage information"
                exit 1
                ;;
        esac
    done
}

# ----------------------------------------------------------------------------
# Main
# ----------------------------------------------------------------------------
main() {
    parse_args "$@"

    if [ "$VERBOSE" = "true" ]; then
        set -x
    fi

    print_banner
    check_prerequisites
    setup_env
    start_infra
    wait_for_services
    run_migrations
    install_frontend_deps
    create_kind_cluster
    print_next_steps
}

main "$@"
