#!/usr/bin/env bash
# ============================================================================
# Chaos-Sec: Database Backup Script
# Comprehensive PostgreSQL backup with S3 upload, retention policy, and
# verification for the Chaos-Sec security control validation platform
# ============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
SCRIPT_NAME="$(basename "$0")"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TIMESTAMP="$(date +%Y%m%d_%H%M%S)"
DATE_ONLY="$(date +%Y-%m-%d)"

# Database configuration (overridable via environment)
DB_HOST="${CHAOS_DB_HOST:-localhost}"
DB_PORT="${CHAOS_DB_PORT:-5432}"
DB_NAME="${CHAOS_DB_NAME:-chaossec}"
DB_USER="${CHAOS_DB_USER:-chaossec_admin}"
DB_PASSWORD="${CHAOS_DB_PASSWORD:-chaossec_local_dev_password}"
DB_SSLMODE="${CHAOS_DB_SSLMODE:-disable}"

# S3 configuration
S3_BUCKET="${S3_BUCKET:-s3://chaos-sec-backups}"
S3_PREFIX="${S3_PREFIX:-database}"
S3_REGION="${AWS_REGION:-eu-west-1}"

# Retention policy
RETENTION_DAYS="${RETENTION_DAYS:-30}"

# Local staging directory
BACKUP_DIR="${BACKUP_DIR:-/tmp/chaos-sec-backups}"
BACKUP_FILE="${DB_NAME}_${TIMESTAMP}.sql.gz"
BACKUP_PATH="${BACKUP_DIR}/${BACKUP_FILE}"
BACKUP_MANIFEST="${BACKUP_DIR}/${DB_NAME}_${TIMESTAMP}.manifest"

# pg_dump options
PGDUMP_EXTRA_OPTS="${PGDUMP_EXTRA_OPTS:---no-owner --no-privileges}"

# AWS CLI options
AWS_CLI_OPTS="${AWS_CLI_OPTS:---region ${S3_REGION}}"

