#!/bin/bash

load_mocks
init_test "user"
INPUT="../examples/user-with-role-grant.yaml"  # override default input

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

mock_pg_instance_as_extra_resource > "$EXTRA_RESOURCES"

echo "TEST 1: rendering Role, Grant and usage-grant Protection (no instance-protection yet)..."
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 1

echo "TEST 2: rendering with observed usage-grant - expect all Usages..."
echo "$OUTPUT" | mock_pg_usage_grants_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Role" 1 "Grant" 1 "Usage" 2