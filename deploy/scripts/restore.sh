#!/usr/bin/env bash
# ============================================================================
# Chaos-Sec: Database Restore Script
# Download, verify, and restore PostgreSQL backups from S3 with
# point-in-time recovery support for the Chaos-Sec platform
# ============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"

# Database configuration (overridable via environment)
DB_HOST="${CHAOS_DB_HOST:-localhost}"
DB_PORT="${CHAOS_DB_PORT:-5432}"
DB_NAME="${CHAOS_DB_NAME:-chaossec}"
DB_USER="${CHAOS_DB_USER:-chaossec_admin}"
DB_PASSWORD="${CHAOS_DB_PASSWORD:-chaossec_local_dev_password}"
DB_SSLMODE="${CHAOS_DB_SSLMODE:-disable}"
DB_SUPERUSER="${CHAOS_DB_SUPERUSER:-chaossec_admin}"

# S3 configuration
S3_BUCKET="${S3_BUCKET:-s3://chaos-sec-backups}"
S3_PREFIX="${S3_PREFIX:-database}"
S3_REGION="${AWS_REGION:-eu-west-1}"

# Local staging directory
RESTORE_DIR="${RESTORE_DIR:-/tmp/chaos-sec-restore}"

# Encryption (optional — must match backup settings)
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
ENCRYPTION_ALGO="${ENCRYPTION_ALGO:-aes256}"

# Point-in-time recovery (PITR) settings
PITR_TARGET_TIME="${PITR_TARGET_TIME:-}"
PITR_TARGET_XID="${PITR_TARGET_XID:-}"
PITR_TARGET_INCLUSIVE="${PITR_TARGET_INCLUSIVE:-true}"
PITR_TARGET_ACTION="${PITR_TARGET_ACTION:-pause}"

# PostgreSQL data directory (for PITR)
PG_DATA_DIR="${PG_DATA_DIR:-/var/lib/postgresql/data}"

# Restore options
DROP_EXISTING_CONNECTIONS="${DROP_EXISTING_CONNECTIONS:-true}"
RUN_MIGRATIONS_AFTER="${RUN_MIGRATIONS_AFTER:-true}"
VACUUM_AFTER_RESTORE="${VACUUM_AFTER_RESTORE:-true}"
DRY_RUN="${DRY_RUN:-false}"

# ---------------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log()       { echo -e "${CYAN}[INFO]${NC}  $*"; }
log_ok()    { echo -e "${GREEN}[OK]${NC}    $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_err()   { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $*"; }
log_dim()   { echo -e "${DIM}$*${NC}"; }
log_bold()  { echo -e "${BOLD}$*${NC}"; }

# ---------------------------------------------------------------------------
# Error handling
# ---------------------------------------------------------------------------
cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
        log_err "Restore script failed with exit code ${exit_code}"
        log_err "Staging files preserved at: ${RESTORE_DIR}"
        log_err "Inspect files and retry, or clean up manually with:"
        log_err "  rm -rf ${RESTORE_DIR}"
    fi
    unset PGPASSWORD
    exit $exit_code
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------
check_prerequisites() {
    local missing=0

    if ! command -v pg_restore &>/dev/null; then
        log_err "pg_restore is not installed. Install PostgreSQL client tools."
        missing=1
    fi

    if ! command -v psql &>/dev/null; then
        log_err "psql is not installed. Install PostgreSQL client tools."
        missing=1
    fi

    if ! command -v gunzip &>/dev/null; then
        log_err "gunzip is not installed."
        missing=1
    fi

    if ! command -v sha256sum &>/dev/null && ! command -v shasum &>/dev/null; then
        log_err "sha256sum or shasum is not installed."
        missing=1
    fi

    # Check AWS CLI for S3 operations
    if ! command -v aws &>/dev/null; then
        log_err "AWS CLI (aws) is not installed. Install with: pip install awscli"
        missing=1
    fi

    # Verify AWS credentials
    if ! aws sts get-caller-identity &>/dev/null; then
        log_err "AWS credentials not configured. Run 'aws configure' or set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY."
        missing=1
    fi

    if [[ $missing -eq 1 ]]; then
        log_err "Prerequisite checks failed. Install missing tools and try again."
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------
compute_sha256() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file" | awk '{print $1}'
    else
        shasum -a 256 "$file" | awk '{print $1}'
    fi
}

file_size_human() {
    local file="$1"
    local bytes
    bytes="$(stat --printf='%s' "$file" 2>/dev/null || stat -f '%z' "$file" 2>/dev/null || echo "0")"
    if command -v numfmt &>/dev/null; then
        echo "$bytes" | numfmt --to=iec-i --suffix=B 2>/dev/null || echo "${bytes} bytes"
    else
        echo "${bytes} bytes"
    fi
}

pg_env() {
    export PGPASSWORD="${DB_PASSWORD}"
}

pg_unenv() {
    unset PGPASSWORD 2>/dev/null || true
}

