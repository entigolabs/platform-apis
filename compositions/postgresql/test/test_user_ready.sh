#!/bin/bash

load_mocks
init_test "user" "/workspace/functions/database"
INPUT="../examples/user-with-role-grant.yaml"  # override default input

mock_pg_instance_as_extra_resource > "$EXTRA_RESOURCES"

# Build observed through role stage (shared setup)
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_role_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")

echo "TEST 1: PostgreSQLUser NOT ready when Grant is not ready..."
echo "$OUTPUT" | mock_pg_grant_without_ready | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_not_ready "$OUTPUT" "PostgreSQLUser"

echo "TEST 2: PostgreSQLUser ready when all resources including Grant are ready..."
# Rebuild observed from scratch to avoid duplicate grant resources from TEST 1
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_role_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_grant_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_usage_grants_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_instance_protection_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "PostgreSQLUser"