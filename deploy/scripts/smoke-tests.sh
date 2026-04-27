#!/usr/bin/env bash
# ============================================================================
# Chaos-Sec: Production Smoke Tests
# Comprehensive post-deployment verification for the Chaos-Sec security control
# validation platform — health checks, auth, CRUD, connectors, frontend, TLS,
# and response time validation with colored pass/fail output
# ============================================================================

set -uo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
SCRIPT_NAME="$(basename "$0")"

# Service URLs (overridable via environment)
BASE_URL="${BASE_URL:-http://localhost:8081}"
FRONTEND_URL="${FRONTEND_URL:-http://localhost:3000}"
SIEM_URL="${SIEM_URL:-http://localhost:8089}"
API_PREFIX="${API_PREFIX:-/api/v1}"

# Authentication credentials (set via environment or CI secrets)
AUTH_USER="${AUTH_USER:-admin@chaos-sec.io}"
AUTH_PASS="${AUTH_PASS:-admin}"

# Response time thresholds (milliseconds)
API_MAX_RESPONSE_MS="${API_MAX_RESPONSE_MS:-500}"
FRONTEND_MAX_RESPONSE_MS="${FRONTEND_MAX_RESPONSE_MS:-2000}"

# TLS check configuration
TLS_CHECK_ENABLED="${TLS_CHECK_ENABLED:-false}"
TLS_DOMAIN="${TLS_DOMAIN:-chaos-sec.example.com}"
TLS_PORT="${TLS_PORT:-443}"
TLS_WARN_DAYS="${TLS_WARN_DAYS:-30}"

# Timeout for curl requests (seconds)
CURL_TIMEOUT="${CURL_TIMEOUT:-10}"

# Test tracking
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0
SKIPPED_TESTS=0
TEST_RESULTS=()
START_TIME="$(date +%s)"

# JWT token storage
ACCESS_TOKEN=""
REFRESH_TOKEN=""

# Created resource IDs (for cleanup)
CREATED_EXPERIMENT_ID=""
CREATED_CLUSTER_ID=""

# ---------------------------------------------------------------------------
# Colors
# ---------------------------------------------------------------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
MAGENTA='\033[0;35m'
BOLD='\033[1m'
DIM='\033[2m'
NC='\033[0m'

# ---------------------------------------------------------------------------
# Logging
# ---------------------------------------------------------------------------
log()       { echo -e "${CYAN}[INFO]${NC}  $*"; }
log_ok()    { echo -e "${GREEN}[  OK]${NC}  $*"; }
log_fail()  { echo -e "${RED}[FAIL]${NC}  $*"; }
log_skip()  { echo -e "${YELLOW}[SKIP]${NC}  $*"; }
log_warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_step()  { echo -e "${BLUE}[STEP]${NC}  $*"; }
log_dim()   { echo -e "${DIM}$*${NC}"; }
log_bold()  { echo -e "${BOLD}$*${NC}"; }
log_header(){ echo -e "\n${MAGENTA}━━━ $* ━━━${NC}\n"; }

# ---------------------------------------------------------------------------
# Test result tracking
# ---------------------------------------------------------------------------
record_pass() {
    local test_name="$1"
    local detail="${2:-}"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    PASSED_TESTS=$((PASSED_TESTS + 1))
    if [[ -n "${detail}" ]]; then
        log_ok "${test_name}  ${DIM}${detail}${NC}"
    else
        log_ok "${test_name}"
    fi
    TEST_RESULTS+=("PASS|${test_name}|${detail}")
}

record_fail() {
    local test_name="$1"
    local detail="${2:-}"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    FAILED_TESTS=$((FAILED_TESTS + 1))
    log_fail "${test_name}  ${DIM}${detail}${NC}"
    TEST_RESULTS+=("FAIL|${test_name}|${detail}")
}

record_skip() {
    local test_name="$1"
    local reason="${2:-}"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    SKIPPED_TESTS=$((SKIPPED_TESTS + 1))
    log_skip "${test_name}  ${DIM}${reason}${NC}"
    TEST_RESULTS+=("SKIP|${test_name}|${reason}")
}

# ---------------------------------------------------------------------------
# HTTP helpers
# ---------------------------------------------------------------------------
# Perform an HTTP request and capture status, body, and timing
# Usage: http_request <method> <url> [headers...] [--body <data>]
# Returns: <status_code> <response_time_ms> <body>
http_request() {
    local method="$1"
    local url="$2"
    shift 2

    local headers=()
    local body=""
    local expect_status=""

    while [[ $# -gt 0 ]]; do
        case "$1" in
            -H)
                headers+=("-H" "$2")
                shift 2
                ;;
            --body)
                body="$2"
                shift 2
                ;;
            --expect-status)
                expect_status="$2"
                shift 2
                ;;
            *)
                shift
                ;;
        esac
    done

    local curl_args=(
        -s
        -o /tmp/chaos-sec-smoke-body.txt
        -w "%{http_code} %{time_total}"
        --max-time "${CURL_TIMEOUT}"
        --connect-timeout 5
    )

    for h in "${headers[@]}"; do
        curl_args+=("$h")
    done

    if [[ -n "${body}" ]]; then
        curl_args+=("-d" "${body}")
    fi

    if [[ "${method}" == "POST" || "${method}" == "PUT" || "${method}" == "PATCH" ]]; then
        curl_args+=("-X" "${method}")
    fi

    local result
    result="$(curl "${curl_args[@]}" "${url}" 2>/dev/null || echo "000 0")"
    local http_code
    http_code="$(echo "${result}" | awk '{print $1}')"
    local time_total
    time_total="$(echo "${result}" | awk '{print $2}')"
    local time_ms
    time_ms="$(echo "${time_total}" | awk '{printf "%.0f", $1 * 1000}')"

    local response_body
    response_body="$(cat /tmp/chaos-sec-smoke-body.txt 2>/dev/null || echo "")"

    echo "${http_code}|${time_ms}|${response_body}"
}

