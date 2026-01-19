mock_observed_resources() {
  yq eval '
      select(.kind == "Service") |
      .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Available", "status": "True"}]
    ' -
}

mock_dep_as_observed_resource() {
  yq eval '
    select(.kind == "Deployment") |
    .status.readyReplicas = 1 |
    .status.replicas = 1 |
    .status.updatedReplicas = 1 |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Available", "status": "True"}]
  ' -
}

mock_sec_as_observed_resource() {
  yq eval '
      select(.kind == "Secret")
     ' -
}