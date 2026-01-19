#!/bin/bash

load_mocks

INPUT="../examples/msk-observer.yaml"
COMPOSITION="../apis/msk-observer-composition.yaml"
ENV_CONFIG="../examples/environment-config.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "MSK") |
  .spec.clusterARN = "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster" |
  .spec.providerConfig = "aws-provider"
 ' "$TEMP_INPUT"

mock_environment "$ENV_CONFIG"

echo "TEST 1: rendering Cluster"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Cluster" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_cluster_as_observed_resource > "$OBSERVED_RESOURCES"

echo "TEST 2: rendering ProviderConfig, Secret when cluster ready"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "ProviderConfig" 1 "Secret" 1
