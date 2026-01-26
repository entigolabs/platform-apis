mock_observed_resources() {
  yq '
    select((.kind == "CronJob" and .apiVersion == "batch/v1") or .kind == "Service") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_sec_as_observed_resource() {
  mock_select "Secret"
}