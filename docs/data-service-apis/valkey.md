# Valkey

Valkey is an in-memory key-value store, an open-source fork of Redis, that applications use as a
cache, session store or message broker.

One kind: `ValkeyInstance`, one primary node with read replicas.

Built on ElastiCache (AWS) and OCI Cache (OCI). GCP's Memorystore for Valkey is mapped, but the kind
exists on GCP only once its Crossplane provider supports password login, which the connection secret
needs (rule 6). Azure has none: Azure Managed Redis runs Redis, not Valkey.

[API Changes](changes.md#valkey) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Kinds

Valkey is offered as one kind, shown below with a manifest a user writes.

### ValkeyInstance

An instance has no sharding: its data lives on the primary and is copied to each replica.

A user creates a `ValkeyInstance` with a manifest like the one below. Its comments note value ranges
and what holds on one cloud only:

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
  secretName: orders-cache-login       # unset, orders-cache-credentials (rule 6)
  # Cloud-specific; every block also takes id (rule 9)
  aws:
    instanceType: cache.t4g.small      # the ElastiCache node type
    # parameterGroupName: orders-valkey8  # an existing group, instead of parameters
    # fromBackup: orders-cache-final   # a snapshot name, at creation only
  gcp:
    instanceType: HIGHMEM_MEDIUM       # the Memorystore node type
  oci:
    memoryGB: 16                       # per node
```

*A `ValkeyInstance` with its common fields and a block per cloud.*

Replicas serve reads, and high availability places at least one of them in another zone, so
`highAvailability: true` with `replicas: 0` is rejected.

`version` is Valkey's major and minor, such as `"8.1"`, the only form all three clouds take. With
`maintenance.autoMinorVersionUpgrade` on, the cloud can move the cache to a newer minor, so
`version` is a floor: raising it upgrades the cache, lowering it is rejected, the status shows the
running version, and the platform never moves the cache back to `version`.

The status adds `readerEndpoint: { host, port }`. A deleted cache is restored with its cloud block's
`fromBackup` and the final backup's ID, which the status loses with the cache, so it is taken from
the cloud console.

The cloud blocks of the status:

```yaml
status:
  aws:
    finalBackup: ...
    id: ...
    engineVersionActual: ...           # ElastiCache reports no state
  gcp:
    id: ...
  oci:
    id: ...
    state: ACTIVE
    softwareVersion: VALKEY_8_1
```

*The cloud blocks of a `ValkeyInstance` status.*

The connection secret is `<instance>-credentials`, with `endpoint`, `port` (6379), `username:
default`, `password` and `readerEndpoint`. `default` is the user Valkey logs a password-only client
in as, so a client that always sends a username and password works unchanged.

## Mapping

The table names what each field maps to in the cloud's provider. A setting the cloud lacks shows as
ignored, with the reason (rule 1), and a field of a cloud's block appears only for that cloud:

| Field                                 | AWS                                          | GCP                                                                                                               | OCI                                                                        |
| ------------------------------------- | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| `observe`                             | `managementPolicies: [Observe]`              | `managementPolicies: [Observe]`                                                                                   | `managementPolicies: [Observe]`                                            |
| `version`                             | `engineVersion`                              | `engineVersion: VALKEY_8_1`                                                                                       | `softwareVersion: VALKEY_8_1`                                              |
| `highAvailability`                    | `multiAzEnabled`, automatic failover         | `zoneDistributionConfig.mode`: `MULTI_ZONE`, or `SINGLE_ZONE` for `false`                                         | the service distributes nodes itself; `false` ignored; failover unverified |
| `replicas`                            | `numCacheClusters` = replicas + 1            | `replicaCount`, with `mode: CLUSTER_DISABLED` and `shardCount: 1`, which the function always sends                | `nodeCount` = replicas + 1                                                 |
| `deletionProtection`                  | platform blocks deletion                     | `deletionProtectionEnabled`                                                                                       | platform blocks deletion                                                   |
| `maintenance.window`                  | `maintenanceWindow`                          | `maintenancePolicy.weeklyMaintenanceWindow` (`day`, `startTime.hours`)                                            | ignored; OCI patches on its own schedule                                   |
| `maintenance.autoMinorVersionUpgrade` | `autoMinorVersionUpgrade`                    | `false` ignored; Memorystore cannot opt out of maintenance                                                        | `false` ignored; OCI applies minor versions on its own schedule            |
| `backup.retentionDays`                | `snapshotRetentionLimit`; 0 disables backups | `automatedBackupConfig.retention`, in seconds (`"604800s"` for 7 days); 0 omits the block, which disables backups | ignored; OCI Cache has manual backups only                                 |
| `backup.startHourUTC`                 | `snapshotWindow`                             | `automatedBackupConfig.fixedFrequencySchedule.startTime.hours`                                                    | ignored                                                                    |
| `parameters`                          | parameter group                              | `engineConfigs`, allowlist of 7                                                                                   | config set, allowlist of about 15 keys                                     |
| `aws.id`                              | the replication group ID                     |                                                                                                                   |                                                                            |
| `gcp.id`                              |                                              | the instance ID                                                                                                   |                                                                            |
| `aws.instanceType`                    | `nodeType`                                   |                                                                                                                   |                                                                            |
| `gcp.instanceType`                    |                                              | `nodeType`                                                                                                        |                                                                            |
| `oci.id`                              |                                              |                                                                                                                   | the cluster's OCID                                                         |
| `aws.parameterGroupName`              | `parameterGroupName`                         |                                                                                                                   |                                                                            |
| `oci.memoryGB`                        |                                              |                                                                                                                   | `nodeMemoryInGbs`                                                          |
| `aws.fromBackup`                      | `snapshotName`, at creation only             |                                                                                                                   |                                                                            |

*Fields of `ValkeyInstance` and their provider fields.*

**AWS:** ElastiCache rejects a version without its minor, such as `8` or `8.x`. A parameter group
applies to one major version (family `valkey<major>`), so a change of the major is rejected unless
`aws.parameterGroupName` changes or is removed in the same edit. `status.aws.finalBackup` is the ID
the final snapshot will receive.

**GCP:** the provider's Memorystore `Instance` offers no password authentication. Data at rest uses
the service's own key, and Memorystore takes no final backup.

**OCI:** nodes are sized by memory only, so the deployment default `oci.memoryGB` sizes an instance
whose spec is empty. `readerEndpoint` comes from `replicasFqdn`. Data at rest uses the service's own
key. The password is set on a user named `default`, as its SHA-256 hash, which replaces the built-in
user that has no password. OCI Cache takes manual backups only, so no final backup is taken.

## Composed Resources

The same `ValkeyInstance`, from [Kinds](#kinds), is composed into different resources on each cloud:
the platform uses the block of the cloud it runs on and ignores the others. A comment names the
source of each value the object does not hold as written: `platform` for a value the platform sets,
and `deployment` for one the deployment sets. The object and its resources:

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyInstance
metadata:
  name: orders-cache
  namespace: shop
spec:
  version: "8.1"
  highAvailability: true
  replicas: 1
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 5
    autoMinorVersionUpgrade: true
  backup:
    retentionDays: 7
    startHourUTC: 3
  parameters:
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login
  aws:
    instanceType: cache.t4g.small
  gcp:
    instanceType: HIGHMEM_MEDIUM
  oci:
    memoryGB: 16
```

```yaml AWS resource
---
apiVersion: elasticache.aws.m.upbound.io/v1beta1
kind: ReplicationGroup
metadata:
  name: orders-cache-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    region: eu-north-1         # deployment
    description: orders-cache
    engine: valkey
    engineVersion: "8.1"
    nodeType: cache.t4g.small
    numCacheClusters: 2        # replicas + 1
    multiAzEnabled: true
    automaticFailoverEnabled: true
    maintenanceWindow: sun:05:00-sun:06:00
    autoMinorVersionUpgrade: "true"
    snapshotRetentionLimit: 7
    snapshotWindow: 03:00-04:00
    parameterGroupName: orders-cache-7f3a2c  # composed from parameters
    subnetGroupName: deployment-cache  # platform
    securityGroupIds:          # platform
    - sg-0123456789abcdef0
    transitEncryptionEnabled: true  # platform
    atRestEncryptionEnabled: "true"  # platform
    kmsKeyId: arn:aws:kms:eu-north-1:111122223333:key/data  # platform
    authTokenSecretRef:        # platform
      name: orders-cache-login
      key: password
```

*A `ValkeyInstance` on AWS and its composed resource.*

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyInstance
metadata:
  name: orders-cache
  namespace: shop
spec:
  version: "8.1"
  highAvailability: true
  replicas: 1
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 5
    autoMinorVersionUpgrade: true
  backup:
    retentionDays: 7
    startHourUTC: 3
  parameters:
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login
  aws:
    instanceType: cache.t4g.small
  gcp:
    instanceType: HIGHMEM_MEDIUM
  oci:
    memoryGB: 16
```

```yaml GCP resource
---
apiVersion: memorystore.gcp.m.upbound.io/v1beta1
kind: Instance
metadata:
  name: orders-cache-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    project: shop-project      # platform
    location: europe-north1    # deployment
    engineVersion: VALKEY_8_1
    nodeType: HIGHMEM_MEDIUM
    mode: CLUSTER_DISABLED     # platform
    shardCount: 1              # platform
    replicaCount: 1
    zoneDistributionConfig:
      mode: MULTI_ZONE
    deletionProtectionEnabled: true
    maintenancePolicy:
      weeklyMaintenanceWindow:
      - day: SUNDAY
        startTime:
          hours: 5
    automatedBackupConfig:
      retention: 604800s       # 7 days
      fixedFrequencySchedule:
        startTime:
          hours: 3
    engineConfigs:
      maxmemory-policy: allkeys-lru
    transitEncryptionMode: SERVER_AUTHENTICATION  # platform
    desiredPscAutoConnections:  # platform
    - network: projects/shop-project/global/networks/deployment
      projectId: shop-project
```

*A `ValkeyInstance` on GCP and its composed resource.*

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyInstance
metadata:
  name: orders-cache
  namespace: shop
spec:
  version: "8.1"
  highAvailability: true
  replicas: 1
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 5
    autoMinorVersionUpgrade: true
  backup:
    retentionDays: 7
    startHourUTC: 3
  parameters:
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login
  aws:
    instanceType: cache.t4g.small
  gcp:
    instanceType: HIGHMEM_MEDIUM
  oci:
    memoryGB: 16
```

```yaml OCI resource
---
apiVersion: redis.oci.m.upbound.io/v1alpha1
kind: RedisCluster
metadata:
  name: orders-cache-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # platform
    displayName: orders-cache-7f3a2c
    softwareVersion: VALKEY_8_1
    clusterMode: NONSHARDED
    nodeCount: 2               # replicas + 1
    nodeMemoryInGbs: 16
    ociCacheConfigSetId: ocid1.ocicacheconfigset.oc1..orders  # composed from parameters
    subnetId: ocid1.subnet.oc1..deployment  # platform
    nsgIds:                    # platform
    - ocid1.networksecuritygroup.oc1..deployment
```

*A `ValkeyInstance` on OCI and its composed resource.*

## Platform Decisions

The platform makes these settings itself:

| Setting           | Platform's Choice                                                                               |
| ----------------- | ----------------------------------------------------------------------------------------------- |
| Network placement | The deployment's private network                                                                |
| Encryption        | TLS in transit; at rest, the platform's data key where the cloud takes one                      |
| Login             | A generated password                                                                            |
| Final backup      | A snapshot taken when a cache is deleted, switched on per deployment, where the cloud takes one |

*Settings the platform makes for a `ValkeyInstance`.*

## Cloud Limits

The cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later. Each limit holds as of its source: the provider release
named, or the cloud's documentation as read in November 2026.

| Field                  | Cloud | Limit    | Source                                                                                   |
| ---------------------- | ----- | -------- | ---------------------------------------------------------------------------------------- |
| `replicas`             | AWS   | up to 5  | `numCacheClusters` up to 6, ElastiCache `ReplicationGroup`                               |
| `replicas`             | GCP   | up to 5  | `replicaCount`, Memorystore `Instance` CRD, provider-upjet-gcp v3.1.0                    |
| `replicas`             | OCI   | up to 4  | `nodeCount` up to 5, `RedisCluster` CRD, crossplane-provider-oci v2.0.0                  |
| `oci.memoryGB`         | OCI   | 2 to 500 | `nodeMemoryInGbs`, `RedisCluster` CRD, crossplane-provider-oci v2.0.0                    |
| `backup.retentionDays` | AWS   | up to 35 | `snapshotRetentionLimit`, ElastiCache `ReplicationGroup` CRD, provider-upjet-aws v2.4.0  |
| `backup.retentionDays` | GCP   | 1 to 365 | `automatedBackupConfig.retention`, Memorystore `Instance` CRD, provider-upjet-gcp v3.1.0 |

*Limits the cloud enforces on Valkey.*

## Notes

The design relies on these facts, which need no decision.

- Azure: Azure Managed Redis runs Redis 7.4 on Redis Enterprise, with no version choice, no backups
  and a 25 GB limit without clustering.
- GCP: Memorystore for Valkey has password authentication (`TOKEN_AUTH`), but [provider-upjet-gcp
  v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0) offers only
  IAM authentication, whose hour-long tokens no connection secret can hold. Its TLS also needs the
  instance's CA in the client, which the connection secret would carry.