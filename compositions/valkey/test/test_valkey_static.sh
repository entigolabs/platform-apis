#!/bin/bash

load_mocks
init_test "valkey"
INPUT="test-input.yaml"  # override default input
setup_resources --env --required

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "TEST 1: rendering ReplicationGroup, Secret, SecretVersion, SecurityGroup resources..."
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "ReplicationGroup" 1 "Secret" 1 "SecretVersion" 1 "SecurityGroup" 1