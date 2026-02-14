source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  yq '
    (select(.kind == "ReplicationGroup") |
      .status.atProvider.primaryEndpointAddress = "example.cache.amazonaws.com" |
      .status.atProvider.port = 6379 |
      .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}] |
      .connectionDetails."attribute.auth_token" = "dGVzdC1hdXRoLXRva2Vu" |
      .connectionDetails.port = "NjM3OQ==" |
      .connectionDetails.primary_endpoint_address = "ZXhhbXBsZS5jYWNoZS5hbWF6b25hd3MuY29t" |
      .connectionDetails.reader_endpoint_address = "ZXhhbXBsZS1yby5jYWNoZS5hbWF6b25hd3MuY29t"
    ),
    (select(.kind == "SecurityGroup" or .kind == "SecurityGroupRule" or .kind == "Secret" or .kind == "SecretVersion") |
      .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
    )
  ' -
}
