# Valkey

Valkey is an in-memory key-value store, an open-source fork of Redis, that applications use as a
cache, session store or message broker.

One kind for users: `ValkeyServer`, one primary node with its read replicas. Two kinds shape it:
`ValkeyClass`, a deployment's defaults and limits, written by Entigo, and `ValkeyParameters`,
reusable Valkey settings.

Built on ElastiCache (AWS) and OCI Cache (OCI), each with a provider configuration kind of its own:
`ValkeyElastiCacheConfig` and `ValkeyOCICacheConfig`. GCP's Memorystore for Valkey is mapped, with
`ValkeyMemorystoreConfig`, but the kind exists on GCP only once its Crossplane provider supports
password login, which the connection secret needs (principle 6). Azure has none: Azure Managed Redis
runs Redis, not Valkey.

## Kinds

Valkey is offered as three kinds, each with the manifest its owner writes.

### ValkeyClass

Entigo writes each deployment's classes, with the defaults a server starts with, in `template`, and
the values it cannot change, in `enforced` (principle 5):

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyClass
metadata:
  name: standard
  annotations:
    database.entigo.com/is-default-class: "true"
spec:
  template:
    deletionProtection: true
    version: "8.1"
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: ValkeyElastiCacheConfig
      spec:
        instanceType: cache.t4g.medium
  enforced:                            # teams cannot change these
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: ValkeyElastiCacheConfig
      spec:
        backupRetentionDays: 7
```

*`ValkeyClass` for ElastiCache.*

Every backend supports `HighAvailability` and `ReadReplicas`.

### ValkeyParameters

`ValkeyParameters` holds Valkey settings that several servers or a class share. A class and a server
each list the ones they use in `parametersRefs`. The settings merge key by key, a later value
winning: the class's list in order, then the server's, then the server's own `parameters`, so a team
can put settings all its servers share beneath one server's own tuning:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyParameters
metadata:
  name: orders-cache-tuning
  namespace: shop
spec:
  parameters:
    maxmemory-policy: allkeys-lru
    timeout: "300"
```

*`ValkeyParameters` shared between a user's servers.*

A `ValkeyParameters` names no other, so references never chain. Settings the backend does not allow
are rejected by the cloud and reported as `CloudAccepted: False` (principle 10).

### ValkeyServer

A server has no sharding: its data is held on the primary and copied to each replica.

A user creates a `ValkeyServer` with a manifest like the one below. It names its class and carries
one provider configuration entry per backend it may run on (principle 2):

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  className: standard                  # unset, the default class; without one, standalone
  version: "8.1"                       # the least version; raising it upgrades
  highAvailability: true               # a replica in another zone, with automatic failover
  readReplicas: 1
  deletionProtection: true
  parametersRefs:
  - name: orders-cache-tuning          # ValkeyParameters objects in this namespace, in order
  parameters:                          # this server's own settings, merged last
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login       # unset, orders-cache-credentials (principle 6)
  providerConfigs:                     # one entry per kind; the class's kind applies
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: ValkeyElastiCacheConfig
    spec:
      instanceType: cache.t4g.small    # the ElastiCache node type
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7           # 0 disables backups
      backupStartHourUTC: 3
      # parameterGroupName: orders-valkey8  # an existing group, instead of parameters
      # fromBackup: orders-cache-final  # a snapshot name, at creation only
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: ValkeyMemorystoreConfig
    spec:
      instanceType: HIGHMEM_MEDIUM     # the Memorystore node type
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: ValkeyOCICacheConfig
    spec:
      memoryGB: 16                     # per node
```

*`ValkeyServer` with common fields and one entry per backend.*

`version` is Valkey's major and minor, such as `"8.1"`, the only form all three backends take. The
backend can move the server to a newer minor, so `version` is a floor: raising it upgrades the
server, a version below the running one is ignored, the status shows the running version, and the
platform never moves the server back to `version`.

`highAvailability` and `readReplicas` are portable features: an object that requests one its class
does not support is refused when applied (principle 1).

The maintenance window, minor upgrades and automated backups differ by backend: OCI patches on its
own schedule and takes manual backups only, and Memorystore cannot opt out of minor upgrades. They
are therefore in the provider configurations of the backends that have them (principle 1).

A deleted server is restored with its provider configuration's `fromBackup` and the final backup's
ID. The status loses that ID with the server, so it is taken from the cloud console.

The status shows the common fields, which entry applied, where each effective value came from, and
the applied backend's own values (principle 4):

```yaml
status:
  version: "8.1"
  readReplicas: 1
  endpoint:
    host: master.orders-cache-7f3a2c.abc.euw1.cache.amazonaws.com
    port: 6379
  readerEndpoint:
    host: replica.orders-cache-7f3a2c.abc.euw1.cache.amazonaws.com
    port: 6379
  connectionSecret: orders-cache-login
  providerConfig:
    applied: aws.database.entigo.com/v1alpha1, ValkeyElastiCacheConfig
    ignored:
    - gcp.database.entigo.com/v1alpha1, ValkeyMemorystoreConfig
    - oci.database.entigo.com/v1alpha1, ValkeyOCICacheConfig
  effective:
    backupRetentionDays: { value: 7, source: enforced/ValkeyElastiCacheConfig }
    deletionProtection: { value: true, source: object }
    instanceType: { value: cache.t4g.small, source: object/ValkeyElastiCacheConfig }
  provider:                            # the shape ValkeyElastiCacheConfig's status defines
    finalBackup: ...
    engineVersionActual: ...           # ElastiCache reports no state
