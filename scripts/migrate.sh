#!/usr/bin/env bash
# ============================================================================
# Chaos-Sec: Database Migration Helper
# Uses golang-migrate (github.com/golang-migrate/migrate) to manage schema
# migrations for the Chaos-Sec platform.
#
# Usage:
#   ./scripts/migrate.sh [up|down|create|status] [migration_name]
#
# Environment:
#   DATABASE_URL  - PostgreSQL connection string
#                   (falls back to .env file or default dev URL)
# ============================================================================

set -euo pipefail

# ----------------------------------------------------------------------------
# Constants & Defaults
# ----------------------------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
MIGRATIONS_DIR="${PROJECT_ROOT}/backend/migrations"
ENV_FILE="${PROJECT_ROOT}/.env"

# Default development database URL (matches docker-compose.yml)
DEFAULT_DATABASE_URL="postgres://chaossec_admin:chaossec_local_dev_password@localhost:5432/chaossec?sslmode=disable"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

# ----------------------------------------------------------------------------
# Helper Functions
# ----------------------------------------------------------------------------

log_info() {
    echo -e "${CYAN}▶ $*${RESET}"
}

log_success() {
    echo -e "${GREEN}✔ $*${RESET}"
}

log_warn() {
    echo -e "${YELLOW}⚠ $*${RESET}"
}

log_error() {
    echo -e "${RED}✘ $*${RESET}" >&2
}

die() {
    log_error "$@"
    exit 1
}

# ----------------------------------------------------------------------------
# Load DATABASE_URL from .env file if not already set
# ----------------------------------------------------------------------------
load_database_url() {
    # If DATABASE_URL is already exported, use it
    if [[ -n "${DATABASE_URL:-}" ]]; then
        log_info "Using DATABASE_URL from environment"
        return 0
    fi

    # Try to load from .env file
    if [[ -f "${ENV_FILE}" ]]; then
        log_info "Loading DATABASE_URL from ${ENV_FILE}"
        # Safely extract DATABASE_URL from .env (handles quoted values)
        DATABASE_URL="$(grep -E '^DATABASE_URL=' "${ENV_FILE}" | head -n1 | sed 's/^DATABASE_URL=//' | tr -d '"' | tr -d "'")"
        if [[ -n "${DATABASE_URL}" ]]; then
            export DATABASE_URL
            return 0
        fi
    fi

    # Fall back to default development URL
    log_warn "DATABASE_URL not set; using default development URL"
    DATABASE_URL="${DEFAULT_DATABASE_URL}"
    export DATABASE_URL
}

# ----------------------------------------------------------------------------
# Verify prerequisites
# ----------------------------------------------------------------------------
check_prerequisites() {
    # Check that migrate is installed
    if ! command -v migrate &>/dev/null; then
        die "golang-migrate is not installed.

  Install with Homebrew:
    brew install golang-migrate

  Or with Go:
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

  Or download from:
    https://github.com/golang-migrate/migrate/releases"
    fi

    # Check that the migrations directory exists
    if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
        log_info "Creating migrations directory: ${MIGRATIONS_DIR}"
        mkdir -p "${MIGRATIONS_DIR}"
    fi

    # Verify migration files exist (except for 'create' command)
    if [[ "${COMMAND}" != "create" ]]; then
        local up_count
        up_count="$(find "${MIGRATIONS_DIR}" -name '*.up.sql' 2>/dev/null | wc -l)"
        if [[ "${up_count}" -eq 0 ]]; then
            log_warn "No migration files found in ${MIGRATIONS_DIR}"
        fi
    fi
}

# ----------------------------------------------------------------------------
# Migration Commands
# ----------------------------------------------------------------------------

# Run all pending migrations (up)
cmd_up() {
    log_info "Running all pending migrations (up)..."
    migrate \
        -source "file://${MIGRATIONS_DIR}" \
        -database "${DATABASE_URL}" \
        up 2>&1 || handle_migrate_error
    log_success "Migrations applied successfully"
}

# Roll back the most recent migration (down 1)
cmd_down() {
    log_warn "Rolling back the most recent migration (down 1)..."
    migrate \
        -source "file://${MIGRATIONS_DIR}" \
        -database "${DATABASE_URL}" \
        down 1 2>&1 || handle_migrate_error
    log_success "Rollback completed successfully"
}

# Create a new pair of migration files (up + down)
cmd_create() {
    local migration_name="${1:-}"
    if [[ -z "${migration_name}" ]]; then
        die "Migration name required.

  Usage: ./scripts/migrate.sh create <migration_name>

  Example:
    ./scripts/migrate.sh create add_users_table"
    fi

    log_info "Creating migration: ${migration_name}"
    migrate create \
        -ext sql \
        -dir "${MIGRATIONS_DIR}" \
        -seq \
        "${migration_name}" 2>&1 || die "Failed to create migration files"

    # Find the newly created files
    local up_file down_file
    up_file="$(find "${MIGRATIONS_DIR}" -name "*${migration_name}.up.sql" | sort | tail -n1)"
    down_file="$(find "${MIGRATIONS_DIR}" -name "*${migration_name}.down.sql" | sort | tail -n1)"

    log_success "Migration files created:"
    echo -e "  ${GREEN}${up_file}${RESET}"
    echo -e "  ${GREEN}${down_file}${RESET}"
    echo ""
    log_info "Edit these files to define your schema changes."
}

