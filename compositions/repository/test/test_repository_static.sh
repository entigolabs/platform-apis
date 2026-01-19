#!/bin/bash

load_mocks

INPUT="../examples/repository.yaml"
COMPOSITION="../apis/repository-composition.yaml"
FUNC_CONFIG="artifact-function.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

setup_function "/workspace/functions/artifact"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"
mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering repository"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Repository" 2 # because our input has same kind

echo "Mocking observed resources"
echo "---" >> $OBSERVED_RESOURCES
echo "$OUTPUT" | mock_observed_resources >> "$OBSERVED_RESOURCES"

echo "TEST 2: Checking Repository Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "Repository"