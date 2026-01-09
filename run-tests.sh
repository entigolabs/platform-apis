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
    echo "Testing Composition: $comp"
    echo "=================================================="

    docker run --rm -v "$(pwd):/workspace" \
        -w "/workspace/compositions/$comp/test" \
        $IMAGE_NAME \
        bash static.sh
}

if [ -z "$TARGET" ]; then
    for d in compositions/*/test/static.sh; do
        comp_name=$(echo $d | awk -F/ '{print $(NF-2)}')
        run_composition_test "$comp_name"
    done
else
    if [ -d "compositions/$TARGET" ]; then
        run_composition_test "$TARGET"
    else
        echo "Error: Composition '$TARGET' not found."
        exit 1
    fi
fi

echo "All requested tests passed."