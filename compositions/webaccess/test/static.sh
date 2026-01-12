#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source "$SCRIPT_DIR/../../../test/lib.sh"
source "$SCRIPT_DIR/mocks.sh"

INPUT="$SCRIPT_DIR/../examples/webaccess.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/webaccess-composition.yaml"
FUNC_CONFIG="$SCRIPT_DIR/networking-function.yaml"
ENV_CONFIG="$SCRIPT_DIR/../examples/environment-config.yaml"

setup_function "$SCRIPT_DIR/../../../functions/networking"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "VirtualService" 1 "ServiceEntry" 2 "DestinationRule" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_vs_se_and_dr_ready_statuses > "$OBSERVED_RESOURCES"

echo "TEST 2: Checking Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "WebAccess"

rm -f "$EXTRA_RESOURCES"
cleanup_test