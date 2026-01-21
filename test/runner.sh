#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
log_pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
log_fail() { echo -e "${RED}[FAIL]${NC} $1"; }

# Parse arguments
TEST_TYPE=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --type=*) TEST_TYPE="${1#*=}"; shift ;;
    *) shift ;;
  esac
done

CURRENT_DIR=$(pwd)
log_info "Test Runner started..."
log_info "Workdir: $CURRENT_DIR"
log_info "Test type: ${TEST_TYPE:-not specified}"

# ============ COMPOSITION TESTS ============
run_composition_tests() {
  source /workspace/test/lib.sh
  source /workspace/test/mock_lib.sh

  TEST_FILES=$(find . -maxdepth 1 -name "test_*.sh" | sort)
  if [ -z "$TEST_FILES" ]; then
    log_info "No test_*.sh files found. Skipping..."
    return 0
  fi

  FAILED_TESTS=0
  for test_file in $TEST_FILES; do
    echo -e "\nRunning Suite: $test_file..."
    ( set -e; source "$test_file" )
    EXIT_CODE=$?

    if [ $EXIT_CODE -eq 0 ]; then
      log_pass "Suite: $test_file"
    else
      log_fail "Suite: $test_file"
      FAILED_TESTS=$((FAILED_TESTS+1))
    fi
    cleanup_test 2>/dev/null || true
  done

  return $FAILED_TESTS
}

# ============ FUNCTION TESTS ============
run_function_tests() {
  log_info "Running Go unit tests..."
  export CGO_ENABLED=1

  if go test -v ./...; then
    log_pass "Go tests"
    return 0
  else
    log_fail "Go tests"
    return 1
  fi
}

# ============ HELM TESTS ============
run_helm_tests() {
  local helm_path="${1:-.}"
  local CRD_CATALOG="https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json"

  log_info "Running Helm lint..."
  if ! helm lint "$helm_path"; then
    log_fail "Helm lint"
    return 1
  fi
  log_pass "Helm lint"

  log_info "Running Kubeconform validation..."
  if helm template test-release "$helm_path" | kubeconform -verbose -summary \
      -ignore-missing-schemas \
      -schema-location default \
      -schema-location "$CRD_CATALOG"; then
    log_pass "Kubeconform validation"
    return 0
  else
    log_fail "Kubeconform validation"
    return 1
  fi
}

# ============ MAIN ============
case "$TEST_TYPE" in
  composition) run_composition_tests ;;
  function)    run_function_tests ;;
  helm)        run_helm_tests ;;
  *)
    echo "Usage: runner.sh --type=<composition|function|helm>"
    exit 1
    ;;
esac
