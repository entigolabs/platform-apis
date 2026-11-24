# PostgreSQL

PostgreSQL is an open-source relational database that applications use for their transactional data.

Three kinds for users: `PostgreSQLCluster`, one database cluster with its standby;
`PostgreSQLUser`, one application's login on a cluster, with its permissions; and
`PostgreSQLDatabase`, one database on a cluster. Two kinds shape them: `PostgreSQLClass`, a
deployment's defaults and limits, written by Entigo, and `PostgreSQLParameters`, reusable PostgreSQL
settings.

Built on Amazon RDS (AWS), Cloud SQL (GCP), OCI Database with PostgreSQL (OCI), Azure Database for
PostgreSQL flexible server (Azure) and CloudNativePG, which runs in the Kubernetes cluster on any of
them, each with a provider configuration kind of its own: `PostgreSQLRDSConfig`,
`PostgreSQLCloudSQLConfig`, `PostgreSQLOCIConfig`, `PostgreSQLFlexibleServerConfig` and
`PostgreSQLCNPGConfig`.

## Kinds

PostgreSQL is offered as five kinds, each with the manifest its owner writes.

### PostgreSQLClass

Entigo writes each deployment's classes: cluster-scoped objects that hold the defaults an object
starts with (`template`) and the values it cannot change (`enforced`), each with common fields and
one provider configuration, whose kind selects the backend (principle 5):

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLClass
metadata:
  name: standard
  annotations:
    database.entigo.com/is-default-class: "true"
spec:
  template:
    deletionProtection: true
    version: "17"
    storage:
      sizeGB: 50
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: PostgreSQLRDSConfig
      spec:
        instanceType: db.t4g.medium
        maxSizeGB: 500
  enforced:                            # teams cannot change these
    backup:
      retentionDays: 7
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: PostgreSQLRDSConfig
      spec:
        maintenanceWindow:
          day: Sun
          hourUTC: 3
  parametersRefs:                      # beneath every cluster's own settings
  - name: pg-baseline
    namespace: platform-apis
```

*`PostgreSQLClass` for RDS.*

A class with no provider configuration uses the one backend its deployment installs, so it holds
only common fields and can be copied to every deployment unchanged.

### PostgreSQLParameters

`PostgreSQLParameters` holds PostgreSQL settings that several clusters or a class share. A class and
a cluster each list the ones they use in `parametersRefs`. The settings merge key by key, a later
value winning: the class's list in order, then the cluster's, then the cluster's own `parameters`,
so a team can put settings all its clusters share beneath one cluster's own tuning:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLParameters
metadata:
  name: orders-tuning
  namespace: shop
spec:
  parameters:
    work_mem: 64MB
    random_page_cost: "1.1"
```

*`PostgreSQLParameters` shared between a user's clusters.*

A `PostgreSQLParameters` names no other, so references never chain. Settings the backend does not
allow are rejected by the cloud and reported as `CloudAccepted: False` (principle 10).

### PostgreSQLCluster

A user creates a `PostgreSQLCluster` with a manifest like the one below. It names its class and
carries one provider configuration entry per backend it may run on (principle 2); each deployment
applies the entry of its own class's kind and ignores the others:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard                  # unset, the default class; without one, standalone
  version: "17"                        # the major version
  storage:
    sizeGB: 20
  highAvailability: false              # a standby in another zone
  deletionProtection: true
  backup:
    retentionDays: 7                   # backups kept; 0 disables them
  parametersRefs:                      # PostgreSQLParameters in this namespace, in order
  - name: shop-common
  - name: orders-tuning                # wins over shop-common
  parameters:                          # this cluster's own settings, merged last
    max_connections: "200"
  secretName: orders-db-admin          # unset, orders-db-dbadmin (principle 6)
  providerConfigs:                     # one entry per kind; the class's kind applies
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small       # the RDS instance class
      maxSizeGB: 100                   # the storage autoscaling ceiling
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      # minorVersion: "17.7"           # stay on this minor
      # parameterGroupName: orders-pg17  # an existing group, instead of parameters
      # fromBackup: orders-final       # an RDS snapshot ID, instead of restore
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680   # the Cloud SQL tier
      edition: ENTERPRISE              # or ENTERPRISE_PLUS
      diskType: PD_SSD                 # PD_HDD, HYPERDISK_BALANCED
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex  # the DB system shape
      ocpus: 2                         # required with a .Flex shape
      memoryGB: 32                     # required with a .Flex shape
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    ref:
      name: orders-azure               # a PostgreSQLFlexibleServerConfig in this namespace
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"                         # per instance, requested and limited alike
      memory: 8Gi
```

*`PostgreSQLCluster` with common fields and one entry per backend.*

An entry holds its settings inline, in `spec`, or names a separate object of its kind, through
`ref`, as the Azure entry does:

```yaml
---
apiVersion: azure.database.entigo.com/v1alpha1
kind: PostgreSQLFlexibleServerConfig
metadata:
  name: orders-azure
  namespace: shop
spec:
  instanceType: GP_Standard_D2ds_v5    # the flexible server SKU
  storageTier: P10                     # P4 to P80; sets the disk's IOPS
  maintenanceWindow:
    day: Sun
    hourUTC: 3