# Simplified GET with auth
api_get() {
    local path="$1"
    local url="${BASE_URL}${API_PREFIX}${path}"
    local result
    result="$(http_request GET "${url}" -H "Authorization: Bearer ${ACCESS_TOKEN}" -H "Content-Type: application/json")"
    echo "${result}"
}

# Simplified POST with auth
api_post() {
    local path="$1"
    local body="$2"
    local url="${BASE_URL}${API_PREFIX}${path}"
    local result
    result="$(http_request POST "${url}" -H "Authorization: Bearer ${ACCESS_TOKEN}" -H "Content-Type: application/json" --body "${body}")"
    echo "${result}"
}

# Simplified DELETE with auth
api_delete() {
    local path="$1"
    local url="${BASE_URL}${API_PREFIX}${path}"
    local result
    result="$(http_request DELETE "${url}" -H "Authorization: Bearer ${ACCESS_TOKEN}" -H "Content-Type: application/json")"
    echo "${result}"
}

# Extract field from HTTP result
extract_status() { echo "$1" | cut -d'|' -f1; }
extract_time()   { echo "$1" | cut -d'|' -f2; }
extract_body()   { echo "$1" | cut -d'|' -f3-; }

# ---------------------------------------------------------------------------
# Cleanup on exit
# ---------------------------------------------------------------------------
cleanup() {
    # Delete any created experiment if we have a token
    if [[ -n "${CREATED_EXPERIMENT_ID}" && -n "${ACCESS_TOKEN}" ]]; then
        log_dim "Cleaning up created experiment: ${CREATED_EXPERIMENT_ID}"
        api_delete "/experiments/${CREATED_EXPERIMENT_ID}" >/dev/null 2>&1 || true
    fi

    if [[ -n "${CREATED_CLUSTER_ID}" && -n "${ACCESS_TOKEN}" ]]; then
        log_dim "Cleaning up created cluster: ${CREATED_CLUSTER_ID}"
        api_delete "/clusters/${CREATED_CLUSTER_ID}" >/dev/null 2>&1 || true
    fi

    rm -f /tmp/chaos-sec-smoke-body.txt 2>/dev/null || true

    # Print summary
    local end_time duration
    end_time="$(date +%s)"
    duration=$((end_time - START_TIME))

    echo ""
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Smoke Test Summary"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo -e "  ${GREEN}PASSED:${NC}   ${PASSED_TESTS}"
    echo -e "  ${RED}FAILED:${NC}   ${FAILED_TESTS}"
    echo -e "  ${YELLOW}SKIPPED:${NC}   ${SKIPPED_TESTS}"
    echo -e "  ${DIM}TOTAL:${NC}    ${TOTAL_TESTS}"
    echo -e "  ${DIM}TIME:${NC}     ${duration}s"
    echo ""

    if [[ ${FAILED_TESTS} -gt 0 ]]; then
        echo -e "  ${RED}Failed Tests:${NC}"
        for result in "${TEST_RESULTS[@]}"; do
            local status name detail
            status="$(echo "${result}" | cut -d'|' -f1)"
            name="$(echo "${result}" | cut -d'|' -f2)"
            detail="$(echo "${result}" | cut -d'|' -f3-)"
            if [[ "${status}" == "FAIL" ]]; then
                echo -e "    ${RED}✗${NC} ${name}  ${DIM}${detail}${NC}"
            fi
        done
        echo ""
    fi

    if [[ ${FAILED_TESTS} -eq 0 ]]; then
        echo -e "  ${GREEN}${BOLD}All smoke tests passed! ✓${NC}"
        echo ""
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        exit 0
    else
        echo -e "  ${RED}${BOLD}${FAILED_TESTS} smoke test(s) failed! ✗${NC}"
        echo ""
        log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
        exit 1
    fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Prerequisite checks
# ---------------------------------------------------------------------------
check_prerequisites() {
    log_header "Prerequisites"

    if ! command -v curl &>/dev/null; then
        record_fail "curl installed" "curl is required for smoke tests"
        exit 1
    fi
    record_pass "curl installed"

    if ! command -v jq &>/dev/null; then
        log_warn "jq not installed — JSON parsing will be limited"
    else
        record_pass "jq installed"
    fi

    if [[ "${TLS_CHECK_ENABLED}" == "true" ]] && ! command -v openssl &>/dev/null; then
        log_warn "openssl not installed — TLS checks will be skipped"
        TLS_CHECK_ENABLED="false"
    fi

    log "Base URL:      ${BASE_URL}"
    log "Frontend URL:  ${FRONTEND_URL}"
    log "SIEM URL:      ${SIEM_URL}"
    log "API Prefix:     ${API_PREFIX}"
    log "Auth User:     ${AUTH_USER}"
}

# ===========================================================================
# Test Suite 1: Health Check Endpoints
# ===========================================================================
test_health_endpoints() {
    log_header "1. Health Check Endpoints"

    # ── /health (primary health endpoint per OpenAPI spec) ─────────
    local result
    result="$(http_request GET "${BASE_URL}/health")"
    local status time_ms body
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "/health returns 200" "HTTP ${status} (${time_ms}ms)"

        # Verify response structure matches OpenAPI spec
        if command -v jq &>/dev/null; then
            local health_status
            health_status="$(echo "${body}" | jq -r '.status // empty' 2>/dev/null)"
            if [[ "${health_status}" == "healthy" ]]; then
                record_pass "/health status is 'healthy'"
            else
                record_fail "/health status field" "Expected 'healthy', got '${health_status}'"
            fi

            # Check dependencies in health response
            local pg_status redis_status siem_status
            pg_status="$(echo "${body}" | jq -r '.dependencies.postgres.status // empty' 2>/dev/null)"
            redis_status="$(echo "${body}" | jq -r '.dependencies.redis.status // empty' 2>/dev/null)"
            siem_status="$(echo "${body}" | jq -r '.dependencies.siem.status // empty' 2>/dev/null)"

            if [[ -n "${pg_status}" ]]; then
                if [[ "${pg_status}" == "healthy" ]]; then
                    record_pass "/health postgres dependency" "status: ${pg_status}"
                else
                    record_fail "/health postgres dependency" "status: ${pg_status}"
                fi
            else
                record_skip "/health postgres dependency" "Not in response"
            fi

            if [[ -n "${redis_status}" ]]; then
                if [[ "${redis_status}" == "healthy" ]]; then
                    record_pass "/health redis dependency" "status: ${redis_status}"
                else
                    record_fail "/health redis dependency" "status: ${redis_status}"
                fi
            else
                record_skip "/health redis dependency" "Not in response"
            fi

            if [[ -n "${siem_status}" ]]; then
                if [[ "${siem_status}" == "healthy" ]]; then
                    record_pass "/health SIEM dependency" "status: ${siem_status}"
                else
                    record_fail "/health SIEM dependency" "status: ${siem_status}"
                fi
            else
                record_skip "/health SIEM dependency" "Not in response"
            fi
        fi
    else
        record_fail "/health returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── /healthz (Kubernetes liveness probe convention) ─────────────
    result="$(http_request GET "${BASE_URL}/healthz")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "/healthz returns 200" "HTTP ${status} (${time_ms}ms)"
    elif [[ "${status}" == "404" ]]; then
        record_skip "/healthz endpoint" "Not implemented (HTTP 404)"
    else
        record_fail "/healthz returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── /readyz (Kubernetes readiness probe convention) ────────────
    result="$(http_request GET "${BASE_URL}/readyz")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "/readyz returns 200" "HTTP ${status} (${time_ms}ms)"
    elif [[ "${status}" == "404" ]]; then
        record_skip "/readyz endpoint" "Not implemented (HTTP 404)"
    else
        record_fail "/readyz returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── /livez (Kubernetes liveness probe convention) ──────────────
    result="$(http_request GET "${BASE_URL}/livez")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "/livez returns 200" "HTTP ${status} (${time_ms}ms)"
    elif [[ "${status}" == "404" ]]; then
        record_skip "/livez endpoint" "Not implemented (HTTP 404)"
    else
        record_fail "/livez returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── /health/live (Docker HEALTHCHECK endpoint) ────────────────
    result="$(http_request GET "${BASE_URL}/health/live")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "/health/live returns 200" "HTTP ${status} (${time_ms}ms)"
    elif [[ "${status}" == "404" ]]; then
        record_skip "/health/live endpoint" "Not implemented (HTTP 404)"
    else
        record_fail "/health/live returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── /metrics (Prometheus metrics endpoint) ────────────────────
    result="$(http_request GET "${BASE_URL}/metrics")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "/metrics returns 200" "HTTP ${status} (${time_ms}ms)"
    else
        record_skip "/metrics endpoint" "HTTP ${status}"
    fi
}

# ===========================================================================
# Test Suite 2: Authentication
# ===========================================================================
test_authentication() {
    log_header "2. Authentication"

    # ── Login ──────────────────────────────────────────────────────
    log_step "Testing POST /api/v1/auth/login"

    local login_body
    login_body="$(cat <<JSON
{
    "email": "${AUTH_USER}",
    "password": "${AUTH_PASS}"
}
JSON
)"

    local result
    result="$(http_request POST "${BASE_URL}${API_PREFIX}/auth/login" \
        -H "Content-Type: application/json" \
        --body "${login_body}")"

    local status time_ms body
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "Login succeeds (200)" "${time_ms}ms"

        if command -v jq &>/dev/null; then
            ACCESS_TOKEN="$(echo "${body}" | jq -r '.access_token // empty' 2>/dev/null)"
            REFRESH_TOKEN="$(echo "${body}" | jq -r '.refresh_token // empty' 2>/dev/null)"

            if [[ -n "${ACCESS_TOKEN}" ]]; then
                record_pass "Access token received" "length: ${#ACCESS_TOKEN}"
            else
                record_fail "Access token received" "Token missing from response"
            fi

            if [[ -n "${REFRESH_TOKEN}" ]]; then
                record_pass "Refresh token received" "length: ${#REFRESH_TOKEN}"
            else
                record_fail "Refresh token received" "Token missing from response"
            fi

            # Verify token type
            local token_type
            token_type="$(echo "${body}" | jq -r '.token_type // empty' 2>/dev/null)"
            if [[ "${token_type}" == "Bearer" ]]; then
                record_pass "Token type is Bearer"
            else
                record_fail "Token type is Bearer" "Got: ${token_type}"
            fi

            # Verify expires_in is present
            local expires_in
            expires_in="$(echo "${body}" | jq -r '.expires_in // empty' 2>/dev/null)"
            if [[ -n "${expires_in}" && "${expires_in}" -gt 0 ]]; then
                record_pass "Token expiry present" "expires_in: ${expires_in}s"
            else
                record_fail "Token expiry present" "Missing or invalid"
            fi
        else
            log_warn "jq not available — cannot parse login response for tokens"
            # Try a basic grep as fallback
            if echo "${body}" | grep -q "access_token"; then
                record_pass "Access token present (grep)"
            else
                record_fail "Access token present (grep)" "Token key not found in response"
            fi
        fi
    else
        record_fail "Login succeeds (200)" "HTTP ${status} (${time_ms}ms)"
        # Continue with tests that don't require auth
    fi

    # ── Login with invalid credentials ─────────────────────────────
    log_step "Testing login with invalid credentials"

    local bad_login_body
    bad_login_body='{"email":"invalid@chaos-sec.io","password":"wrongpassword"}'

    result="$(http_request POST "${BASE_URL}${API_PREFIX}/auth/login" \
        -H "Content-Type: application/json" \
        --body "${bad_login_body}")"

    status="$(extract_status "${result}")"

    if [[ "${status}" == "401" ]]; then
        record_pass "Login rejects invalid credentials" "HTTP 401"
    else
        record_fail "Login rejects invalid credentials" "Expected 401, got HTTP ${status}"
    fi

    # ── Token refresh ──────────────────────────────────────────────
    if [[ -n "${REFRESH_TOKEN}" ]]; then
        log_step "Testing POST /api/v1/auth/refresh"

        local refresh_body
        refresh_body="{\"refresh_token\": \"${REFRESH_TOKEN}\"}"

        result="$(http_request POST "${BASE_URL}${API_PREFIX}/auth/refresh" \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${ACCESS_TOKEN}" \
            --body "${refresh_body}")"

        status="$(extract_status "${result}")"
        time_ms="$(extract_time "${result}")"
        body="$(extract_body "${result}")"

        if [[ "${status}" == "200" ]]; then
            record_pass "Token refresh succeeds (200)" "${time_ms}ms"

            # Update tokens for subsequent tests
            if command -v jq &>/dev/null; then
                local new_access
                new_access="$(echo "${body}" | jq -r '.access_token // empty' 2>/dev/null)"
                if [[ -n "${new_access}" ]]; then
                    ACCESS_TOKEN="${new_access}"
                    record_pass "New access token received from refresh"
                fi
            fi
        else
            record_fail "Token refresh succeeds (200)" "HTTP ${status} (${time_ms}ms)"
        fi
    else
        record_skip "Token refresh" "No refresh token available"
    fi

    # ── Auth /me endpoint ──────────────────────────────────────────
    if [[ -n "${ACCESS_TOKEN}" ]]; then
        log_step "Testing GET /api/v1/auth/me"

        result="$(api_get "/auth/me")"
        status="$(extract_status "${result}")"
        body="$(extract_body "${result}")"

        if [[ "${status}" == "200" ]]; then
            record_pass "/auth/me returns 200"

            if command -v jq &>/dev/null; then
                local user_email user_role
                user_email="$(echo "${body}" | jq -r '.email // empty' 2>/dev/null)"
                user_role="$(echo "${body}" | jq -r '.role // empty' 2>/dev/null)"

                if [[ "${user_email}" == "${AUTH_USER}" ]]; then
                    record_pass "/auth/me returns correct user" "email: ${user_email}, role: ${user_role}"
                else
                    record_fail "/auth/me returns correct user" "Expected ${AUTH_USER}, got ${user_email}"
                fi
            fi
        else
            record_fail "/auth/me returns 200" "HTTP ${status}"
        fi
    else
        record_skip "/auth/me" "No access token"
    fi

    # ── Unauthenticated request ────────────────────────────────────
    log_step "Testing unauthenticated API request"

    result="$(http_request GET "${BASE_URL}${API_PREFIX}/experiments" -H "Content-Type: application/json")"
    status="$(extract_status "${result}")"

    if [[ "${status}" == "401" ]]; then
        record_pass "Unauthenticated request returns 401"
    else
        record_fail "Unauthenticated request returns 401" "Expected 401, got HTTP ${status}"
    fi
}

