# PostgreSQL

PostgreSQL is an open-source relational database, used by applications for their transactional data.

Three kinds: `PostgreSQLInstance`, one database server; `PostgreSQLUser`, one application's login on
an instance, with its permissions; and `PostgreSQLDatabase`, one database on an instance.

Built on Amazon RDS (AWS), Cloud SQL (GCP), OCI Database with PostgreSQL (OCI) and Azure Database
for PostgreSQL flexible server (Azure).

## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Status](#status)
- [Other Kinds](#other-kinds)
- [Platform Decisions](#platform-decisions)
- [Ignored Settings](#ignored-settings)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Spec

What a user writes to create a `PostgreSQLInstance`. The comments note value ranges and what holds
on one cloud only.

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLInstance
metadata:
  name: orders-db
  namespace: shop
spec:
  # Common
  version: "17"                        # the full "17.7" with minor upgrades off
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100                     # autoGrow ceiling
    iops: 12000
    throughputMBps: 500
  highAvailability: false              # a standby in another zone
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
    autoMinorVersionUpgrade: true
    allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7                   # backups kept; 0 disables them
    pointInTimeDays: 7                 # how far back restore.pointInTime reaches
    startHourUTC: 2
  parameters:                          # PostgreSQL settings
    max_connections: "200"
  parameterApplyMethod: Immediate      # or NextRestart
  restore:                             # create time only
    fromInstance: orders-db-old        # a PostgreSQLInstance in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
  # Cloud-specific; every block also takes id (rule 9)
  aws:
    instanceType: db.t4g.small         # the RDS instance class
    parameterGroupName: orders-pg17    # not with parameters
    fromBackup: orders-final           # an RDS snapshot ID; not with restore
  gcp:
    instanceType: db-custom-2-7680     # the Cloud SQL tier
    edition: ENTERPRISE                # or ENTERPRISE_PLUS
    diskType: PD_SSD                   # PD_HDD, HYPERDISK_BALANCED
  oci:
    instanceType: PostgreSQL.VM.Standard.E5.Flex  # the DB system shape
    ocpus: 2                           # required with a .Flex shape
    memoryGB: 32                       # required with a .Flex shape
  azure:
    instanceType: GP_Standard_D2ds_v5  # the flexible server SKU
    storageTier: P10                   # P4 to P80; sets the disk's IOPS
```

The size is in the cloud blocks, as `instanceType` with the cloud's own name, because RDS has fixed
classes while OCI and GCP take CPU and memory, which no neutral size expresses. With deployment
defaults, a user without a size requirement can omit it. On GCP the size belongs to an edition:
`db-custom-*` to `ENTERPRISE`, `db-perf-optimized-*` to `ENTERPRISE_PLUS`, which Cloud SQL selects
by default from PostgreSQL 16. The platform therefore always sets `gcp.edition`, and a deployment's
defaults for `gcp.edition` and `gcp.instanceType` must belong together.

`version` is the major version, such as `"17"`, and the cloud applies minor versions in the
maintenance window. Only RDS can disable minor upgrades, and it then needs a full version
(`engineVersion` in `rds.aws.m.upbound.io_instances`, [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0)), so a user
who sets `maintenance.autoMinorVersionUpgrade: false` writes the full version, such as `"17.7"`, and
changes it to upgrade. A parameter group applies to one major version, so a major change is rejected
unless `aws.parameterGroupName` changes or is removed in the same edit.

`storage.sizeGB` is the size at creation and a floor afterwards: raising it grows the disk, and
lowering it is rejected. The platform rounds it up to a size the cloud accepts: at least 20 GiB on
AWS, and the next of Azure's 12 fixed sizes; AWS itself rounds an increase under 10% up to 10%.
Rounding and `autoGrow` can take the disk past `sizeGB`, so the status reports the actual size, and
the platform never shrinks the disk back to `sizeGB`.

`aws.parameterGroupName`, kept from today's API, uses an existing RDS parameter group in place of
`parameters`.

Azure's same-zone standby keeps no copy in another zone, so `highAvailability: true` maps to its
zone-redundant mode only.

The kind is one database server. Read replicas belong to a cluster-shaped kind, if one is added.

## Mapping

How each field maps to the managed service of each cloud, named by the fields of that cloud's
Crossplane provider. Level is the field's support level (rule 1).

| Field                                  | Level                                              | AWS (RDS `Instance`)                                                        | GCP (Cloud SQL `DatabaseInstance`)                                                                                                       | OCI (`PsqlDbSystem`)                                                      | Azure (`FlexibleServer`)                                |
| -------------------------------------- | -------------------------------------------------- | --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------- |
| `observe`                              | Core                                               | `managementPolicies: [Observe]`                                             | `managementPolicies: [Observe]`                                                                                                          | `managementPolicies: [Observe]`                                           | `managementPolicies: [Observe]`                         |
| `version`                              | Core; a full version ignored on GCP, OCI and Azure | `engineVersion`; the full version with minor upgrades off                   | `databaseVersion: POSTGRES_17`                                                                                                           | `dbVersion: "17"`                                                         | `version: "17"`                                         |
| `storage.sizeGB`                       | Core                                               | `allocatedStorage`; below 20 rounded up to 20                               | `settings.diskSize`                                                                                                                      | ignored; storage grows automatically                                      | `storageMb`, rounded up to the next of its 12 sizes     |
| `storage.autoGrow`                     | Core; `false` ignored on OCI                       | on with `maxAllocatedStorage`                                               | `settings.diskAutoresize`                                                                                                                | always on; `false` ignored                                                | `autoGrowEnabled`                                       |
| `storage.maxSizeGB`                    | Ignored on OCI and Azure                           | `maxAllocatedStorage`; a deployment default, since RDS needs a ceiling      | `settings.diskAutoresizeLimit`                                                                                                           | ignored; storage grows without a ceiling                                  | ignored                                                 |
| `storage.iops`                         | Ignored on Azure, and on GCP without Hyperdisk     | `iops`                                                                      | `settings.dataDiskProvisionedIops`; ignored without Hyperdisk                                                                            | `storageDetails.iops`                                                     | ignored; `azure.storageTier` sets IOPS                  |
| `storage.throughputMBps`               | Ignored on OCI, Azure, and GCP without Hyperdisk   | `storageThroughput`                                                         | `settings.dataDiskProvisionedThroughput`; ignored without Hyperdisk                                                                      | ignored                                                                   | ignored                                                 |
| `highAvailability`                     | Core                                               | `multiAz`                                                                   | `settings.availabilityType: REGIONAL`; with `backup.retentionDays: 0` rejected, as a regional instance needs backups                     | `instanceCount: 2`, regional storage                                      | `highAvailability.mode: ZoneRedundant`                  |
| `deletionProtection`                   | Core                                               | `deletionProtection`                                                        | `settings.deletionProtectionEnabled`                                                                                                     | platform blocks deletion                                                  | platform blocks deletion                                |
| `maintenance.window`                   | Core                                               | `maintenanceWindow: sun:03:00-sun:03:30`                                    | `settings.maintenanceWindow` (`day: 7`, `hour: 3`)                                                                                       | `managementPolicy.maintenanceWindowStart: "SUN 03:00"`                    | `maintenanceWindow` (`dayOfWeek: 0`, `startHour: 3`)    |
| `maintenance.autoMinorVersionUpgrade`  | Core; `false` ignored on GCP, OCI and Azure        | `autoMinorVersionUpgrade`                                                   | always on; `false` ignored                                                                                                               | always on; `false` ignored                                                | always on; `false` ignored                              |
| `maintenance.allowMajorVersionUpgrade` | Core; a major upgrade fails on OCI                 | `allowMajorVersionUpgrade`                                                  | `databaseVersion` change allowed                                                                                                         | a major upgrade fails until the provider supports it                      | `version` change allowed                                |
| `backup.retentionDays`                 | Core; `0` ignored on Azure                         | `backupRetentionPeriod`                                                     | `settings.backupConfiguration.backupRetentionSettings.retainedBackups`, with `pointInTimeRecoveryEnabled: true`; 0 sets `enabled: false` | `managementPolicy.backupPolicy.retentionDays`, DAILY; 0 sets `kind: NONE` | `backupRetentionDays`; 0 ignored, backups are always on |
| `backup.pointInTimeDays`               | Core                                               | the same setting; above `retentionDays` rejected                            | `settings.backupConfiguration.transactionLogRetentionDays`                                                                               | `managementPolicy.pitrPolicy.restoreDays`, STANDARD                       | the same setting; above `retentionDays` rejected        |
| `backup.startHourUTC`                  | Ignored on Azure                                   | `backupWindow`, 30 minutes                                                  | `settings.backupConfiguration.startTime`                                                                                                 | `managementPolicy.backupPolicy.backupStart`                               | ignored; Azure picks the hour                           |
| `parameters`                           | Core                                               | a `ParameterGroup`                                                          | `settings.databaseFlags`; keys outside its allowlist rejected                                                                            | a new `PsqlConfiguration` per change                                      | one `FlexibleServerConfiguration` each                  |
| `parameterApplyMethod`                 | Core; `NextRestart` ignored on GCP, OCI and Azure  | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` | restarts when a flag needs it; `NextRestart` ignored                                                                                     | `NextRestart` ignored                                                     | restarts when a setting needs it; `NextRestart` ignored |
| `restore.fromInstance`                 | Core                                               | `restoreToPointInTime.sourceDbInstanceIdentifier`                           | `clone.sourceInstanceName`                                                                                                               | `source.sourceType: POINT_IN_TIME_DB_SYSTEM`, `source.dbSystemId`         | `createMode: PointInTimeRestore`, `sourceServerId`      |
| `restore.pointInTime`                  | Core                                               | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`        | `clone.pointInTime`                                                                                                                      | `source.timeToRestore`                                                    | `pointInTimeRestoreTimeInUtc`                           |

The cloud-specific fields:

| Field                    | Maps To                                                                  |
| ------------------------ | ------------------------------------------------------------------------ |
| `aws.id`                 | the console's Resource ID (`db-...`)                                     |
| `gcp.id`                 | the instance name                                                        |
| `oci.id`                 | the DB system's OCID                                                     |
| `azure.id`               | the server's resource ID; the function takes the name and resource group |
| `aws.instanceType`       | `instanceClass`                                                          |
| `gcp.instanceType`       | `settings.tier`                                                          |
| `oci.instanceType`       | `shape`; a `.Flex` shape without `oci.ocpus` and `oci.memoryGB` rejected |
| `azure.instanceType`     | `skuName`                                                                |
| `aws.fromBackup`         | `snapshotIdentifier`                                                     |
| `gcp.fromBackup`         | `restoreBackupContext` (`instanceId`, `backupRunId`)                     |
| `oci.fromBackup`         | `source.sourceType: BACKUP`, `source.backupId`                           |
| `aws.parameterGroupName` | `parameterGroupName`                                                     |
| `gcp.edition`            | `settings.edition`                                                       |
| `gcp.diskType`           | `settings.diskType`                                                      |
| `oci.ocpus`              | `instanceOcpuCount`                                                      |
| `oci.memoryGB`           | `instanceMemorySizeInGbs`                                                |
| `azure.storageTier`      | `storageTier`                                                            |

Provider keys are from the CRDs of provider-upjet-aws v2.4.0, [provider-upjet-gcp
v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0),
[crossplane-provider-oci
v2.0.0](https://github.com/oracle/crossplane-provider-oci/releases/tag/v2.0.0) and
[provider-upjet-azure
v2.8.0](https://github.com/crossplane-contrib/provider-upjet-azure/releases/tag/v2.8.0). GCP's
`updateTrack` only delays minor upgrades.

## Status

`storage.sizeGB` stays empty on OCI, which reports no size. `status.aws.finalBackup` is the ID the
final snapshot will receive; GCP's provider reports only `finalBackupDescription`, and OCI and Azure
take no final backup. A deleted instance is restored with the cloud block's `fromBackup` and the
final backup's ID, which the status loses with the instance, so it is taken from the cloud console.
`endpoint.host` is a hostname on AWS and Azure and an IP address on GCP and OCI. The running minor
version, the latest restorable time and the instance state are reported by only some providers
(provider-upjet-aws v2.4.0 and the versions in rule 7), so they go in the cloud blocks:

```yaml
status:
  aws:
    finalBackup: ...
    identifier: ...
    resourceId: ...
    engineVersionActual: "17.7"
    latestRestorableTime: ...
    status: available
  gcp:
    id: ...
    connectionName: ...
    maintenanceVersion: ...
  oci:
    id: ...
    instanceOcpuCount: 2
    instanceMemorySizeInGbs: 32
    state: ACTIVE
  azure:
    id: ...
```

The admin login is in the secret `<instance>-dbadmin`, with the keys of rule 6.

## Other Kinds

`PostgreSQLUser` and `PostgreSQLDatabase` act inside PostgreSQL, through provider-sql, so they are
the same on every cloud. They retain today's fields; what changes is the version, `namespace` in
`instanceRef` (rule 11), defaults moving to the admission policy (rule 5), and `name` and
`instanceRef` becoming immutable.

### PostgreSQLUser

A set of permissions shared by several users is a role: a `PostgreSQLUser` with `login: false`,
which other users receive through `grant.roles`.

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLUser
metadata:
  name: orders-app
  namespace: shop
spec:
  name: orders_app                     # the role's name in PostgreSQL; unset, metadata.name
  instanceRef:
    name: orders-db
  login: true
  createDb: false
  createRole: false
  inherit: true
  grant:
    roles:
    - reporting
```

### PostgreSQLDatabase

`extensions` creates an extension in that database; on Azure a server must also allow it in
`azure.extensions` ([Allow
extensions](https://learn.microsoft.com/en-us/azure/postgresql/extensions/how-to-allow-extensions)),
so every Azure instance allows the whole allowlist, and any database can create any of them, as on
the other clouds.

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLDatabase
metadata:
  name: orders
  namespace: shop
spec:
  instanceRef:
    name: orders-db
  owner: orders_app                    # a role name in PostgreSQL
  encoding: UTF8
  extensions:
  - postgis
  extensionConfig:
    postgis:
      schema: gis
  deletionProtection: true
```

## Platform Decisions

Network placement (subnets, security group or NSG, Azure private DNS zone), no public access, gp3
storage on AWS, encryption with the platform's data key, admin username and generated password,
workspace and Zone tags, port 5432, and a final backup when an instance is deleted, switched on per
deployment as today, on the clouds that take one (AWS, GCP). crossplane-provider-oci v2.0.0 has no
key field for OCI Database with PostgreSQL, so OCI uses the service's own key until the provider
adds one.

## Ignored Settings

Settings this service accepts on every cloud but that have no effect on some (rule 1), and what that
means there.

| Setting                                                        | Ignored On                        | Effect                                                                    |
| -------------------------------------------------------------- | --------------------------------- | ------------------------------------------------------------------------- |
| `storage.autoGrow: false`                                      | OCI                               | The storage keeps growing.                                                |
| `storage.maxSizeGB`                                            | OCI, Azure                        | Growth has no ceiling.                                                    |
| `storage.iops`                                                 | Azure; GCP without Hyperdisk      | The disk keeps its default IOPS; on Azure, `azure.storageTier` sets them. |
| `storage.throughputMBps`                                       | OCI, Azure; GCP without Hyperdisk | The disk keeps its default throughput.                                    |
| `maintenance.autoMinorVersionUpgrade: false`, a full `version` | GCP, OCI, Azure                   | New minor versions are applied in the maintenance window.                 |
| `backup.retentionDays: 0`                                      | Azure                             | Automated backups stay on.                                                |
| `backup.startHourUTC`                                          | Azure                             | Azure picks the backup hour.                                              |
| `parameterApplyMethod: NextRestart`                            | GCP, OCI, Azure                   | A setting change that needs a restart restarts the instance at once.      |

## Cloud Limits

Limits each cloud enforces itself, which the API does not check (rule 10); a value outside them is
rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference, should
validating them be decided later.

| Field                                     | Cloud | Limit                                    | Source                                                                           |
| ----------------------------------------- | ----- | ---------------------------------------- | -------------------------------------------------------------------------------- |
| `backup.retentionDays`                    | AWS   | 0 to 35                                  | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0           |
| `backup.retentionDays`                    | Azure | 7 to 35                                  | `backupRetentionDays`, `FlexibleServer` CRD, provider-upjet-azure v2.8.0         |
| `backup.retentionDays`, `pointInTimeDays` | OCI   | up to 35                                 | OCI Database with PostgreSQL docs, backups and point-in-time recovery            |
| `backup.pointInTimeDays`                  | GCP   | 1 to 7; 1 to 35 on `ENTERPRISE_PLUS`     | `transactionLogRetentionDays`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0 |
| `storage.sizeGB`                          | GCP   | at least 10; 20 on `HYPERDISK_BALANCED`  | `settings.diskSize`, `DatabaseInstance` CRD                                      |
| `storage.iops`, `throughputMBps`          | AWS   | only from 400 GiB of disk                | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0       |
| `storage.iops`                            | OCI   | steps of 75,000                          | `storageDetails.iops`, `PsqlDbSystem` CRD, crossplane-provider-oci v2.0.0        |
| `gcp.edition`                             | GCP   | `ENTERPRISE`, `ENTERPRISE_PLUS`          | `settings.edition`, `DatabaseInstance` CRD                                       |
| `gcp.diskType`                            | GCP   | `PD_SSD`, `PD_HDD`, `HYPERDISK_BALANCED` | `settings.diskType`, `DatabaseInstance` CRD                                      |
| `azure.storageTier`                       | Azure | `P4` to `P80`                            | `storageTier`, `FlexibleServer` CRD                                              |

## Notes

### Alternatives Considered

Designs that were weighed and rejected, each with the reason.

#### 1 Catalog of fixed OCI shapes

Allow only OCI's fixed shapes, so `oci.instanceType` is one name, as on the other
clouds.

**Rejected:** The platform would have to keep the catalog current, and a customer needing a custom
OCPU and memory combination could not have it.
