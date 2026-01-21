source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  mock_available "Service"
}

mock_dep_as_observed_resource() {
  yq '
    select(.kind == "Deployment") |
    .status.readyReplicas = 1 |
    .status.replicas = 1 |
    .status.updatedReplicas = 1 |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Available", "status": "True"}]
  ' -
}

mock_sec_as_observed_resource() {
  mock_select "Secret"
}
