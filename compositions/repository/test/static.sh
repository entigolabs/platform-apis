#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source "$SCRIPT_DIR/../../../test/lib.sh"
source "$SCRIPT_DIR/mocks.sh"

INPUT="$SCRIPT_DIR/../examples/repository.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/repository-composition.yaml"
FUNC_CONFIG="$SCRIPT_DIR/artifact-function.yaml"
ENV_CONFIG="$SCRIPT_DIR/../examples/environment-config.yaml"

setup_function "$SCRIPT_DIR/../../../functions/artifact"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Repository" 2

echo "Mocking observed resources"
echo "---" >> $OBSERVED_RESOURCES
echo "$OUTPUT" | mock_rep_ready_status >> "$OBSERVED_RESOURCES"

echo "TEST 2: Checking Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "Repository"

yq -i 'del(select(.kind == "EnvironmentConfig"))' "$EXTRA_RESOURCES"
cleanup_test