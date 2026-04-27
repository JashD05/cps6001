#!/usr/bin/env bash
# ============================================================================
# Chaos-Sec: Database Optimization Script
# Description: Connects to PostgreSQL, runs diagnostics, and produces a
#              formatted optimization report covering ANALYZE, unused indexes,
#              VACUUM candidates, slow queries, and table sizes.
# Usage:       ./db-optimize.sh [--analyze] [--report] [--full-vacuum]
#              Default (no flags): runs ANALYZE and prints a full report.
# ============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration — override via environment variables or .env file
# ---------------------------------------------------------------------------
DB_HOST="${POSTGRES_HOST:-localhost}"
DB_PORT="${POSTGRES_PORT:-5432}"
DB_NAME="${POSTGRES_DB:-chaos_sec}"
DB_USER="${POSTGRES_USER:-chaos_sec}"
PGPASSWORD="${POSTGRES_PASSWORD:-}"  # exported below if set

# Slowness threshold for pg_stat_statements (milliseconds)
SLOW_QUERY_THRESHOLD_MS="${SLOW_QUERY_THRESHOLD_MS:-500}"

# Number of slow queries to display
SLOW_QUERY_LIMIT="${SLOW_QUERY_LIMIT:-10}"

# ---------------------------------------------------------------------------
# Colors & formatting helpers
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m' # No Color

header() {
    echo ""
    echo -e "${CYAN}${BOLD}========================================================================${NC}"
    echo -e "${CYAN}${BOLD}  $1${NC}"
    echo -e "${CYAN}${BOLD}========================================================================${NC}"
    echo ""
}

subheader() {
    echo -e "${BOLD}--- $1 ---${NC}"
    echo ""
}

info()  { echo -e "${GREEN}[INFO]${NC}  $1"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# ---------------------------------------------------------------------------
# Argument parsing
# ---------------------------------------------------------------------------
RUN_ANALYZE=true
RUN_REPORT=true
RUN_FULL_VACUUM=false

while [[ $# -gt 0 ]]; do
    case "$1" in
        --analyze)      RUN_ANALYZE=true;  RUN_REPORT=false; shift ;;
        --report)       RUN_ANALYZE=false; RUN_REPORT=true;  shift ;;
        --full-vacuum)  RUN_FULL_VACUUM=true; shift ;;
        --help|-h)
            echo "Usage: $0 [--analyze] [--report] [--full-vacuum]"
            echo ""
            echo "  --analyze       Run ANALYZE on all tables only (no report)"
            echo "  --report        Print optimization report only (no ANALYZE)"
            echo "  --full-vacuum   Run VACUUM FULL on tables that need it (destructive — locks tables)"
            echo ""
            echo "  Default (no flags): runs ANALYZE and prints the full report."
            echo ""
            echo "Environment variables:"
            echo "  POSTGRES_HOST          Database host (default: localhost)"
            echo "  POSTGRES_PORT          Database port (default: 5432)"
            echo "  POSTGRES_DB            Database name (default: chaos_sec)"
            echo "  POSTGRES_USER          Database user (default: chaos_sec)"
            echo "  POSTGRES_PASSWORD       Database password"
            echo "  SLOW_QUERY_THRESHOLD_MS Slow query threshold in ms (default: 500)"
            echo "  SLOW_QUERY_LIMIT        Number of slow queries to show (default: 10)"
            exit 0
            ;;
        *)
            error "Unknown argument: $1"
            exit 1
            ;;
    esac
done

# ---------------------------------------------------------------------------
# PostgreSQL connection helper
# ---------------------------------------------------------------------------
if [[ -n "${PGPASSWORD}" ]]; then
    export PGPASSWORD
fi

PSQL_CMD=(psql -h "${DB_HOST}" -p "${DB_PORT}" -U "${DB_USER}" -d "${DB_NAME}")

# Verify connectivity
if ! "${PSQL_CMD[@]}" -c "SELECT 1" > /dev/null 2>&1; then
    error "Cannot connect to PostgreSQL at ${DB_HOST}:${DB_PORT}/${DB_NAME} as ${DB_USER}"
    error "Ensure the database is running and credentials are correct."
    exit 1
