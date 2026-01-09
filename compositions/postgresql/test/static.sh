#!/bin/bash

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

source "$SCRIPT_DIR/../../../test/lib.sh"
source "$SCRIPT_DIR/mocks.sh"

INPUT="$SCRIPT_DIR/../examples/instance.yaml"
COMPOSITION="$SCRIPT_DIR/../apis/instance-composition.yaml"
FUNC_CONFIG="$SCRIPT_DIR/database-function.yaml"
ENV_CONFIG="$SCRIPT_DIR/../examples/environment-config.yaml"

setup_function "$SCRIPT_DIR/../../../functions/database"

mock_environment "$ENV_CONFIG"
yq -i '
  select(.kind == "PostgreSQLInstance") |
  .metadata.uid = "000000000000"
 ' "$INPUT"

echo "TEST 1: rendering SecurityGroup and SecurityGroupRules..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "SecurityGroup" 1 "SecurityGroupRule" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_sg_ready_status > "$OBSERVED_RESOURCES"

echo "TEST 2: rendering RDS Instance..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "Instance" 1

echo "Mocking observed resources"
echo "---" >> $OBSERVED_RESOURCES
echo "$OUTPUT" | mock_rds_instance_ready_status >> "$OBSERVED_RESOURCES"

echo "TEST 3: rendering ExternalSecret and ProviderConfig..."
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "ExternalSecret" 1 "ProviderConfig" 1

echo "Mocking observed resources"
echo "---" >> $OBSERVED_RESOURCES
echo "$OUTPUT" | mock_es_and_config_ready_status >> "$OBSERVED_RESOURCES"

echo "TEST 4: Checking Readiness"
OUTPUT=$(run_render "$INPUT" "$COMPOSITION" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "PostgreSQLInstance"