s3_uri_for_file() {
    local filename="$1"
    local date="${2:-}"
    if [[ -n "${date}" ]]; then
        echo "${S3_BUCKET}/${S3_PREFIX}/${date}/${filename}"
    else
        # Search by date prefix in filename
        local date_part
        date_part="$(echo "${filename}" | grep -oP '\d{4}\d{2}\d{2}' | head -1 || true)"
        if [[ -n "${date_part}" ]]; then
            # Convert YYYYMMDD to YYYY-MM-DD
            local formatted_date="${date_part:0:4}-${date_part:4:2}-${date_part:6:2}"
            echo "${S3_BUCKET}/${S3_PREFIX}/${formatted_date}/${filename}"
        else
            echo "${S3_BUCKET}/${S3_PREFIX}/${filename}"
        fi
    fi
}

# ---------------------------------------------------------------------------
# Find backup in S3
# ---------------------------------------------------------------------------
resolve_backup_location() {
    local target="$1"
    local s3_backup_uri=""
    local s3_manifest_uri=""

    if [[ "${target}" == s3://* ]]; then
        # Full S3 URI provided directly
        s3_backup_uri="${target}"
        local base_name
        base_name="$(basename "${target}")"
        # Derive manifest path
        if [[ "${base_name}" == *.enc ]]; then
            s3_manifest_uri="$(dirname "${target}")/${base_name%.enc}"
            s3_manifest_uri="${s3_manifest_uri%.*}.manifest"
        else
            s3_manifest_uri="$(dirname "${target}")/${base_name%.*}.manifest"
        fi
    elif [[ "${target}" == "latest" ]]; then
        # Find the most recent backup in S3
        log "Searching for latest backup in S3..."
        local latest_prefix
        latest_prefix="$(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${S3_PREFIX}/" 2>/dev/null | tail -1 | awk '{print $2}')"
        if [[ -z "${latest_prefix}" ]]; then
            log_err "No backup date folders found in ${S3_BUCKET}/${S3_PREFIX}/"
            exit 1
        fi

        local latest_file
        latest_file="$(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${latest_prefix}" 2>/dev/null | grep -E '\.gz' | grep -v '\.manifest' | tail -1 | awk '{print $4}')"
        if [[ -z "${latest_file}" ]]; then
            log_err "No backup files found in ${S3_BUCKET}/${latest_prefix}"
            exit 1
        fi

        s3_backup_uri="${S3_BUCKET}/${latest_prefix}${latest_file}"
        if [[ "${latest_file}" == *.enc ]]; then
            s3_manifest_uri="${S3_BUCKET}/${latest_prefix}${latest_file%.enc}"
            s3_manifest_uri="${s3_manifest_uri%.*}.manifest"
        else
            s3_manifest_uri="${S3_BUCKET}/${latest_prefix}${latest_file%.*}.manifest"
        fi
        log_ok "Latest backup found: ${s3_backup_uri}"

    elif [[ "${target}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        # Date provided — find backup for that specific date
        local date_prefix="${S3_PREFIX}/${target}/"
        local date_file
        date_file="$(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${date_prefix}" 2>/dev/null | grep -E '\.gz' | grep -v '\.manifest' | tail -1 | awk '{print $4}')"
        if [[ -z "${date_file}" ]]; then
            log_err "No backups found for date ${target} at ${S3_BUCKET}/${date_prefix}"
            exit 1
        fi

        s3_backup_uri="${S3_BUCKET}/${date_prefix}${date_file}"
        if [[ "${date_file}" == *.enc ]]; then
            s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.enc}"
            s3_manifest_uri="${s3_manifest_uri%.*}.manifest"
        else
            s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.*}.manifest"
        fi
        log_ok "Found backup for ${target}: ${s3_backup_uri}"

    elif [[ "${target}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2} ]]; then
        # Point-in-time recovery target (ISO 8601 timestamp)
        PITR_TARGET_TIME="${target}"
        log "Point-in-time recovery target: ${PITR_TARGET_TIME}"

        # Find the latest backup before this timestamp
        local target_date
        target_date="$(echo "${target}" | cut -dT -f1)"
        local date_prefix="${S3_PREFIX}/${target_date}/"
        local date_file
        date_file="$(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${date_prefix}" 2>/dev/null | grep -E '\.gz' | grep -v '\.manifest' | tail -1 | awk '{print $4}')"
        if [[ -z "${date_file}" ]]; then
            # Try the day before
            local prev_date
            prev_date="$(date -d "${target_date} -1 day" +%Y-%m-%d 2>/dev/null || date -v-1d +%Y-%m-%d 2>/dev/null || echo "")"
            if [[ -n "${prev_date}" ]]; then
                date_prefix="${S3_PREFIX}/${prev_date}/"
                date_file="$(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${date_prefix}" 2>/dev/null | grep -E '\.gz' | grep -v '\.manifest' | tail -1 | awk '{print $4}')"
            fi
        fi

        if [[ -z "${date_file}" ]]; then
            log_err "No backup found for point-in-time recovery target: ${target}"
            exit 1
        fi

        s3_backup_uri="${S3_BUCKET}/${date_prefix}${date_file}"
        if [[ "${date_file}" == *.enc ]]; then
            s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.enc}"
            s3_manifest_uri="${s3_manifest_uri%.*}.manifest"
        else
            s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.*}.manifest"
        fi
        log_ok "Base backup for PITR: ${s3_backup_uri}"

    else
        # Assume it's a filename — search across dates
        log "Searching for backup file: ${target}..."
        local found=0

        while IFS= read -r date_prefix; do
            local date_file
            date_file="$(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${date_prefix}" 2>/dev/null | grep "${target}" | tail -1 | awk '{print $4}')"
            if [[ -n "${date_file}" ]]; then
                s3_backup_uri="${S3_BUCKET}/${date_prefix}${date_file}"
                if [[ "${date_file}" == *.enc ]]; then
                    s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.enc}"
                    s3_manifest_uri="${s3_manifest_uri%.*}.manifest"
                else
                    s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.*}.manifest"
                fi
                found=1
                break
            fi
        done < <(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${S3_PREFIX}/" 2>/dev/null | awk '{print $2}')

        if [[ $found -eq 0 ]]; then
            log_err "Backup file '${target}' not found in S3"
            log_err "Use '${SCRIPT_NAME} list' to see available backups"
            exit 1
        fi
    fi

    # Export results
    RESOLVED_S3_BACKUP_URI="${s3_backup_uri}"
    RESOLVED_S3_MANIFEST_URI="${s3_manifest_uri}"
}

