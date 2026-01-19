#!/bin/bash

load_mocks

INPUT="../examples/zone.yaml"
COMPOSITION="../apis/zone-composition.yaml"
FUNC_CONFIG="tenancy-function.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

setup_function "/workspace/functions/tenancy"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"
mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering AppProject,  MutatingPolicy, ValidatingPolicy, LaunchTemplate, Namespace resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "AppProject" 1 "MutatingPolicy" 4 "ValidatingPolicy" 2 "LaunchTemplate" 2 "Namespace" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"

echo "TEST 2: rendering NetworkPolicy, Role resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "NetworkPolicy" 2 "Role" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"

echo "TEST 3: rendering AccessEntry, Role, RolePolicyAttachment resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "AccessEntry" 1 "Role" 5 "RolePolicyAttachment" 4

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"

echo "TEST 4: rendering RoleBinding resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "RoleBinding" 6

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources > "$OBSERVED_RESOURCES"

echo "TEST 5: Checking Zone Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "Zone"