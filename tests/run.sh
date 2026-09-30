#!/usr/bin/env bash
# Aura Automated Test Runner
#
# Usage:
#   tests/run.sh [suite...]
#
# Examples:
#   tests/run.sh                     # run all suites
#   tests/run.sh config              # run config suite only
#   tests/run.sh config hooks        # run config + hooks suites
#
# Environment:
#   AURA_BIN    - path to aura binary (default: ./aura)
#   TEST_DIR    - base directory for test artifacts (default: /tmp/aura-tests)
#   VERBOSE     - set to 1 for verbose output

set -euo pipefail
trap 'echo -e "\n\033[1;31mInterrupted.\033[0m"; exit 130' INT

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
AURA_BIN="${AURA_BIN:-/tmp/aura-build/aura}"
TEST_DIR="${TEST_DIR:-/tmp/aura-tests}"
VERBOSE="${VERBOSE:-0}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

# Counters
TOTAL_PASS=0
TOTAL_FAIL=0
SUITE_PASS=0
SUITE_FAIL=0
FAILURES=""
FINDINGS=""

# ─── Test Framework Functions ────────────────────────────────────────────────

log() { echo -e "${BLUE}[test]${NC} $*"; }
verbose() { [[ "$VERBOSE" == "1" ]] && echo -e "       $*" || true; }
suite_header() {
    SUITE_PASS=0; SUITE_FAIL=0
    echo ""
    echo -e "${BOLD}━━━ Suite: $1 ━━━${NC}"
}
suite_footer() {
    local name="$1"
    echo -e "    ${GREEN}$SUITE_PASS passed${NC}, ${RED}$SUITE_FAIL failed${NC}"
}

pass() {
    local name="$1"
    TOTAL_PASS=$((TOTAL_PASS + 1))
    SUITE_PASS=$((SUITE_PASS + 1))
    echo -e "  ${GREEN}✓${NC} $name"
}

fail() {
    local name="$1"; shift
    local reason="${*:-}"
    TOTAL_FAIL=$((TOTAL_FAIL + 1))
    SUITE_FAIL=$((SUITE_FAIL + 1))
    echo -e "  ${RED}✗${NC} $name"
    [[ -n "$reason" ]] && echo -e "    ${RED}→ $reason${NC}"
    FAILURES+="  FAIL: $name — $reason\n"
}

finding() {
    local severity="$1" desc="$2"
    FINDINGS+="  [$severity] $desc\n"
    echo -e "  ${YELLOW}⚑${NC} Finding [$severity]: $desc"
}

# ─── Assertion Helpers ───────────────────────────────────────────────────────

# assert_file_exists <test_name> <path>
assert_file_exists() {
    if [[ -f "$2" ]]; then pass "$1"; else fail "$1" "file not found: $2"; fi
}

# assert_file_not_exists <test_name> <path>
assert_file_not_exists() {
    if [[ ! -f "$2" ]]; then pass "$1"; else fail "$1" "file should not exist: $2"; fi
}

# assert_file_not_empty <test_name> <path>
assert_file_not_empty() {
    if [[ -f "$2" ]] && [[ -s "$2" ]]; then pass "$1"; else fail "$1" "file empty or not found: $2"; fi
}

# assert_file_contains <test_name> <path> <pattern>
assert_file_contains() {
    if [[ -f "$2" ]] && grep -q "$3" "$2" 2>/dev/null; then
        pass "$1"
    else
        fail "$1" "pattern '$3' not found in $2"
    fi
}

# assert_file_not_contains <test_name> <path> <pattern>
assert_file_not_contains() {
    if [[ -f "$2" ]] && grep -q "$3" "$2" 2>/dev/null; then
        fail "$1" "pattern '$3' should not be in $2"
    else
        pass "$1"
    fi
}

# assert_file_contains_count <test_name> <path> <pattern> <min_count>
assert_file_contains_count() {
    local count
    count=$(grep -c "$3" "$2" 2>/dev/null || echo 0)
    if [[ "$count" -ge "$4" ]]; then
        pass "$1 (found $count, need >=$4)"
    else
        fail "$1" "pattern '$3' found $count times, expected >=$4"
    fi
}

