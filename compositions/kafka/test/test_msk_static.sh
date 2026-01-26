#!/bin/bash

load_mocks
init_test "msk-observer"
COMPOSITION="../apis/msk-observer-composition.yaml"  # override default
setup_resources --env --required

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "MSK") |
  .spec.clusterARN = "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster" |
  .spec.providerConfig = "aws-provider"
 ' "$TEMP_INPUT"

echo "TEST 1: rendering Cluster"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Cluster" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_cluster_as_observed_resource | start_observed

echo "TEST 2: rendering ProviderConfig, Secret when cluster ready"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "ProviderConfig" 1 "Secret" 1
