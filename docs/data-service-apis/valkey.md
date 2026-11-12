# Valkey

Applies the rules in [rules.md](rules.md). Can be installed on AWS and OCI (rule 7).
Schema: [valkeyinstance.yaml](schemas/valkeyinstance.yaml).

`ValkeyInstance` is one primary node with replicas, without sharding. Its concepts are checked
against ElastiCache (AWS), Memorystore for Valkey (GCP) and OCI Cache (OCI). Azure Managed Redis
does not count as Valkey: it runs Redis 7.4 on Redis Enterprise, with no version choice, no backups
and a 25 GB limit without clustering.

## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Platform Decisions](#platform-decisions)
- [Connection Secret](#connection-secret)
- [Status](#status)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Spec

What a user writes to create a `ValkeyInstance`. The comments note value ranges and what holds on
one cloud only.

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyInstance
metadata:
  name: orders-cache
  namespace: shop
spec:
  # Common
  version: "8.1"                      # the least version; raising it upgrades
  highAvailability: true               # a replica in another zone, with automatic failover
  replicas: 1
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 5
    autoMinorVersionUpgrade: true
  backup:
    retentionDays: 7                   # 0 disables backups
    startHourUTC: 3
  parameters:                          # Valkey settings, within each cloud's allowlist
    maxmemory-policy: allkeys-lru
  # Cloud-specific; every block also takes id (rule 9)
  aws:
    instanceType: cache.t4g.small      # the ElastiCache node type
    parameterGroupName: orders-valkey8 # not with parameters
  oci:
    memoryGB: 16                       # per node
```

Replicas serve reads, and high availability places at least one of them in another zone, so
`highAvailability: true` with `replicas: 0` is rejected. On OCI, which sizes nodes by memory only,
the deployment default `oci.memoryGB` sizes an instance whose spec is empty.

`version` is Valkey's major and minor, such as `"8.1"`, the only form all three clouds take
(ElastiCache rejects `8` and `8.x`; GCP and OCI name versions `VALKEY_8_1`). With
`maintenance.autoMinorVersionUpgrade` on, the cloud can move the cache to a newer minor, so
`version` is a floor: raising it upgrades the cache, lowering it is rejected, the status shows the
running version, and the platform never moves the cache back to `version`.

`aws.parameterGroupName`, kept from today's API, uses an existing ElastiCache parameter group in
place of `parameters`. A parameter group applies to one major version (family `valkey<major>`), so a
change of the major is rejected unless `aws.parameterGroupName` changes or is removed in the same
edit.

## Mapping

How each field maps to the managed service of each cloud, named by the fields of that cloud's
Crossplane provider. Level is the field's support level (rule 1).

| Field                                 | Level                                      | AWS (ElastiCache replication group)          | GCP (Memorystore `Instance`), no password auth in provider                                                        | OCI (OCI Cache `RedisCluster`)                                             |
| ------------------------------------- | ------------------------------------------ | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `observe`                             | Core                                       | `managementPolicies: [Observe]`              | `managementPolicies: [Observe]`                                                                                   | `managementPolicies: [Observe]`                                            |
| `version`                             | Core                                       | `engineVersion`                              | `engineVersion: VALKEY_8_1`                                                                                       | `softwareVersion: VALKEY_8_1`                                              |
| `highAvailability`                    | Core                                       | `multiAzEnabled`, automatic failover         | `zoneDistributionConfig.mode`: `MULTI_ZONE`, or `SINGLE_ZONE` for `false`                                         | the service distributes nodes itself; `false` ignored; failover unverified |
| `replicas`                            | Core                                       | `numCacheClusters` = replicas + 1            | `replicaCount`, with `mode: CLUSTER_DISABLED` and `shardCount: 1`, which the function always sends                | `nodeCount` = replicas + 1                                                 |
| `deletionProtection`                  | Core                                       | platform blocks deletion                     | `deletionProtectionEnabled`                                                                                       | platform blocks deletion                                                   |
| `maintenance.window`                  | Extended: AWS, GCP                         | `maintenanceWindow`                          | `maintenancePolicy.weeklyMaintenanceWindow` (`day`, `startTime.hours`)                                            | rejected; OCI patches on its own schedule                                  |
| `maintenance.autoMinorVersionUpgrade` | Core for `true`; Extended for `false`: AWS | `autoMinorVersionUpgrade`                    | `false` rejected; Memorystore cannot opt out of maintenance                                                       | `false` rejected                                                           |
| `backup.retentionDays`                | Extended: AWS, GCP                         | `snapshotRetentionLimit`; 0 disables backups | `automatedBackupConfig.retention`, in seconds (`"604800s"` for 7 days); 0 omits the block, which disables backups | rejected; OCI Cache has manual backups only                                |
| `backup.startHourUTC`                 | Extended: AWS, GCP                         | `snapshotWindow`                             | `automatedBackupConfig.fixedFrequencySchedule.startTime.hours`                                                    | rejected                                                                   |
| `parameters`                          | Core                                       | parameter group                              | `engineConfigs`, allowlist of 7                                                                                   | config set, allowlist of about 15 keys                                     |

The cloud-specific fields:

| Field                    | Maps To                  |
| ------------------------ | ------------------------ |
| `aws.id`                 | the replication group ID |
| `gcp.id`                 | the instance ID          |
| `aws.instanceType`       | `nodeType`               |
| `gcp.instanceType`       | `nodeType`               |
| `oci.id`                 | the cluster's OCID       |
| `aws.parameterGroupName` | `parameterGroupName`     |
| `oci.memoryGB`           | `nodeMemoryInGbs`        |

## Platform Decisions

Network placement (subnet, security group or NSG), TLS, encryption at rest with the platform's data
key on AWS, the password, and a final snapshot when a cache is deleted, switched on per deployment
as today, on AWS (`finalSnapshotIdentifier`), the only cloud that takes one: OCI Cache takes manual
backups only, and Memorystore has no final backup. On OCI the platform creates an OCI Cache user
named `default` with the password's SHA-256 hash, which replaces the built-in user that has no
password.

## Connection Secret

`<instance>-credentials`, with `endpoint`, `port` (6379), `username: default`, `password` and
`readerEndpoint`. `default` is the user Valkey logs a password-only client in as, so a client that
always sends a username and password works unchanged.

## Status

Adds `readerEndpoint: { host, port }` (on OCI from `replicasFqdn`), and `finalBackup`, the ID the
final snapshot will get. A deleted cache's ID, which the status loses with the cache, is taken from
the cloud console.

```yaml
status:
  aws:
    id: ...
    engineVersionActual: ...           # ElastiCache reports no state
  gcp:
    id: ...
  oci:
    id: ...
    state: ACTIVE
    softwareVersion: VALKEY_8_1
```

## Cloud Limits

Limits each cloud enforces itself, which the API does not check (rule 10); a value outside them is
rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference, should
validating them be decided later.

| Field                  | Cloud | Limit    | Source                                                                                  |
| ---------------------- | ----- | -------- | --------------------------------------------------------------------------------------- |
| `replicas`             | AWS   | up to 5  | `numCacheClusters` up to 6, ElastiCache `ReplicationGroup`                              |
| `replicas`             | GCP   | up to 5  | `replicaCount`, Memorystore `Instance` CRD, provider-upjet-gcp v3.1.0                   |
| `replicas`             | OCI   | up to 4  | `nodeCount` up to 5, `RedisCluster` CRD, crossplane-provider-oci v2.0.0                 |
| `oci.memoryGB`         | OCI   | 2 to 500 | `nodeMemoryInGbs`, `RedisCluster` CRD                                                   |
| `backup.retentionDays` | AWS   | up to 35 | `snapshotRetentionLimit`, ElastiCache `ReplicationGroup` CRD, provider-upjet-aws v2.4.0 |
| `backup.retentionDays` | GCP   | 1 to 365 | `automatedBackupConfig.retention`, Memorystore `Instance` CRD                           |

## Notes

Facts about this service that the design relies on and that need no decision.

- GCP: Memorystore for Valkey has password auth (`TOKEN_AUTH`), but [provider-upjet-gcp
  v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0) offers only
  IAM auth, whose hour-long tokens no connection secret can hold. Its TLS also needs the instance's
  CA in the client, which the connection secret would carry.
