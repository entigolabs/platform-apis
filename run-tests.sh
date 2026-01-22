#!/bin/bash
set -e

GIT_ROOT=$(git rev-parse --show-toplevel)
cd "$GIT_ROOT"

DOCKERFILE="test/build/Dockerfile.test"
DOCKER_HUB_REPO="entigolabs/platform-apis-test-runner"

BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }

resolve_image() {
  if [ -n "$IMAGE_NAME" ]; then
    return
  fi

  if [ "$CI" == "true" ]; then
    LATEST_IMAGE="${DOCKER_HUB_REPO}:latest"

    if [ -n "$GITHUB_EVENT_NUMBER" ]; then
      PR_IMAGE="${DOCKER_HUB_REPO}:pr-${GITHUB_EVENT_NUMBER}"
      log_info "Attempting to pull PR image: $PR_IMAGE"

      if docker pull "$PR_IMAGE" 2>/dev/null; then
        log_info "Using PR image."
        IMAGE_NAME="$PR_IMAGE"
        SKIP_BUILD="true"
        return
      fi
      log_info "PR image not found. Falling back to 'latest'..."
    fi

    docker pull "$LATEST_IMAGE"
    IMAGE_NAME="$LATEST_IMAGE"
    SKIP_BUILD="true"
  else
    IMAGE_NAME="platform-apis-test-runner"
  fi
}

build_image() {
  if [ "$SKIP_BUILD" == "true" ]; then
    log_info "Skipping Docker build (SKIP_BUILD=true)..."
    return
  fi

  if [[ "$(docker images -q $IMAGE_NAME 2> /dev/null)" == "" ]] || [ "$FORCE_BUILD" == "true" ]; then
    log_info "Building Test Runner Image..."
    docker build -t $IMAGE_NAME -f "$DOCKERFILE" .
  else
    log_info "Image $IMAGE_NAME exists. Skipping build."
  fi
}

# Unified docker test runner
run_docker_test() {
  local test_type="$1"
  local workdir="$2"

  docker run --rm \
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