#!/bin/bash

FUNC_PID=""
EXTRA_RESOURCES="extra-resources-mock.yaml"
OBSERVED_RESOURCES="observed-resources-mock.yaml"

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

setup_function() {
    local func_path="$1"
    echo "--- Starting Function at $func_path ---"

    pushd "$func_path" > /dev/null
    go run . --insecure --debug &
    FUNC_PID=$!

    popd > /dev/null

    echo "Waiting for function to be ready on port 9443..."
        local retries=0
        while ! nc -z localhost 9443; do
            sleep 1
            retries=$((retries+1))
            if [ "$retries" -ge 60 ]; then
                echo "${RED}Timeout waiting for function to start."
                kill $FUNC_PID
                exit 1
            fi
        done
        echo "✔ Function is ready!"
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
    crossplane render "$INPUT" "$COMPOSITION" "$FUNC_CONFIG" \
      -e "$EXTRA_RESOURCES" \
      -o "$OBSERVED_RESOURCES" 2>&1
}

append_mock() {
    local content="$1"
    echo "---" >> "$OBSERVED_RESOURCES"
    echo "$content" >> "$OBSERVED_RESOURCES"
}

cleanup_test() {
    #rm -f "$OBSERVED_RESOURCES"

    if [ ! -z "$FUNC_PID" ]; then
        echo "Stopping Function (PID $FUNC_PID)..."
        kill -9 "$FUNC_PID" 2>/dev/null || true
    fi
}
