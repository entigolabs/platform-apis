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

mock_pg_owner_role_as_extra_resource() {
  cat <<'EOF'
apiVersion: postgresql.sql.m.crossplane.io/v1alpha1
kind: Role
metadata:
  name: owner
  labels:
    database.entigo.com/role-name: owner
status:
  conditions:
    - type: Ready
      status: "True"
EOF
}

mock_pg_database_as_observed_resource() {
  yq '
    select(.kind == "Database") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_extension_as_observed_resource() {
  yq '
    select(.kind == "Extension") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
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

mock_pg_grant_without_ready() {
  yq '
    select(.kind == "Grant") |
    .status.conditions = [{"type": "Synced", "status": "True"}]
  ' -
}

mock_pg_extension_without_ready() {
  yq '
    select(.kind == "Extension") |
    .status.conditions = [{"type": "Synced", "status": "True"}]
  ' -
}

mock_pg_grant_usage_as_observed_resource() {
  yq '
    select(.kind == "Usage" and .metadata.annotations["crossplane.io/composition-resource-name"] == "grant-usage") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_owner_protection_as_observed_resource() {
  yq '
    select(.kind == "Usage" and .metadata.annotations["crossplane.io/composition-resource-name"] == "owner-protection") |
    .status.conditions = [{"type": "Synced", "status": "True"}, {"type": "Ready", "status": "True"}]
  ' -
}

mock_pg_instance_protection_as_observed_resource() {
  yq '
    select(.kind == "Usage" and .metadata.annotations["crossplane.io/composition-resource-name"] == "instance-protection") |
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
    select(.kind == "SecurityGroup" or .kind == "SecurityGroupRule" or .kind == "ProviderConfig") |
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