# assert_json <test_name> <json_file> <jq_filter> <expected_pattern>
assert_json() {
    local result
    result=$(jq -r "$3" "$2" 2>/dev/null || echo "JQ_ERROR")
    if echo "$result" | grep -q "$4" 2>/dev/null; then
        pass "$1"
    else
        fail "$1" "jq '$3' returned '$result', expected match for '$4'"
    fi
}

# assert_json_gt <test_name> <json_file> <jq_filter> <min_value>
assert_json_gt() {
    local result
    result=$(jq -r "$3" "$2" 2>/dev/null || echo "0")
    if [[ "$result" -gt "$4" ]] 2>/dev/null; then
        pass "$1 (got $result)"
    else
        fail "$1" "jq '$3' returned '$result', expected > $4"
    fi
}

# assert_exit_code <test_name> <expected_code> <actual_code>
assert_exit_code() {
    if [[ "$3" == "$2" ]]; then pass "$1"; else fail "$1" "exit code $3, expected $2"; fi
}

# assert_cmd_succeeds <test_name> <cmd...>
assert_cmd_succeeds() {
    local name="$1"; shift
    if "$@" >/dev/null 2>&1; then pass "$name"; else fail "$name" "command failed: $*"; fi
}

# ─── Test Environment Setup ─────────────────────────────────────────────────

# setup_env <test_name> [overlay...]
# Creates isolated test environment, returns path in $ENV_DIR
setup_env() {
    local name="$1"; shift
    ENV_DIR="$TEST_DIR/$name"
    rm -rf "$ENV_DIR"
    mkdir -p "$ENV_DIR"

    # Copy base fixtures
    cp -r "$SCRIPT_DIR/fixtures/base/.aura" "$ENV_DIR/.aura"

    # Apply overlays
    for overlay in "$@"; do
        local overlay_dir="$SCRIPT_DIR/fixtures/$overlay"
        if [[ -d "$overlay_dir/.aura" ]]; then
            cp -r "$overlay_dir/.aura/"* "$ENV_DIR/.aura/" 2>/dev/null || true
        fi
    done

    # Create plugin directories for install_plugin to copy into
    if [[ -d "$SCRIPT_DIR/plugins" ]]; then
        mkdir -p "$ENV_DIR/.aura/plugins/tools" "$ENV_DIR/.aura/plugins/injectors" "$ENV_DIR/.aura/plugins/diagnostics"
    fi

    verbose "env: $ENV_DIR"
}

# install_plugin <env_dir> <category> <plugin_source_dir>
install_plugin() {
    local env="$1" category="$2" src="$3"
    local name
    name=$(basename "$src")
    cp -r "$src" "$env/.aura/plugins/$category/$name"
}

# run_aura <args...>
# Runs aura in $ENV_DIR with standard test flags.
# Use only the suite's fixtures, independent of the developer's global config.
run_aura() {
    (cd "${ENV_DIR}" && "${AURA_BIN}" --home "" --config "${ENV_DIR}/.aura" "$@")
}

# show_aura <args...>
# Runs aura --show and writes output to $SHOW_OUT_FILE (file-based).
# Use grep_show "pattern" to check the output.
# Avoids bash $() variable capture which can silently truncate on null bytes.
SHOW_OUT_FILE=""
show_aura() {
    SHOW_OUT_FILE="$ENV_DIR/.show-output.txt"
    (cd "${ENV_DIR}" && "${AURA_BIN}" --home "" --config "${ENV_DIR}/.aura" --show "$@") > "${SHOW_OUT_FILE}" 2>&1 || true
}

# grep_show [flags] <pattern>
# Greps the last show_aura output file. Returns 0 if found.
# Supports grep flags (e.g., grep_show -i "pattern", grep_show -qi "pattern").
grep_show() {
    grep -q "$@" "$SHOW_OUT_FILE" 2>/dev/null
}

