#!/bin/bash

load_mocks
init_test "user"
INPUT="../examples/user-with-role-grant.yaml"  # override default input

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

mock_pg_instance_as_extra_resource > "$EXTRA_RESOURCES"

echo "TEST 1: rendering Role, Grant and Usages (instance-protection not-ready until role observed)..."
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 2

echo "TEST 2: rendering with observed Role - instance-protection becomes ready..."
echo "$OUTPUT" | mock_pg_role_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 2