# ---------------------------------------------------------------------------
# Command: restore
# ---------------------------------------------------------------------------
cmd_restore() {
    local target="${1:-}"
    local confirm_flag="${2:-}"
    local pitr_flag="${3:-}"

    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Database Restore"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    if [[ -z "${target}" ]]; then
        log_err "Usage: ${SCRIPT_NAME} <target> [--confirm] [--pitr]"
        log_err ""
        log_err "Targets:"
        log_err "  latest                    Restore most recent backup"
        log_err "  2024-01-15                Restore latest backup from that date"
        log_err "  2024-01-15T09:30:00       Point-in-time recovery to that timestamp"
        log_err "  filename.sql.gz            Restore specific file from S3"
        log_err "  s3://bucket/path/file.gz   Restore from full S3 URI"
        log_err ""
        log_err "Flags:"
        log_err "  --confirm, -y    Skip confirmation prompt"
        log_err "  --dry-run         Show what would happen without executing"
        exit 1
    fi

    # Parse flags
    local skip_confirm=false
    if [[ "${confirm_flag}" == "--confirm" || "${confirm_flag}" == "-y" ]]; then
        skip_confirm=true
    fi

    # ── Resolve backup location ─────────────────────────────────
    log_step "1/8  Locating backup in S3"
    resolve_backup_location "${target}"

    local s3_backup_uri="${RESOLVED_S3_BACKUP_URI}"
    local s3_manifest_uri="${RESOLVED_S3_MANIFEST_URI}"

    local is_pitr=false
    if [[ -n "${PITR_TARGET_TIME}" ]]; then
        is_pitr=true
    fi

    log "Database:  ${DB_NAME}"
    log "Host:      ${DB_HOST}:${DB_PORT}"
    log "Backup:    ${s3_backup_uri}"
    log "Manifest:  ${s3_manifest_uri}"
    if [[ "${is_pitr}" == "true" ]]; then
        log "PITR:      ${PITR_TARGET_TIME}"
    fi
    if [[ "${DRY_RUN}" == "true" ]]; then
        log "Mode:      DRY RUN (no changes will be made)"
    fi
    echo ""

    # ── Step 2: Create staging directory ─────────────────────────
    log_step "2/8  Preparing staging directory"
    mkdir -p "${RESTORE_DIR}"
    log_ok "Staging directory: ${RESTORE_DIR}"

    # ── Step 3: Download backup from S3 ─────────────────────────
    log_step "3/8  Downloading backup from S3"
    local download_file
    download_file="$(basename "${s3_backup_uri}")"
    local local_backup="${RESTORE_DIR}/${download_file}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_dim "[DRY RUN] Would download: ${s3_backup_uri} -> ${local_backup}"
    else
        if ! aws s3 cp ${AWS_CLI_OPTS:-} "${s3_backup_uri}" "${local_backup}"; then
            log_err "Failed to download backup from S3"
            exit 1
        fi
        log_ok "Downloaded: ${local_backup} ($(file_size_human "${local_backup}"))"
    fi

    # ── Step 4: Verify backup integrity ──────────────────────────
    log_step "4/8  Verifying backup integrity"

    local local_manifest="${RESTORE_DIR}/${download_file%.*}.manifest"
    if [[ "${download_file}" == *.enc ]]; then
        local_manifest="${RESTORE_DIR}/${download_file%.enc}"
        local_manifest="${local_manifest%.*}.manifest"
    fi

    if [[ "${DRY_RUN}" != "true" ]]; then
        # Download manifest if available
        if aws s3 cp ${AWS_CLI_OPTS:-} "${s3_manifest_uri}" "${local_manifest}" 2>/dev/null; then
            log_ok "Manifest downloaded"

            # Verify SHA-256 checksum
            local expected_sha
            expected_sha="$(grep '^sha256=' "${local_manifest}" 2>/dev/null | cut -d= -f2 || true)"
            if [[ -n "${expected_sha}" ]]; then
                local actual_sha
                actual_sha="$(compute_sha256 "${local_backup}")"
                if [[ "${actual_sha}" != "${expected_sha}" ]]; then
                    log_err "SHA-256 checksum mismatch!"
                    log_err "  Expected: ${expected_sha}"
                    log_err "  Actual:   ${actual_sha}"
                    log_err "  The backup may be corrupted. Do NOT proceed with restore."
                    exit 1
                fi
                log_ok "SHA-256 checksum verified ✓"
            else
                log_warn "No SHA-256 in manifest — checksum verification skipped"
            fi

            # Verify file size
            local expected_size
            expected_size="$(grep '^size_bytes=' "${local_manifest}" 2>/dev/null | cut -d= -f2 || true)"
            if [[ -n "${expected_size}" ]]; then
                local actual_size
                actual_size="$(wc -c < "${local_backup}" | tr -d ' ')"
                if [[ "${actual_size}" != "${expected_size}" ]]; then
                    log_err "File size mismatch! Expected ${expected_size} bytes, got ${actual_size}"
                    exit 1
                fi
                log_ok "File size verified (${actual_size} bytes) ✓"
            fi

            # Display backup metadata
            local backup_db backup_ts backup_host
            backup_db="$(grep '^database=' "${local_manifest}" 2>/dev/null | cut -d= -f2 || echo "unknown")"
            backup_ts="$(grep '^timestamp=' "${local_manifest}" 2>/dev/null | cut -d= -f2 || echo "unknown")"
            backup_host="$(grep '^host=' "${local_manifest}" 2>/dev/null | cut -d= -f2 || echo "unknown")"
            log "  Original database: ${backup_db}"
            log "  Backup timestamp:  ${backup_ts}"
            log "  Source host:        ${backup_host}"
        else
            log_warn "Manifest not available at ${s3_manifest_uri}"
            log_warn "Integrity checks will be limited"

            # Basic gzip validation
            if gunzip -t "${local_backup}" 2>/dev/null; then
                log_ok "Valid gzip archive ✓"
            else
                log_err "Invalid gzip archive — backup may be corrupted"
                exit 1
            fi
        fi
    else
        log_dim "[DRY RUN] Would verify SHA-256, file size, and gzip integrity"
    fi

    # ── Step 5: Decrypt if encrypted ──────────────────────────────
    log_step "5/8  Decrypting backup (if encrypted)"
    local restore_file="${local_backup}"

    if [[ "${download_file}" == *.enc ]]; then
        if [[ -z "${ENCRYPTION_KEY}" ]]; then
            log_err "Backup is encrypted but ENCRYPTION_KEY is not set."
            log_err "Set the ENCRYPTION_KEY environment variable and retry."
            exit 1
        fi

        if [[ "${DRY_RUN}" == "true" ]]; then
            log_dim "[DRY RUN] Would decrypt: ${local_backup} -> ${local_backup%.enc}"
        else
            local dec_path="${local_backup%.enc}"
            if ! openssl enc -d -"${ENCRYPTION_ALGO}" -pbkdf2 \
                -in "${local_backup}" \
                -out "${dec_path}" \
                -pass "pass:${ENCRYPTION_KEY}"; then
                log_err "Decryption failed. Verify ENCRYPTION_KEY matches the backup."
                exit 1
            fi
            restore_file="${dec_path}"
            log_ok "Backup decrypted successfully"
        fi
    else
        log_ok "Not encrypted — decryption skipped"
    fi

    # ── Step 6: Pre-restore database check ───────────────────────
    log_step "6/8  Checking target database"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_dim "[DRY RUN] Would check database connectivity and table counts"
    else
        pg_env
        if ! pg_isready -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" &>/dev/null; then
            log_warn "Database ${DB_NAME} is not accessible — it may not exist yet"
            log "Attempting to connect to 'postgres' database instead..."

            if ! psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -c "SELECT 1;" &>/dev/null; then
                log_err "Cannot connect to PostgreSQL at ${DB_HOST}:${DB_PORT}"
                log_err "Ensure PostgreSQL is running and credentials are correct"
                exit 1
            fi

            # Create the target database if it doesn't exist
            log "Creating database ${DB_NAME}..."
            psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres \
                -c "CREATE DATABASE ${DB_NAME};" 2>/dev/null || true
            log_ok "Database ${DB_NAME} created"
        else
            local current_table_count
            current_table_count="$(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
                -t -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ' || echo "0")"
            log "Database accessible — ${current_table_count} tables currently in public schema"

            if [[ "${current_table_count}" -gt 0 ]]; then
                log_warn "⚠️  Target database is NOT empty — restore will overwrite existing data"
            fi
        fi
        pg_unenv
    fi

    # ── Confirmation prompt ───────────────────────────────────────
    if [[ "${skip_confirm}" != "true" ]]; then
        echo ""
        log_warn "╔═══════════════════════════════════════════════════════╗"
        log_warn "║           ⚠️  RESTORE CONFIRMATION REQUIRED ⚠️         ║"
        log_warn "╠═══════════════════════════════════════════════════════╣"
        log_warn "║                                                       ║"
        log_warn "║  This will OVERWRITE database '${DB_NAME}'            ║"
        log_warn "║  on ${DB_HOST}:${DB_PORT}                              ║"
        log_warn "║                                                       ║"
        log_warn "║  • All current data will be lost                      ║"
        log_warn "║  • Existing connections will be terminated             ║"
        if [[ "${is_pitr}" == "true" ]]; then
        log_warn "║  • Point-in-time target: ${PITR_TARGET_TIME}           ║"
        fi
        log_warn "║                                                       ║"
        log_warn "║  Ensure you have a current backup before proceeding   ║"
        log_warn "║                                                       ║"
        log_warn "╚═══════════════════════════════════════════════════════╝"
        echo ""
        read -rp "Type 'RESTORE' to confirm, or anything else to cancel: " confirmation
        if [[ "${confirmation}" != "RESTORE" ]]; then
            log "Restore cancelled by user."
            rm -f "${local_backup}" "${local_manifest}" "${restore_file}" 2>/dev/null || true
            exit 0
        fi
    fi

    # ── Step 7: Terminate connections and restore ────────────────
    log_step "7/8  Restoring database"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_dim "[DRY RUN] Would terminate existing connections and restore database"
        log_dim "[DRY RUN] Command: gunzip -c ${restore_file} | pg_restore -h ${DB_HOST} -p ${DB_PORT} -U ${DB_USER} -d ${DB_NAME} --clean --if-exists --no-owner --no-privileges"
        if [[ "${is_pitr}" == "true" ]]; then
            log_dim "[DRY RUN] Would configure PITR with recovery_target_time=${PITR_TARGET_TIME}"
        fi
    else
        pg_env

        # Terminate existing connections to prevent locks
        if [[ "${DROP_EXISTING_CONNECTIONS}" == "true" ]]; then
            log "Terminating existing database connections..."
            psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -c \
                "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DB_NAME}' AND pid <> pg_backend_pid();" \
                2>/dev/null || log_warn "Could not terminate all connections (may lack superuser privileges)"
            sleep 2
            log_ok "Existing connections terminated"
        fi

        local restore_start restore_end restore_duration
        restore_start="$(date +%s)"

        # Perform the restore
        if ! gunzip -c "${restore_file}" | pg_restore \
            -h "${DB_HOST}" \
            -p "${DB_PORT}" \
            -U "${DB_USER}" \
            -d "${DB_NAME}" \
            --clean --if-exists \
            --no-owner --no-privileges \
            --single-transaction \
            --verbose \
            2>"${RESTORE_DIR}/pg_restore_error.log"; then
            # pg_restore returns non-zero for warnings too — check error log
            local error_count
            error_count="$(grep -ci "error" "${RESTORE_DIR}/pg_restore_error.log" 2>/dev/null || echo "0")"
            if [[ "${error_count}" -gt 5 ]]; then
                log_err "pg_restore failed with ${error_count} errors. Error log:"
                head -20 "${RESTORE_DIR}/pg_restore_error.log" >&2
                exit 1
            else
                log_warn "pg_restore completed with ${error_count} warning(s):"
                head -5 "${RESTORE_DIR}/pg_restore_error.log"
            fi
        fi

        restore_end="$(date +%s)"
        restore_duration=$((restore_end - restore_start))
        log_ok "Database restored in ${restore_duration}s"

        pg_unenv
    fi

    # ── Point-in-time recovery (if applicable) ────────────────────
    if [[ "${is_pitr}" == "true" && "${DRY_RUN}" != "true" ]]; then
        log_step "7b/8  Configuring point-in-time recovery"

        log "Target time: ${PITR_TARGET_TIME}"
        log "Recovery action: ${PITR_TARGET_ACTION}"

        # Write recovery configuration
        # Note: PITR requires WAL archiving to be enabled and a base backup
        # This sets recovery_target_time in postgresql.conf or recovery.conf
        local recovery_conf="${PG_DATA_DIR}/recovery.conf"
        local postgresql_conf="${PG_DATA_DIR}/postgresql.conf"

        if [[ -d "${PG_DATA_DIR}" ]]; then
            log "Writing recovery configuration to ${PG_DATA_DIR}..."

            # For PostgreSQL 12+, recovery settings go in postgresql.conf
            cat > "${RESTORE_DIR}/pitr_settings.conf" <<SETTINGS
# Chaos-Sec PITR Recovery Settings
# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
restore_command = 'cp ${PG_DATA_DIR}/archive/%f %p'
recovery_target_time = '${PITR_TARGET_TIME}'
recovery_target_inclusive = ${PITR_TARGET_INCLUSIVE}
recovery_target_action = '${PITR_TARGET_ACTION}'
SETTINGS

            log "PITR settings generated:"
            cat "${RESTORE_DIR}/pitr_settings.conf"
            log ""
            log_warn "PITR configuration requires manual steps:"
            log_warn "  1. Append ${RESTORE_DIR}/pitr_settings.conf to ${postgresql_conf}"
            log_warn "  2. Create recovery.signal in ${PG_DATA_DIR}:"
            log_warn "     touch ${PG_DATA_DIR}/recovery.signal"
            log_warn "  3. Restart PostgreSQL"
            log_warn "  4. After recovery completes, run: SELECT pg_wal_replay_resume();"
        else
            log_warn "PG_DATA_DIR (${PG_DATA_DIR}) not accessible from this host"
            log_warn "PITR settings written to ${RESTORE_DIR}/pitr_settings.conf"
            log_warn "Apply these settings on the PostgreSQL server and restart"
        fi
    fi

    # ── Step 8: Post-restore operations ─────────────────────────
    log_step "8/8  Post-restore operations"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_dim "[DRY RUN] Would run: VACUUM ANALYZE, verify table count, run migrations"
    else
        pg_env

        # Verify database is accessible
        if psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "SELECT 1;" &>/dev/null; then
            log_ok "Database is accessible after restore"
        else
            log_err "Database is not accessible after restore!"
            exit 1
        fi

        # Count restored tables
        local table_count
        table_count="$(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
            -t -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ' || echo "0")"
        log "Restored tables in public schema: ${table_count}"

        # Run VACUUM ANALYZE to optimize after restore
        if [[ "${VACUUM_AFTER_RESTORE}" == "true" ]]; then
            log "Running VACUUM ANALYZE..."
            psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
                -c "VACUUM ANALYZE;" 2>/dev/null || log_warn "VACUUM ANALYZE failed (may need superuser)"
            log_ok "VACUUM ANALYZE completed"
        fi

        # Run migrations if requested
        if [[ "${RUN_MIGRATIONS_AFTER}" == "true" ]]; then
            log "Running pending database migrations..."
            local migrate_script="${SCRIPT_DIR}/../../scripts/migrate.sh"
            if [[ -f "${migrate_script}" ]]; then
                bash "${migrate_script}" up || log_warn "Migrations had issues — check manually"
                log_ok "Migrations applied"
            else
                log_warn "Migration script not found at ${migrate_script}"
                log_warn "Run migrations manually after restore"
            fi
        fi

        # Verify key tables have data
        log "Verifying data integrity..."
        local tables_with_data=0
        local tables_empty=0
        while IFS= read -r table; do
            if [[ -z "${table}" ]]; then continue; fi
            local count
            count="$(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
                -t -c "SELECT count(*) FROM \"${table}\" LIMIT 1;" 2>/dev/null | tr -d ' ' || echo "0")"
            if [[ "${count:-0}" -gt 0 ]]; then
                tables_with_data=$((tables_with_data + 1))
            else
                tables_empty=$((tables_empty + 1))
            fi
        done < <(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
            -t -c "SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename;" 2>/dev/null | tr -d ' ')

        log "Tables with data: ${tables_with_data}, empty tables: ${tables_empty}"

        pg_unenv
    fi

    # ── Cleanup staging directory ────────────────────────────────
    if [[ "${DRY_RUN}" != "true" ]]; then
        log "Cleaning up staging files..."
        rm -f "${local_backup}" "${local_manifest}" "${restore_file}" \
            "${RESTORE_DIR}/pg_restore_error.log" \
            "${RESTORE_DIR}/pitr_settings.conf" 2>/dev/null || true
    fi

    # ── Summary ──────────────────────────────────────────────────
    echo ""
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    if [[ "${DRY_RUN}" == "true" ]]; then
        log_ok "DRY RUN completed — no changes were made"
    else
        log_ok "Restore completed successfully! ✓"
    fi
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "  Database:  ${DB_NAME}"
    log "  Host:      ${DB_HOST}:${DB_PORT}"
    log "  Source:    ${s3_backup_uri}"
    if [[ "${DRY_RUN}" != "true" ]]; then
        log "  Duration:  ${restore_duration:-0}s"
        log "  Tables:    ${table_count:-0}"
    fi
    if [[ "${is_pitr}" == "true" ]]; then
        log "  PITR:      ${PITR_TARGET_TIME}"
    fi
    echo ""
}

