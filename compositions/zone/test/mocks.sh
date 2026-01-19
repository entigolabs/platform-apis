mock_observed_resources() {
  yq '
    select(.kind == "AccessEntry" or .kind == "AppProject" or .kind == "MutatingPolicy" or .kind == "ValidatingPolicy" or .kind == "LaunchTemplate" or .kind == "Namespace" or .kind == "NetworkPolicy" or .kind == "RoleBinding" or .kind == "Role" or .kind == "RolePolicyAttachment") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

#mock_netpol_role_ready_status() {
#  yq '
#    select(.kind == "NetworkPolicy" or .kind == "Role") |
#    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
#  ' -
#}