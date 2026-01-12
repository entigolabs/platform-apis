#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source "$SCRIPT_DIR/../../../test/lib.sh"
source "$SCRIPT_DIR/mocks.sh"

INPUT="$SCRIPT_DIR/../examples/cronjob.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/cronjob-composition.yaml"
FUNC_CONFIG="$SCRIPT_DIR/workload-function.yaml"
ENV_CONFIG="$SCRIPT_DIR/../examples/environment-config.yaml"

setup_function "$SCRIPT_DIR/../../../functions/workload"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "CronJob" 2 "Service" 1 "Secret" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_cj_and_ser_ready_statuses > "$OBSERVED_RESOURCES"
echo "---" >> "$OBSERVED_RESOURCES"
echo "$OUTPUT" | mock_sec >> "$OBSERVED_RESOURCES"

echo "TEST 2: Checking Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "CronJob"

rm -f "$EXTRA_RESOURCES"
cleanup_test
