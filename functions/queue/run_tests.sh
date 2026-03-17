#!/bin/bash
set -e

GIT_ROOT=$(git rev-parse --show-toplevel)
REL_PATH=$(git rev-parse --show-prefix)

bash "$GIT_ROOT/run-tests.sh" "${REL_PATH%/}"
