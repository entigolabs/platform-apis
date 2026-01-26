#!/bin/bash

load_mocks
init_test "user"
INPUT="../examples/user-with-role-grant.yaml"  # override default input

echo "TEST 1: rendering Grant and Role"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Role" 1