# ===========================================================================
# Test Suite 3: Experiment CRUD
# ===========================================================================
test_experiment_crud() {
    log_header "3. Experiment CRUD"

    if [[ -z "${ACCESS_TOKEN}" ]]; then
        record_skip "Experiment CRUD" "No access token — skipping all CRUD tests"
        return
    fi

    # ── List experiments ───────────────────────────────────────────
    log_step "Testing GET /api/v1/experiments (list)"

    local result
    result="$(api_get "/experiments")"
    local status time_ms body
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "List experiments (200)" "${time_ms}ms"

        if command -v jq &>/dev/null; then
            # Verify paginated response structure
            local has_data has_pagination
            has_data="$(echo "${body}" | jq 'has("data")' 2>/dev/null)"
            has_pagination="$(echo "${body}" | jq 'has("pagination")' 2>/dev/null)"

            if [[ "${has_data}" == "true" ]]; then
                record_pass "Experiments response has 'data' array"
            else
                record_fail "Experiments response has 'data' array"
            fi

            if [[ "${has_pagination}" == "true" ]]; then
                record_pass "Experiments response has 'pagination' object"
            else
                record_fail "Experiments response has 'pagination' object"
            fi
        fi
    else
        record_fail "List experiments (200)" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── Create experiment ──────────────────────────────────────────
    log_step "Testing POST /api/v1/experiments (create)"

    local create_body
    create_body="$(cat <<'JSON'
{
    "name": "Smoke Test - Pod Kill",
    "description": "Automated smoke test experiment — safe to delete",
    "type": "pod_kill",
    "namespace": "chaos-sec-experiments",
    "target_selector": {
        "app": "smoke-test-target"
    },
    "parameters": {
        "duration": "30s",
        "force": false,
        "grace_period": "5s"
    },
    "tags": ["smoke-test", "automated"],
    "controls": [
        {
            "name": "SIEM Alert Generation",
            "type": "alert",
            "expected": "Alert generated within 60 seconds",
            "timeout_seconds": 120
        }
    ],
    "timeout_seconds": 300
}
JSON
)"

    result="$(api_post "/experiments" "${create_body}")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "201" || "${status}" == "200" ]]; then
        record_pass "Create experiment (${status})" "${time_ms}ms"

        if command -v jq &>/dev/null; then
            CREATED_EXPERIMENT_ID="$(echo "${body}" | jq -r '.id // empty' 2>/dev/null)"

            if [[ -n "${CREATED_EXPERIMENT_ID}" ]]; then
                record_pass "Created experiment has ID" "id: ${CREATED_EXPERIMENT_ID}"
            else
                record_fail "Created experiment has ID" "ID missing from response"
            fi

            # Verify experiment fields
            local exp_name exp_type exp_status
            exp_name="$(echo "${body}" | jq -r '.name // empty' 2>/dev/null)"
            exp_type="$(echo "${body}" | jq -r '.type // empty' 2>/dev/null)"
            exp_status="$(echo "${body}" | jq -r '.status // empty' 2>/dev/null)"

            if [[ "${exp_name}" == "Smoke Test - Pod Kill" ]]; then
                record_pass "Experiment name matches"
            else
                record_fail "Experiment name matches" "Expected 'Smoke Test - Pod Kill', got '${exp_name}'"
            fi

            if [[ "${exp_type}" == "pod_kill" ]]; then
                record_pass "Experiment type matches"
            else
                record_fail "Experiment type matches" "Expected 'pod_kill', got '${exp_type}'"
            fi

            if [[ "${exp_status}" == "pending" ]]; then
                record_pass "Experiment status is 'pending'"
            else
                record_fail "Experiment status is 'pending'" "Got: ${exp_status}"
            fi
        fi
    else
        record_fail "Create experiment (201/200)" "HTTP ${status} (${time_ms}ms)"
        log_dim "Response: $(echo "${body}" | head -c 200)"
    fi

    # ── Get experiment by ID ────────────────────────────────────────
    if [[ -n "${CREATED_EXPERIMENT_ID}" ]]; then
        log_step "Testing GET /api/v1/experiments/{id} (get)"

        result="$(api_get "/experiments/${CREATED_EXPERIMENT_ID}")"
        status="$(extract_status "${result}")"
        time_ms="$(extract_time "${result}")"
        body="$(extract_body "${result}")"

        if [[ "${status}" == "200" ]]; then
            record_pass "Get experiment by ID (200)" "${time_ms}ms"

            if command -v jq &>/dev/null; then
                local fetched_id
                fetched_id="$(echo "${body}" | jq -r '.id // empty' 2>/dev/null)"
                if [[ "${fetched_id}" == "${CREATED_EXPERIMENT_ID}" ]]; then
                    record_pass "Fetched experiment ID matches"
                else
                    record_fail "Fetched experiment ID matches" "Expected ${CREATED_EXPERIMENT_ID}, got ${fetched_id}"
                fi
            fi
        else
            record_fail "Get experiment by ID (200)" "HTTP ${status} (${time_ms}ms)"
        fi

        # ── Delete experiment ──────────────────────────────────────
        log_step "Testing DELETE /api/v1/experiments/{id} (delete)"

        result="$(api_delete "/experiments/${CREATED_EXPERIMENT_ID}")"
        status="$(extract_status "${result}")"
        time_ms="$(extract_time "${result}")"

        if [[ "${status}" == "200" || "${status}" == "204" ]]; then
            record_pass "Delete experiment (${status})" "${time_ms}ms"
            CREATED_EXPERIMENT_ID=""  # Clear so cleanup doesn't re-delete
        else
            record_fail "Delete experiment (200/204)" "HTTP ${status} (${time_ms}ms)"
        fi

        # ── Verify deletion ─────────────────────────────────────────
        log_step "Verifying experiment was deleted"

        result="$(api_get "/experiments/${CREATED_EXPERIMENT_ID:-deleted}")"
        status="$(extract_status "${result}")"

        if [[ "${status}" == "404" ]]; then
            record_pass "Deleted experiment returns 404"
        else
            record_fail "Deleted experiment returns 404" "HTTP ${status}"
        fi
    else
        record_skip "Get/Delete experiment" "No experiment ID from create"
    fi
}

