mock_vs_se_and_dr_ready_statuses() {
  yq eval '
    select(.kind == "VirtualService" or .kind == "ServiceEntry" or .kind == "DestinationRule") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}