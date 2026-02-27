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

run_go_tests() {
  local label="$1"
  log_info "Running $label..."
  if go test -v ./...; then
    log_pass "$label"
    return 0
  else
    log_fail "$label"
    return 1
  fi
}

# ============ COMPOSITION TESTS ============
run_composition_tests() { run_go_tests "Go composition tests"; }

# Runs all composition test suites in a single container (shares module/build cache).
run_all_composition_tests() {
  local failed=0
  for dir in /workspace/compositions/*/test; do
    [ -f "$dir/go.mod" ] || continue
    local name
    name=$(basename "$(dirname "$dir")")
    log_info "Running: compositions/$name"
    if (cd "$dir" && go test -v ./...); then
      log_pass "compositions/$name"
    else
      log_fail "compositions/$name"
      failed=$((failed + 1))
    fi
  done
  return $failed
}

# ============ FUNCTION TESTS ============
run_function_tests() {
  export CGO_ENABLED=1
  run_go_tests "Go unit tests"
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
  compositions) run_all_composition_tests ;;
  composition)  run_composition_tests ;;
  function)     run_function_tests ;;
  helm)         run_helm_tests ;;
  *)
    echo "Usage: runner.sh --type=<compositions|composition|function|helm>"
    exit 1
    ;;
esac
