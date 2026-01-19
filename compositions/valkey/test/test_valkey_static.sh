#!/bin/bash

load_mocks

INPUT="test-input.yaml"
COMPOSITION="../apis/valkey-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"
mock_environment "$ENV_CONFIG"

TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "TEST 1: rendering ReplicationGroup, Secret, SecretVersion, SecurityGroup resources..."
OUTPUT=$(run_render "$INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
#TODO: Why not "SecurityGroupRule" created???
assert_counts "$OUTPUT" "ReplicationGroup" 1 "Secret" 1 "SecretVersion" 1 "SecurityGroup" 1