fi

# Timestamp for the report
REPORT_TIMESTAMP=$(date -u '+%Y-%m-%d %H:%M:%S UTC')

# Temporary file to capture report output
REPORT_FILE=$(mktemp /tmp/chaos-sec-db-optimize.XXXXXX)
trap 'rm -f "${REPORT_FILE}"' EXIT

# ---------------------------------------------------------------------------
# Function: run_analyze
# Runs ANALYZE on every table to update planner statistics
# ---------------------------------------------------------------------------
run_analyze() {
    header "ANALYZE — Updating Planner Statistics"

    # Get list of tables in the public schema
    local tables
    tables=$("${PSQL_CMD[@]}" -t -A -c "
        SELECT tablename
        FROM pg_tables
        WHERE schemaname = 'public'
        ORDER BY tablename;
    ")

    if [[ -z "${tables}" ]]; then
        warn "No tables found in public schema."
        return
    fi

    local count=0
    local failed=0

    while IFS= read -r table; do
        [[ -z "${table}" ]] && continue
        if "${PSQL_CMD[@]}" -c "ANALYZE ${table};" > /dev/null 2>&1; then
            info "ANALYZE ${table} — OK"
            ((count++))
        else
            warn "ANALYZE ${table} — FAILED"
            ((failed++))
        fi
    done <<< "${tables}"

    echo ""
    info "Analyzed ${count} table(s), ${failed} failure(s)."
}

# ---------------------------------------------------------------------------
# Function: list_unused_indexes
# Finds indexes that have never been used (or very rarely) since last stats reset
# ---------------------------------------------------------------------------
list_unused_indexes() {
    subheader "Unused / Rarely-Used Indexes"

    "${PSQL_CMD[@]}" -c "
        SELECT
            schemaname  AS schema,
            relname      AS table,
            indexrelname AS index,
            idx_scan     AS index_scans,
            pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
        FROM pg_stat_user_indexes
        WHERE idx_scan = 0
          AND indexrelname NOT LIKE '%_pkey'
          AND indexrelname NOT LIKE '%_key'
        ORDER BY pg_relation_size(indexrelid) DESC;
    " 2>/dev/null || warn "Could not query unused indexes."

    echo ""
    warn "Indexes with 0 scans since the last statistics reset are candidates for removal."
    warn "Review with your workload before dropping — they may serve write-time constraints."
}

# ---------------------------------------------------------------------------
# Function: list_vacuum_candidates
# Shows tables with significant dead tuples that would benefit from VACUUM
# ---------------------------------------------------------------------------
list_vacuum_candidates() {
    subheader "Tables Needing VACUUM"

    "${PSQL_CMD[@]}" -c "
        SELECT
            schemaname  AS schema,
            relname     AS table,
            n_live_tup  AS live_rows,
            n_dead_tup  AS dead_rows,
            CASE
                WHEN n_live_tup > 0
                THEN round(100.0 * n_dead_tup / nullif(n_live_tup + n_dead_tup, 0), 2)
                ELSE 0
            END AS dead_ratio_pct,
            last_vacuum,
            last_autovacuum,
            last_analyze,
            last_autoanalyze
        FROM pg_stat_user_tables
        WHERE n_dead_tup > 1000
           OR (n_live_tup > 0 AND n_dead_tup::float / nullif(n_live_tup + n_dead_tup, 0) > 0.1)
        ORDER BY n_dead_tup DESC;
    " 2>/dev/null || warn "Could not query vacuum candidates."

    echo ""

    if [[ "${RUN_FULL_VACUUM}" == "true" ]]; then
        warn "VACUUM FULL requested — this will lock tables!"
        # Get tables with high bloat
        local bloated
        bloated=$("${PSQL_CMD[@]}" -t -A -c "
            SELECT relname
            FROM pg_stat_user_tables
            WHERE n_dead_tup > 10000
               OR (n_live_tup > 0 AND n_dead_tup::float / nullif(n_live_tup + n_dead_tup, 0) > 0.25)
            ORDER BY n_dead_tup DESC;
        " 2>/dev/null || echo "")

        if [[ -n "${bloated}" ]]; then
            while IFS= read -r table; do
                [[ -z "${table}" ]] && continue
                info "Running VACUUM FULL on ${table}..."
                if "${PSQL_CMD[@]}" -c "VACUUM FULL ${table};" 2>/dev/null; then
                    info "VACUUM FULL ${table} — OK"
                else
                    error "VACUUM FULL ${table} — FAILED"
                fi
            done <<< "${bloated}"
        else
            info "No tables meet the threshold for VACUUM FULL (dead_ratio > 25% or dead_rows > 10000)."
        fi
    else
        info "Run with --full-vacuum to execute VACUUM FULL on heavily bloated tables."
        info "A regular VACUUM (non-FULL) runs automatically via autovacuum."
    fi
}

# ---------------------------------------------------------------------------
# Function: show_slow_queries
# Queries pg_stat_statements for the slowest queries
# ---------------------------------------------------------------------------
show_slow_queries() {
    subheader "Slow Queries (from pg_stat_statements)"

    # Check if pg_stat_statements extension is available
    local ext_available
    ext_available=$("${PSQL_CMD[@]}" -t -A -c "
        SELECT count(*)
        FROM pg_extension
        WHERE extname = 'pg_stat_statements';
    " 2>/dev/null || echo "0")

    if [[ "${ext_available}" -lt 1 ]]; then
        warn "pg_stat_statements extension is not installed."
        echo ""
        info "To enable it, run:"
        echo "    CREATE EXTENSION pg_stat_statements;"
        echo ""
        info "And add to postgresql.conf:"
        echo "    shared_preload_libraries = 'pg_stat_statements'"
        echo "    pg_stat_statements.track = all"
        echo ""
        info "Then restart PostgreSQL."

        # Fall back to pg_stat_activity for currently running queries
        subheader "Currently Running Queries (fallback)"
        "${PSQL_CMD[@]}" -c "
            SELECT
                pid,
                now() - pg_stat_activity.query_start AS duration,
                state,
                left(query, 200) AS query_snippet
            FROM pg_stat_activity
            WHERE state IN ('active', 'running')
              AND query NOT LIKE '%pg_stat_activity%'
            ORDER BY duration DESC;
        " 2>/dev/null || warn "Could not query pg_stat_activity."
        return
    fi

    "${PSQL_CMD[@]}" -c "
        SELECT
            round(total_exec_time::numeric, 2)     AS total_time_ms,
            round(mean_exec_time::numeric, 2)      AS mean_time_ms,
            calls,
            round((100 * total_exec_time / sum(total_exec_time) OVER ())::numeric, 2) AS pct_total,
            left(query, 300)                       AS query_snippet
        FROM pg_stat_statements
        WHERE mean_exec_time > ${SLOW_QUERY_THRESHOLD_MS}
          AND dbid = (SELECT oid FROM pg_database WHERE datname = current_database())
        ORDER BY mean_exec_time DESC
        LIMIT ${SLOW_QUERY_LIMIT};
    " 2>/dev/null || warn "Could not query pg_stat_statements."

    echo ""
    info "Showing queries with mean execution time > ${SLOW_QUERY_THRESHOLD_MS}ms (top ${SLOW_QUERY_LIMIT})."
}

# ---------------------------------------------------------------------------
# Function: show_table_sizes
# Shows total, data, index, and toast sizes for each table
# ---------------------------------------------------------------------------
show_table_sizes() {
    subheader "Table Sizes"

    "${PSQL_CMD[@]}" -c "
        SELECT
            schemaname                                    AS schema,
            relname                                        AS table,
            pg_size_pretty(pg_total_relation_size(relid))  AS total_size,
            pg_size_pretty(pg_relation_size(relid))        AS data_size,
            pg_size_pretty(pg_indexes_size(relid))        AS index_size,
            pg_size_pretty(
                pg_total_relation_size(relid)
                - pg_relation_size(relid)
                - pg_indexes_size(relid)
            )                                              AS toast_plus_extra,
            n_live_tup                                     AS row_estimate
        FROM pg_stat_user_tables
        ORDER BY pg_total_relation_size(relid) DESC;
    " 2>/dev/null || warn "Could not query table sizes."

    echo ""

    subheader "Index Sizes"
    "${PSQL_CMD[@]}" -c "
        SELECT
            schemaname   AS schema,
            relname      AS table,
            indexrelname AS index,
            idx_scan     AS scans,
            pg_size_pretty(pg_relation_size(indexrelid)) AS index_size
        FROM pg_stat_user_indexes
        ORDER BY pg_relation_size(indexrelid) DESC
        LIMIT 20;
    " 2>/dev/null || warn "Could not query index sizes."
}

# ---------------------------------------------------------------------------
# Function: show_index_health
# Duplicate / redundant index detection
# ---------------------------------------------------------------------------
show_index_health() {
    subheader "Potentially Redundant Indexes"

    "${PSQL_CMD[@]}" -c "
        SELECT
            pg_size_pretty(sum(pg_relation_size(idx))::bigint) AS total_size,
            count(*) AS dup_count,
            string_agg(idx::text, ', ') AS indexes
        FROM (
            SELECT
                indexrelid::regclass AS idx,
                indrelid,
                indkey,
                indpred
            FROM pg_index
            WHERE NOT indisprimary
              AND NOT indisunique
        ) sub
        GROUP BY indrelid, indkey, indpred
        HAVING count(*) > 1
        ORDER BY sum(pg_relation_size(idx)) DESC;
    " 2>/dev/null || warn "Could not query redundant indexes."

    echo ""
    info "Redundant indexes share the same table, columns, and predicate."
    info "Consider keeping only the most-used one and dropping the rest."
}

# ---------------------------------------------------------------------------
# Function: show_connection_stats
# ---------------------------------------------------------------------------
show_connection_stats() {
    subheader "Connection Pool Statistics"

    "${PSQL_CMD[@]}" -c "
        SELECT
            state,
            count(*) AS connections,
            count(*) FILTER (WHERE now() - query_start > interval '5 seconds') AS long_running
        FROM pg_stat_activity
        WHERE datname = current_database()
        GROUP BY state
        ORDER BY count(*) DESC;
    " 2>/dev/null || warn "Could not query connection stats."

    echo ""
    info "Max connections setting:"
    "${PSQL_CMD[@]}" -t -A -c "SHOW max_connections;" 2>/dev/null || echo "unknown"
}

# ---------------------------------------------------------------------------
# Function: generate_report
# Combines all sections into a single formatted report
# ---------------------------------------------------------------------------
generate_report() {
    {
        header "Chaos-Sec Database Optimization Report"
        echo -e "  Database:   ${DB_NAME}"
        echo -e "  Host:       ${DB_HOST}:${DB_PORT}"
        echo -e "  User:       ${DB_USER}"
        echo -e "  Generated:  ${REPORT_TIMESTAMP}"
        echo ""

        list_unused_indexes
        list_vacuum_candidates
        show_slow_queries
        show_table_sizes
        show_index_health
        show_connection_stats

        header "Summary & Recommendations"
        info "1. Review unused indexes above — consider dropping those that serve no constraint."
        info "2. Tables with high dead-row ratios may need VACUUM or VACUUM FULL."
        info "3. Slow queries should be EXPLAIN ANALYZE'd and may need new indexes."
        info "4. Redundant indexes waste disk and slow writes — consolidate them."
        info "5. Run this script periodically (e.g., weekly via cron) to track trends."
        echo ""
        info "To reset pg_stat_statements counters:"
        echo "    SELECT pg_stat_statements_reset();"
        echo ""
        info "To reset statistics counters:"
        echo "    SELECT pg_stat_reset();"
    } | tee "${REPORT_FILE}"

    echo ""
    info "Report saved to ${REPORT_FILE}"
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
if [[ "${RUN_ANALYZE}" == "true" ]]; then
    run_analyze
fi

if [[ "${RUN_REPORT}" == "true" ]]; then
    generate_report
fi
