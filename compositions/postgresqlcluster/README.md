# PostgreSQLCluster API

`PostgreSQLCluster` (`database.entigo.com/v1alpha2`) is a PostgreSQL cluster whose spec stays the
same whatever runs it. The deployment chooses the backend; the cluster's `providerConfigs` entry for
that backend holds its own settings, such as the OCI shape.

## Known Limits of the OCI Backend

The OCI backend composes a `PsqlDbSystem` through crossplane-provider-oci v2.0.0, which embeds
Terraform OCI provider 9.0.0. These limits come from that provider and stay until it is upgraded.

**A provider restart during a create leaves a second DB system.**

1. A cluster is created, and the provider starts creating its DB system, which takes 5 to 15
   minutes.
2. The provider pod restarts before OCI reports the DB system active.
3. The restarted provider has no ID for the DB system and creates another one. The first stays in
   OCI, unmanaged and billed.

A restart has the same effect while the provider recreates a DB system someone deleted in OCI:
until the new one is active, the managed resource still names the deleted OCID.

Find a leftover by its freeform tag `entigo-resource`, `<namespace>-<cluster>`: a DB system with the
tag whose OCID no `PsqlDbSystem` names in `crossplane.io/external-name` is one. Upstream:
[crossplane/upjet#561](https://github.com/crossplane/upjet/issues/561).

**A password reset leaves a DB system that never scaled out of sync.**

1. A cluster runs without having added or removed an instance since it was created.
2. Its password changes: someone edits the password Secret, or the function generates a new one
   because the Secret was deleted while the DB system was being created.
3. The provider resets the password in OCI, then sends an empty instance patch, which OCI refuses.
4. The DB system stays `Synced: False` and the cluster `Ready: False`. Every retry resets the
   password again; later changes still reach OCI.

A DB system that has added or removed an instance since its creation is not affected, so turning
`highAvailability` on and off once clears a stuck one and keeps later password changes safe.
Terraform OCI provider 9.11.0 skips the empty patch.

**A major version change is refused.** Changing `spec.version` would replace the DB system; the
in-place upgrade needs Terraform OCI provider 9.11.0. Upstream:
[oracle/crossplane-provider-oci#156](https://github.com/oracle/crossplane-provider-oci/issues/156).

**A failover during a scale-down can stall it.** OCI does not report which instance is primary and
refuses to remove it. The function removes another instance after the refusal, but when the
primary fails over in between, the second removal is refused as well. Turning high availability on
and off again clears it. Upstream:
[oracle/crossplane-provider-oci#157](https://github.com/oracle/crossplane-provider-oci/issues/157).
