#!/bin/bash
# Common Mock Helpers for Crossplane composition tests
# Source this file in mocks.sh to use the helper functions

mock_ready() {
  local selector=""
  for kind in "$@"; do
    if [ -n "$selector" ]; then
      selector="$selector or "
    fi
    selector="$selector.kind == \"$kind\""
  done
  yq "select($selector) | .status.conditions = [{\"type\": \"Synced\", \"status\": \"True\"}, {\"type\": \"Ready\", \"status\": \"True\"}]" -
}

mock_available() {
  local selector=""
  for kind in "$@"; do
    if [ -n "$selector" ]; then
      selector="$selector or "
    fi
    selector="$selector.kind == \"$kind\""
  done
  yq "select($selector) | .status.conditions = [{\"type\": \"Synced\", \"status\": \"True\"}, {\"type\": \"Available\", \"status\": \"True\"}]" -
}

mock_select() {
  local selector=""
  for kind in "$@"; do
    if [ -n "$selector" ]; then
      selector="$selector or "
    fi
    selector="$selector.kind == \"$kind\""
  done
  yq "select($selector)" -
}