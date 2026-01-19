mock_observed_resources() {
  yq '
    select(.kind == "ExternalSecret" or .kind == "ProviderConfig") |
    .status.conditions = [{"type": "Ready", "status": "True"}]
  ' -
}

mock_sg_as_observed_resource() {
  yq '
    select(.kind == "SecurityGroup" or .kind == "SecurityGroupRule") |
    .status.atProvider.securityGroupId = "sg-mock-123" |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_rds_instance_as_observed_resource() {
  yq '
    select(.kind == "Instance") |
    .status.atProvider.status = "Available" |
    .status.atProvider.address = "mock-db.cluster-123.eu-north-1.rds.amazonaws.com" |
    .status.atProvider.port = 5432 |
    .status.atProvider.hostedZoneId = "mock-zone" |
    .status.atProvider.masterUserSecret = [{"secretArn": "arn:aws:kms:eu-north-1:012345678901:key/mrk-1", "secretStatus": "active"}] |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}