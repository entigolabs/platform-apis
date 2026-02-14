source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  yq '
    (select(.kind == "ReplicationGroup") |
      .status.atProvider.primaryEndpointAddress = "example.cache.amazonaws.com" |
      .status.atProvider.port = 6379 |
      .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
    ),
    (select(.kind == "SecurityGroup" or .kind == "SecurityGroupRule" or .kind == "Secret" or .kind == "SecretVersion") |
      .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
    )
  ' -
}