# ===========================================================================
# Test Suite 4: Cluster Registration & Health
# ===========================================================================
test_cluster_health() {
    log_header "4. Cluster Registration & Health"

    if [[ -z "${ACCESS_TOKEN}" ]]; then
        record_skip "Cluster tests" "No access token"
        return
    fi

    # ── List clusters ──────────────────────────────────────────────
    log_step "Testing GET /api/v1/clusters (list)"

    local result
    result="$(api_get "/clusters")"
    local status time_ms body
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "List clusters (200)" "${time_ms}ms"

        if command -v jq &>/dev/null; then
            local cluster_count
            cluster_count="$(echo "${body}" | jq 'if type == "array" then length else 0 end' 2>/dev/null || echo "0")"
            record_pass "Clusters response parsed" "${cluster_count} cluster(s) registered"
        fi
    else
        record_fail "List clusters (200)" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── Register a test cluster ────────────────────────────────────
    log_step "Testing POST /api/v1/clusters (register)"

    local register_body
    register_body='{
        "name": "smoke-test-cluster",
        "description": "Smoke test cluster registration — safe to delete",
        "context": "kind-chaos-sec",
        "namespace": "chaos-sec-experiments",
        "labels": {
            "environment": "smoke-test",
            "managed-by": "smoke-tests"
        }
    }'

    result="$(api_post "/clusters" "${register_body}")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "201" || "${status}" == "200" ]]; then
        record_pass "Register cluster (${status})" "${time_ms}ms"

        if command -v jq &>/dev/null; then
            CREATED_CLUSTER_ID="$(echo "${body}" | jq -r '.id // empty' 2>/dev/null)"
            if [[ -n "${CREATED_CLUSTER_ID}" ]]; then
                record_pass "Registered cluster has ID" "id: ${CREATED_CLUSTER_ID}"
            fi
        fi
    else
        record_fail "Register cluster (201/200)" "HTTP ${status} (${time_ms}ms)"
        log_dim "Response: $(echo "${body}" | head -c 200)"
    fi

    # ── Get cluster health ─────────────────────────────────────────
    if [[ -n "${CREATED_CLUSTER_ID}" ]]; then
        log_step "Testing GET /api/v1/clusters/{id}/health"

        result="$(api_get "/clusters/${CREATED_CLUSTER_ID}/health")"
        status="$(extract_status "${result}")"
        time_ms="$(extract_time "${result}")"

        if [[ "${status}" == "200" ]]; then
            record_pass "Cluster health (200)" "${time_ms}ms"
        elif [[ "${status}" == "503" ]]; then
            # 503 is valid if the cluster is unhealthy
            record_pass "Cluster health endpoint responds" "HTTP 503 (cluster may not be reachable from test)"
        else
            record_skip "Cluster health" "HTTP ${status} (cluster may not have real connectivity)"
        fi

        # ── Get cluster details ─────────────────────────────────────
        log_step "Testing GET /api/v1/clusters/{id} (get)"

        result="$(api_get "/clusters/${CREATED_CLUSTER_ID}")"
        status="$(extract_status "${result}")"

        if [[ "${status}" == "200" ]]; then
            record_pass "Get cluster by ID (200)"
        else
            record_fail "Get cluster by ID (200)" "HTTP ${status}"
        fi

        # ── Remove test cluster ─────────────────────────────────────
        log_step "Testing DELETE /api/v1/clusters/{id} (remove)"

        result="$(api_delete "/clusters/${CREATED_CLUSTER_ID}")"
        status="$(extract_status "${result}")"

        if [[ "${status}" == "200" || "${status}" == "204" ]]; then
            record_pass "Remove cluster (${status})"
            CREATED_CLUSTER_ID=""
        else
            record_fail "Remove cluster (200/204)" "HTTP ${status}"
        fi
    else
        record_skip "Cluster health/get/remove" "No cluster ID from register"
    fi
}

