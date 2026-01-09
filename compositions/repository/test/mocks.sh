mock_rep_ready_status() {
  yq eval '
    select(.kind == "Repository" and .apiVersion == "ecr.aws.m.upbound.io/v1beta1") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}