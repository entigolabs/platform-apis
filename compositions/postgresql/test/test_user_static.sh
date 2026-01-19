#!/bin/bash

load_mocks

INPUT="../examples/user-with-role-grant.yaml"
COMPOSITION="../apis/user-composition.yaml"
FUNC_CONFIG="/workspace/test/common/functions.yaml"

echo "TEST 1: rendering Grant and Role"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Role" 1