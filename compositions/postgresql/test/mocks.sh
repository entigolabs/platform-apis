mock_pg_instance_as_extra_resource() {
  cat <<'EOF'
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: postgresql-example
status:
  conditions:
    - type: Ready
      status: "True"
EOF
}

mock_pg_user_as_extra_resource() {
  cat <<'EOF'
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLUser
metadata:
  name: owner
status:
  conditions:
    - type: Ready
      status: "True"
EOF
}

mock_observed_resources() {
  yq '
    select(.kind == "ExternalSecret" or .kind == "ProviderConfig") |
    .status.conditions = [{"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_role_as_observed_resource() {
  yq '
    select(.kind == "Role") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_grant_as_observed_resource() {
  yq '
    select(.kind == "Grant") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_grant_usage_as_observed_resource() {
  yq '
    select(.kind == "Usage" and .metadata.annotations["crossplane.io/composition-resource-name"] == "grant-usage") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_usage_grants_as_observed_resource() {
  yq '
    select(.kind == "Usage" and (.metadata.annotations["crossplane.io/composition-resource-name"] // "" | test("^usage-grant-"))) |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_sg_as_observed_resource() {
  yq '
    select(.kind == "SecurityGroup" or .kind == "SecurityGroupRule") |
    .status.atProvider.securityGroupId = "sg-mock-123" |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_rds_instance_as_observed_resource() {
  yq '
    select(.kind == "Instance") |
    .status.atProvider.status = "Available" |
    .status.atProvider.address = "mock-db.cluster-123.eu-north-1.rds.amazonaws.com" |
    .status.atProvider.port = 5432 |
    .status.atProvider.hostedZoneId = "mock-zone" |
    .status.atProvider.masterUserSecret = [{"secretArn": "arn:aws:kms:eu-north-1:012345678901:key/mrk-1", "secretStatus": "active"}] |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}
