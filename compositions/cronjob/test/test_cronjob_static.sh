#!/bin/bash

load_mocks

INPUT="../examples/cronjob.yaml"
COMPOSITION="../apis/cronjob-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

setup_function "/workspace/functions/workload"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "CronJob" 2 "Service" 1 "Secret" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"
echo "---" >> "$OBSERVED_RESOURCES"
echo "$OUTPUT" | mock_sec_as_observed_resource >> "$OBSERVED_RESOURCES"

echo "TEST 2: Checking CronJob Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "CronJob"
