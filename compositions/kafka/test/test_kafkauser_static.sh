#!/bin/bash

load_mocks
init_test "kafka-user"
INPUT="../examples/user-a.yaml"  # override default input
COMPOSITION="../apis/kafka-user-composition.yaml"  # override default
setup_resources --required

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

echo "TEST 1: rendering Secret, SecretVersion, SecretPolicy, SingleScramSecretAssociation, AccessControlList"
OUTPUT=$(run_render "$TEMP_INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Secret" 1 "SecretVersion" 1 "SecretPolicy" 1 "SingleScramSecretAssociation" 1 "AccessControlList" 4