```

*`ValkeyServer` status on AWS.*

The connection secret `<instance>-credentials` holds `endpoint`, `port` (6379), `username: default`,
`password` and `readerEndpoint`. `default` is the user a password-only client authenticates as, so a
client that always sends a username and password works unchanged.

## Read Replicas

`readReplicas: N` requests N read-only copies of the primary. Each replica is a node of the server
with the primary's node type, parameters and major version, since it copies the primary through
Valkey's own replication. Applications send reads to them through the secret's `readerEndpoint` key,
which holds the backend's balanced read endpoint. `highAvailability` places one of them in another
zone, so it needs `readReplicas` of at least 1:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  version: "8.1"
  highAvailability: true               # one of the replicas, in another zone
  readReplicas: 2
```

*`ValkeyServer` with two read replicas, one in another zone.*

## Fields

The first table maps each common field to the backend's provider. A setting the backend lacks shows
as ignored, with the reason (principle 1):

| Field                | AWS                                  | GCP                                                                                                | OCI                                                                                                                  |
| -------------------- | ------------------------------------ | -------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| `observe`            | `managementPolicies: [Observe]`      | `managementPolicies: [Observe]`                                                                    | `managementPolicies: [Observe]`                                                                                      |
| `version`            | `engineVersion`                      | `engineVersion: VALKEY_8_1`                                                                        | `softwareVersion: VALKEY_8_1`                                                                                        |
| `highAvailability`   | `multiAzEnabled`, automatic failover | `zoneDistributionConfig.mode`: `MULTI_ZONE`, or `SINGLE_ZONE` for `false`                          | at least one replica node; OCI Cache spreads nodes across availability or fault domains and fails over automatically |
| `readReplicas`       | `numCacheClusters` = replicas + 1    | `replicaCount`, with `mode: CLUSTER_DISABLED` and `shardCount: 1`, which the function always sends | `nodeCount` = replicas + 1                                                                                           |
| `deletionProtection` | platform blocks deletion             | `deletionProtectionEnabled`                                                                        | platform blocks deletion                                                                                             |
| `parameters`         | parameter group                      | `engineConfigs`, allowlist of 7                                                                    | a configuration set, allowlist of about 15 keys                                                                      |
| `parametersRefs`     | merged into `parameters`             | merged into `parameters`                                                                           | merged into `parameters`                                                                                             |

*`ValkeyServer` common fields and their provider fields.*

The second names the backend's provider configuration kind and what its fields map to:

| Field                     | AWS                                          | GCP                                                                                                               | OCI                    |
| ------------------------- | -------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- | ---------------------- |
| kind                      | `ValkeyElastiCacheConfig`                    | `ValkeyMemorystoreConfig`                                                                                         | `ValkeyOCICacheConfig` |
| `id`                      | the replication group ID                     | the instance ID                                                                                                   | the cluster's OCID     |
| `instanceType`            | `nodeType`                                   | `nodeType`                                                                                                        |                        |
| `memoryGB`                |                                              |                                                                                                                   | `nodeMemoryInGbs`      |
| `parameterGroupName`      | `parameterGroupName`                         |                                                                                                                   |                        |
| `fromBackup`              | `snapshotName`, at creation only             |                                                                                                                   |                        |
| `maintenanceWindow`       | `maintenanceWindow`                          | `maintenancePolicy.weeklyMaintenanceWindow` (`day`, `startTime.hours`)                                            |                        |
| `autoMinorVersionUpgrade` | `autoMinorVersionUpgrade`                    |                                                                                                                   |                        |
| `backupRetentionDays`     | `snapshotRetentionLimit`; 0 disables backups | `automatedBackupConfig.retention`, in seconds (`"604800s"` for 7 days); 0 omits the block, which disables backups |                        |
| `backupStartHourUTC`      | `snapshotWindow`                             | `automatedBackupConfig.fixedFrequencySchedule.startTime.hours`                                                    |                        |

