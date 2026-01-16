mock_cluster_ready_status() {
  yq eval '
    select(.kind == "Cluster") |
    .status.atProvider.bootstrapBrokersSaslIam = "test-broker-saas-iam"|
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_msk_observer_ready_status() {
  yq eval '
    select(.kind == "MSK") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}