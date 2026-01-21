#!/bin/bash

load_mocks
init_test "kafka-topic"
INPUT="../examples/topic-a.yaml"  # override default input
COMPOSITION="../apis/kafka-topic-composition.yaml"  # override default
setup_resources --required

# Mock MSK as a required resource
MSK="../examples/msk-observer.yaml"
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
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Topic" 1
