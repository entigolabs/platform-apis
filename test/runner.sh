#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

CURRENT_DIR=$(pwd)
echo -e "Test Runner started..."
echo -e "Workdir: $CURRENT_DIR"

source /workspace/test/lib.sh

TEST_FILES=$(find . -maxdepth 1 -name "test_*.sh" | sort)

if [ -z "$TEST_FILES" ]; then
  echo -e "No test_*.sh files found in $CURRENT_DIR. Skipping..."
  exit 0
fi

FAILED_TESTS=0
for test_file in $TEST_FILES; do
  echo -e "\nRunning Suite: $test_file..."

  (
    set -e
    source "$test_file"
  )

  EXIT_CODE=$?

  if [ $EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}Suite Passed: $test_file!${NC}"
  else
    echo -e "${RED}Suite Failed: $test_file!${NC}"
    FAILED_TESTS=$((FAILED_TESTS+1))
  fi

  cleanup_test 2>/dev/null || true
done