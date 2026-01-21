source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  mock_ready "MSK"
}

mock_cluster_as_observed_resource() {
  yq '
    select(.kind == "Cluster") |
    .status.atProvider.bootstrapBrokersSaslIam = "test-broker-saas-iam" |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}
