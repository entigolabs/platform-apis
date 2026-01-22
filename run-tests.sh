#!/bin/bash
set -e

GIT_ROOT=$(git rev-parse --show-toplevel)
cd "$GIT_ROOT"

IMAGE_NAME="${IMAGE_NAME:-entigolabs/platform-apis-test-runner:latest}"

BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }

# Unified docker test runner
run_docker_test() {
  local test_type="$1"
  local workdir="$2"

  docker run --rm --pull always \
    -v "$GIT_ROOT:/workspace" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    --network host \
    -e TERM=xterm \
    -w "$workdir" \
    $IMAGE_NAME \
    /bin/bash /workspace/test/runner.sh --type="$test_type"
}

run_composition_test() {
  local target_path="${1%/}"

  if [[ "$target_path" == *"/test" ]]; then
    target_path=$(dirname "$target_path")
  fi

  log_info "Running Composition Suite: $target_path"
  run_docker_test "composition" "/workspace/$target_path/test"
}

run_function_test() {
  local func_name=$(basename "$1")
  log_info "Running Function Tests: functions/$func_name"
  run_docker_test "function" "/workspace/functions/$func_name"
}

run_helm_test() {
  log_info "Running Helm Tests..."
  run_docker_test "helm" "/workspace/helm"
}

TARGET="${1%/}"

if [ "$TARGET" == "build" ]; then
  resolve_image
  FORCE_BUILD=true build_image
  exit 0
fi

resolve_image
build_image

if [ -z "$TARGET" ]; then
  log_info "No target specified. Auto-discovering all tests..."

  for d in compositions/*; do
    if [ -d "$d/test" ]; then
      run_composition_test "$d"
    fi
  done

  for d in functions/*; do
    if [ -d "$d" ] && [ "$(basename "$d")" != "common" ]; then
      run_function_test "$d"
    fi
  done

  # Helm
  if [ -d "helm" ]; then run_helm_test; fi

else
  TARGET=${TARGET#./}

  if [ "$TARGET" == "helm" ]; then
    run_helm_test
  elif [[ "$TARGET" == functions* ]]; then
    run_function_test "$TARGET"
  elif [[ "$TARGET" == compositions* ]]; then
    run_composition_test "$TARGET"
  else
    if [ -d "$TARGET/test" ] || [[ "$PWD" == *"/test" ]]; then
       run_composition_test "$TARGET"
    else
       echo "Unknown target: $TARGET"
       exit 1
    fi
  fi
fi