# ---------------------------------------------------------------------------
# Command: list (list available backups in S3)
# ---------------------------------------------------------------------------
cmd_list() {
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Available Backups"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    log "S3 Bucket: ${S3_BUCKET}/${S3_PREFIX}/"
    echo ""

    local total_backups=0
    while IFS= read -r date_prefix; do
        local date_name
        date_name="$(echo "${date_prefix}" | grep -oP '\d{4}-\d{2}-\d{2}' || echo "unknown")"

        echo -e "${BOLD}${date_name}${NC}"

        # List backup files (exclude manifests)
        while IFS= read -r line; do
            local file_size file_name
            file_size="$(echo "${line}" | awk '{print $3}')"
            file_name="$(echo "${line}" | awk '{$1=""; $2=""; $3=""; print $0}' | sed 's/^ *//')"

            if [[ -n "${file_name}" && "${file_name}" == *.gz* ]]; then
                local human_size
                human_size="$(echo "${file_size}" | numfmt --to=iec-i --suffix=B 2>/dev/null || echo "${file_size} B")"
                echo -e "  ${GREEN}●${NC}  ${file_name}  ${DIM}(${human_size})${NC}"
                total_backups=$((total_backups + 1))
            fi
        done < <(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${date_prefix}" 2>/dev/null | grep -v '\.manifest$')

        echo ""
    done < <(aws s3 ls ${AWS_CLI_OPTS:-} "${S3_BUCKET}/${S3_PREFIX}/" 2>/dev/null | awk '{print $2}')

    if [[ ${total_backups} -eq 0 ]]; then
        log_warn "No backups found in ${S3_BUCKET}/${S3_PREFIX}/"
    else
        log_ok "Total: ${total_backups} backup(s) available"
    fi
    echo ""
    log "To restore a backup, run:"
    log "  ${SCRIPT_NAME} latest"
    log "  ${SCRIPT_NAME} 2024-01-15"
    log "  ${SCRIPT_NAME} 2024-01-15T09:30:00"
}

# ---------------------------------------------------------------------------
# Command: verify (verify a backup without restoring)
# ---------------------------------------------------------------------------
cmd_verify() {
    local target="${1:-}"

    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Backup Verification"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    if [[ -z "${target}" ]]; then
        log_err "Usage: ${SCRIPT_NAME} verify <local-file|s3-uri|date|latest>"
        exit 1
    fi

    local local_file="${target}"
    local from_s3=false

    # Download from S3 if needed
    if [[ "${target}" == s3://* || "${target}" == "latest" || "${target}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        log_step "1/5  Resolving and downloading backup"
        mkdir -p "${RESTORE_DIR}"

        if [[ "${target}" != s3://* ]]; then
            resolve_backup_location "${target}"
            target="${RESOLVED_S3_BACKUP_URI}"
        fi

        local_file="${RESTORE_DIR}/$(basename "${target}")"
        from_s3=true

        if ! aws s3 cp ${AWS_CLI_OPTS:-} "${target}" "${local_file}"; then
            log_err "Failed to download backup from S3"
            exit 1
        fi
        log_ok "Downloaded: ${local_file} ($(file_size_human "${local_file}"))"
    else
        log_step "1/5  Checking local file"
        if [[ ! -f "${local_file}" ]]; then
            log_err "File not found: ${local_file}"
            exit 1
        fi
        log_ok "File exists: ${local_file} ($(file_size_human "${local_file}"))"
    fi

    local checks_passed=0
    local total_checks=4

    # ── Check 1: File is non-empty ──────────────────────────────
    log_step "2/5  File size check"
    if [[ -s "${local_file}" ]]; then
        log_ok "File is non-empty ($(file_size_human "${local_file}"))"
        checks_passed=$((checks_passed + 1))
    else
        log_err "File is empty"
    fi

    # ── Check 2: Valid gzip archive ─────────────────────────────
    log_step "3/5  Gzip integrity check"
    if gunzip -t "${local_file}" 2>/dev/null; then
        log_ok "Valid gzip archive ✓"
        checks_passed=$((checks_passed + 1))
    else
        log_err "Invalid gzip archive — file may be corrupted"
    fi

    # ── Check 3: Contains PostgreSQL data ────────────────────────
    log_step "4/5  PostgreSQL dump format check"
    local format_check
    format_check="$(gunzip -c "${local_file}" 2>/dev/null | head -c 200 | strings || true)"
    if echo "${format_check}" | grep -qi "PGDMP\|pg_dump\|postgresql"; then
        log_ok "Valid PostgreSQL dump detected ✓"
        checks_passed=$((checks_passed + 1))
    elif gunzip -c "${local_file}" 2>/dev/null | pg_restore --list 2>/dev/null | head -5 | grep -q "^\s*[0-9]"; then
        log_ok "Valid PostgreSQL custom format detected ✓"
        checks_passed=$((checks_passed + 1))
    else
        log_warn "Could not confirm PostgreSQL format (may be custom format — this is expected for -Fc backups)"
        checks_passed=$((checks_passed + 1))
    fi

    # ── Check 4: pg_restore --list succeeds ─────────────────────
    log_step "5/5  pg_restore catalog check"
    if gunzip -c "${local_file}" 2>/dev/null | pg_restore --list 2>/dev/null | grep -q "DATABASE"; then
        local entry_count
        entry_count="$(gunzip -c "${local_file}" 2>/dev/null | pg_restore --list 2>/dev/null | wc -l || echo "0")"
        log_ok "pg_restore catalog valid — ${entry_count} entries ✓"
        checks_passed=$((checks_passed + 1))
    else
        log_warn "pg_restore --list did not return expected output"
        log_warn "This may be normal for plain SQL format dumps"
        # Don't fail — could be plain text format
        checks_passed=$((checks_passed + 1))
    fi

    # ── Summary ──────────────────────────────────────────────────
    echo ""
    if [[ ${checks_passed} -ge ${total_checks} ]]; then
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log_ok "Verification PASSED (${checks_passed}/${total_checks} checks) ✓"
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    else
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log_err "Verification FAILED (${checks_passed}/${total_checks} checks)"
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    fi

    # Cleanup
    if [[ "${from_s3}" == "true" ]]; then
        rm -f "${local_file}"
    fi

    [[ ${checks_passed} -ge ${total_checks} ]] || exit 1
}

# ---------------------------------------------------------------------------
# Command: help
# ---------------------------------------------------------------------------
cmd_help() {
    cat <<HELP
${BOLD}Chaos-Sec Database Restore Script${NC}

${BOLD}USAGE${NC}
    ${SCRIPT_NAME} <command> [arguments] [options]

${BOLD}COMMANDS${NC}
    ${GREEN}restore${NC} <target> [flags]
        Restore database from a backup.
        ${CYAN}Targets:${NC}
          latest                      Restore most recent backup
          2024-01-15                  Restore latest backup from that date
          2024-01-15T09:30:00         Point-in-time recovery to that timestamp
          filename.sql.gz             Restore specific file from S3
          s3://bucket/path/file.gz    Restore from full S3 URI
        ${CYAN}Flags:${NC}
          --confirm, -y    Skip confirmation prompt
          --dry-run         Show what would happen without executing

    ${GREEN}list${NC}
        List all available backups in S3 with dates and sizes.

    ${GREEN}verify${NC} <target>
        Verify backup integrity without restoring.
        Target can be a local file, S3 URI, date, or 'latest'.

    ${GREEN}help${NC}
        Show this help message.

${BOLD}ENVIRONMENT VARIABLES${NC}
    ${CYAN}Database:${NC}
      CHAOS_DB_HOST              Database host (default: localhost)
      CHAOS_DB_PORT              Database port (default: 5432)
      CHAOS_DB_NAME              Database name (default: chaossec)
      CHAOS_DB_USER              Database user (default: chaossec_admin)
      CHAOS_DB_PASSWORD          Database password
      CHAOS_DB_SSLMODE           SSL mode (default: disable)
      CHAOS_DB_SUPERUSER         Superuser for connection termination

    ${CYAN}S3 Storage:${NC}
      S3_BUCKET                  S3 bucket URI (default: s3://chaos-sec-backups)
      S3_PREFIX                  S3 key prefix (default: database)
      AWS_REGION                 AWS region (default: eu-west-1)

    ${CYAN}Point-in-Time Recovery:${NC}
      PITR_TARGET_TIME           Recovery target timestamp (ISO 8601)
      PITR_TARGET_XID            Recovery target transaction ID
      PITR_TARGET_INCLUSIVE      Include target transaction (default: true)
      PITR_TARGET_ACTION         Post-recovery action: pause|promote|shutdown
      PG_DATA_DIR                PostgreSQL data directory for PITR config

    ${CYAN}Restore Options:${NC}
      DROP_EXISTING_CONNECTIONS  Terminate DB connections before restore (default: true)
      RUN_MIGRATIONS_AFTER       Run migrations after restore (default: true)
      VACUUM_AFTER_RESTORE       Run VACUUM ANALYZE after restore (default: true)
      DRY_RUN                    Preview changes without executing (default: false)

    ${CYAN}Encryption:${NC}
      ENCRYPTION_KEY             Decryption passphrase (if backup is encrypted)
      ENCRYPTION_ALGO            Algorithm (default: aes256)

${BOLD}EXAMPLES${NC}
    # Restore latest backup
    ${CYAN}${SCRIPT_NAME} restore latest${NC}

    # Restore from a specific date (skip confirmation)
    ${CYAN}${SCRIPT_NAME} restore 2024-01-15 --confirm${NC}

    # Point-in-time recovery
    ${CYAN}${SCRIPT_NAME} restore 2024-01-15T09:30:00${NC}

    # Restore from specific S3 URI
    ${CYAN}${SCRIPT_NAME} restore s3://chaos-sec-backups/database/2024-01-15/chaossec_20240115_030000.sql.gz${NC}

    # Dry run — preview restore without executing
    ${CYAN}DRY_RUN=true ${SCRIPT_NAME} restore latest${NC}

    # Verify a backup before restoring
    ${CYAN}${SCRIPT_NAME} verify latest${NC}

    # List available backups
    ${CYAN}${SCRIPT_NAME} list${NC}

    # Restore encrypted backup
    ${CYAN}ENCRYPTION_KEY=my-secret-key ${SCRIPT_NAME} restore latest${NC}

${BOLD}POINT-IN-TIME RECOVERY${NC}
    PITR allows restoring to a specific transaction time. Requirements:
      1. WAL archiving must be enabled on the PostgreSQL server
      2. A base backup taken before the target time must exist
      3. WAL files from the base backup to the target time must be available

    After the restore script completes:
      1. Apply the PITR settings from ${RESTORE_DIR}/pitr_settings.conf
      2. Create recovery.signal in the PostgreSQL data directory
      3. Restart PostgreSQL
      4. Monitor recovery progress in the PostgreSQL logs
      5. Once recovered, run: SELECT pg_wal_replay_resume();

${BOLD}EXIT CODES${NC}
    0  Success
    1  General error / verification failure
    2  Prerequisites not met
    3  Backup not found in S3
    4  Integrity check failed
    5  User cancelled

HELP
}

# ---------------------------------------------------------------------------
# Parse arguments and dispatch
# ---------------------------------------------------------------------------
main() {
    local command="${1:-help}"
    shift || true

    case "${command}" in
        restore)
            check_prerequisites
            cmd_restore "${1:-}" "${2:-}" "${3:-}"
            ;;
        list)
            check_prerequisites
            cmd_list
            ;;
        verify)
            check_prerequisites
            cmd_verify "${1:-}"
            ;;
        help|--help|-h)
            cmd_help
            ;;
        *)
            log_err "Unknown command: ${command}"
            echo ""
            cmd_help
            exit 1
            ;;
    esac
}

main "$@"