```

*`PostgreSQLFlexibleServerConfig` referenced by a cluster entry.*

Most clusters name nothing backend-specific and take the class's provider configuration as it is:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  version: "17"
  storage:
    sizeGB: 100
```

*`PostgreSQLCluster` using its class defaults.*

In a deployment without a default class, a cluster that names no class is standalone (Common
Design). It sets the values a class would otherwise supply, `version`, `storage.sizeGB` and each
backend's size, and is refused when applied without them:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:                                  # no className: standalone
  version: "17"                        # required without a class
  storage:
    sizeGB: 20                         # required without a class
  providerConfigs:                     # each backend's size, also required
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"
      memory: 8Gi
```

*Standalone `PostgreSQLCluster`.*

Every field it leaves out takes the implementation's default, `highAvailability: false`,
`deletionProtection: true` and `backup.retentionDays: 7`, and its status shows each with the source
`implementation`.

`restore` creates a cluster from another cluster's backups, at a point in time. A deleted cluster is
restored instead with its provider configuration's `fromBackup` and the final backup's ID, such as
an RDS snapshot ID. The status loses that ID with the cluster, so it is taken from the cloud
console. OCI and Azure take no final backup, so a deleted cluster cannot be restored there;
`deletionProtection` is the only guard. `restore` and `fromBackup` are set at creation only and
cannot be combined:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db-restored
  namespace: shop
spec:
  version: "17"
  restore:
    fromInstance: orders-db            # a PostgreSQLCluster in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
```

*`PostgreSQLCluster` restored from another cluster's backups.*

`version` is the major version, such as `"17"`. Raising it upgrades the cluster where the backend
can, and the backend applies minor versions itself, in the maintenance window its provider
configuration sets.

Settings that backends run differently, such as disk growth, provisioned IOPS, the maintenance
window, the point-in-time window and the backup hour, are in each backend's provider configuration,
where the backend has them (principle 1).

`storage.sizeGB` is the size at creation and a floor afterwards: raising it grows the disk, and a
value below the real size is ignored, so a rejected increase can still be corrected. The backend may
round it up and a backend that grows the disk can take it further, so the status reports the actual
size, and the platform never shrinks the disk back to `sizeGB`.

`highAvailability` is a portable feature: an object that requests it where its backend does not
support it is refused when applied (principle 1).

The status shows the common fields, which entry applied, where each effective value came from, and
the applied backend's own values (principle 4):

```yaml
status:
  version: "17"
  endpoint:
    host: orders-db-7f3a2c.abc.eu-north-1.rds.amazonaws.com
    port: 5432
  connectionSecret: orders-db-admin
  providerConfig:
    applied: aws.database.entigo.com/v1alpha1, PostgreSQLRDSConfig
    ignored:
    - gcp.database.entigo.com/v1alpha1, PostgreSQLCloudSQLConfig
    - oci.database.entigo.com/v1alpha1, PostgreSQLOCIConfig
    - azure.database.entigo.com/v1alpha1, PostgreSQLFlexibleServerConfig
  effective:
    backup.retentionDays: { value: 7, source: enforced }
    deletionProtection: { value: true, source: object }
    instanceType: { value: db.t4g.small, source: object/PostgreSQLRDSConfig }
  provider:                            # the shape PostgreSQLRDSConfig's status defines
    finalBackup: ...
    identifier: ...
    resourceId: ...
    engineVersionActual: ...
    latestRestorableTime: ...
    state: available
```

*`PostgreSQLCluster` status on AWS.*

The connection secret `<instance>-dbadmin` holds the admin login, with the keys of principle 6.

### PostgreSQLUser

`PostgreSQLUser` acts inside PostgreSQL, through provider-sql, so it is the same in every backend.
Its `name` and `clusterRef` are immutable, `clusterRef` may name another namespace of the same Zone
(principle 11), and it has no class or provider configurations.

A set of permissions shared by several users is a role: a `PostgreSQLUser` with `login: false`,
which other users receive through `grant.roles`.

A user creates a `PostgreSQLUser` with a manifest like the one below:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLUser
metadata:
  name: orders-app
  namespace: shop
spec:
  name: orders_app                     # the role's name in PostgreSQL; unset, metadata.name
  clusterRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (principle 11)
  login: true
  createDb: false
  createRole: false
  inherit: true
  grant:
    roles:
    - reporting
  secretName: orders-app-login         # unset, orders-db-orders-app (principle 6)
```

*`PostgreSQLUser` granted the `reporting` role.*

### PostgreSQLDatabase

`PostgreSQLDatabase` acts inside PostgreSQL, through provider-sql, so it is the same in every
backend. Its `name` and `clusterRef` are immutable, `clusterRef` may name another namespace of the
same Zone (principle 11), and it has no class or provider configurations.

`extensions` creates each listed extension in the database. Azure also requires the server to allow
an extension, so every Azure cluster allows the whole allowlist, and any database can create any
extension on it, as on the other clouds.

A user creates a `PostgreSQLDatabase` with a manifest like the one below:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLDatabase
metadata:
  name: orders
  namespace: shop
spec:
  name: orders                         # the database's name in PostgreSQL; unset, metadata.name
  clusterRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (principle 11)
  owner: orders_app                    # a role name in PostgreSQL
  encoding: UTF8
  lcCType: en_US.UTF-8
  lcCollate: en_US.UTF-8
  dbTemplate: template0                # the template database
  extensions:
  - postgis
  extensionConfig:
    postgis:
      schema: gis
  deletionProtection: true
```