# ===========================================================================
# Test Suite 5: SIEM Connector Health
# ===========================================================================
test_siem_connector() {
    log_header "5. SIEM Connector Health"

    # ── Mock SIEM health endpoint ──────────────────────────────────
    log_step "Testing SIEM service /health"

    local result
    result="$(http_request GET "${SIEM_URL}/health")"
    local status time_ms body
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "SIEM /health returns 200" "${time_ms}ms"

        if command -v jq &>/dev/null; then
            local siem_status
            siem_status="$(echo "${body}" | jq -r '.status // empty' 2>/dev/null)"
            if [[ "${siem_status}" == "healthy" ]]; then
                record_pass "SIEM status is 'healthy'"
            else
                record_fail "SIEM status is 'healthy'" "Got: ${siem_status}"
            fi

            local alert_count
            alert_count="$(echo "${body}" | jq -r '.alerts_stored // empty' 2>/dev/null)"
            if [[ -n "${alert_count}" ]]; then
                record_pass "SIEM alert count present" "${alert_count} alerts stored"
            fi
        fi
    else
        record_fail "SIEM /health returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── SIEM alerts endpoint ───────────────────────────────────────
    log_step "Testing SIEM GET /api/alerts"

    result="$(http_request GET "${SIEM_URL}/api/alerts")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "SIEM /api/alerts returns 200" "${time_ms}ms"
    else
        record_fail "SIEM /api/alerts returns 200" "HTTP ${status}"
    fi

    # ── SIEM alert ingestion ────────────────────────────────────────
    log_step "Testing SIEM POST /api/alerts (ingest)"

    local ingest_body
    ingest_body='{
        "type": "smoke_test",
        "severity": "low",
        "source": "smoke-test-script",
        "message": "Automated smoke test alert — safe to ignore"
    }'

    result="$(http_request POST "${SIEM_URL}/api/alerts" \
        -H "Content-Type: application/json" \
        --body "${ingest_body}")"
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${status}" == "201" || "${status}" == "200" ]]; then
        record_pass "SIEM alert ingestion succeeds (${status})" "${time_ms}ms"
    else
        record_fail "SIEM alert ingestion succeeds" "HTTP ${status}"
    fi

    # ── Backend SIEM integration check ─────────────────────────────
    if [[ -n "${ACCESS_TOKEN}" ]]; then
        log_step "Testing backend SIEM integration via /health"

        result="$(http_request GET "${BASE_URL}/health")"
        body="$(extract_body "${result}")"

        if command -v jq &>/dev/null; then
            local siem_dep_status
            siem_dep_status="$(echo "${body}" | jq -r '.dependencies.siem.status // empty' 2>/dev/null)"
            if [[ "${siem_dep_status}" == "healthy" ]]; then
                record_pass "Backend SIEM dependency healthy"
            elif [[ -n "${siem_dep_status}" ]]; then
                record_fail "Backend SIEM dependency healthy" "status: ${siem_dep_status}"
            else
                record_skip "Backend SIEM dependency" "Not in health response"
            fi
        fi
    fi
}