# run_aura_capture <output_file> <args...>
# Runs aura in $ENV_DIR with --debug --output, captures everything.
# Sets LAST_EXIT to the exit code (0=ok, 124=timeout, other=crash).
run_aura_capture() {
    local outfile="$1"; shift
    local args=("$@")
    LAST_EXIT=0
    (cd "${ENV_DIR}" && timeout 120 "${AURA_BIN}" --home "" --config "${ENV_DIR}/.aura" --debug --output "${outfile}" "${args[@]}") > /dev/null 2>&1 || LAST_EXIT=$?
}

# run_aura_bg <timeout_secs> <args...>
# Runs aura with a timeout, captures stdout+stderr.
# Sets LAST_EXIT to the exit code (0=ok, 124=timeout, other=crash).
run_aura_bg() {
    local timeout_secs="$1"; shift
    local args=("$@")
    LAST_EXIT=0
    (cd "${ENV_DIR}" && timeout "${timeout_secs}" "${AURA_BIN}" --home "" --config "${ENV_DIR}/.aura" "${args[@]}") 2>&1 || LAST_EXIT=$?
}

# assert_aura_ok <test_name>
# Fails if LAST_EXIT indicates a crash (non-zero, non-timeout).
# Timeout (124) is not a crash — the test may still be valid.
assert_aura_ok() {
    if [[ "${LAST_EXIT:-0}" -ne 0 ]] && [[ "${LAST_EXIT:-0}" -ne 124 ]]; then
        fail "$1" "aura exited with code $LAST_EXIT (crash)"
    fi
}

# ─── Test Suites ─────────────────────────────────────────────────────────────

source_suite() {
    local suite="$1"
    local suite_file="$SCRIPT_DIR/suites/${suite}.sh"
    if [[ -f "$suite_file" ]]; then
        source "$suite_file"
    else
        echo -e "${RED}Suite not found: $suite_file${NC}"
        return 1
    fi
}

# ─── Main ────────────────────────────────────────────────────────────────────

main() {
    echo -e "${BOLD}Aura Automated Test Runner${NC}"
    echo -e "Binary: $AURA_BIN"
    echo -e "Artifacts: $TEST_DIR"
    echo ""

    # Verify binary
    if [[ ! -x "$AURA_BIN" ]]; then
        echo -e "${RED}ERROR: aura binary not found at $AURA_BIN${NC}"
        echo "Build it: go build -o /tmp/aura-build/aura ."
        exit 1
    fi

    # Clean up previous run (only clean specific suite dirs, not all)
    mkdir -p "$TEST_DIR"

    # Determine which suites to run
    local suites=("$@")
    if [[ ${#suites[@]} -eq 0 ]]; then
        # Run all suites in order
        suites=()
        for f in "$SCRIPT_DIR/suites/"*.sh; do
            [[ -f "$f" ]] && suites+=("$(basename "$f" .sh)")
        done
    fi

    # Run suites
    for suite in "${suites[@]}"; do
        source_suite "$suite"
    done

    # Summary
    echo ""
    echo -e "${BOLD}━━━ Summary ━━━${NC}"
    echo -e "  ${GREEN}$TOTAL_PASS passed${NC}"
    echo -e "  ${RED}$TOTAL_FAIL failed${NC}"

    if [[ -n "$FAILURES" ]]; then
        echo ""
        echo -e "${RED}Failures:${NC}"
        echo -e "$FAILURES"
    fi

    if [[ -n "$FINDINGS" ]]; then
        echo ""
        echo -e "${YELLOW}Findings:${NC}"
        echo -e "$FINDINGS"
    fi

    # Write results to file
    cat > "$TEST_DIR/results.txt" << EOF
Aura Test Results
=================
Passed:  $TOTAL_PASS
Failed:  $TOTAL_FAIL

Failures:
$(echo -e "$FAILURES")

Findings:
$(echo -e "$FINDINGS")
EOF

    [[ "$TOTAL_FAIL" -gt 0 ]] && exit 1 || exit 0
}

main "$@"
