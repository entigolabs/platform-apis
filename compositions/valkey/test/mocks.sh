source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  mock_ready "ReplicationGroup" "SecurityGroup" "SecurityGroupRule" "Secret" "SecretVersion"
}