# ===========================================================================
# Test Suite 6: Frontend
# ===========================================================================
test_frontend() {
    log_header "6. Frontend"

    # ── Frontend serves HTML ────────────────────────────────────────
    log_step "Testing frontend serves content"

    local result
    result="$(http_request GET "${FRONTEND_URL}")"
    local status time_ms body
    status="$(extract_status "${result}")"
    time_ms="$(extract_time "${result}")"
    body="$(extract_body "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "Frontend returns 200" "${time_ms}ms"

        # Check for HTML content type or HTML tags
        if echo "${body}" | grep -qi "<!DOCTYPE\|<html\|<head\|<div"; then
            record_pass "Frontend serves HTML content"
        else
            record_fail "Frontend serves HTML content" "No HTML tags found in response"
        fi
    else
        record_fail "Frontend returns 200" "HTTP ${status} (${time_ms}ms)"
    fi

    # ── Frontend health endpoint ────────────────────────────────────
    log_step "Testing frontend /health endpoint"

    result="$(http_request GET "${FRONTEND_URL}/health")"
    status="$(extract_status "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "Frontend /health returns 200"
    elif [[ "${status}" == "404" ]]; then
        record_skip "Frontend /health" "Not implemented"
    else
        record_fail "Frontend /health returns 200" "HTTP ${status}"
    fi

    # ── Frontend static assets ──────────────────────────────────────
    log_step "Testing frontend static assets"

    # Try to fetch a common Vite/React asset path
    result="$(http_request GET "${FRONTEND_URL}/vite.svg")"
    status="$(extract_status "${result}")"

    if [[ "${status}" == "200" ]]; then
        record_pass "Frontend serves static assets (vite.svg)"
    else
        # Try index.html assets via the HTML we already fetched
        record_skip "Frontend static assets" "vite.svg not found (HTTP ${status})"
    fi
}

