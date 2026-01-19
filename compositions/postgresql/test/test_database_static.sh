#!/bin/bash

load_mocks

INPUT="../examples/database.yaml"
COMPOSITION="../apis/database-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"

echo "TEST 1: rendering Grant and Database..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1