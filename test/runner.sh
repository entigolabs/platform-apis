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

# ============ GO TESTS (compositions and functions) ============
run_go_tests() {
  log_info "Running Go tests..."

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
  local CRD_CATALOG="https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json"

  log_info "Running Helm lint..."
  if ! helm lint .; then
    log_fail "Helm lint"
    return 1
  fi
  log_pass "Helm lint"

  log_info "Running Kubeconform validation..."
  if helm template test-release . | kubeconform -verbose -summary \
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
  composition) run_go_tests ;;
  function)    run_go_tests ;;
  helm)        run_helm_tests ;;
  *)
    echo "Usage: runner.sh --type=<composition|function|helm>"
    exit 1
    ;;
esac