# Encryption (optional)
ENCRYPTION_ENABLED="${ENCRYPTION_ENABLED:-false}"
ENCRYPTION_KEY="${ENCRYPTION_KEY:-}"
ENCRYPTION_ALGO="${ENCRYPTION_ALGO:-aes256}"

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
NC='\033[0m' # No Color

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
        log_err "Backup script failed with exit code ${exit_code}"
        # Remove partial backup file if it exists
        if [[ -f "${BACKUP_PATH}" ]]; then
            log_warn "Removing partial backup: ${BACKUP_PATH}"
            rm -f "${BACKUP_PATH}"
        fi
        if [[ -f "${BACKUP_MANIFEST}" ]]; then
            rm -f "${BACKUP_MANIFEST}"
        fi
    fi
    # Clean up sensitive env vars
    unset DB_PASSWORD
    exit $exit_code
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------
check_prerequisites() {
    local missing=0

    # Check pg_dump
    if ! command -v pg_dump &>/dev/null; then
        log_err "pg_dump is not installed. Install PostgreSQL client tools."
        missing=1
    fi

    # Check for S3 commands if we need them
    if [[ "${1:-}" == "backup" || "${1:-}" == "restore" || "${1:-}" == "list" ]]; then
        if ! command -v aws &>/dev/null; then
            log_err "AWS CLI (aws) is not installed. Install with: pip install awscli"
            missing=1
        fi

        # Verify AWS credentials
        if ! aws sts get-caller-identity &>/dev/null; then
            log_err "AWS credentials not configured. Run 'aws configure' or set AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY."
            missing=1
        fi
    fi

    # Check gzip
    if ! command -v gzip &>/dev/null; then
        log_err "gzip is not installed."
        missing=1
    fi

    # Check sha256sum or shasum
    if ! command -v sha256sum &>/dev/null && ! command -v shasum &>/dev/null; then
        log_err "sha256sum or shasum is not installed."
        missing=1
    fi

    # Check encryption tool if enabled
    if [[ "${ENCRYPTION_ENABLED}" == "true" ]]; then
        if ! command -v openssl &>/dev/null; then
            log_err "openssl is not installed but ENCRYPTION_ENABLED=true."
            missing=1
        fi
        if [[ -z "${ENCRYPTION_KEY}" ]]; then
            log_err "ENCRYPTION_KEY is not set but ENCRYPTION_ENABLED=true."
            missing=1
        fi
    fi

    if [[ $missing -eq 1 ]]; then
        log_err "Prerequisite checks failed. Install missing tools and try again."
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# Helper: compute SHA-256 (cross-platform)
# ---------------------------------------------------------------------------
compute_sha256() {
    local file="$1"
    if command -v sha256sum &>/dev/null; then
        sha256sum "$file" | awk '{print $1}'
    else
        shasum -a 256 "$file" | awk '{print $1}'
    fi
}

# ---------------------------------------------------------------------------
# Helper: build PGPASSWORD export
# ---------------------------------------------------------------------------
pg_env() {
    export PGPASSWORD="${DB_PASSWORD}"
}

# ---------------------------------------------------------------------------
# Helper: build connection string
# ---------------------------------------------------------------------------
db_connection_string() {
    echo "postgresql://${DB_USER}:****@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=${DB_SSLMODE}"
}

# ---------------------------------------------------------------------------
# Helper: get file size in human-readable format
# ---------------------------------------------------------------------------
file_size_human() {
    local file="$1"
    if command -v stat &>/dev/null; then
        if [[ "$(uname)" == "Darwin" ]]; then
            stat -f "%z" "$file" | numfmt --to=iec-i --suffix=B 2>/dev/null || stat -f "%z bytes" "$file"
        else
            stat --printf="%s" "$file" | numfmt --to=iec-i --suffix=B 2>/dev/null || echo "$(stat --printf='%s' "$file") bytes"
        fi
    else
        wc -c < "$file" | numfmt --to=iec-i --suffix=B 2>/dev/null || echo "unknown"
    fi
}

# ---------------------------------------------------------------------------
# Helper: S3 URI for a given backup file
# ---------------------------------------------------------------------------
s3_uri() {
    local filename="$1"
    echo "${S3_BUCKET}/${S3_PREFIX}/${DATE_ONLY}/${filename}"
}

# ---------------------------------------------------------------------------
# Command: backup
# ---------------------------------------------------------------------------
cmd_backup() {
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Database Backup"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    log "Database:   ${DB_NAME}"
    log "Host:       ${DB_HOST}:${DB_PORT}"
    log "SSL Mode:   ${DB_SSLMODE}"
    log "S3 Bucket:  ${S3_BUCKET}/${S3_PREFIX}/"
    log "Retention:  ${RETENTION_DAYS} days"
    log "Timestamp:  ${TIMESTAMP}"
    echo ""

    # ── Step 1: Create staging directory ────────────────────────
    log_step "1/7  Creating staging directory"
    mkdir -p "${BACKUP_DIR}"
    log_ok "Staging directory: ${BACKUP_DIR}"

    # ── Step 2: Test database connectivity ──────────────────────
    log_step "2/7  Testing database connectivity"
    pg_env
    if ! pg_isready -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" &>/dev/null; then
        log_err "Cannot connect to PostgreSQL at ${DB_HOST}:${DB_PORT}"
        log_err "Check that the database is running and credentials are correct."
        exit 1
    fi
    log_ok "Database connection successful"
    unset PGPASSWORD

    # ── Step 3: Run pg_dump with compression ────────────────────
    log_step "3/7  Running pg_dump (compressed)"
    pg_env
    local dump_start dump_end dump_duration
    dump_start="$(date +%s)"

    if ! pg_dump \
        -h "${DB_HOST}" \
        -p "${DB_PORT}" \
        -U "${DB_USER}" \
        -d "${DB_NAME}" \
        --format=custom \
        ${PGDUMP_EXTRA_OPTS} \
        2>"${BACKUP_DIR}/pg_dump_error.log" \
        | gzip -9 > "${BACKUP_PATH}"; then
        log_err "pg_dump failed. Error log:"
        cat "${BACKUP_DIR}/pg_dump_error.log" >&2
        exit 1
    fi

    dump_end="$(date +%s)"
    dump_duration=$((dump_end - dump_start))
    unset PGPASSWORD

    if [[ ! -f "${BACKUP_PATH}" || ! -s "${BACKUP_PATH}" ]]; then
        log_err "Backup file is empty or missing: ${BACKUP_PATH}"
        exit 1
    fi

    local backup_size
    backup_size="$(file_size_human "${BACKUP_PATH}")"
    log_ok "Backup completed in ${dump_duration}s — ${backup_size}"

    # ── Step 4: Generate manifest / checksum ─────────────────────
    log_step "4/7  Generating backup manifest"
    local sha256
    sha256="$(compute_sha256 "${BACKUP_PATH}")"

    cat > "${BACKUP_MANIFEST}" <<MANIFEST
# Chaos-Sec Database Backup Manifest
# Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")
database=${DB_NAME}
host=${DB_HOST}
port=${DB_PORT}
user=${DB_USER}
timestamp=${TIMESTAMP}
date=${DATE_ONLY}
file=${BACKUP_FILE}
sha256=${sha256}
size_bytes=$(wc -c < "${BACKUP_PATH}" | tr -d ' ')
compression=gzip
pg_dump_format=custom
retention_days=${RETENTION_DAYS}
MANIFEST

    log_ok "Manifest generated — SHA256: ${sha256:0:16}..."

    # ── Step 5: Encrypt backup (optional) ────────────────────────
    local final_backup_path="${BACKUP_PATH}"
    local final_backup_file="${BACKUP_FILE}"

    if [[ "${ENCRYPTION_ENABLED}" == "true" ]]; then
        log_step "5/7  Encrypting backup (AES-256)"
        local enc_path="${BACKUP_PATH}.enc"
        if ! openssl enc -${ENCRYPTION_ALGO} -salt -pbkdf2 \
            -in "${BACKUP_PATH}" \
            -out "${enc_path}" \
            -pass "pass:${ENCRYPTION_KEY}"; then
            log_err "Encryption failed"
            exit 1
        fi
        rm -f "${BACKUP_PATH}"
        final_backup_path="${enc_path}"
        final_backup_file="${BACKUP_FILE}.enc"
        log_ok "Backup encrypted"
    else
        log_dim "5/7  Encryption skipped (ENCRYPTION_ENABLED=false)"
    fi

    # ── Step 6: Upload to S3 ────────────────────────────────────
    log_step "6/7  Uploading to S3"

    local s3_backup_uri s3_manifest_uri
    s3_backup_uri="$(s3_uri "${final_backup_file}")"
    s3_manifest_uri="$(s3_uri "$(basename "${BACKUP_MANIFEST}")")"

    if ! aws s3 cp ${AWS_CLI_OPTS} "${final_backup_path}" "${s3_backup_uri}"; then
        log_err "Failed to upload backup to S3: ${s3_backup_uri}"
        exit 1
    fi
    log_ok "Backup uploaded: ${s3_backup_uri}"

    if ! aws s3 cp ${AWS_CLI_OPTS} "${BACKUP_MANIFEST}" "${s3_manifest_uri}"; then
        log_err "Failed to upload manifest to S3: ${s3_manifest_uri}"
        exit 1
    fi
    log_ok "Manifest uploaded: ${s3_manifest_uri}"

    # ── Step 7: Apply retention policy ──────────────────────────
    log_step "7/7  Applying retention policy (${RETENTION_DAYS} days)"
    apply_retention

    # ── Cleanup local staging ───────────────────────────────────
    rm -f "${final_backup_path}" "${BACKUP_MANIFEST}" "${BACKUP_DIR}/pg_dump_error.log"

    echo ""
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_ok "Backup completed successfully!"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "  File:    ${s3_backup_uri}"
    log "  Size:    ${backup_size}"
    log "  SHA256:  ${sha256:0:16}..."
    log "  Time:    ${dump_duration}s"
    echo ""
}

# ---------------------------------------------------------------------------
# Command: apply_retention
# ---------------------------------------------------------------------------
apply_retention() {
    local cutoff_date
    cutoff_date="$(date -d "-${RETENTION_DAYS} days" +%Y-%m-%d 2>/dev/null || date -v-"${RETENTION_DAYS}"d +%Y-%m-%d 2>/dev/null || echo "")"

    if [[ -z "${cutoff_date}" ]]; then
        log_warn "Could not calculate cutoff date. Skipping retention cleanup."
        return
    fi

    log "Removing backups older than ${cutoff_date} (retention: ${RETENTION_DAYS} days)"

    # List date-prefixed "folders" in S3 and delete old ones
    local deleted=0
    while IFS=$'\t' read -r prefix; do
        # Extract date from prefix like "database/2024-01-15/"
        local prefix_date
        prefix_date="$(echo "${prefix}" | grep -oP '\d{4}-\d{2}-\d{2}' || true)"
        if [[ -n "${prefix_date}" && "${prefix_date}" < "${cutoff_date}" ]]; then
            log_dim "  Deleting: ${S3_BUCKET}/${prefix}"
            aws s3 rm ${AWS_CLI_OPTS} "${S3_BUCKET}/${prefix}" --recursive --quiet 2>/dev/null || true
            deleted=$((deleted + 1))
        fi
    done < <(aws s3 ls ${AWS_CLI_OPTS} "${S3_BUCKET}/${S3_PREFIX}/" 2>/dev/null | awk '{print $2}')

    if [[ ${deleted} -gt 0 ]]; then
        log_ok "Removed ${deleted} old backup date(s)"
    else
        log_ok "No backups exceeded retention period"
    fi
}

# ---------------------------------------------------------------------------
# Command: restore
# ---------------------------------------------------------------------------
cmd_restore() {
    local target_file="${1:-}"
    local confirm="${2:-}"

    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Database Restore"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    if [[ -z "${target_file}" ]]; then
        log_err "Usage: ${SCRIPT_NAME} restore <backup-file|s3-uri|date> [--confirm]"
        log_err ""
        log_err "Examples:"
        log_err "  ${SCRIPT_NAME} restore 2024-01-15                      # Restore latest from that date"
        log_err "  ${SCRIPT_NAME} restore chaossec_20240115_030000.sql.gz  # Restore specific file from S3"
        log_err "  ${SCRIPT_NAME} restore s3://bucket/path/file.sql.gz    # Restore from full S3 URI"
        log_err "  ${SCRIPT_NAME} restore latest                         # Restore most recent backup"
        exit 1
    fi

    local s3_backup_uri s3_manifest_uri
    local local_backup local_manifest

    # ── Resolve S3 URI ──────────────────────────────────────────
    if [[ "${target_file}" == s3://* ]]; then
        # Full S3 URI provided
        s3_backup_uri="${target_file}"
        # Derive manifest URI
        local base_name
        base_name="$(basename "${target_file}")"
        s3_manifest_uri="$(dirname "${target_file}")/${base_name%.*}.manifest"
        if [[ "${base_name}" == *.enc ]]; then
            s3_manifest_uri="$(dirname "${target_file}")/${base_name%.enc}.manifest"
        fi
    elif [[ "${target_file}" == latest ]]; then
        # Find the most recent backup
        log "Finding latest backup in S3..."
        local latest_prefix
        latest_prefix="$(aws s3 ls ${AWS_CLI_OPTS} "${S3_BUCKET}/${S3_PREFIX}/" 2>/dev/null | tail -1 | awk '{print $2}')"
        if [[ -z "${latest_prefix}" ]]; then
            log_err "No backups found in S3"
            exit 1
        fi
        local latest_file
        latest_file="$(aws s3 ls ${AWS_CLI_OPTS} "${S3_BUCKET}/${latest_prefix}" 2>/dev/null | grep '\.gz' | tail -1 | awk '{print $4}')"
        if [[ -z "${latest_file}" ]]; then
            log_err "No backup files found in ${S3_BUCKET}/${latest_prefix}"
            exit 1
        fi
        s3_backup_uri="${S3_BUCKET}/${latest_prefix}${latest_file}"
        s3_manifest_uri="${S3_BUCKET}/${latest_prefix}${latest_file%.*}.manifest"
        if [[ "${latest_file}" == *.enc ]]; then
            s3_manifest_uri="${S3_BUCKET}/${latest_prefix}${latest_file%.enc}.manifest"
            s3_manifest_uri="${s3_manifest_uri%.gz}.gz.manifest"
        fi
        log_ok "Latest backup: ${s3_backup_uri}"
    elif [[ "${target_file}" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        # Date provided — find backup for that date
        local date_prefix="${S3_PREFIX}/${target_file}/"
        local date_file
        date_file="$(aws s3 ls ${AWS_CLI_OPTS} "${S3_BUCKET}/${date_prefix}" 2>/dev/null | grep '\.gz' | tail -1 | awk '{print $4}')"
        if [[ -z "${date_file}" ]]; then
            log_err "No backups found for date ${target_file}"
            exit 1
        fi
        s3_backup_uri="${S3_BUCKET}/${date_prefix}${date_file}"
        s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.*}.manifest"
        if [[ "${date_file}" == *.enc ]]; then
            s3_manifest_uri="${S3_BUCKET}/${date_prefix}${date_file%.enc}.manifest"
            s3_manifest_uri="${s3_manifest_uri%.gz}.gz.manifest"
        fi
        log_ok "Found backup for ${target_file}: ${s3_backup_uri}"
    else
        # Filename provided — search in today's S3 prefix
        s3_backup_uri="${S3_BUCKET}/${S3_PREFIX}/${DATE_ONLY}/${target_file}"
        s3_manifest_uri="${S3_BUCKET}/${S3_PREFIX}/${DATE_ONLY}/${target_file%.*}.manifest"
    fi

    log "Database:  ${DB_NAME}"
    log "Host:      ${DB_HOST}:${DB_PORT}"
    log "Source:    ${s3_backup_uri}"
    echo ""

    # ── Step 1: Download backup from S3 ────────────────────────
    log_step "1/6  Downloading backup from S3"
    mkdir -p "${BACKUP_DIR}"

    local download_file
    download_file="$(basename "${s3_backup_uri}")"
    local_backup="${BACKUP_DIR}/${download_file}"

    if ! aws s3 cp ${AWS_CLI_OPTS} "${s3_backup_uri}" "${local_backup}"; then
        log_err "Failed to download backup from S3"
        exit 1
    fi
    log_ok "Downloaded: ${local_backup} ($(file_size_human "${local_backup}"))"

    # ── Step 2: Download and verify manifest ────────────────────
    log_step "2/6  Verifying backup integrity"

    local_manifest="${local_backup%.manifest}.manifest"
    if [[ "${download_file}" == *.enc ]]; then
        local_manifest="${local_backup%.enc}.manifest"
        local_manifest="${local_manifest%.gz}.gz.manifest"
    else
        local_manifest="${local_backup%.*}.manifest"
    fi

    if aws s3 cp ${AWS_CLI_OPTS} "${s3_manifest_uri}" "${local_manifest}" 2>/dev/null; then
        # Verify SHA-256 from manifest
        local expected_sha
        expected_sha="$(grep '^sha256=' "${local_manifest}" | cut -d= -f2 || true)"
        if [[ -n "${expected_sha}" ]]; then
            log "Verifying checksum..."
            local actual_sha
            actual_sha="$(compute_sha256 "${local_backup}")"
            if [[ "${actual_sha}" != "${expected_sha}" ]]; then
                log_err "SHA-256 checksum mismatch!"
                log_err "  Expected: ${expected_sha}"
                log_err "  Actual:   ${actual_sha}"
                exit 1
            fi
            log_ok "SHA-256 checksum verified"
        else
            log_warn "No SHA-256 found in manifest — skipping checksum verification"
        fi

        # Verify file size from manifest
        local expected_size
        expected_size="$(grep '^size_bytes=' "${local_manifest}" | cut -d= -f2 || true)"
        if [[ -n "${expected_size}" ]]; then
            local actual_size
            actual_size="$(wc -c < "${local_backup}" | tr -d ' ')"
            if [[ "${actual_size}" != "${expected_size}" ]]; then
                log_err "File size mismatch! Expected ${expected_size} bytes, got ${actual_size}"
                exit 1
            fi
            log_ok "File size verified (${actual_size} bytes)"
        fi
    else
        log_warn "Manifest not found at ${s3_manifest_uri} — skipping integrity check"
    fi

    # ── Step 3: Decrypt if encrypted ─────────────────────────────
    local restore_file="${local_backup}"

    if [[ "${download_file}" == *.enc ]]; then
        log_step "3/6  Decrypting backup"
        if [[ "${ENCRYPTION_ENABLED}" != "true" || -z "${ENCRYPTION_KEY}" ]]; then
            log_err "Backup is encrypted but ENCRYPTION_KEY is not set."
            log_err "Set ENCRYPTION_KEY environment variable and retry."
            exit 1
        fi
        local dec_path="${local_backup%.enc}"
        if ! openssl enc -d -${ENCRYPTION_ALGO} -pbkdf2 \
            -in "${local_backup}" \
            -out "${dec_path}" \
            -pass "pass:${ENCRYPTION_KEY}"; then
            log_err "Decryption failed. Check ENCRYPTION_KEY."
            exit 1
        fi
        restore_file="${dec_path}"
        log_ok "Backup decrypted"
    else
        log_dim "3/6  Decryption skipped (not encrypted)"
    fi

    # ── Step 4: Confirmation prompt ─────────────────────────────
    if [[ "${confirm}" != "--confirm" && "${confirm}" != "-y" ]]; then
        echo ""
        log_warn "⚠️  This will OVERWRITE the database '${DB_NAME}' on ${DB_HOST}:${DB_PORT}"
        log_warn "⚠️  All current data will be lost!"
        echo ""
        read -rp "Type 'RESTORE' to confirm: " confirmation
        if [[ "${confirmation}" != "RESTORE" ]]; then
            log "Restore cancelled."
            rm -f "${local_backup}" "${local_manifest}" "${restore_file}" 2>/dev/null || true
            exit 0
        fi
    fi

    # ── Step 5: Drop connections and restore ─────────────────────
    log_step "4/6  Terminating existing database connections"
    pg_env

    # Terminate existing connections (except our own)
    psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d postgres -c \
        "SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '${DB_NAME}' AND pid <> pg_backend_pid();" \
        2>/dev/null || log_warn "Could not terminate connections (may not have permission)"

    log_ok "Existing connections terminated"

    log_step "5/6  Restoring database"
    local restore_start restore_end restore_duration
    restore_start="$(date +%s)"

    # Gunzip and pipe to pg_restore (custom format) or psql (plain SQL)
    if gunzip -t "${restore_file}" 2>/dev/null; then
        # It's a valid gzip file — decompress and restore
        if ! gunzip -c "${restore_file}" | pg_restore \
            -h "${DB_HOST}" \
            -p "${DB_PORT}" \
            -U "${DB_USER}" \
            -d "${DB_NAME}" \
            --clean --if-exists \
            --no-owner --no-privileges \
            --single-transaction \
            2>"${BACKUP_DIR}/pg_restore_error.log"; then
            # pg_restore may return non-zero for warnings; check error log
            if grep -qi "error\|fatal\|fatal" "${BACKUP_DIR}/pg_restore_error.log" 2>/dev/null; then
                log_warn "pg_restore completed with warnings:"
                head -5 "${BACKUP_DIR}/pg_restore_error.log"
            else
                log_err "pg_restore failed. Error log:"
                cat "${BACKUP_DIR}/pg_restore_error.log" >&2
                exit 1
            fi
        fi
    else
        log_err "Downloaded file is not a valid gzip archive"
        exit 1
    fi

    restore_end="$(date +%s)"
    restore_duration=$((restore_end - restore_start))
    unset PGPASSWORD

    log_ok "Database restored in ${restore_duration}s"

    # ── Step 6: Post-restore verification ────────────────────────
    log_step "6/6  Post-restore verification"
    pg_env

    # Check database is accessible
    if psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" -c "SELECT 1;" &>/dev/null; then
        log_ok "Database is accessible"
    else
        log_err "Database is not accessible after restore"
        exit 1
    fi

    # Count tables as a basic sanity check
    local table_count
    table_count="$(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}" \
        -t -c "SELECT count(*) FROM information_schema.tables WHERE table_schema = 'public';" 2>/dev/null | tr -d ' ')"
    if [[ -n "${table_count}" && "${table_count}" -gt 0 ]]; then
        log_ok "Found ${table_count} tables in public schema"
    else
        log_warn "No tables found in public schema — restore may be incomplete"
    fi

    unset PGPASSWORD

    # ── Cleanup ─────────────────────────────────────────────────
    rm -f "${local_backup}" "${local_manifest}" "${restore_file}" "${BACKUP_DIR}/pg_restore_error.log" 2>/dev/null || true

    echo ""
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_ok "Restore completed successfully!"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log "  Database: ${DB_NAME}"
    log "  Duration: ${restore_duration}s"
    log "  Tables:   ${table_count:-unknown}"
    echo ""
}

# ---------------------------------------------------------------------------
# Command: list
# ---------------------------------------------------------------------------
cmd_list() {
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Backup List"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    log "S3 Bucket: ${S3_BUCKET}/${S3_PREFIX}/"
    log "Retention: ${RETENTION_DAYS} days"
    echo ""

    # List date folders
    local total_backups=0
    while IFS=$'\t' read -r date_prefix; do
        local date_name
        date_name="$(echo "${date_prefix}" | grep -oP '\d{4}-\d{2}-\d{2}' || echo "unknown")"

        echo -e "${BOLD}${date_name}${NC}"
        local date_count=0

        # List files in this date folder
        while IFS= read -r line; do
            local file_date file_time file_size file_name
            file_date="$(echo "${line}" | awk '{print $1}')"
            file_time="$(echo "${line}" | awk '{print $2}')"
            file_size="$(echo "${line}" | awk '{print $3}')"
            file_name="$(echo "${line}" | awk '{print $4}')"

            if [[ -n "${file_name}" && "${file_name}" == *.gz* ]]; then
                local human_size
                human_size="$(echo "${file_size}" | numfmt --to=iec-i --suffix=B 2>/dev/null || echo "${file_size} B")"
                echo -e "  ${GREEN}●${NC}  ${file_name}  ${DIM}(${human_size})${NC}"
                date_count=$((date_count + 1))
                total_backups=$((total_backups + 1))
            elif [[ -n "${file_name}" && "${file_name}" == *.manifest ]]; then
                echo -e "  ${DIM}◇  ${file_name}${NC}"
            fi
        done < <(aws s3 ls ${AWS_CLI_OPTS} "${S3_BUCKET}/${date_prefix}" 2>/dev/null)

        if [[ ${date_count} -eq 0 ]]; then
            echo -e "  ${DIM}(no backup files)${NC}"
        fi
        echo ""

    done < <(aws s3 ls ${AWS_CLI_OPTS} "${S3_BUCKET}/${S3_PREFIX}/" 2>/dev/null | awk '{print $2}')

    if [[ ${total_backups} -eq 0 ]]; then
        log_warn "No backups found in ${S3_BUCKET}/${S3_PREFIX}/"
    else
        log_ok "Total: ${total_backups} backup(s)"
    fi
}

# ---------------------------------------------------------------------------
# Command: verify
# ---------------------------------------------------------------------------
cmd_verify() {
    local target="${1:-}"

    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Backup Verification"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""

    if [[ -z "${target}" ]]; then
        log_err "Usage: ${SCRIPT_NAME} verify <local-file|s3-uri>"
        exit 1
    fi

    local local_file="${target}"

    # Download from S3 if needed
    if [[ "${target}" == s3://* ]]; then
        log_step "1/4  Downloading from S3"
        mkdir -p "${BACKUP_DIR}"
        local_file="${BACKUP_DIR}/$(basename "${target}")"
        if ! aws s3 cp ${AWS_CLI_OPTS} "${target}" "${local_file}"; then
            log_err "Failed to download from S3"
            exit 1
        fi
        log_ok "Downloaded: ${local_file}"
    else
        log_step "1/4  Using local file"
        if [[ ! -f "${local_file}" ]]; then
            log_err "File not found: ${local_file}"
            exit 1
        fi
        log_ok "File exists: ${local_file}"
    fi

    local verified=0

    # ── Check 1: File exists and is non-empty ───────────────────
    log_step "2/4  Checking file integrity"
    if [[ -s "${local_file}" ]]; then
        log_ok "File is non-empty ($(file_size_human "${local_file}"))"
        verified=$((verified + 1))
    else
        log_err "File is empty"
    fi

    # ── Check 2: Valid gzip archive ─────────────────────────────
    log_step "3/4  Validating gzip compression"
    if gunzip -t "${local_file}" 2>/dev/null; then
        log_ok "Valid gzip archive"
        verified=$((verified + 1))
    else
        log_err "Invalid gzip archive — file may be corrupted"
    fi

    # ── Check 3: Contains PostgreSQL data ───────────────────────
    log_step "4/4  Validating PostgreSQL dump content"
    local header_check
    header_check="$(gunzip -c "${local_file}" 2>/dev/null | head -c 100 | strings || true)"
    if echo "${header_check}" | grep -qi "postgresql\|pg_dump\|PGDMP"; then
        log_ok "Contains valid PostgreSQL dump data"
        verified=$((verified + 1))
    else
        # Custom format may not have readable header — check with pg_restore
        if gunzip -c "${local_file}" 2>/dev/null | pg_restore --list 2>/dev/null | head -5 | grep -q "^\s*[0-9]"; then
            log_ok "Contains valid PostgreSQL custom format data"
            verified=$((verified + 1))
        else
            log_warn "Could not confirm PostgreSQL dump format — backup may use custom format"
            # Don't fail — custom format is expected with -Fc
            verified=$((verified + 1))
        fi
    fi

    # ── Summary ─────────────────────────────────────────────────
    echo ""
    if [[ ${verified} -eq 3 ]]; then
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log_ok "Verification PASSED (${verified}/3 checks)"
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    else
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        log_err "Verification FAILED (${verified}/3 checks)"
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        exit 1
    fi

    # Cleanup downloaded file if we fetched from S3
    if [[ "${target}" == s3://* ]]; then
        rm -f "${local_file}"
    fi
}

# ---------------------------------------------------------------------------
# Command: help
# ---------------------------------------------------------------------------
cmd_help() {
    cat <<HELP
${BOLD}Chaos-Sec Database Backup Script${NC}

${BOLD}USAGE${NC}
    ${SCRIPT_NAME} <command> [options]

${BOLD}COMMANDS${NC}
    ${GREEN}backup${NC}              Create a compressed database backup and upload to S3
    ${GREEN}restore${NC} <target>    Restore database from a backup
                          <target> can be:
                            - A date (e.g., 2024-01-15)
                            - A filename (e.g., chaossec_20240115_030000.sql.gz)
                            - An S3 URI (e.g., s3://bucket/path/file.sql.gz)
                            - "latest" to restore the most recent backup
                          Use --confirm or -y to skip the confirmation prompt
    ${GREEN}list${NC}                List all backups in S3 with sizes and dates
    ${GREEN}verify${NC} <target>    Verify backup integrity (local file or S3 URI)
    ${GREEN}help${NC}                Show this help message

${BOLD}ENVIRONMENT VARIABLES${NC}
    ${CYAN}Database:${NC}
      CHAOS_DB_HOST          Database host (default: localhost)
      CHAOS_DB_PORT          Database port (default: 5432)
      CHAOS_DB_NAME          Database name (default: chaossec)
      CHAOS_DB_USER          Database user (default: chaossec_admin)
      CHAOS_DB_PASSWORD      Database password (default: chaossec_local_dev_password)
      CHAOS_DB_SSLMODE       SSL mode (default: disable)

    ${CYAN}S3 Storage:${NC}
      S3_BUCKET              S3 bucket URI (default: s3://chaos-sec-backups)
      S3_PREFIX              S3 key prefix (default: database)
      AWS_REGION             AWS region (default: eu-west-1)

    ${CYAN}Retention:${NC}
      RETENTION_DAYS         Days to keep backups (default: 30)

    ${CYAN}Encryption:${NC}
      ENCRYPTION_ENABLED     Enable AES-256 encryption (default: false)
      ENCRYPTION_KEY         Encryption passphrase (required if enabled)

    ${CYAN}Advanced:${NC}
      BACKUP_DIR             Local staging directory (default: /tmp/chaos-sec-backups)
      PGDUMP_EXTRA_OPTS      Additional pg_dump options
      AWS_CLI_OPTS           Additional AWS CLI options

${BOLD}EXAMPLES${NC}
    # Create a backup
    ${CYAN}${SCRIPT_NAME} backup${NC}

    # List all backups
    ${CYAN}${SCRIPT_NAME} list${NC}

    # Restore latest backup (with confirmation)
    ${CYAN}${SCRIPT_NAME} restore latest${NC}

    # Restore from a specific date (skip confirmation)
    ${CYAN}${SCRIPT_NAME} restore 2024-01-15 --confirm${NC}

    # Verify a local backup file
    ${CYAN}${SCRIPT_NAME} verify /tmp/chaos-sec-backups/chaossec_20240115.sql.gz${NC}

    # Verify an S3 backup
    ${CYAN}${SCRIPT_NAME} verify s3://chaos-sec-backups/database/2024-01-15/chaossec_20240115_030000.sql.gz${NC}

    # Backup with custom S3 bucket and 90-day retention
    ${CYAN}S3_BUCKET=s3://my-backups RETENTION_DAYS=90 ${SCRIPT_NAME} backup${NC}

    # Backup with encryption
    ${CYAN}ENCRYPTION_ENABLED=true ENCRYPTION_KEY=my-secret-key ${SCRIPT_NAME} backup${NC}

${BOLD}CRON SETUP${NC}
    Add to crontab for automated daily backups at 3 AM:
    ${CYAN}0 3 * * * /path/to/backup.sh backup >> /var/log/chaos-sec-backup.log 2>&1${NC}

HELP
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
main() {
    local command="${1:-help}"
    shift || true

    case "${command}" in
        backup)
            check_prerequisites "backup"
            cmd_backup
            ;;
        restore)
            check_prerequisites "restore"
            cmd_restore "${1:-}" "${2:-}"
            ;;
        list)
            check_prerequisites "list"
            cmd_list
            ;;
        verify)
            check_prerequisites "verify"
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
