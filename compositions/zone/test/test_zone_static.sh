#!/bin/bash

load_mocks
init_test "zone" "/workspace/functions/tenancy"
setup_resources --env --required

echo "TEST 1: rendering AppProject,  MutatingPolicy, ValidatingPolicy, LaunchTemplate, Namespace resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "AppProject" 1 "MutatingPolicy" 4 "ValidatingPolicy" 2 "LaunchTemplate" 2 "Namespace" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 2: rendering NetworkPolicy, Role resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "NetworkPolicy" 2 "Role" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 3: rendering AccessEntry, Role, RolePolicyAttachment resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "AccessEntry" 1 "Role" 5 "RolePolicyAttachment" 4

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 4: rendering RoleBinding resources..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "RoleBinding" 6

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | start_observed

echo "TEST 5: Checking Zone Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "Zone"