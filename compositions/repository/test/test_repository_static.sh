#!/bin/bash

load_mocks
init_test "repository" "/workspace/functions/artifact"
setup_resources --env --required

echo "TEST 1: rendering repository"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Repository" 2 # because our input has same kind

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | append_observed

echo "TEST 2: Checking Repository Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "Repository"