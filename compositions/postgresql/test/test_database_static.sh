#!/bin/bash

load_mocks
init_test "database"

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

mock_pg_user_as_extra_resource > "$EXTRA_RESOURCES"

echo "TEST 1: rendering Grant, Database, Extensions and Usages..."
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Extension" 1 "Usage" 2