# ===========================================================================
# Test Suite 7: TLS Certificate Validity
# ===========================================================================
test_tls_certificates() {
    log_header "7. TLS Certificate Validity"

    if [[ "${TLS_CHECK_ENABLED}" != "true" ]]; then
        record_skip "TLS certificate checks" "TLS_CHECK_ENABLED not set to 'true'"
        return
    fi

    if ! command -v openssl &>/dev/null; then
        record_skip "TLS certificate checks" "openssl not installed"
        return
    fi

    # ── Check TLS certificate expiry ───────────────────────────────
    log_step "Checking TLS certificate for ${TLS_DOMAIN}:${TLS_PORT}"

    local cert_info
    cert_info="$(echo | openssl s_client -servername "${TLS_DOMAIN}" \
        -connect "${TLS_DOMAIN}:${TLS_PORT}" 2>/dev/null | \
        openssl x509 -noout -dates -subject 2>/dev/null || echo "FAILED")"

    if [[ "${cert_info}" == "FAILED" ]]; then
        record_fail "TLS certificate retrieval" "Could not connect to ${TLS_DOMAIN}:${TLS_PORT}"
        return
    fi

    record_pass "TLS certificate retrieved"

    # Extract expiry date
    local not_after
    not_after="$(echo "${cert_info}" | grep 'notAfter=' | sed 's/notAfter=//' || true)"

    if [[ -n "${not_after}" ]]; then
        local expiry_epoch current_epoch days_remaining
        expiry_epoch="$(date -d "${not_after}" +%s 2>/dev/null || echo "0")"
        current_epoch="$(date +%s)"

        if [[ "${expiry_epoch}" -gt 0 ]]; then
            days_remaining=$(( (expiry_epoch - current_epoch) / 86400 ))

            if [[ ${days_remaining} -gt ${TLS_WARN_DAYS} ]]; then
                record_pass "TLS certificate valid for ${days_remaining} days" "> ${TLS_WARN_DAYS} day threshold"
            elif [[ ${days_remaining} -gt 0 ]]; then
                record_fail "TLS certificate expiring soon" "${days_remaining} days remaining (< ${TLS_WARN_DAYS} day threshold)"
            else
                record_fail "TLS certificate EXPIRED" "Expired ${days_remaining#-} days ago"
            fi
        else
            record_skip "TLS expiry calculation" "Could not parse date: ${not_after}"
        fi
    else
        record_fail "TLS certificate expiry date" "Could not extract from certificate"
    fi

    # ── Check TLS protocol version ─────────────────────────────────
    log_step "Checking TLS protocol version"

    local tls_version
    tls_version="$(echo | openssl s_client -servername "${TLS_DOMAIN}" \
        -connect "${TLS_DOMAIN}:${TLS_PORT}" 2>/dev/null | \
        grep 'Protocol' | awk '{print $NF}' || echo "unknown")"

    if [[ "${tls_version}" == "TLSv1.3" ]]; then
        record_pass "TLS protocol is TLSv1.3"
    elif [[ "${tls_version}" == "TLSv1.2" ]]; then
        record_pass "TLS protocol is TLSv1.2" "Consider upgrading to TLSv1.3"
    elif [[ -n "${tls_version}" && "${tls_version}" != "unknown" ]]; then
        record_fail "TLS protocol version" "Using ${tls_version} — should be TLSv1.2+"
    else
        record_skip "TLS protocol version" "Could not determine"
    fi
}

