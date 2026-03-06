#!/bin/bash

load_mocks
init_test "database"

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

mock_pg_user_as_extra_resource > "$EXTRA_RESOURCES"

echo "TEST 1: rendering Grant, Database, Extensions and instance-protection (not-ready until grant-usage observed)..."
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 1

echo "TEST 2: rendering with observed Grant - expect grant-usage Protection..."
echo "$OUTPUT" | mock_pg_grant_as_observed_resource | start_observed
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 1

echo "TEST 3: rendering with observed Grant and grant-usage - expect all Usages..."
echo "$OUTPUT" | mock_pg_grant_usage_as_observed_resource | append_observed
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 2