#!/usr/bin/env bash
# =============================================================================
# Chaos-Sec Security Scan Script
# =============================================================================
# Runs a comprehensive security analysis across the backend (Go) and frontend
# (Node.js) codebases. Produces a formatted report with pass/fail status for
# each scanner.
#
# Scanners:
#   1. gosec          – Go security scanner (https://github.com/securego/gosec)
#   2. go vet         – Go static analysis
#   3. golangci-lint  – Go meta-linter
#   4. npm audit      – Node.js dependency vulnerability audit
#
# Usage:
#   chmod +x scripts/security-scan.sh
#   ./scripts/security-scan.sh
#
# Environment variables:
#   BACKEND_DIR   – Path to the backend directory (default: ./backend)
#   FRONTEND_DIR  – Path to the frontend directory (default: ./frontend)
#   STRICT        – Exit with non-zero code on any failure (default: 1)
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BACKEND_DIR="${BACKEND_DIR:-${PROJECT_ROOT}/backend}"
FRONTEND_DIR="${FRONTEND_DIR:-${PROJECT_ROOT}/frontend}"
STRICT="${STRICT:-1}"

# Colours (disabled when stdout is not a terminal)
if [[ -t 1 ]]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    NC='\033[0m' # No Colour
else
    RED=''
    GREEN=''
    YELLOW=''
    CYAN=''
    BOLD=''
    NC=''
fi

# ---------------------------------------------------------------------------
# Helper functions
# ---------------------------------------------------------------------------

timestamp() {
    date -u +"%Y-%m-%dT%H:%M:%SZ"
}

print_header() {
    echo ""
    echo -e "${BOLD}${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo -e "${BOLD}${CYAN}  $1${NC}"
    echo -e "${BOLD}${CYAN}═══════════════════════════════════════════════════════════════${NC}"
    echo ""
}

print_section() {
    echo ""
    echo -e "${BOLD}── $1 ──${NC}"
    echo ""
}

print_result() {
    local name="$1"
    local status="$2"
    local details="${3:-}"

    if [[ "$status" == "PASS" ]]; then
        echo -e "  ${GREEN}✔ PASS${NC}  ${name}"
    elif [[ "$status" == "FAIL" ]]; then
        echo -e "  ${RED}✘ FAIL${NC}  ${name}"
    elif [[ "$status" == "WARN" ]]; then
        echo -e "  ${YELLOW}⚠ WARN${NC}  ${name}"
    else
        echo -e "  ${BOLD}● ${status}${NC}  ${name}"
    fi

    if [[ -n "$details" ]]; then
        echo -e "         ${details}"
    fi
}

check_tool() {
    if command -v "$1" &>/dev/null; then
        return 0
    else
        return 1
    fi
}

# ---------------------------------------------------------------------------
# Tracking results
# ---------------------------------------------------------------------------

declare -A SCAN_RESULTS
SCAN_PASS=0
SCAN_FAIL=0
SCAN_WARN=0
SCAN_SKIP=0
TOTAL_ISSUES=0

record_result() {
    local key="$1"
    local status="$2"
    local issues="${3:-0}"
    local detail="${4:-}"

    SCAN_RESULTS["${key}_status"]="$status"
    SCAN_RESULTS["${key}_issues"]="$issues"
    SCAN_RESULTS["${key}_detail"]="$detail"

    case "$status" in
        PASS) ((SCAN_PASS++)) ;;
        FAIL) ((SCAN_FAIL++)) ;;
        WARN) ((SCAN_WARN++)) ;;
        SKIP) ((SCAN_SKIP++)) ;;
    esac

    TOTAL_ISSUES=$((TOTAL_ISSUES + issues))
}

# ---------------------------------------------------------------------------
# Temporary files for capturing output
# ---------------------------------------------------------------------------

GOSEC_OUTPUT=$(mktemp /tmp/chaos-sec-gosec-XXXXXX.log)
GOVET_OUTPUT=$(mktemp /tmp/chaos-sec-govet-XXXXXX.log)
LINT_OUTPUT=$(mktemp /tmp/chaos-sec-lint-XXXXXX.log)
NPM_AUDIT_OUTPUT=$(mktemp /tmp/chaos-sec-npmaudit-XXXXXX.log)

