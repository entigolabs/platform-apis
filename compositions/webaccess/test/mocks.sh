mock_observed_resources() {
  yq eval '
    select(.kind == "VirtualService" or .kind == "ServiceEntry" or .kind == "DestinationRule") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}