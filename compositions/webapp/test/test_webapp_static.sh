#!/bin/bash

load_mocks

INPUT="../examples/webapp.yaml"
COMPOSITION="../apis/webapp-composition.yaml"
FUNC_CONFIG="workload-function.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

setup_function "/workspace/functions/workload"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering Deployment, Service and Secret resources"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Deployment" 1 "Service" 1 "Secret" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"
echo "---" >> "$OBSERVED_RESOURCES"
echo "$OUTPUT" | mock_dep_as_observed_resource >> "$OBSERVED_RESOURCES"
echo "---" >> "$OBSERVED_RESOURCES"
echo "$OUTPUT" | mock_sec_as_observed_resource >> "$OBSERVED_RESOURCES"

echo "TEST 2: Checking WebApp Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "WebApp"