cleanup() {
    rm -f "$GOSEC_OUTPUT" "$GOVET_OUTPUT" "$LINT_OUTPUT" "$NPM_AUDIT_OUTPUT"
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Start of report
# ---------------------------------------------------------------------------

print_header "Chaos-Sec Security Scan Report"
echo -e "  Timestamp : ${BOLD}$(timestamp)${NC}"
echo -e "  Project   : ${BOLD}${PROJECT_ROOT}${NC}"
echo -e "  Backend   : ${BACKEND_DIR}"
echo -e "  Frontend  : ${FRONTEND_DIR}"

# ---------------------------------------------------------------------------
# 1. gosec – Go Security Scanner
# ---------------------------------------------------------------------------

print_section "1 / 4 — gosec (Go Security Scanner)"

if check_tool gosec; then
    echo -e "  ${CYAN}Running:${NC} gosec ./...  (in ${BACKEND_DIR})"
    echo ""

    GOSEC_ISSUES=0
    GOSEC_EXIT_CODE=0

    # gosec exits with non-zero on findings; capture and count
    cd "$BACKEND_DIR"
    gosec -fmt=text -out="${GOSEC_OUTPUT}" -no-fail ./... 2>&1 || true

    if [[ -s "$GOSEC_OUTPUT" ]]; then
        # Count actual issue lines (skip header/summary lines)
        GOSEC_ISSUES=$(grep -cE '^\[' "${GOSEC_OUTPUT}" 2>/dev/null || echo 0)

        cat "${GOSEC_OUTPUT}"
        echo ""

        if [[ "$GOSEC_ISSUES" -gt 0 ]]; then
            record_result "gosec" "FAIL" "$GOSEC_ISSUES" "${GOSEC_ISSUES} security issue(s) found"
        else
            record_result "gosec" "PASS" 0 "No issues found"
        fi
    else
        record_result "gosec" "PASS" 0 "Clean — no issues detected"
    fi

    cd "$PROJECT_ROOT"
else
    echo -e "  ${YELLOW}gosec is not installed. Skipping.${NC}"
    echo -e "  ${YELLOW}Install with: go install github.com/securego/gosec/v2/cmd/gosec@latest${NC}"
    record_result "gosec" "SKIP" 0 "Tool not installed"
fi

# ---------------------------------------------------------------------------
# 2. go vet – Go Static Analysis
# ---------------------------------------------------------------------------

print_section "2 / 4 — go vet (Go Static Analysis)"

if check_tool go; then
    echo -e "  ${CYAN}Running:${NC} go vet ./...  (in ${BACKEND_DIR})"
    echo ""

    GOVET_ISSUES=0
    cd "$BACKEND_DIR"

    if go vet ./... >"${GOVET_OUTPUT}" 2>&1; then
        record_result "govet" "PASS" 0 "No issues found"
    else
        cat "${GOVET_OUTPUT}"
        echo ""
        GOVET_ISSUES=$(wc -l < "${GOVET_OUTPUT}" | tr -d ' ')
        if [[ "$GOVET_ISSUES" -eq 0 ]]; then
            GOVET_ISSUES=1
        fi
        record_result "govet" "FAIL" "$GOVET_ISSUES" "${GOVET_ISSUES} vet issue(s) found"
    fi

    cd "$PROJECT_ROOT"
else
    echo -e "  ${YELLOW}go is not installed. Skipping.${NC}"
    record_result "govet" "SKIP" 0 "Tool not installed"
fi

# ---------------------------------------------------------------------------
# 3. golangci-lint – Go Meta-Linter
# ---------------------------------------------------------------------------

print_section "3 / 4 — golangci-lint (Go Meta-Linter)"

if check_tool golangci-lint; then
    echo -e "  ${CYAN}Running:${NC} golangci-lint run ./...  (in ${BACKEND_DIR})"
    echo ""

    LINT_ISSUES=0
    cd "$BACKEND_DIR"

    if golangci-lint run ./... >"${LINT_OUTPUT}" 2>&1; then
        record_result "golangci-lint" "PASS" 0 "No issues found"
    else
        cat "${LINT_OUTPUT}"
        echo ""
        # Each finding is typically one line
        LINT_ISSUES=$(grep -cE '[A-Za-z]+\.go' "${LINT_OUTPUT}" 2>/dev/null || echo 0)
        if [[ "$LINT_ISSUES" -eq 0 ]]; then
            LINT_ISSUES=$(wc -l < "${LINT_OUTPUT}" | tr -d ' ')
            if [[ "$LINT_ISSUES" -eq 0 ]]; then
                LINT_ISSUES=1
            fi
        fi
        record_result "golangci-lint" "FAIL" "$LINT_ISSUES" "${LINT_ISSUES} lint issue(s) found"
    fi

    cd "$PROJECT_ROOT"
else
    echo -e "  ${YELLOW}golangci-lint is not installed. Skipping.${NC}"
    echo -e "  ${YELLOW}Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest${NC}"
    record_result "golangci-lint" "SKIP" 0 "Tool not installed"
fi

# ---------------------------------------------------------------------------
# 4. npm audit – Frontend Dependency Audit
# ---------------------------------------------------------------------------

print_section "4 / 4 — npm audit (Frontend Dependency Audit)"

if check_tool npm; then
    if [[ -d "${FRONTEND_DIR}" ]]; then
        echo -e "  ${CYAN}Running:${NC} npm audit  (in ${FRONTEND_DIR})"
        echo ""

        cd "$FRONTEND_DIR"

        NPM_ISSUES=0
        if npm audit --production >"${NPM_AUDIT_OUTPUT}" 2>&1; then
            cat "${NPM_AUDIT_OUTPUT}"
            echo ""
            record_result "npm-audit" "PASS" 0 "No known vulnerabilities"
        else
            cat "${NPM_AUDIT_OUTPUT}"
            echo ""

            # Try to parse the vulnerability counts from npm audit output
            NPM_VULN_LINE=$(grep -E 'found [0-9]+ vulnerabilities' "${NPM_AUDIT_OUTPUT}" 2>/dev/null | head -1 || true)
            if [[ -n "$NPM_VULN_LINE" ]]; then
                NPM_ISSUES=$(echo "$NPM_VULN_LINE" | grep -oE '[0-9]+' | head -1 || echo 1)
            else
                NPM_ISSUES=1
            fi

            if [[ "$NPM_ISSUES" -eq 0 ]]; then
                record_result "npm-audit" "WARN" 0 "Audit exited non-zero but no vuln count parsed"
            else
                record_result "npm-audit" "FAIL" "$NPM_ISSUES" "${NPM_ISSUES} vulnerability/vulnerabilities found"
            fi
        fi

        cd "$PROJECT_ROOT"
    else
        echo -e "  ${YELLOW}Frontend directory not found at ${FRONTEND_DIR}. Skipping.${NC}"
        record_result "npm-audit" "SKIP" 0 "Frontend directory not found"
    fi
else
    echo -e "  ${YELLOW}npm is not installed. Skipping.${NC}"
    record_result "npm-audit" "SKIP" 0 "Tool not installed"
fi

# ---------------------------------------------------------------------------
# Summary Report
# ---------------------------------------------------------------------------

print_header "Security Scan Summary"

echo -e "  ${BOLD}Scan Results:${NC}"
echo ""

for key in gosec govet golangci-lint npm-audit; do
    status="${SCAN_RESULTS[${key}_status]:-SKIP}"
    issues="${SCAN_RESULTS[${key}_issues]:-0}"
    detail="${SCAN_RESULTS[${key}_detail]:-}"

    label=""
    case "$key" in
        gosec)       label="gosec" ;;
        govet)       label="go vet" ;;
        golangci-lint) label="golangci-lint" ;;
        npm-audit)   label="npm audit" ;;
    esac

    print_result "$label" "$status" "$detail"
