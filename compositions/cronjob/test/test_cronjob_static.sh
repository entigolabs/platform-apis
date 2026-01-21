#!/bin/bash

load_mocks
init_test "cronjob" "/workspace/functions/workload"
setup_resources --env

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "CronJob" 2 "Service" 1 "Secret" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed
echo "$OUTPUT" | mock_sec_as_observed_resource | append_observed

echo "TEST 2: Checking CronJob Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "CronJob"
