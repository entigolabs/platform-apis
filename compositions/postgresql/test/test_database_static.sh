#!/bin/bash

load_mocks
init_test "database" "/workspace/functions/database"

mock_pg_owner_role_as_extra_resource > "$EXTRA_RESOURCES"

echo "TEST 1: rendering Grant only (database blocked until grant observed)..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 0 "Extension" 0 "Usage" 0

echo "TEST 2: rendering with observed Grant - database unblocked..."
echo "$OUTPUT" | mock_pg_grant_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 0 "Usage" 0

echo "TEST 3: rendering with observed Database - extensions unblocked..."
echo "$OUTPUT" | mock_pg_database_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 0

echo "TEST 4: rendering with observed Extension - owner-protection unblocked..."
echo "$OUTPUT" | mock_pg_extension_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 1

echo "TEST 5: rendering with observed owner-protection - instance-protection unblocked..."
echo "$OUTPUT" | mock_pg_owner_protection_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 2