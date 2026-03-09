#!/bin/bash

FUNC_PIDS=()
FUNC_PIDS_FILE="/tmp/func_pids_$$.txt"
EXTRA_RESOURCES="extra-resources-mock.yaml"
OBSERVED_RESOURCES="observed-resources-mock.yaml"

RED='\033[0;31m'
NC='\033[0m'

load_mocks() {
  if [ -f "./mocks.sh" ]; then
    source "./mocks.sh"
  fi
}

# Initialize test environment by deriving paths from test filename.
# Usage: init_test "resource_name" [function_path]
# Example: init_test "cronjob" "/workspace/functions/workload"
init_test() {
  local resource="$1"
  local func_path="$2"

  INPUT="../examples/${resource}.yaml"
  COMPOSITION="../apis/${resource}-composition.yaml"
  ENV_CONFIG="../examples/environment-config.yaml"
  FUNC_CONFIG="/workspace/test/common/functions.yaml"

  if [ -n "$func_path" ]; then
    setup_function "$func_path"
  fi
}

# Setup extra resources from required-resources.yaml and optionally mock environment
# Usage: setup_resources [--env] [--required]
#   --env       Mock the environment config
#   --required  Copy required-resources.yaml to EXTRA_RESOURCES
setup_resources() {
  local mock_env=false
  local copy_required=false

  for arg in "$@"; do
    case "$arg" in
      --env) mock_env=true ;;
      --required) copy_required=true ;;
    esac
  done

  if [ "$copy_required" = true ] && [ -f "../examples/required-resources.yaml" ]; then
    cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"
  fi

  if [ "$mock_env" = true ]; then
    mock_environment "$ENV_CONFIG"
  fi
}

# Append output to observed resources with YAML document separator
# Usage: echo "$OUTPUT" | mock_function | append_observed
# Or:    append_observed < <(echo "$OUTPUT" | mock_function)
append_observed() {
  if [ -s "$OBSERVED_RESOURCES" ]; then
    echo "---" >> "$OBSERVED_RESOURCES"
  fi
  cat >> "$OBSERVED_RESOURCES"
}

# Start fresh observed resources (overwrite)
# Usage: echo "$OUTPUT" | mock_function | start_observed
start_observed() {
  cat > "$OBSERVED_RESOURCES"
}

mock_environment() {
  local env_config_path="$1"
  echo "Mocking EnvironmentConfig from $env_config_path..."

  touch "$EXTRA_RESOURCES"

  local mocked_env
  mocked_env=$(cat "$EXTRA_RESOURCES" | yq 'select(.kind == "EnvironmentConfig") | length' - | wc -l | xargs)

  if [ "$mocked_env" -eq 0 ]; then
    if [ -s "$EXTRA_RESOURCES" ]; then
      echo "---" >> "$EXTRA_RESOURCES";
    fi
    yq 'select(.kind == "EnvironmentConfig")' "$env_config_path" >> "$EXTRA_RESOURCES"
  fi
}

