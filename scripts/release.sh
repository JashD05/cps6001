#!/usr/bin/env bash
# =============================================================================
# Chaos-Sec Release Preparation Script
# =============================================================================
# Runs the full CI pipeline and builds a release archive ready for deployment.
#
# Steps:
#   1. Run backend tests          (go test ./...)
#   2. Run frontend tests         (npm run test)
#   3. Run Go linter              (golangci-lint run)
#   4. Run frontend linter         (npm run lint)
#   5. Run security scan           (./scripts/security-scan.sh)
#   6. Build backend binary        (go build -o bin/backend ./cmd/backend)
#   7. Build frontend              (npm run build)
#   8. Generate version info        (from git tag/commit)
#   9. Create release archive       (tar.gz)
#  10. Output SHA256 checksums
#  11. Print release summary
#
# Usage:
#   chmod +x scripts/release.sh
#   ./scripts/release.sh
#
# Environment variables:
#   BACKEND_DIR    – Path to the backend directory   (default: ./backend)
#   FRONTEND_DIR   – Path to the frontend directory  (default: ./frontend)
#   SKIP_TESTS     – Set to "1" to skip test steps
#   SKIP_LINT      – Set to "1" to skip lint steps
#   SKIP_SECURITY  – Set to "1" to skip security scan
#   OUTPUT_DIR     – Directory for build artefacts   (default: ./dist)
# =============================================================================

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

BACKEND_DIR="${BACKEND_DIR:-${PROJECT_ROOT}/backend}"
FRONTEND_DIR="${FRONTEND_DIR:-${PROJECT_ROOT}/frontend}"
OUTPUT_DIR="${OUTPUT_DIR:-${PROJECT_ROOT}/dist}"
SKIP_TESTS="${SKIP_TESTS:-0}"
SKIP_LINT="${SKIP_LINT:-0}"
SKIP_SECURITY="${SKIP_SECURITY:-0}"

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

print_step() {
    echo -e "  ${CYAN}▶${NC} $1"
}

print_success() {
    echo -e "  ${GREEN}✔${NC} $1"
}

print_fail() {
    echo -e "  ${RED}✘${NC} $1"
}