*`PostgreSQLDatabase` with PostGIS in its own schema.*

## Fields

The first table maps each common field to the backend's provider. A setting the backend lacks shows
as ignored, with the reason (principle 1):

| Field                  | AWS                                                                                                                       | GCP                                                                                                                                      | OCI                                                                            | Azure                                                   | Kubernetes                                                                        |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `observe`              | `managementPolicies: [Observe]`                                                                                           | `managementPolicies: [Observe]`                                                                                                          | `managementPolicies: [Observe]`                                                | `managementPolicies: [Observe]`                         | the named `Cluster` read as a required resource; nothing composed                 |
| `version`              | `engineVersion`; with `minorVersion`, that full version; raising the major sets `allowMajorVersionUpgrade` for the change | `databaseVersion: POSTGRES_17`; raising it upgrades                                                                                      | `dbVersion: "17"`; raising it fails until the provider supports major upgrades | `version: "17"`; raising it upgrades                    | `imageCatalogRef.major: 17`; raising it is an offline in-place upgrade            |
| `storage.sizeGB`       | `allocatedStorage`; below 20 rounded up to 20                                                                             | `settings.diskSize`                                                                                                                      | ignored; storage grows automatically                                           | `storageMb`, rounded up to the next of its 12 sizes     | `storage.size: 20Gi`; growing needs a StorageClass that allows expansion          |
| `highAvailability`     | `multiAz`                                                                                                                 | `settings.availabilityType: REGIONAL`; with `backup.retentionDays: 0` rejected, as a regional instance needs backups                     | `instanceCount: 2`, regional storage                                           | `highAvailability.mode: ZoneRedundant`                  | `instances: 2` on separate zones; `false` sets `instances: 1`                     |
| `deletionProtection`   | `deletionProtection`                                                                                                      | `settings.deletionProtectionEnabled`                                                                                                     | platform blocks deletion                                                       | platform blocks deletion                                | platform blocks deletion                                                          |
| `backup.retentionDays` | `backupRetentionPeriod`                                                                                                   | `settings.backupConfiguration.backupRetentionSettings.retainedBackups`, with `pointInTimeRecoveryEnabled: true`; 0 sets `enabled: false` | `managementPolicy.backupPolicy.retentionDays`, DAILY; 0 sets `kind: NONE`      | `backupRetentionDays`; 0 ignored, backups are always on | `ObjectStore` `retentionPolicy: 7d`; 0 sets no backup plugin or `ScheduledBackup` |
| `parameters`           | a `ParameterGroup`                                                                                                        | `settings.databaseFlags`; keys outside its allowlist rejected                                                                            | a new `PsqlConfiguration` per change                                           | one `FlexibleServerConfiguration` each                  | `postgresql.parameters`; the operator's fixed settings, such as `port`, rejected  |
| `parametersRefs`       | merged into `parameters`                                                                                                  | merged into `parameters`                                                                                                                 | merged into `parameters`                                                       | merged into `parameters`                                | merged into `parameters`                                                          |
| `restore.fromInstance` | `restoreToPointInTime.sourceDbInstanceIdentifier`                                                                         | `clone.sourceInstanceName`                                                                                                               | `source.sourceType: POINT_IN_TIME_DB_SYSTEM`, `source.dbSystemId`              | `createMode: PointInTimeRestore`, `sourceServerId`      | `bootstrap.recovery.source`, an `externalClusters` entry for the source's backups |
| `restore.pointInTime`  | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`                                                      | `clone.pointInTime`                                                                                                                      | `source.timeToRestore`                                                         | `pointInTimeRestoreTimeInUtc`                           | `bootstrap.recovery.recoveryTarget.targetTime`                                    |

*`PostgreSQLCluster` common fields and their provider fields.*

The second names the backend's provider configuration kind and what its fields map to:

| Field                  | AWS                                                                         | GCP                                                        | OCI                                                              | Azure                                                                    | Kubernetes                                             |
| ---------------------- | --------------------------------------------------------------------------- | ---------------------------------------------------------- | ---------------------------------------------------------------- | ------------------------------------------------------------------------ | ------------------------------------------------------ |
| kind                   | `PostgreSQLRDSConfig`                                                       | `PostgreSQLCloudSQLConfig`                                 | `PostgreSQLOCIConfig`                                            | `PostgreSQLFlexibleServerConfig`                                         | `PostgreSQLCNPGConfig`                                 |
| `id`                   | the console's Resource ID (`db-...`)                                        | the instance name                                          | the DB system's OCID                                             | the server's resource ID; the function takes the name and resource group | the `Cluster`'s name in the object's namespace         |
| `instanceType`         | `instanceClass`                                                             | `settings.tier`                                            | `shape`; a `.Flex` shape without `ocpus` and `memoryGB` rejected | `skuName`                                                                |                                                        |
| `fromBackup`           | `snapshotIdentifier`                                                        | `restoreBackupContext` (`instanceId`, `backupRunId`)       | `source.sourceType: BACKUP`, `source.backupId`                   |                                                                          | `bootstrap.recovery.backup.name`                       |
| `parameterGroupName`   | `parameterGroupName`                                                        |                                                            |                                                                  |                                                                          |                                                        |
| `edition`              |                                                                             | `settings.edition`                                         |                                                                  |                                                                          |                                                        |
| `diskType`             |                                                                             | `settings.diskType`                                        |                                                                  |                                                                          |                                                        |
| `ocpus`                |                                                                             |                                                            | `instanceOcpuCount`                                              |                                                                          |                                                        |
| `memoryGB`             |                                                                             |                                                            | `instanceMemorySizeInGbs`                                        |                                                                          |                                                        |
| `storageTier`          |                                                                             |                                                            |                                                                  | `storageTier`                                                            |                                                        |
| `cpu`                  |                                                                             |                                                            |                                                                  |                                                                          | `resources.requests.cpu`, `resources.limits.cpu`       |
| `memory`               |                                                                             |                                                            |                                                                  |                                                                          | `resources.requests.memory`, `resources.limits.memory` |
| `storageClass`         |                                                                             |                                                            |                                                                  |                                                                          | `storage.storageClass`                                 |
| `minorVersion`         | `engineVersion` with that full version, `autoMinorVersionUpgrade: false`    |                                                            |                                                                  |                                                                          |                                                        |
| `maxSizeGB`            | `maxAllocatedStorage`                                                       | `settings.diskAutoresizeLimit`                             |                                                                  |                                                                          |                                                        |
| `autoGrow`             |                                                                             | `settings.diskAutoresize`                                  |                                                                  | `autoGrowEnabled`                                                        |                                                        |
| `iops`                 | `iops`                                                                      | `settings.dataDiskProvisionedIops`, with Hyperdisk         | `storageDetails.iops`                                            |                                                                          |                                                        |
| `throughputMBps`       | `storageThroughput`                                                         | `settings.dataDiskProvisionedThroughput`, with Hyperdisk   |                                                                  |                                                                          |                                                        |
| `maintenanceWindow`    | `maintenanceWindow: sun:03:00-sun:03:30`                                    | `settings.maintenanceWindow` (`day: 7`, `hour: 3`)         | `managementPolicy.maintenanceWindowStart: "SUN 03:00"`           | `maintenanceWindow` (`dayOfWeek: 0`, `startHour: 3`)                     |                                                        |
| `pointInTimeDays`      |                                                                             | `settings.backupConfiguration.transactionLogRetentionDays` | `managementPolicy.pitrPolicy.restoreDays`, STANDARD              |                                                                          |                                                        |
| `backupStartHourUTC`   | `backupWindow`, 30 minutes                                                  | `settings.backupConfiguration.startTime`                   | `managementPolicy.backupPolicy.backupStart`                      |                                                                          | `ScheduledBackup` `schedule`                           |
| `parameterApplyMethod` | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` |                                                            |                                                                  |                                                                          |                                                        |

