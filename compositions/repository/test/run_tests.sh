#!/bin/bash
set -e

GIT_ROOT=$(git rev-parse --show-toplevel)
CURRENT_DIR_REL=$(git rev-parse --show-prefix)

IMAGE_NAME="platform-apis-test-runner"

echo "Building image..."
docker build -t $IMAGE_NAME -f "$GIT_ROOT/test/build/Dockerfile.test" "$GIT_ROOT"

echo "Running Tests..."
docker run --rm \
    -v "$GIT_ROOT:/workspace" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    --network host \
    -w "/workspace/${CURRENT_DIR_REL%/}" \
    $IMAGE_NAME \
    /bin/bash /workspace/test/runner.sh