print_warn() {
    echo -e "  ${YELLOW}⚠${NC} $1"
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

STEPS_PASSED=0
STEPS_FAILED=0
STEPS_SKIPPED=0
OVERALL_EXIT=0

record_pass() {
    ((STEPS_PASSED++))
    print_success "$1"
}

record_fail() {
    ((STEPS_FAILED++))
    OVERALL_EXIT=1
    print_fail "$1"
}

record_skip() {
    ((STEPS_SKIPPED++))
    print_warn "$1 — skipped"
}

# ---------------------------------------------------------------------------
# Version information
# ---------------------------------------------------------------------------

VERSION=""
COMMIT=""
BRANCH=""
BUILD_DATE=""

generate_version_info() {
    print_section "Generating version info"

    # Determine version from git tags, falling back to commit hash.
    if git describe --tags --exact-match 2>/dev/null; then
        VERSION="$(git describe --tags --exact-match 2>/dev/null)"
    elif git describe --tags 2>/dev/null; then
        VERSION="$(git describe --tags 2>/dev/null)"
    else
        VERSION="v0.0.0-dev"
    fi

    COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")"
    BRANCH="$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "unknown")"
    BUILD_DATE="$(timestamp)"

    print_success "Version   : ${VERSION}"
    print_success "Commit     : ${COMMIT}"
    print_success "Branch     : ${BRANCH}"
    print_success "Build date : ${BUILD_DATE}"
}

# ---------------------------------------------------------------------------
# Step 1: Backend tests
# ---------------------------------------------------------------------------

step_backend_tests() {
    print_section "1 / 7 — Backend Tests (Go)"

    if [[ "${SKIP_TESTS}" == "1" ]]; then
        record_skip "Backend tests"
        return
    fi

    if ! check_tool go; then
        record_fail "Backend tests — go not found"
        return
    fi

    print_step "Running: go test ./... (in ${BACKEND_DIR})"
    if (cd "${BACKEND_DIR}" && go test ./...); then
        record_pass "Backend tests passed"
    else
        record_fail "Backend tests failed"
    fi
}

# ---------------------------------------------------------------------------
# Step 2: Frontend tests
# ---------------------------------------------------------------------------

step_frontend_tests() {
    print_section "2 / 7 — Frontend Tests (npm)"

    if [[ "${SKIP_TESTS}" == "1" ]]; then
        record_skip "Frontend tests"
        return
    fi

    if ! check_tool npm; then
        record_fail "Frontend tests — npm not found"
        return
    fi

    if [[ ! -d "${FRONTEND_DIR}" ]]; then
        record_fail "Frontend tests — directory not found: ${FRONTEND_DIR}"
        return
    fi

    print_step "Running: npm run test (in ${FRONTEND_DIR})"
    if (cd "${FRONTEND_DIR}" && npm run test); then
        record_pass "Frontend tests passed"
    else
        record_fail "Frontend tests failed"
    fi
}

# ---------------------------------------------------------------------------
# Step 3: Lint checks
# ---------------------------------------------------------------------------

step_lint() {
    print_section "3 / 7 — Lint Checks"

    if [[ "${SKIP_LINT}" == "1" ]]; then
        record_skip "Lint checks"
        return
    fi

    # Go lint
    if check_tool golangci-lint; then
        print_step "Running: golangci-lint run (in ${BACKEND_DIR})"
        if (cd "${BACKEND_DIR}" && golangci-lint run ./...); then
            record_pass "Go lint passed"
        else
            record_fail "Go lint failed"
        fi
    else
        record_skip "golangci-lint not installed"
    fi

    # Frontend lint
    if check_tool npm && [[ -d "${FRONTEND_DIR}" ]]; then
        print_step "Running: npm run lint (in ${FRONTEND_DIR})"
        if (cd "${FRONTEND_DIR}" && npm run lint); then
            record_pass "Frontend lint passed"
        else
            record_fail "Frontend lint failed"
        fi
    else
        record_skip "npm or frontend directory not available"
    fi
}

# ---------------------------------------------------------------------------
# Step 4: Security scan
# ---------------------------------------------------------------------------

step_security_scan() {
    print_section "4 / 7 — Security Scan"

    if [[ "${SKIP_SECURITY}" == "1" ]]; then
        record_skip "Security scan"
        return
    fi

    local scan_script="${PROJECT_ROOT}/scripts/security-scan.sh"
    if [[ ! -f "${scan_script}" ]]; then
        record_fail "Security scan — script not found: ${scan_script}"
        return
    fi

    print_step "Running: ${scan_script}"
    if bash "${scan_script}"; then
        record_pass "Security scan passed"
    else
        record_fail "Security scan failed"
    fi
}

# ---------------------------------------------------------------------------
# Step 5: Build backend binary
# ---------------------------------------------------------------------------

step_build_backend() {
    print_section "5 / 7 — Build Backend Binary"

    if ! check_tool go; then
        record_fail "Backend build — go not found"
        return
    fi

    local bin_dir="${OUTPUT_DIR}/bin"
    mkdir -p "${bin_dir}"

    print_step "Running: go build -o ${bin_dir}/backend ./cmd/backend (in ${BACKEND_DIR})"

    # Inject version info via ldflags.
    local ldflags=""
    ldflags="${ldflags} -X main.Version=${VERSION}"
    ldflags="${ldflags} -X main.Commit=${COMMIT}"
    ldflags="${ldflags} -X main.Branch=${BRANCH}"
    ldflags="${ldflags} -X main.BuildDate=${BUILD_DATE}"

    if (cd "${BACKEND_DIR}" && go build -ldflags "${ldflags}" -o "${bin_dir}/backend" ./cmd/backend); then
        record_pass "Backend binary built → ${bin_dir}/backend"
    else
        record_fail "Backend build failed"
        return
    fi

    # Print binary size.
    local bin_size
    bin_size="$(du -h "${bin_dir}/backend" | cut -f1)"
    print_step "Binary size: ${bin_size}"
}

# ---------------------------------------------------------------------------
# Step 6: Build frontend
# ---------------------------------------------------------------------------

step_build_frontend() {
    print_section "6 / 7 — Build Frontend"

    if ! check_tool npm; then
        record_fail "Frontend build — npm not found"
        return
    fi

    if [[ ! -d "${FRONTEND_DIR}" ]]; then
        record_fail "Frontend build — directory not found: ${FRONTEND_DIR}"
        return
    fi

    print_step "Running: npm run build (in ${FRONTEND_DIR})"
    if (cd "${FRONTEND_DIR}" && npm run build); then
        record_pass "Frontend built"
    else
        record_fail "Frontend build failed"
        return
    fi
}

# ---------------------------------------------------------------------------
# Step 7: Create release archive and checksums
# ---------------------------------------------------------------------------

step_create_release() {
    print_section "7 / 7 — Create Release Archive"

    if ! check_tool tar; then
        record_fail "Release archive — tar not found"
        return
    fi

    if ! check_tool sha256sum; then
        record_fail "Release archive — sha256sum not found"
        return
    fi

    mkdir -p "${OUTPUT_DIR}"

    local archive_name="chaos-sec-${VERSION}"
    local staging_dir="${OUTPUT_DIR}/${archive_name}"
    mkdir -p "${staging_dir}"

    # --- Copy backend binary ---
    local bin_path="${OUTPUT_DIR}/bin/backend"
    if [[ -f "${bin_path}" ]]; then
        mkdir -p "${staging_dir}/bin"
        cp "${bin_path}" "${staging_dir}/bin/backend"
        print_step "Copied backend binary"
    else
        print_warn "Backend binary not found — skipping"
    fi

    # --- Copy frontend dist ---
    local frontend_dist="${FRONTEND_DIR}/dist"
    if [[ -d "${frontend_dist}" ]]; then
        mkdir -p "${staging_dir}/frontend"
        cp -r "${frontend_dist}/" "${staging_dir}/frontend/"
        print_step "Copied frontend dist"
    else
        print_warn "Frontend dist not found — skipping"
    fi

    # --- Copy migrations ---
    local migrations_dir="${BACKEND_DIR}/migrations"
    if [[ -d "${migrations_dir}" ]]; then
        mkdir -p "${staging_dir}/migrations"
        cp -r "${migrations_dir}/" "${staging_dir}/migrations/"
        print_step "Copied database migrations"
    else
        print_warn "Migrations directory not found — skipping"
    fi

    # --- Copy docs ---
    local docs_dir="${PROJECT_ROOT}/docs"
    if [[ -d "${docs_dir}" ]]; then
        mkdir -p "${staging_dir}/docs"
        cp -r "${docs_dir}/" "${staging_dir}/docs/"
        print_step "Copied documentation"
    else
        print_warn "Docs directory not found — skipping"
    fi

    # --- Copy scripts ---
    local scripts_dir="${PROJECT_ROOT}/scripts"
    if [[ -d "${scripts_dir}" ]]; then
        mkdir -p "${staging_dir}/scripts"
        cp -r "${scripts_dir}/" "${staging_dir}/scripts/"
        print_step "Copied scripts"
    else
        print_warn "Scripts directory not found — skipping"
    fi

    # --- Generate version file ---
    cat > "${staging_dir}/VERSION" <<EOF
version:    ${VERSION}
commit:     ${COMMIT}
branch:     ${BRANCH}
build_date: ${BUILD_DATE}
EOF
    print_step "Generated VERSION file"

    # --- Create tar.gz archive ---
    local archive_path="${OUTPUT_DIR}/${archive_name}.tar.gz"
    print_step "Creating archive: ${archive_path}"

    (cd "${OUTPUT_DIR}" && tar -czf "${archive_name}.tar.gz" "${archive_name}/")

    if [[ -f "${archive_path}" ]]; then
        record_pass "Release archive created → ${archive_path}"
    else
        record_fail "Failed to create release archive"
        return
    fi

    # --- Generate SHA256 checksums ---
    print_step "Generating SHA256 checksums"

    local checksum_file="${OUTPUT_DIR}/${archive_name}.sha256"
    (cd "${OUTPUT_DIR}" && sha256sum "${archive_name}.tar.gz" > "${archive_name}.sha256")

    if [[ -f "${checksum_file}" ]]; then
        record_pass "Checksums generated → ${checksum_file}"
        echo ""
        echo -e "  ${BOLD}SHA256 Checksum:${NC}"
        cat "${checksum_file}" | sed 's/^/  /'
    else
        record_fail "Failed to generate checksums"
        return
    fi

    # Clean up staging directory.
    rm -rf "${staging_dir}"
    print_step "Cleaned up staging directory"
}

# ---------------------------------------------------------------------------
# Release summary
# ---------------------------------------------------------------------------

print_summary() {
    print_header "Chaos-Sec Release Summary"

    echo -e "  ${BOLD}Version     :${NC}  ${VERSION}"
    echo -e "  ${BOLD}Commit      :${NC}  ${COMMIT}"
    echo -e "  ${BOLD}Branch      :${NC}  ${BRANCH}"
    echo -e "  ${BOLD}Build date  :${NC}  ${BUILD_DATE}"
    echo ""
    echo -e "  ${BOLD}─────────────────────────────────────────────────${NC}"
    echo ""

    TOTAL=$((STEPS_PASSED + STEPS_FAILED + STEPS_SKIPPED))

    echo -e "  ${BOLD}Pipeline Results:${NC}"
    echo -e "    Total steps   : ${TOTAL}"
    echo -e "    Passed        : ${GREEN}${STEPS_PASSED}${NC}"
    echo -e "    Failed        : ${RED}${STEPS_FAILED}${NC}"
    echo -e "    Skipped       : ${YELLOW}${STEPS_SKIPPED}${NC}"
    echo ""
    echo -e "  ${BOLD}─────────────────────────────────────────────────${NC}"
    echo ""

    # --- Archive details ---
    local archive_name="chaos-sec-${VERSION}"
    local archive_path="${OUTPUT_DIR}/${archive_name}.tar.gz"

    if [[ -f "${archive_path}" ]]; then
        local archive_size
        archive_size="$(du -h "${archive_path}" | cut -f1)"
        echo -e "  ${BOLD}Release Archive:${NC}"
        echo -e "    File       : ${archive_path}"
        echo -e "    Size       : ${archive_size}"
        echo ""
    fi

    # --- Binary size ---
    local bin_path="${OUTPUT_DIR}/bin/backend"
    if [[ -f "${bin_path}" ]]; then
        local bin_size
        bin_size="$(du -h "${bin_path}" | cut -f1)"
        echo -e "  ${BOLD}Backend Binary:${NC}"
        echo -e "    File       : ${bin_path}"
        echo -e "    Size       : ${bin_size}"
        echo ""
    fi

    # --- Frontend dist size ---
    local frontend_dist="${FRONTEND_DIR}/dist"
    if [[ -d "${frontend_dist}" ]]; then
        local dist_size
        dist_size="$(du -sh "${frontend_dist}" | cut -f1)"
        echo -e "  ${BOLD}Frontend Dist:${NC}"
        echo -e "    Path       : ${frontend_dist}"
        echo -e "    Size       : ${dist_size}"
        echo ""
    fi

    # --- Checksum file ---
    local checksum_file="${OUTPUT_DIR}/${archive_name}.sha256"
    if [[ -f "${checksum_file}" ]]; then
        echo -e "  ${BOLD}Checksums:${NC}"
        cat "${checksum_file}" | sed 's/^/    /'
        echo ""
    fi

    echo -e "  ${BOLD}─────────────────────────────────────────────────${NC}"
    echo ""

    if [[ "${STEPS_FAILED}" -gt 0 ]]; then
        echo -e "  ${RED}${BOLD}✘ RELEASE PIPELINE FAILED — ${STEPS_FAILED} step(s) reported errors${NC}"
        echo -e "  ${RED}${BOLD}  Review the output above before deploying.${NC}"
    else
        echo -e "  ${GREEN}${BOLD}✔ RELEASE PIPELINE PASSED${NC}"
        echo -e "  ${GREEN}${BOLD}  Archive is ready for deployment.${NC}"
    fi

    echo ""
    echo -e "  Report generated at $(timestamp)"
    echo ""
}

# ---------------------------------------------------------------------------
# Cleanup
# ---------------------------------------------------------------------------

cleanup() {
    # Remove the staging directory if it still exists (e.g., on early exit).
    if [[ -n "${VERSION:-}" ]]; then
        rm -rf "${OUTPUT_DIR}/chaos-sec-${VERSION}" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# ---------------------------------------------------------------------------
# Main execution
# ---------------------------------------------------------------------------

print_header "Chaos-Sec Release Candidate Builder"
echo -e "  Timestamp : ${BOLD}$(timestamp)${NC}"
echo -e "  Project   : ${BOLD}${PROJECT_ROOT}${NC}"
echo -e "  Backend   : ${BACKEND_DIR}"
echo -e "  Frontend  : ${FRONTEND_DIR}"
echo -e "  Output    : ${OUTPUT_DIR}"

generate_version_info
step_backend_tests
step_frontend_tests
step_lint
step_security_scan
step_build_backend
step_build_frontend
step_create_release

print_summary

exit "${OVERALL_EXIT}"
