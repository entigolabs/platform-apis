source "/workspace/test/mock_lib.sh"

mock_observed_resources() {
  mock_ready "AccessEntry" "AppProject" "MutatingPolicy" "ValidatingPolicy" "LaunchTemplate" "Namespace" "NetworkPolicy" "RoleBinding" "Role" "RolePolicyAttachment"
}