*PostgreSQL provider configuration fields.*

**AWS:** provider keys are from the CRDs of
[provider-upjet-aws v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0).
Only RDS can turn minor upgrades off: `minorVersion` sets the full version in `engineVersion`, such
as `"17.7"`, and the user changes it to upgrade. A parameter group applies to one major version, so
a major change is rejected unless `parameterGroupName` changes or is removed in the same edit.
`storage.sizeGB` is rounded up to at least 20 GiB, and RDS rounds an increase under 10% up to 10%.
`status.provider.finalBackup` is the ID the final snapshot will receive. Storage is gp3.

**GCP:** provider keys are from the CRDs of [provider-upjet-gcp
v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0). `updateTrack`
only delays minor upgrades. The size belongs to an edition: `db-custom-*` to `ENTERPRISE`,
`db-perf-optimized-*` to `ENTERPRISE_PLUS`, which Cloud SQL selects by default from PostgreSQL 16.
The platform therefore always sets `edition`, and a class's `edition` and `instanceType` must belong
together. The Crossplane provider reports only `finalBackupDescription`, and `endpoint.host` is an
IP address.

**OCI:** provider keys are from the CRDs of [crossplane-provider-oci
v2.0.0](https://github.com/oracle/crossplane-provider-oci/releases/tag/v2.0.0). `storage.sizeGB`
stays empty in the status, since OCI reports no size. No final backup is taken, and `endpoint.host`
is an IP address. Storage is encrypted with the service's own key, since crossplane-provider-oci
v2.0.0 has no key field for OCI Database with PostgreSQL.

**Azure:** provider keys are from the CRDs of [provider-upjet-azure
v2.8.0](https://github.com/crossplane-contrib/provider-upjet-azure/releases/tag/v2.8.0).
`storage.sizeGB` is rounded up to the next of Azure's 12 fixed sizes. Azure's same-zone standby
keeps no copy in another zone, so `highAvailability: true` maps to its zone-redundant mode only. No
final backup is taken. The platform also creates a private DNS zone for the server.

**Kubernetes:** keys are from the CRDs of [CloudNativePG
v1.30.2](https://github.com/cloudnative-pg/cloudnative-pg/releases/tag/v1.30.2) and its [Barman
Cloud plugin v0.16.0](https://github.com/cloudnative-pg/plugin-barman-cloud/releases/tag/v0.16.0).
The deployment installs the operator, the plugin and a `ClusterImageCatalog` with an image per
major version, and names the object store backups go to; the platform updates the catalog's images
for minor versions. Backups stay in the object store after a cluster is deleted, so a deleted
cluster can be restored with `restore.fromInstance` until the store's own lifecycle removes them.
`endpoint.host` is the cluster's read-write Service, such as `orders-db-7f3a2c-rw.shop.svc`.

## Provider Configurations

Each backend's provider configuration kind holds the settings of that backend, with every field it
takes; commented fields are optional, alternatives, or apply only with `observe`. The settings sit
either in a separate object, named by an entry's `ref`, or inline in an entry's `spec`, as each pair
shows, and they override the class's settings for the same kind field by field (principle 2):

```yaml Separate object
---
apiVersion: aws.database.entigo.com/v1alpha1
kind: PostgreSQLRDSConfig
metadata:
  name: orders-db-aws
  namespace: shop
spec:
  instanceType: db.t4g.small       # the RDS instance class
  maxSizeGB: 100                   # the storage autoscaling ceiling
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  backupStartHourUTC: 2            # unset, RDS picks the hour
  # minorVersion: "17.7"           # stay on this minor; unset, minor upgrades apply
  # iops: 12000                    # from 400 GiB of disk
  # throughputMBps: 500            # from 400 GiB of disk
  # parameterApplyMethod: NextRestart  # unset, Immediate
  # parameterGroupName: orders-pg17  # an existing group, instead of parameters
  # fromBackup: orders-final       # an RDS snapshot ID, at creation only, instead of restore
  # id: orders-db                  # an existing instance's ID, with observe
```

```yaml AWS entry
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small       # the RDS instance class
      maxSizeGB: 100                   # the storage autoscaling ceiling
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2            # unset, RDS picks the hour
      # minorVersion: "17.7"           # stay on this minor; unset, minor upgrades apply
      # iops: 12000                    # from 400 GiB of disk
      # throughputMBps: 500            # from 400 GiB of disk
      # parameterApplyMethod: NextRestart  # unset, Immediate
      # parameterGroupName: orders-pg17  # an existing group, instead of parameters
      # fromBackup: orders-final       # an RDS snapshot ID, at creation only, instead of restore
      # id: orders-db                  # an existing instance's ID, with observe
```

*`PostgreSQLRDSConfig`: a separate object, and the same settings in an entry.*

```yaml Separate object
---
apiVersion: gcp.database.entigo.com/v1alpha1
kind: PostgreSQLCloudSQLConfig
metadata:
  name: orders-db-gcp
  namespace: shop
spec:
  instanceType: db-custom-2-7680   # the Cloud SQL tier, which belongs to edition
  edition: ENTERPRISE              # or ENTERPRISE_PLUS
  diskType: PD_SSD                 # PD_HDD, HYPERDISK_BALANCED
  maxSizeGB: 100                   # the autoGrow ceiling; unset, none
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  pointInTimeDays: 7               # unset, 7
  backupStartHourUTC: 2
  # autoGrow: false                # unset, true
  # iops: 10000                    # with Hyperdisk
  # throughputMBps: 400            # with Hyperdisk
  # fromBackup: orders-db/1700000000000  # <instance>/<backup run ID>, at creation only
  # id: shop-project:orders-db     # an existing instance's ID, with observe
```

```yaml GCP entry
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680   # the Cloud SQL tier, which belongs to edition
      edition: ENTERPRISE              # or ENTERPRISE_PLUS
      diskType: PD_SSD                 # PD_HDD, HYPERDISK_BALANCED
      maxSizeGB: 100                   # the autoGrow ceiling; unset, none
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7               # unset, 7
      backupStartHourUTC: 2
      # autoGrow: false                # unset, true
      # iops: 10000                    # with Hyperdisk
      # throughputMBps: 400            # with Hyperdisk
      # fromBackup: orders-db/1700000000000  # <instance>/<backup run ID>, at creation only
      # id: shop-project:orders-db     # an existing instance's ID, with observe
```

*`PostgreSQLCloudSQLConfig`: a separate object, and the same settings in an entry.*

```yaml Separate object
---
apiVersion: oci.database.entigo.com/v1alpha1
kind: PostgreSQLOCIConfig
metadata:
  name: orders-db-oci
  namespace: shop
spec:
  instanceType: PostgreSQL.VM.Standard.E5.Flex  # the DB system shape
  ocpus: 2                         # required with a .Flex shape
  memoryGB: 32                     # required with a .Flex shape
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  pointInTimeDays: 7               # unset, backup.retentionDays
  backupStartHourUTC: 2
  # iops: 75000                    # in steps of 75,000
  # fromBackup: ocid1.postgresqlbackup.oc1..example  # a backup OCID, at creation only
  # id: ocid1.postgresqldbsystem.oc1..example  # an existing DB system's OCID, with observe
```

```yaml OCI entry
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex  # the DB system shape
      ocpus: 2                         # required with a .Flex shape
      memoryGB: 32                     # required with a .Flex shape
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7               # unset, backup.retentionDays
      backupStartHourUTC: 2
      # iops: 75000                    # in steps of 75,000
      # fromBackup: ocid1.postgresqlbackup.oc1..example  # a backup OCID, at creation only
      # id: ocid1.postgresqldbsystem.oc1..example  # an existing DB system's OCID, with observe
```

*`PostgreSQLOCIConfig`: a separate object, and the same settings in an entry.*

```yaml Separate object
---
apiVersion: azure.database.entigo.com/v1alpha1
kind: PostgreSQLFlexibleServerConfig
metadata:
  name: orders-db-azure
  namespace: shop
spec:
  instanceType: GP_Standard_D2ds_v5  # the flexible server SKU
  storageTier: P10                 # P4 to P80; sets the disk's IOPS
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  # autoGrow: false                # unset, true
  # id: /subscriptions/.../flexibleServers/orders-db  # an existing server's ID, with observe
```

```yaml Azure entry
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    spec:
      instanceType: GP_Standard_D2ds_v5  # the flexible server SKU
      storageTier: P10                 # P4 to P80; sets the disk's IOPS
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      # autoGrow: false                # unset, true
      # id: /subscriptions/.../flexibleServers/orders-db  # an existing server's ID, with observe
```

*`PostgreSQLFlexibleServerConfig`: a separate object, and the same settings in an entry.*

```yaml Separate object
---
apiVersion: kubernetes.database.entigo.com/v1alpha1
kind: PostgreSQLCNPGConfig
metadata:
  name: orders-db-cnpg
  namespace: shop
spec:
  cpu: "2"                         # per instance, requested and limited alike
  memory: 8Gi
  storageClass: fast-ssd           # unset, the cluster's default StorageClass
  backupStartHourUTC: 2            # the ScheduledBackup hour
  # fromBackup: orders-db-20261120  # a CloudNativePG Backup, at creation only
  # id: orders-db                  # an existing Cluster's name, with observe
```

```yaml Kubernetes entry
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"                         # per instance, requested and limited alike
      memory: 8Gi
      storageClass: fast-ssd           # unset, the cluster's default StorageClass
      backupStartHourUTC: 2            # the ScheduledBackup hour
      # fromBackup: orders-db-20261120  # a CloudNativePG Backup, at creation only
      # id: orders-db                  # an existing Cluster's name, with observe
```

*`PostgreSQLCNPGConfig`: a separate object, and the same settings in an entry.*

## Composed Resources

The `PostgreSQLCluster` from [Kinds](#kinds) is composed into the resources below on the cloud it
runs on: the platform applies the entry of its class's kind and ignores the others. A comment names
the source of each value the object does not hold as written: `platform` for a value the platform
sets, and `deployment` for one the deployment sets. The object and its resources:

```yaml Object on AWS
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard
  version: "17"
  storage:
    sizeGB: 20
  highAvailability: false
  deletionProtection: true
  backup:
    retentionDays: 7
  parametersRefs:
  - name: orders-tuning
  parameters:
    max_connections: "200"
  secretName: orders-db-admin
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680
      edition: ENTERPRISE
      diskType: PD_SSD
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex
      ocpus: 2
      memoryGB: 32
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    ref:
      name: orders-azure
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"
      memory: 8Gi
      backupStartHourUTC: 2
```

```yaml AWS resource
---
apiVersion: rds.aws.m.upbound.io/v1beta1
kind: Instance
metadata:
  name: orders-db-7f3a2c       # name + UID hash
  namespace: shop
spec:
  forProvider:
    region: eu-north-1         # deployment
    engine: postgres
    engineVersion: "17"
    instanceClass: db.t4g.small
    allocatedStorage: 20
    maxAllocatedStorage: 100
    storageType: gp3           # platform
    multiAz: false
    deletionProtection: true
    maintenanceWindow: sun:03:00-sun:03:30
    autoMinorVersionUpgrade: true  # platform; no minorVersion
    allowMajorVersionUpgrade: false  # platform; true for a major change
    backupRetentionPeriod: 7
    backupWindow: 02:00-02:30
    parameterGroupName: orders-db-7f3a2c  # composed from parameters
    username: dbadmin          # platform
    manageMasterUserPassword: true  # platform; the admin Secret is read from the RDS-managed secret
    dbSubnetGroupName: deployment-db  # platform
    vpcSecurityGroupIds:       # platform
    - sg-0123456789abcdef0
    publiclyAccessible: false  # platform
    storageEncrypted: true     # platform
    kmsKeyId: arn:aws:kms:eu-north-1:111122223333:key/data  # platform
```

*`PostgreSQLCluster` on AWS and its composed resource.*

```yaml Object on GCP
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard
  version: "17"
  storage:
    sizeGB: 20
  highAvailability: false
  deletionProtection: true
  backup:
    retentionDays: 7
  parametersRefs:
  - name: orders-tuning
  parameters:
    max_connections: "200"
  secretName: orders-db-admin
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680
      edition: ENTERPRISE
      diskType: PD_SSD
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex
      ocpus: 2
      memoryGB: 32
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    ref:
      name: orders-azure
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"
      memory: 8Gi
      backupStartHourUTC: 2
```

```yaml GCP resource
---
apiVersion: sql.gcp.m.upbound.io/v1beta1
kind: DatabaseInstance
metadata:
  name: orders-db-7f3a2c       # name + UID hash
  namespace: shop
spec:
  forProvider:
    region: europe-north1      # deployment
    databaseVersion: POSTGRES_17
    encryptionKeyName: projects/p/locations/europe-north1/keyRings/r/cryptoKeys/data  # platform
    settings:
      tier: db-custom-2-7680
      edition: ENTERPRISE
      diskType: PD_SSD
      diskSize: 20
      diskAutoresize: true     # platform
      diskAutoresizeLimit: 100
      availabilityType: ZONAL
      deletionProtectionEnabled: true
      maintenanceWindow:
        day: 7
        hour: 3
      backupConfiguration:
        enabled: true
        pointInTimeRecoveryEnabled: true
        backupRetentionSettings:
          retainedBackups: 7
        transactionLogRetentionDays: 7
        startTime: "02:00"
      databaseFlags:
      - name: max_connections
        value: "200"
      ipConfiguration:         # platform
        ipv4Enabled: false
        privateNetwork: projects/p/global/networks/deployment
```

*`PostgreSQLCluster` on GCP and its composed resource.*

```yaml Object on OCI
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard
  version: "17"
  storage:
    sizeGB: 20
  highAvailability: false
  deletionProtection: true
  backup:
    retentionDays: 7
  parametersRefs:
  - name: orders-tuning
  parameters:
    max_connections: "200"
  secretName: orders-db-admin
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680
      edition: ENTERPRISE
      diskType: PD_SSD
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex
      ocpus: 2
      memoryGB: 32
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    ref:
      name: orders-azure
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"
      memory: 8Gi
      backupStartHourUTC: 2
```

```yaml OCI resource
---
apiVersion: psql.oci.m.upbound.io/v1alpha1
kind: PsqlDbSystem
metadata:
  name: orders-db-7f3a2c       # name + UID hash
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # platform
    displayName: orders-db-7f3a2c
    dbVersion: "17"
    shape: PostgreSQL.VM.Standard.E5.Flex
    instanceOcpuCount: 2
    instanceMemorySizeInGbs: 32
    instanceCount: 1
    configId: ocid1.postgresqlconfiguration.oc1..orders  # composed from parameters
    storageDetails:
    - systemType: OCI_OPTIMIZED_STORAGE
      isRegionallyDurable: false
    managementPolicy:
    - maintenanceWindowStart: SUN 03:00
      backupPolicy:
      - kind: DAILY
        retentionDays: 7
        backupStart: "02:00"
      pitrPolicy:
      - kind: STANDARD
        restoreDays: 7
    networkDetails:            # platform
    - subnetId: ocid1.subnet.oc1..deployment
      nsgIds:
      - ocid1.networksecuritygroup.oc1..deployment
    credentials:               # platform
    - username: dbadmin
```

*`PostgreSQLCluster` on OCI and its composed resource.*

```yaml Object on Azure
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard
  version: "17"
  storage:
    sizeGB: 20
  highAvailability: false
  deletionProtection: true
  backup:
    retentionDays: 7
  parametersRefs:
  - name: orders-tuning
  parameters:
    max_connections: "200"
  secretName: orders-db-admin
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680
      edition: ENTERPRISE
      diskType: PD_SSD
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex
      ocpus: 2
      memoryGB: 32
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    ref:
      name: orders-azure
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"
      memory: 8Gi
      backupStartHourUTC: 2
```

```yaml Azure resource
---
apiVersion: dbforpostgresql.azure.m.upbound.io/v1beta1
kind: FlexibleServer
metadata:
  name: orders-db-7f3a2c       # name + UID hash
  namespace: shop
spec:
  forProvider:
    location: northeurope      # deployment
    resourceGroupName: deployment  # platform
    version: "17"
    skuName: GP_Standard_D2ds_v5
    storageMb: 32768           # 20 GB, rounded up
    storageTier: P10
    autoGrowEnabled: true      # platform
    maintenanceWindow:
      dayOfWeek: 0
      startHour: 3
    backupRetentionDays: 7
    delegatedSubnetId: /subscriptions/s/resourceGroups/deployment/providers/Microsoft.Network/virtualNetworks/v/subnets/db  # platform
    privateDnsZoneId: /subscriptions/s/resourceGroups/deployment/providers/Microsoft.Network/privateDnsZones/db.postgres.database.azure.com  # platform
    publicNetworkAccessEnabled: false  # platform
    administratorLogin: dbadmin  # platform
    administratorPasswordSecretRef:  # platform
      name: orders-db-admin
      key: password
```

*`PostgreSQLCluster` on Azure and its composed resource.*

```yaml Object on Kubernetes
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard
  version: "17"
  storage:
    sizeGB: 20
  highAvailability: false
  deletionProtection: true
  backup:
    retentionDays: 7
  parametersRefs:
  - name: orders-tuning
  parameters:
    max_connections: "200"
  secretName: orders-db-admin
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2
  - apiVersion: gcp.database.entigo.com/v1alpha1
    kind: PostgreSQLCloudSQLConfig
    spec:
      instanceType: db-custom-2-7680
      edition: ENTERPRISE
      diskType: PD_SSD
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: oci.database.entigo.com/v1alpha1
    kind: PostgreSQLOCIConfig
    spec:
      instanceType: PostgreSQL.VM.Standard.E5.Flex
      ocpus: 2
      memoryGB: 32
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      pointInTimeDays: 7
      backupStartHourUTC: 2
  - apiVersion: azure.database.entigo.com/v1alpha1
    kind: PostgreSQLFlexibleServerConfig
    ref:
      name: orders-azure
  - apiVersion: kubernetes.database.entigo.com/v1alpha1
    kind: PostgreSQLCNPGConfig
    spec:
      cpu: "2"
      memory: 8Gi
      backupStartHourUTC: 2
```

```yaml Kubernetes resources
---
apiVersion: postgresql.cnpg.io/v1
kind: Cluster
metadata:
  name: orders-db-7f3a2c       # name + UID hash
  namespace: shop
spec:
  instances: 1
  imageCatalogRef:
    apiGroup: postgresql.cnpg.io
    kind: ClusterImageCatalog
    name: postgresql           # deployment
    major: 17
  storage:
    size: 20Gi
  resources:
    requests:
      cpu: "2"
      memory: 8Gi
    limits:
      cpu: "2"
      memory: 8Gi
  affinity:
    topologyKey: topology.kubernetes.io/zone  # platform
  primaryUpdateMethod: switchover  # platform
  enableSuperuserAccess: true  # platform; the admin Secret is read from the superuser Secret
  postgresql:
    parameters:
      max_connections: "200"
  plugins:
  - name: barman-cloud.cloudnative-pg.io
    isWALArchiver: true
    parameters:
      barmanObjectName: orders-db-7f3a2c
---
apiVersion: barmancloud.cnpg.io/v1
kind: ObjectStore
metadata:
  name: orders-db-7f3a2c
  namespace: shop
spec:
  retentionPolicy: 7d
  configuration:
    destinationPath: s3://deployment-db-backups/shop  # deployment
    s3Credentials:             # deployment
      inheritFromIAMRole: true
---
apiVersion: postgresql.cnpg.io/v1
kind: ScheduledBackup
metadata:
  name: orders-db-7f3a2c
  namespace: shop
spec:
  schedule: "0 0 2 * * *"      # seconds first
  cluster:
    name: orders-db-7f3a2c
  method: plugin
  pluginConfiguration:
    name: barman-cloud.cloudnative-pg.io
```

*`PostgreSQLCluster` on Kubernetes and its composed resources.*

## Platform Decisions

Besides the settings it makes for every service
([Platform Decisions](common-design.md#platform-decisions)), the platform makes these for a
`PostgreSQLCluster` itself:

| Setting      | Platform's Choice                                                                                                                                                                                                                                    |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Admin login  | An admin username and a generated password; on RDS, the password RDS manages, read into the admin Secret, so taking ownership of an instance changes no password; on CloudNativePG, the `postgres` superuser and the password the operator generates |
| Port         | 5432                                                                                                                                                                                                                                                 |
| Final backup | A backup taken when a cluster is deleted, switched on per deployment, where the cloud takes one                                                                                                                                                      |
| Zones        | On CloudNativePG, instances on separate zones, preferred rather than required, so a cluster in one zone still starts                                                                                                                                 |
| Resources    | On CloudNativePG, CPU and memory requested and limited alike, so an instance is never squeezed by its neighbours                                                                                                                                     |

*`PostgreSQLCluster` settings chosen by the platform.*

## Limits

The backend enforces these limits itself, and the API does not check them (principle 10); a value
outside them is rejected by the backend and shown as `CloudAccepted: False`. They are listed for
reference, should validating them be decided later. Each limit holds as of its source: the provider
release named, or the cloud's documentation as read in November 2026.

| Field                                     | Backend    | Limit                                                                           | Source                                                                                     |
| ----------------------------------------- | ---------- | ------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| `backup.retentionDays`                    | AWS        | 0 to 35                                                                         | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0                     |
| `backup.retentionDays`                    | Azure      | 7 to 35                                                                         | `backupRetentionDays`, `FlexibleServer` CRD, provider-upjet-azure v2.8.0                   |
| `backup.retentionDays`, `pointInTimeDays` | OCI        | up to 35                                                                        | OCI Database with PostgreSQL documentation, backups and point-in-time recovery             |
| `pointInTimeDays`                         | GCP        | 1 to 7; 1 to 35 on `ENTERPRISE_PLUS`                                            | `transactionLogRetentionDays`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0           |
| `storage.sizeGB`                          | GCP        | at least 10; 20 on `HYPERDISK_BALANCED`                                         | `settings.diskSize`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0                     |
| `iops`, `throughputMBps`                  | AWS        | only from 400 GiB of disk                                                       | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0                 |
| `iops`                                    | OCI        | steps of 75,000                                                                 | `storageDetails.iops`, `PsqlDbSystem` CRD, crossplane-provider-oci v2.0.0                  |
| `edition`                                 | GCP        | `ENTERPRISE`, `ENTERPRISE_PLUS`                                                 | `settings.edition`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0                      |
| `diskType`                                | GCP        | `PD_SSD`, `PD_HDD`, `HYPERDISK_BALANCED`                                        | `settings.diskType`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0                     |
| `storageTier`                             | Azure      | `P4` to `P80`                                                                   | `storageTier`, `FlexibleServer` CRD, provider-upjet-azure v2.8.0                           |
| `storage.sizeGB`                          | Kubernetes | never lowered; growing needs a StorageClass with `allowVolumeExpansion`         | `storage.size`, `Cluster` CRD, CloudNativePG v1.30.2; CloudNativePG documentation, storage |
| `parameters`                              | Kubernetes | not `port`, `listen_addresses`, `ssl` and the other settings the operator fixes | CloudNativePG documentation, PostgreSQL configuration, fixed parameters                    |

*PostgreSQL field limits and their sources.*
