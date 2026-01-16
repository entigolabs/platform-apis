#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source "$SCRIPT_DIR/../../../test/lib.sh"
source "$SCRIPT_DIR/mocks.sh"

#===================== MSK - OBSERVER - COMPOSITION  TESTS =====================

INPUT="$SCRIPT_DIR/../examples/msk-observer.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/msk-observer-composition.yaml"
FUNC_CONFIG="$SCRIPT_DIR/../../../test/common/functions.yaml"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "MSK") |
  .spec.clusterARN = "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster" |
  .spec.providerConfig = "aws-provider"
 ' "$TEMP_INPUT"

echo "---" >> $EXTRA_RESOURCES
echo "kind: EnvironmentConfig
apiVersion: apiextensions.crossplane.io/v1beta1
metadata:
  name: platform-apis-kafka
spec:
  awsProvider: crossplane-aws
" >> "$EXTRA_RESOURCES"

echo "MSK. TEST 1: rendering resources"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "MSK" 1 "Cluster" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_cluster_ready_status > "$OBSERVED_RESOURCES"

echo "MSK. TEST 2: rendering resources"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "Cluster" 1 "ProviderConfig" 1 "MSK" 1 "Secret" 1

echo "Mocking required resources"
echo "---" >> $EXTRA_RESOURCES
cat "$TEMP_INPUT" | mock_msk_observer_ready_status >> "$EXTRA_RESOURCES"
rm -f "$TEMP_INPUT"

#===================== KAFKA - TOPIC - COMPOSITION  TESTS =====================

INPUT="$SCRIPT_DIR/../examples/topic-a.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/kafka-topic-composition.yaml"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "XTopic") |
  .spec.claimRef.name = "topic-claimRef"
 ' "$TEMP_INPUT"

echo "TOPIC. TEST 1: rendering resources"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Topic" 1

#===================== KAFKA - USER - COMPOSITION  TESTS =====================

INPUT="$SCRIPT_DIR/../examples/user-a.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/kafka-user-composition.yaml"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "XKafkaUser") |
  .spec.claimRef.name = "user-claimRef" |
  .spec.claimRef.namespace = "default"
 ' "$TEMP_INPUT"

TEMP_COMPOSITION="temp_composition.yaml"
cp "$COMPOSITION" "$TEMP_COMPOSITION"

yq -i 'del(.spec.pipeline[] | select(.step == "sequence-creation"))' "$TEMP_COMPOSITION"

echo "USER. TEST 1: rendering resources"
OUTPUT=$(run_render "$TEMP_INPUT" "$TEMP_COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "XKafkaUser" 1 "Secret" 1 "SecretVersion" 1 "SecretPolicy" 1 "SingleScramSecretAssociation" 1 "AccessControlList" 4

rm -f "$TEMP_INPUT"
rm -f "$TEMP_COMPOSITION"
yq -i 'del(select(.kind == "MSK"))' "$EXTRA_RESOURCES"
yq -i 'del(select(.kind == "EnvironmentConfig"))' "$EXTRA_RESOURCES"
cleanup_test