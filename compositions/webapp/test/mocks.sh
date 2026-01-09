mock_dep_ready_status() {
  yq eval '
    select(.kind == "Deployment") |
    .status.readyReplicas = 1 |
    .status.replicas = 1 |
    .status.updatedReplicas = 1 |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Available", "status": "True"}]
  ' -
}

mock_ser_ready_status() {
  yq eval '
      select(.kind == "Service") |
      .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Available", "status": "True"}]
    ' -
}

mock_sec() {
  yq eval '
      select(.kind == "Secret")
     ' -
}