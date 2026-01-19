#!/bin/bash
set -e

IMAGE_NAME="platform-apis-test-runner"
TARGET=$1

echo "Building Test Docker Image..."
docker build -t $IMAGE_NAME -f test/build/Dockerfile.test .

run_suite() {
  local workdir=$1
  echo "Running Tests in: $workdir..."

  docker run --rm \
    -v "$(pwd):/workspace" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    --network host \
    -w "/workspace/$workdir" \
    $IMAGE_NAME \
    /bin/bash /workspace/test/runner.sh
}

run_function_test() {
  local func_name=$1
    echo "Running Function Tests: $func_name"

  if [ ! -d "functions/$func_name" ]; then
    echo "Function folder '$func_name' not found."
    return
  fi

  docker run --rm \
    -v "$(pwd):/workspace" \
    -w "/workspace/functions/$func_name" \
    -e CGO_ENABLED=1 \
    $IMAGE_NAME \
    go test -v .
}

run_helm_test() {
  echo "Running Helm Lint Test"

  docker run --rm \
    -v "$(pwd):/workspace" \
    -w "/workspace" \
    $IMAGE_NAME \
    helm lint ./helm

  echo "Running Kubeconform Schema Validation Test"

  CRD_CATALOG="https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json"

  docker run --rm \
    -v "$(pwd):/workspace" \
    -w "/workspace" \
    $IMAGE_NAME \
    /bin/bash -c "helm template my-release ./helm | kubeconform -verbose -summary -ignore-missing-schemas -schema-location default -schema-location '$CRD_CATALOG'"
}

if [ -z "$TARGET" ]; then
  for d in compositions/*/test; do
    if [ -d "$d" ]; then
      rel_path=${d%/}
      run_suite "$rel_path"
    fi
  done

  for d in functions/*; do
    if [ -d "$d" ]; then
      func_name=$(basename "$d")
      if [ "$func_name" == "common" ]; then
        echo "Skipping 'common' folder (library code)..."
        continue
      fi
      run_function_test "$func_name"
    fi
  done

  if [ -d "helm" ]; then
    run_helm_test
  fi
else
  if [ -d "compositions/$TARGET" ]; then
    run_suite "compositions/$TARGET/test"
  elif [ -d "functions/$TARGET" ]; then
    run_function_test "$TARGET"
  elif [ "$TARGET" == "helm" ]; then
    run_helm_test
  else
    echo "Error: '$TARGET' not found."
    exit 1
  fi
fi