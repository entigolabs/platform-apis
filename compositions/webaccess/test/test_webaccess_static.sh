#!/bin/bash

load_mocks

INPUT="../examples/webaccess.yaml"
COMPOSITION="../apis/webaccess-composition.yaml"
FUNC_CONFIG="networking-function.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

setup_function "/workspace/functions/networking"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering VirtualService, ServiceEntry, DestinationRule resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "VirtualService" 1 "ServiceEntry" 2 "DestinationRule" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"

echo "TEST 2: Checking WebAccess Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "WebAccess"