#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source "$SCRIPT_DIR/../../../test/lib.sh"
source "$SCRIPT_DIR/mocks.sh"

INPUT="$SCRIPT_DIR/../examples/webapp.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/webapp-composition.yaml"
FUNC_CONFIG="$SCRIPT_DIR/workload-function.yaml"
ENV_CONFIG="$SCRIPT_DIR/../examples/environment-config.yaml"

setup_function "$SCRIPT_DIR/../../../functions/workload"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Deployment" 1 "Service" 1 "Secret" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_dep_ready_status > "$OBSERVED_RESOURCES"
echo "---" >> "$OBSERVED_RESOURCES"
echo "$OUTPUT" | mock_ser_ready_status >> "$OBSERVED_RESOURCES"
echo "---" >> "$OBSERVED_RESOURCES"
echo "$OUTPUT" | mock_sec >> "$OBSERVED_RESOURCES"

echo "TEST 2: Checking Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "WebApp"