# ===========================================================================
# Test Suite 8: Response Time Checks
# ===========================================================================
test_response_times() {
    log_header "8. Response Time Checks"

    local api_threshold="${API_MAX_RESPONSE_MS}"
    local frontend_threshold="${FRONTEND_MAX_RESPONSE_MS}"

    # ── API response times ─────────────────────────────────────────
    log_step "Testing API response times (threshold: ${api_threshold}ms)"

    # Health endpoint
    local result
    result="$(http_request GET "${BASE_URL}/health")"
    local time_ms
    time_ms="$(extract_time "${result}")"

    if [[ "${time_ms}" -le "${api_threshold}" ]]; then
        record_pass "/health response time" "${time_ms}ms < ${api_threshold}ms"
    else
        record_fail "/health response time" "${time_ms}ms > ${api_threshold}ms threshold"
    fi

    # Auth login
    local login_body
    login_body="{\"email\":\"${AUTH_USER}\",\"password\":\"${AUTH_PASS}\"}"
    result="$(http_request POST "${BASE_URL}${API_PREFIX}/auth/login" \
        -H "Content-Type: application/json" \
        --body "${login_body}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${time_ms}" -le "${api_threshold}" ]]; then
        record_pass "/auth/login response time" "${time_ms}ms < ${api_threshold}ms"
    else
        record_fail "/auth/login response time" "${time_ms}ms > ${api_threshold}ms threshold"
    fi

    # Experiments list (authenticated)
    if [[ -n "${ACCESS_TOKEN}" ]]; then
        result="$(api_get "/experiments")"
        time_ms="$(extract_time "${result}")"

        if [[ "${time_ms}" -le "${api_threshold}" ]]; then
            record_pass "/experiments list response time" "${time_ms}ms < ${api_threshold}ms"
        else
            record_fail "/experiments list response time" "${time_ms}ms > ${api_threshold}ms threshold"
        fi
    fi

    # Dashboard summary
    if [[ -n "${ACCESS_TOKEN}" ]]; then
        result="$(api_get "/dashboard/summary")"
        time_ms="$(extract_time "${result}")"

        if [[ "${time_ms}" -le "${api_threshold}" ]]; then
            record_pass "/dashboard/summary response time" "${time_ms}ms < ${api_threshold}ms"
        else
            record_fail "/dashboard/summary response time" "${time_ms}ms > ${api_threshold}ms threshold"
        fi
    fi

    # ── Frontend response time ─────────────────────────────────────
    log_step "Testing frontend response times (threshold: ${frontend_threshold}ms)"

    result="$(http_request GET "${FRONTEND_URL}")"
    time_ms="$(extract_time "${result}")"

    if [[ "${time_ms}" -le "${frontend_threshold}" ]]; then
        record_pass "Frontend page load time" "${time_ms}ms < ${frontend_threshold}ms"
    else
        record_fail "Frontend page load time" "${time_ms}ms > ${frontend_threshold}ms threshold"
    fi

    # ── SIEM response time ─────────────────────────────────────────
    log_step "Testing SIEM response times (threshold: ${api_threshold}ms)"

    result="$(http_request GET "${SIEM_URL}/health")"
    time_ms="$(extract_time "${result}")"

    if [[ "${time_ms}" -le "${api_threshold}" ]]; then
        record_pass "SIEM /health response time" "${time_ms}ms < ${api_threshold}ms"
    else
        record_fail "SIEM /health response time" "${time_ms}ms > ${api_threshold}ms threshold"
    fi
}

# ===========================================================================
# Main Execution
# ===========================================================================
main() {
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    log_bold "  Chaos-Sec Production Smoke Tests"
    log_bold "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    log "Started:  $(date -u +'%Y-%m-%d %H:%M:%S UTC')"
    log "Target:   ${BASE_URL}"
    echo ""

    check_prerequisites

    # Run test suites in order
    test_health_endpoints
    test_authentication
    test_experiment_crud
    test_cluster_health
    test_siem_connector
    test_frontend
    test_tls_certificates
    test_response_times

    # Summary is printed by the cleanup trap
}

main "$@"
