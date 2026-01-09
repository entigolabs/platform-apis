mock_cj_and_ser_ready_statuses() {
  yq eval '
    select((.kind == "CronJob" and .apiVersion == "batch/v1") or .kind == "Service") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_sec() {
  yq eval 'select(.kind == "Secret")' -
}