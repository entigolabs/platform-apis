#!/bin/bash

load_mocks
init_test "database"

echo "TEST 1: rendering Grant, Database and Usage..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Grant" 1 "Database" 1 "Usage" 1