*Valkey provider configuration fields.*

**AWS:** ElastiCache rejects a version without its minor, such as `8` or `8.x`. A parameter group
applies to one major version (family `valkey<major>`), so a change of the major is rejected unless
`parameterGroupName` changes or is removed in the same edit. `status.provider.finalBackup` is the ID
the final snapshot will receive.

**GCP:** the Crossplane provider's Memorystore `Instance` offers no password authentication. Data at
rest uses the platform's data key, through `kmsKey`, and Memorystore takes no final backup.

**OCI:** nodes are sized by memory only, so the class's `ValkeyOCICacheConfig` sets `memoryGB` for a
server that names none. `readerEndpoint` comes from `replicasFqdn`. Data at rest uses the service's
own key. The password is set on a user named `default`, as its SHA-256 hash, which replaces the
built-in user that has no password. OCI Cache takes manual backups only, so no final backup is
taken.

## Provider Configurations

Each backend's provider configuration kind holds the settings of that backend, with every field it
takes; commented fields are optional, alternatives, or apply only with `observe`. The settings sit
either in a separate object, named by an entry's `ref`, or inline in an entry's `spec`, as each pair
shows, and they override the class's settings for the same kind field by field (principle 2):

```yaml Separate object
---
apiVersion: aws.database.entigo.com/v1alpha1
kind: ValkeyElastiCacheConfig
metadata:
  name: orders-cache-aws
  namespace: shop
spec:
  instanceType: cache.t4g.small    # the ElastiCache node type
  maintenanceWindow:
    day: Sun
    hourUTC: 5
  backupRetentionDays: 7           # 0 disables backups; unset, 7
  backupStartHourUTC: 3            # unset, ElastiCache picks the hour
  # autoMinorVersionUpgrade: false  # unset, true
  # parameterGroupName: orders-valkey8  # an existing group, instead of parameters
  # fromBackup: orders-cache-final  # a snapshot name, at creation only
  # id: orders-cache               # an existing replication group's ID, with observe
```

```yaml AWS entry
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: ValkeyElastiCacheConfig
    spec:
      instanceType: cache.t4g.small    # the ElastiCache node type
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7           # 0 disables backups; unset, 7
      backupStartHourUTC: 3            # unset, ElastiCache picks the hour
      # autoMinorVersionUpgrade: false  # unset, true
      # parameterGroupName: orders-valkey8  # an existing group, instead of parameters
      # fromBackup: orders-cache-final  # a snapshot name, at creation only
      # id: orders-cache               # an existing replication group's ID, with observe
```

*`ValkeyElastiCacheConfig`: a separate object, and the same settings in an entry.*

```yaml Separate object
---
apiVersion: gcp.database.entigo.com/v1alpha1
kind: ValkeyMemorystoreConfig
metadata:
  name: orders-cache-gcp
  namespace: shop
spec:
  instanceType: HIGHMEM_MEDIUM     # the Memorystore node type
  maintenanceWindow:
    day: Sun
    hourUTC: 5
  backupRetentionDays: 7           # 0 disables backups; unset, 7
  backupStartHourUTC: 3            # unset, Memorystore picks the hour
  # id: projects/shop-project/locations/europe-north1/instances/orders-cache  # with observe
```

```yaml GCP entry
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: ValkeyMemorystoreConfig
    spec:
      instanceType: HIGHMEM_MEDIUM     # the Memorystore node type
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7           # 0 disables backups; unset, 7
      backupStartHourUTC: 3            # unset, Memorystore picks the hour
      # id: projects/shop-project/locations/europe-north1/instances/orders-cache  # with observe
```

*`ValkeyMemorystoreConfig`: a separate object, and the same settings in an entry.*

```yaml Separate object
---
apiVersion: oci.database.entigo.com/v1alpha1
kind: ValkeyOCICacheConfig
metadata:
  name: orders-cache-oci
  namespace: shop
spec:
  memoryGB: 16                     # per node, within OCI's range
  # id: ocid1.rediscluster.oc1..example  # an existing cluster's OCID, with observe
```

```yaml OCI entry
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: ValkeyOCICacheConfig
    spec:
      memoryGB: 16                     # per node, within OCI's range
      # id: ocid1.rediscluster.oc1..example  # an existing cluster's OCID, with observe
```

*`ValkeyOCICacheConfig`: a separate object, and the same settings in an entry.*

## Composed Resources

