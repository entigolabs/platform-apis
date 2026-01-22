#!/bin/bash

load_mocks
init_test "webapp" "/workspace/functions/workload"
setup_resources --env

echo "TEST 1: rendering Deployment, Service and Secret resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Deployment" 1 "Service" 1 "Secret" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed
echo "$OUTPUT" | mock_dep_as_observed_resource | append_observed
echo "$OUTPUT" | mock_sec_as_observed_resource | append_observed

echo "TEST 2: Checking WebApp Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "WebApp"