done

echo ""
echo -e "  ${BOLD}─────────────────────────────────────────────────${NC}"
echo ""

TOTAL_SCANS=$((SCAN_PASS + SCAN_FAIL + SCAN_WARN + SCAN_SKIP))

echo -e "  Total scans   : ${BOLD}${TOTAL_SCANS}${NC}"
echo -e "  Passed        : ${GREEN}${SCAN_PASS}${NC}"
echo -e "  Failed        : ${RED}${SCAN_FAIL}${NC}"
echo -e "  Warnings      : ${YELLOW}${SCAN_WARN}${NC}"
echo -e "  Skipped       : ${SCAN_SKIP}"
echo -e "  Total issues  : ${BOLD}${TOTAL_ISSUES}${NC}"

echo ""
echo -e "  ${BOLD}─────────────────────────────────────────────────${NC}"
echo ""

if [[ "$SCAN_FAIL" -gt 0 ]]; then
    echo -e "  ${RED}${BOLD}✘ SECURITY SCAN FAILED — ${SCAN_FAIL} scanner(s) reported issues${NC}"
    OVERALL_EXIT=1
elif [[ "$SCAN_WARN" -gt 0 ]]; then
    echo -e "  ${YELLOW}${BOLD}⚠ SECURITY SCAN PASSED WITH WARNINGS${NC}"
    OVERALL_EXIT=0
elif [[ "$SCAN_SKIP" -gt 0 ]]; then
    echo -e "  ${YELLOW}${BOLD}⚠ SECURITY SCAN INCOMPLETE — ${SCAN_SKIP} scanner(s) were skipped${NC}"
    OVERALL_EXIT=0
else
    echo -e "  ${GREEN}${BOLD}✔ ALL SECURITY SCANS PASSED${NC}"
    OVERALL_EXIT=0
fi

echo ""
echo -e "  Report generated at $(timestamp)"
echo ""

# ---------------------------------------------------------------------------
# Exit handling
# ---------------------------------------------------------------------------

if [[ "${STRICT}" == "1" && "$SCAN_FAIL" -gt 0 ]]; then
    exit "$OVERALL_EXIT"
fi

exit 0
