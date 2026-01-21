#!/bin/bash

load_mocks
init_test "webaccess" "/workspace/functions/networking"
setup_resources --env

echo "TEST 1: rendering VirtualService, ServiceEntry, DestinationRule resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "VirtualService" 1 "ServiceEntry" 2 "DestinationRule" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 2: Checking WebAccess Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "WebAccess"