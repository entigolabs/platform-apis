#!/bin/bash

FUNC_PID=""
EXTRA_RESOURCES="extra-resources-mock.yaml"
OBSERVED_RESOURCES="observed-resources-mock.yaml"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

setup_binary_function() {
    local cmd="$1"
    local port="$2"

    if [ -z "$port" ]; then port="9443"; fi

    echo "--- Starting $cmd on port $port ---"

    $cmd --insecure --debug --address ":$port" &

    local pid=$!
    FUNC_PIDS+=($pid)

    wait_for_function "$port"
}

setup_function() {
    local func_path="$1"
    echo "--- Starting Custom Function at $func_path ---"

    pushd "$func_path" > /dev/null
    go run . --insecure --debug &
    local pid=$!
    FUNC_PIDS+=($pid)
    popd > /dev/null

    wait_for_function "9443"
}

wait_for_function() {
    local port="$1"
    echo "Waiting for function on port $port..."
    local retries=0
    while ! nc -z localhost $port; do
        sleep 1
        retries=$((retries+1))
        if [ "$retries" -ge 30 ]; then
            echo "${RED}Timeout waiting for function on port $port.${NC}"
            cleanup_test
            exit 1
        fi
    done
    echo "✔ Function is ready on $port!"
}

mock_environment() {
    local env_config_path="$1"
    echo "Mocking EnvironmentConfig..."

    touch "$EXTRA_RESOURCES"
    touch "$OBSERVED_RESOURCES"

    local mocked_env=$(cat "$EXTRA_RESOURCES" | yq eval 'select(.kind == "EnvironmentConfig") | length' - | wc -l | xargs)
    if [ "$mocked_env" -eq 0 ]; then
        echo "---" >> "$EXTRA_RESOURCES"
        yq 'select(.kind == "EnvironmentConfig")' "$env_config_path" >> "$EXTRA_RESOURCES"
    fi
}

assert_counts() {
    local output="$1"
    shift
    local failed=0

    if [[ "$output" == *"ERROR"* ]] || [[ "$output" == *"error"* ]]; then
            echo -e "${RED}Render Failed:${NC}"
            echo "$output"
            cleanup_test
            exit 1
    fi

    while (( "$#" )); do
        local kind=$1
        local expected=$2

        local actual=$(echo "$output" | yq eval "select(.kind == \"$kind\") | length" - | wc -l | xargs)

        if [ "$actual" -eq "$expected" ]; then
            echo -e "   ${GREEN}✔ $kind count: $actual/$expected${NC}"
        else
            echo -e "   ${RED}✘ $kind count: Expected $expected, found $actual${NC}"
            failed=1
        fi
        shift 2
    done

    if [ "$failed" -eq 1 ]; then
        echo "--- RENDER OUTPUT ---"
        echo "$output"
        cleanup_test
        exit 1
    fi
}

assert_ready() {
    local output="$1"
    local kind="$2"

    if echo "$output" | yq eval "select(.kind == \"$kind\")" - | grep -q 'status: "True"'; then
        echo -e "${GREEN} SUCCESS: $kind is Ready.${NC}"
    else
        echo -e "${RED} FAILED: $kind is still Unready.${NC}"
        echo "$output"
        cleanup_test
        exit 1
    fi
}

run_render() {
    local input="${1:-$INPUT}"
    local composition="${2:-$COMPOSITION}"
    local func_config="${3:-$FUNC_CONFIG}"

    touch "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES"

    crossplane render "$input" "$composition" "$func_config" \
        -e "$EXTRA_RESOURCES" \
        -o "$OBSERVED_RESOURCES" 2>&1 -r -x
}

append_mock() {
    local content="$1"
    echo "---" >> "$OBSERVED_RESOURCES"
    echo "$content" >> "$OBSERVED_RESOURCES"
}

cleanup_test() {
    rm -f "$OBSERVED_RESOURCES"

    if [ ${#FUNC_PIDS[@]} -gt 0 ]; then
      echo "Stopping Functions (PIDs: ${FUNC_PIDS[*]} )..."
      for pid in "${FUNC_PIDS[@]}"; do
        kill -9 "$pid" 2>/dev/null || true
      done
    fi
}
