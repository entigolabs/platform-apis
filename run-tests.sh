#!/bin/bash
set -e

GIT_ROOT=$(git rev-parse --show-toplevel)
cd "$GIT_ROOT"

IMAGE_NAME="platform-apis-test-runner"
DOCKERFILE="test/build/Dockerfile.test"

BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${BLUE}[INFO]${NC} $1"; }

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

run_composition_test() {
  local target_path="${1%/}"

  if [[ "$target_path" == *"/test" ]]; then
    target_path=$(dirname "$target_path")
  fi

  local full_workdir="/workspace/$target_path"

  log_info "Running Composition Suite: $target_path"

  docker run --rm \
    -v "$GIT_ROOT:/workspace" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    --network host \
    -e TERM=xterm \
    -w "$full_workdir/test" \
    $IMAGE_NAME \
    /bin/bash /workspace/test/runner.sh
}

run_function_test() {
  local func_name=$(basename "$1")
  log_info "Running Go Tests: functions/$func_name"

  docker run --rm \
    -v "$GIT_ROOT:/workspace" \
    -w "/workspace/functions/$func_name" \
    -e CGO_ENABLED=1 \
    $IMAGE_NAME \
    go test -v .
}

run_helm_test() {
  log_info "Running Helm Lint & Kubeconform..."
  CRD_CATALOG="https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json"
  docker run --rm \
    -v "$GIT_ROOT:/workspace" \
    -w "/workspace" \
    $IMAGE_NAME \
    /bin/bash -c "helm lint ./helm && helm template my-release ./helm | kubeconform -verbose -summary -ignore-missing-schemas -schema-location default -schema-location '$CRD_CATALOG'"
}

TARGET="${1%/}"

if [ "$TARGET" == "build" ]; then
  FORCE_BUILD=true build_image
  exit 0
fi

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

  # 3. Helm
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
       log_error "Unknown target: $TARGET"
       exit 1
    fi
  fi
fi