# Show current migration status (version + dirty state)
cmd_status() {
    log_info "Checking migration status..."

    # Use migrate version to get the current state
    local output exit_code
    set +e
    output="$(migrate \
        -source "file://${MIGRATIONS_DIR}" \
        -database "${DATABASE_URL}" \
        version 2>&1)"
    exit_code=$?
    set -e

    if [[ ${exit_code} -eq 0 ]]; then
        local current_version
        current_version="$(echo "${output}" | head -n1 | awk '{print $NF}')"
        log_success "Current migration version: ${current_version}"
    elif [[ ${exit_code} -eq 1 ]]; then
        # Exit code 1 from `migrate version` means no migrations applied yet
        log_warn "No migrations have been applied yet"
    else
        log_error "Failed to get migration status"
        echo "${output}"
    fi

    # Count migration files
    local total_up total_down pending
    total_up="$(find "${MIGRATIONS_DIR}" -name '*.up.sql' 2>/dev/null | wc -l)"
    total_down="$(find "${MIGRATIONS_DIR}" -name '*.down.sql' 2>/dev/null | wc -l)"
    echo ""
    echo -e "  ${BOLD}Migration files:${RESET}"
    echo -e "    Up files:     ${total_up}"
    echo -e "    Down files:   ${total_down}"
    echo -e "    Directory:    ${MIGRATIONS_DIR}"
    echo ""

    # List all migration files
    if [[ "${total_up}" -gt 0 ]]; then
        echo -e "  ${BOLD}Available migrations:${RESET}"
        local i=1
        for f in "$(find "${MIGRATIONS_DIR}" -name '*.up.sql' | sort)"; do
            local basename
            basename="$(basename "${f}" .up.sql)"
            echo -e "    ${i}. ${basename}"
            i=$((i + 1))
        done
    fi
}

# Handle migrate command errors and provide helpful messages
handle_migrate_error() {
    local exit_code=$?
    case ${exit_code} in
        1)
            log_error "Migration failed: no migrations to apply (already at latest version)"
            ;;
        2)
            log_error "Migration failed: dirty database state detected!

  This usually means a previous migration was interrupted.
  To fix this, you need to manually force the migration version:

    # Check the current dirty state
    migrate -source 'file://${MIGRATIONS_DIR}' -database '<url>' version

    # Force set the version (USE WITH CAUTION)
    migrate -source 'file://${MIGRATIONS_DIR}' -database '<url>' force <version>"
            ;;
        *)
            log_error "Migration command failed with exit code: ${exit_code}"
            ;;
    esac
    return ${exit_code}
}

# ----------------------------------------------------------------------------
# Usage / Help
# ----------------------------------------------------------------------------
print_usage() {
    cat <<EOF

${BOLD}Chaos-Sec Database Migration Helper${RESET}

${BOLD}Usage:${RESET}
  ./scripts/migrate.sh <command> [arguments]

${BOLD}Commands:${RESET}
  up              Run all pending migrations
  down            Roll back the most recent migration
  create <name>   Create a new migration file pair (up + down)
  status          Show current migration version and file list

${BOLD}Environment:${RESET}
  DATABASE_URL    PostgreSQL connection string
                  (loaded from .env or defaults to local dev URL)

${BOLD}Examples:${RESET}
  # Apply all pending migrations
  ./scripts/migrate.sh up

  # Roll back last migration
  ./scripts/migrate.sh down

  # Create a new migration
  ./scripts/migrate.sh create add_experiment_logs

  # Check migration status
  ./scripts/migrate.sh status

  # Override database URL
  DATABASE_URL='postgres://user:pass@host:5432/db' ./scripts/migrate.sh status

EOF
}

# ----------------------------------------------------------------------------
# Main
# ----------------------------------------------------------------------------
main() {
    local COMMAND="${1:-}"
    local MIGRATION_NAME="${2:-}"

    # Show help if no command or --help/-h
    if [[ -z "${COMMAND}" ]] || [[ "${COMMAND}" == "-h" ]] || [[ "${COMMAND}" == "--help" ]]; then
        print_usage
        exit 0
    fi

    # Load database URL
    load_database_url

    # Verify tooling
    check_prerequisites

    # Dispatch command
    case "${COMMAND}" in
        up)
            cmd_up
            ;;
        down)
            cmd_down
            ;;
        create)
            cmd_create "${MIGRATION_NAME}"
            ;;
        status)
            cmd_status
            ;;
        force)
            # Hidden command for recovering from dirty state
            local version="${MIGRATION_NAME}"
            if [[ -z "${version}" ]]; then
                die "Version number required: ./scripts/migrate.sh force <version>"
            fi
            log_warn "Force-setting migration version to ${version}..."
            migrate \
                -source "file://${MIGRATIONS_DIR}" \
                -database "${DATABASE_URL}" \
                force "${version}" 2>&1 || die "Force version failed"
            log_success "Migration version forced to ${version}"
            ;;
        *)
            log_error "Unknown command: ${COMMAND}"
            print_usage
            exit 1
            ;;
    esac
}

main "$@"
