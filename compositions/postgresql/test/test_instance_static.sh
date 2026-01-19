load_mocks

INPUT="../examples/instance.yaml"
COMPOSITION="../apis/instance-composition.yaml"
FUNC_CONFIG="database-function.yaml"
ENV_CONFIG="../examples/environment-config.yaml"

setup_function "/workspace/functions/database"

cp "../examples/required-resources.yaml" "$EXTRA_RESOURCES"
mock_environment "$ENV_CONFIG"

TEMP_INPUT="temp_instance.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "PostgreSQLInstance") |
  .metadata.uid = "000000000000"
 ' "$TEMP_INPUT"

echo "TEST 1: rendering SecurityGroup and SecurityGroupRules..."
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES")
assert_counts "$OUTPUT" "SecurityGroup" 1 "SecurityGroupRule" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_sg_as_observed_resource > "$OBSERVED_RESOURCES"

echo "TEST 2: rendering RDS Instance..."
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "Instance" 1

echo "Mocking observed resources"
echo "---" >> $OBSERVED_RESOURCES
echo "$OUTPUT" | mock_rds_instance_as_observed_resource >> "$OBSERVED_RESOURCES"

echo "TEST 3: rendering ExternalSecret and ProviderConfig..."
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_counts "$OUTPUT" "ExternalSecret" 1 "ProviderConfig" 1

echo "Mocking observed resources"
echo "---" >> $OBSERVED_RESOURCES
echo "$OUTPUT" | mock_observed_resources >> "$OBSERVED_RESOURCES"

echo "INSTANCE. TEST 4: Checking PostgreSQLInstance Readiness"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG" "$EXTRA_RESOURCES" "$OBSERVED_RESOURCES")
assert_ready "$OUTPUT" "PostgreSQLInstance"