cleanup_test() {
  rm -f "$OBSERVED_RESOURCES"
  rm -f "$EXTRA_RESOURCES"

  rm -f temp_*.yaml

  local pids=()
  # Collect PIDs from in-process tracking (same shell)
  if [ ${#FUNC_PIDS[@]} -gt 0 ]; then
    pids+=("${FUNC_PIDS[@]}")
    FUNC_PIDS=()
  fi
  # Collect PIDs from file (written by subshells)
  if [ -f "$FUNC_PIDS_FILE" ]; then
    while IFS= read -r pid; do
      pids+=("$pid")
    done < "$FUNC_PIDS_FILE"
    rm -f "$FUNC_PIDS_FILE"
  fi

  if [ ${#pids[@]} -gt 0 ]; then
    echo -e "Stopping Background Functions (PIDs: ${pids[*]})..."
    for pid in "${pids[@]}"; do
      pkill -P "$pid" > /dev/null 2>&1 || true
      kill -9 "$pid" > /dev/null 2>&1 || true
    done
  fi
}

wait_for_function() {
  local port="$1"
  echo -n "Waiting for function on port $port..."
  local retries=0
  while ! nc -z localhost "$port"; do
    sleep 1
    retries=$((retries+1))
    if [ "$retries" -ge 300 ]; then
      echo -e "\n${RED}Timeout waiting for function on port $port.${NC}"
      cleanup_test
      exit 1
    fi
    echo -n "."
  done
  echo -e "Ready."
}

setup_function() {
  local func_path="$1"
  echo -e "Starting Custom Function at $func_path..."

  if [ ! -d "$func_path" ]; then
    echo -e "${RED}Error: Function path '$func_path' does not exist.${NC}"
    cleanup_test
    exit 1
  fi

  pushd "$func_path" > /dev/null
  go run . --insecure --debug &
  local pid=$!
  FUNC_PIDS+=($pid)
  echo "$pid" >> "$FUNC_PIDS_FILE"
  popd > /dev/null

  wait_for_function "9443"
}

run_render() {
  local input="$1"
  local composition="$2"
  local func_config="$3"
  local extra_resources_file="$4"
  local observed_resources_file="$5"

  touch "$EXTRA_RESOURCES"
  touch "$OBSERVED_RESOURCES"

  local args=("$input" "$composition" "$func_config")

  if [ -n "$extra_resources_file" ]; then
    args+=("-e" "$extra_resources_file")
  elif [ -s "$EXTRA_RESOURCES" ]; then
    args+=("-e" "$EXTRA_RESOURCES")
  fi

  if [ -n "$observed_resources_file" ]; then
    args+=("-o" "$observed_resources_file")
  elif [ -s "$OBSERVED_RESOURCES" ]; then
    args+=("-o" "$OBSERVED_RESOURCES")
  fi

  crossplane render "${args[@]}" -r -x 2>&1 || true
}

assert_counts() {
  local output="$1"
  shift
  local failed=0

  if [[ "$output" == *"ERROR"* ]] || [[ "$output" == *"error"* ]]; then
    if [[ "$output" != *"no matches for kind"* ]]; then
      echo -e "${RED}Render Failed with Errors:${NC}"
      echo "$output"
      cleanup_test
      exit 1
    fi
  fi

  while (( "$#" )); do
    local kind=$1
    local expected=$2

    local actual
    actual=$(echo "$output" | yq "select(.kind == \"$kind\") | length" - | wc -l | xargs)

    if [ "$actual" -eq "$expected" ]; then
      echo -e "   ${GREEN}SUCCESS: $kind count: $actual/$expected${NC}"
    else
      echo -e "   ${RED}FAIL: $kind count: Expected $expected, found $actual${NC}"
      failed=1
    fi
    shift 2
  done

  if [ "$failed" -eq 1 ]; then
    echo -e "\n${RED}FAILED RENDER OUTPUT.${NC}"
    echo "$output"
    cleanup_test
    exit 1
  fi
}

assert_ready() {
  local output="$1"
  local kind="$2"

  if echo "$output" | yq "select(.kind == \"$kind\")" - | grep -q 'status: "True"'; then
    echo -e "${GREEN} SUCCESS: $kind is Ready.${NC}"
  else
    echo -e "${RED} FAIL: $kind is NOT Ready.${NC}"
    echo -e "${YELLOW}--- Debug Output for $kind ---${NC}"
    echo "$output" | yq "select(.kind == \"$kind\")" -
    cleanup_test
    exit 1
  fi
}

assert_not_ready() {
  local output="$1"
  local kind="$2"

  if echo "$output" | yq "select(.kind == \"$kind\")" - | grep -q 'status: "True"'; then
    echo -e "${RED} FAIL: $kind is Ready but should NOT be.${NC}"
    echo -e "${YELLOW}--- Debug Output for $kind ---${NC}"
    echo "$output" | yq "select(.kind == \"$kind\")" -
    cleanup_test
    exit 1
  else
    echo -e "${GREEN} SUCCESS: $kind is NOT Ready (as expected).${NC}"
  fi
}