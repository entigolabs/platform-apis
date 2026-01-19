mock_observed_resources() {
  yq eval '
    select(.kind == "MSK") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_cluster_as_observed_resource() {
  yq eval '
    select(.kind == "Cluster") |
    .status.atProvider.bootstrapBrokersSaslIam = "test-broker-saas-iam"|
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}