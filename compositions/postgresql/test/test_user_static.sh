#!/bin/bash

load_mocks
init_test "user"
INPUT="../examples/user-with-role-grant.yaml"  # override default input

mock_pg_instance_as_extra_resource > "$EXTRA_RESOURCES"

echo "TEST 1: rendering Role only (grants blocked until role observed)..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 0 "Usage" 0

echo "TEST 2: rendering with observed Role - grants unblocked..."
echo "$OUTPUT" | mock_pg_role_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 0

echo "TEST 3: rendering with observed Grant - usage-grant unblocked..."
echo "$OUTPUT" | mock_pg_grant_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 1

echo "TEST 4: rendering with observed usage-grant - instance-protection unblocked..."
echo "$OUTPUT" | mock_pg_usage_grants_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 2