The `ValkeyServer` from [Kinds](#kinds) is composed into the resources below on the cloud it runs
on: the platform applies the entry of its class's kind and ignores the others. A comment names the
source of each value the object does not hold as written: `platform` for a value the platform sets,
and `deployment` for one the deployment sets. The object and its resources:

```yaml Object on AWS
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  className: standard
  version: "8.1"
  highAvailability: true
  readReplicas: 1
  deletionProtection: true
  parameters:
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: ValkeyElastiCacheConfig
    spec:
      instanceType: cache.t4g.small
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: ValkeyMemorystoreConfig
    spec:
      instanceType: HIGHMEM_MEDIUM
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: ValkeyOCICacheConfig
    spec:
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

*`ValkeyServer` on AWS and its composed resource.*

```yaml Object on GCP
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  className: standard
  version: "8.1"
  highAvailability: true
  readReplicas: 1
  deletionProtection: true
  parameters:
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: ValkeyElastiCacheConfig
    spec:
      instanceType: cache.t4g.small
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: ValkeyMemorystoreConfig
    spec:
      instanceType: HIGHMEM_MEDIUM
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: ValkeyOCICacheConfig
    spec:
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
    kmsKey: projects/shop-project/locations/europe-north1/keyRings/data/cryptoKeys/data  # platform
    desiredPscAutoConnections:  # platform
    - network: projects/shop-project/global/networks/deployment
      projectId: shop-project
```

*`ValkeyServer` on GCP and its composed resource.*

```yaml Object on OCI
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
  namespace: shop
spec:
  className: standard
  version: "8.1"
  highAvailability: true
  readReplicas: 1
  deletionProtection: true
  parameters:
    maxmemory-policy: allkeys-lru
  secretName: orders-cache-login
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: ValkeyElastiCacheConfig
    spec:
      instanceType: cache.t4g.small
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: ValkeyMemorystoreConfig
    spec:
      instanceType: HIGHMEM_MEDIUM
      maintenanceWindow:
        day: Sun
        hourUTC: 5
      backupRetentionDays: 7
      backupStartHourUTC: 3
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: ValkeyOCICacheConfig
    spec:
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

*`ValkeyServer` on OCI and its composed resource.*

## Platform Decisions

Besides the settings it makes for every service
([Platform Decisions](common-design.md#platform-decisions)), the platform makes these for a
`ValkeyServer` itself:

| Setting               | Platform's Choice                                                                                |
| --------------------- | ------------------------------------------------------------------------------------------------ |
| Encryption in transit | TLS                                                                                              |
| Login                 | A generated password                                                                             |
| Final backup          | A snapshot taken when a server is deleted, switched on per deployment, where the cloud takes one |

*`ValkeyServer` settings chosen by the platform.*

## Limits

The backend enforces these limits itself, and the API does not check them (principle 10); a value
outside them is rejected by the backend and shown as `CloudAccepted: False`. They are listed for
reference, should validating them be decided later. Each limit holds as of its source: the provider
release named, or the cloud's documentation as read in November 2026.

| Field                 | Backend | Limit    | Source                                                                                   |
| --------------------- | ------- | -------- | ---------------------------------------------------------------------------------------- |
| `readReplicas`        | AWS     | up to 5  | `numCacheClusters` up to 6, ElastiCache `ReplicationGroup`                               |
| `readReplicas`        | GCP     | up to 5  | `replicaCount`, Memorystore `Instance` CRD, provider-upjet-gcp v3.1.0                    |
| `readReplicas`        | OCI     | up to 4  | `nodeCount` up to 5, `RedisCluster` CRD, crossplane-provider-oci v2.0.0                  |
| `memoryGB`            | OCI     | 2 to 500 | `nodeMemoryInGbs`, `RedisCluster` CRD, crossplane-provider-oci v2.0.0                    |
| `backupRetentionDays` | AWS     | up to 35 | `snapshotRetentionLimit`, ElastiCache `ReplicationGroup` CRD, provider-upjet-aws v2.4.0  |
| `backupRetentionDays` | GCP     | 1 to 365 | `automatedBackupConfig.retention`, Memorystore `Instance` CRD, provider-upjet-gcp v3.1.0 |

*Valkey field limits and their sources.*

## Notes

The design relies on these facts, which need no decision.

- Azure: Azure Managed Redis runs Redis 7.4 on Redis Enterprise, with no version choice, no backups
  and a 25 GB limit without clustering.
- GCP: Memorystore for Valkey has password authentication (`TOKEN_AUTH`), but [provider-upjet-gcp
  v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0) offers only
  IAM authentication, whose hour-long tokens no connection secret can hold. Its TLS also needs the
  instance's CA in the client, which the connection secret would carry.
