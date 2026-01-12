#!/bin/bash
set -e

# Usage: ./run-tests.sh [composition-name]
# Example: ./run-tests.sh cronjob
# If no argument provided, runs all.

IMAGE_NAME="platform-apis-test-runner"

TARGET=$1

echo "--- Building Test Docker Image ---"
docker build -t $IMAGE_NAME -f test/build/Dockerfile.test .

echo "--- Running Tests ---"

run_composition_test() {
    local comp=$1
    echo "=================================================="
    echo "COMPOSITIONS TEST: $comp"
    echo "=================================================="

    if [ ! -f "compositions/$comp/test/static.sh" ]; then
        echo "No test/static.sh found for composition '$comp'. Skipping."
        return
    fi

    docker run --rm -v "$(pwd):/workspace" \
        -w "/workspace/compositions/$comp/test" \
        $IMAGE_NAME \
        bash static.sh
}

run_function_test() {
    local func_name=$1
    echo "=================================================="
    echo "FUNCTIONS TEST: $func_name"
    echo "=================================================="

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
    echo "=================================================="
    echo "HELM LINT (Syntax & Best Practices)"
    echo "=================================================="

    docker run --rm \
        -v "$(pwd):/workspace" \
        -w "/workspace" \
        $IMAGE_NAME \
        helm lint ./helm

    echo "=================================================="
    echo "KUBECONFORM (Schema Validation)"
    echo "=================================================="

    CRD_CATALOG="https://raw.githubusercontent.com/datreeio/CRDs-catalog/main/{{.Group}}/{{.ResourceKind}}_{{.ResourceAPIVersion}}.json"

    docker run --rm \
        -v "$(pwd):/workspace" \
        -w "/workspace" \
        $IMAGE_NAME \
        /bin/bash -c "helm template my-release ./helm | kubeconform -verbose -summary -ignore-missing-schemas -schema-location default -schema-location '$CRD_CATALOG'"
}

if [ -z "$TARGET" ]; then
    for d in compositions/*/test/static.sh; do
        comp_name=$(echo $d | awk -F/ '{print $(NF-2)}')
        run_composition_test "$comp_name"
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
        run_composition_test "$TARGET"
    elif [ -d "functions/$TARGET" ]; then
        run_function_test "$TARGET"
    elif [ "$TARGET" == "helm" ]; then
        run_helm_test
    else
        echo "Error: '$TARGET' not found."
        exit 1
    fi
fi

echo "All requested tests passed."