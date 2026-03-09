#!/bin/bash

load_mocks
init_test "database" "/workspace/functions/database"

mock_pg_owner_role_as_extra_resource > "$EXTRA_RESOURCES"

# Build observed through database stage (shared setup)
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_grant_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_database_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")

echo "TEST 1: PostgreSQLDatabase NOT ready when Extension is not ready..."
echo "$OUTPUT" | mock_pg_extension_without_ready | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_not_ready "$OUTPUT" "PostgreSQLDatabase"

echo "TEST 2: PostgreSQLDatabase ready when all resources including Extension are ready..."
# Rebuild observed from scratch to avoid duplicate extension resources from TEST 1
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_grant_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_database_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_extension_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_grant_usage_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_owner_protection_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
echo "$OUTPUT" | mock_pg_instance_protection_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "PostgreSQLDatabase"