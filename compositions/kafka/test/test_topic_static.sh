#!/bin/bash

load_mocks

INPUT="../examples/topic-a.yaml"
COMPOSITION="../apis/kafka-topic-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"
MSK="../examples/msk-observer.yaml"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"

echo "Mocking required resources"
echo "---" >> $EXTRA_RESOURCES
cat "$MSK" | mock_observed_resources >> "$EXTRA_RESOURCES"

TEMP_INPUT="temp_input.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "Topic") |
  .kind = "XTopic" |
  .spec.claimRef.name = "topic-claimRef"
 ' "$TEMP_INPUT"

echo "TEST 1: rendering Topic"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "Topic" 1
