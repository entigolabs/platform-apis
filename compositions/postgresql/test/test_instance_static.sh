load_mocks
init_test "instance" "/workspace/functions/database"
setup_resources --env --required

TEMP_INPUT="temp_instance_1i.yaml"
cp "$INPUT" "$TEMP_INPUT"
yq -i '
  select(.kind == "PostgreSQLInstance") |
  .metadata.uid = "000000000000"
 ' "$TEMP_INPUT"

echo "TEST 1: rendering SecurityGroup and SecurityGroupRules..."
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "SecurityGroup" 1 "SecurityGroupRule" 2

echo "Mocking observed resources"
echo "$OUTPUT" | mock_sg_as_observed_resource | start_observed

echo "TEST 2: rendering RDS Instance..."
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "Instance" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_rds_instance_as_observed_resource | append_observed

echo "TEST 3: rendering ExternalSecret and ProviderConfig..."
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_counts "$OUTPUT" "ExternalSecret" 1 "ProviderConfig" 1

echo "Mocking observed resources"
echo "$OUTPUT" | mock_observed_resources | append_observed

echo "INSTANCE. TEST 4: Checking PostgreSQLInstance Readiness"
OUTPUT=$(run_render "$TEMP_INPUT" "$COMPOSITION" "$FUNC_CONFIG")
assert_ready "$OUTPUT" "PostgreSQLInstance"