#!/bin/bash

load_mocks

INPUT="../examples/user-a.yaml"
COMPOSITION="../apis/kafka-user-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"
MSK="../examples/msk-observer.yaml"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"

echo "Mocking required resources"
echo "---" >> $EXTRA_RESOURCES
cat "$MSK" | mock_observed_resources >> "$EXTRA_RESOURCES"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "KafkaUser") |
  .kind = "XKafkaUser" |
  .spec.claimRef.name = "user-claimRef" |
  .spec.claimRef.namespace = "default"
 ' "$TEMP_INPUT"

TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"

yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "TEST 1: rendering Secret, SecretVersion, SecretPolicy, SingleScramSecretAssociation, AccessControlList"
OUTPUT=$(run_render "$TEMP_INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Secret" 1 "SecretVersion" 1 "SecretPolicy" 1 "SingleScramSecretAssociation" 1 "AccessControlList" 4
