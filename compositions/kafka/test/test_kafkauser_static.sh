#!/bin/bash

load_mocks
init_test "kafka-user" "/workspace/functions/queue"
INPUT="../examples/user-a.yaml"  # override default input
COMPOSITION="../apis/kafka-user-composition.yaml"  # override default
setup_resources --env --required

# Mock MSK as a required resource
MSK="../examples/msk-observer.yaml"
echo "---" >> $EXTRA_RESOURCES
cat "$MSK" | mock_observed_resources >> "$EXTRA_RESOURCES"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "KafkaUser") |
  .spec.claimRef.name = "user-claimRef" |
  .spec.claimRef.namespace = "default"
 ' "$TEMP_INPUT"

# Remove sequence-creation step for testing
TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"
yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "TEST 1: rendering K8s Secret (first phase, k8s-secret not yet observed)"
OUTPUT=$(run_render "$TEMP_INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Secret" 1

echo "Mocking k8s-secret as observed"
echo "$OUTPUT" | mock_select "Secret" | mock_ready "Secret" | start_observed

echo "TEST 2: rendering K8s Secret + AWS Secret (k8s-secret observed and ready)"
OUTPUT=$(run_render "$TEMP_INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Secret" 2
