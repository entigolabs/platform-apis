# PostgreSQL

Applies the rules in [rules.md](rules.md). Can be installed on every cloud (rule 7).
Schemas: [postgresqlinstance.yaml](schemas/postgresqlinstance.yaml),
[postgresqluser.yaml](schemas/postgresqluser.yaml),
[postgresqldatabase.yaml](schemas/postgresqldatabase.yaml).



## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Platform Decisions](#platform-decisions)
- [Status](#status)
- [Users and Databases](#users-and-databases)
- [Cloud Limits](#cloud-limits)
- [Alternatives Considered](#alternatives-considered)

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
  instanceType: db.t4g.small           # the cloud's own size name
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
    retentionDays: 7                   # backups kept; 0 turns them off
    pointInTimeDays: 7                 # how far back restore.pointInTime reaches
    startHourUTC: 2
  parameters:                          # PostgreSQL settings
    max_connections: "200"
  parameterApplyMethod: Immediate      # or NextRestart
  restore:                             # create time only
    fromInstance: orders-db-old        # a PostgreSQLInstance in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
    fromBackup: orders-final           # a cloud backup ID, in place of fromInstance
  # Cloud-specific; every block also takes id (rule 9)
  aws:
    parameterGroupName: orders-pg17    # not with parameters
  gcp:
    edition: ENTERPRISE                # or ENTERPRISE_PLUS
    diskType: PD_SSD                   # PD_HDD, HYPERDISK_BALANCED
  oci:
    ocpus: 2                           # required with a .Flex shape
    memoryGB: 32                       # required with a .Flex shape
  azure:
    storageTier: P10                   # P4 to P80; sets the disk's IOPS
```

`instanceType` takes the cloud's own name because RDS has fixed classes while OCI and GCP take CPU
and memory, which no neutral size expresses. With deployment defaults, a user without a size
requirement can omit it. On GCP the size belongs to an edition: `db-custom-*` to `ENTERPRISE`,
`db-perf-optimized-*` to `ENTERPRISE_PLUS`, which Cloud SQL picks by default from PostgreSQL 16. The
platform therefore always sets `gcp.edition`, and a deployment's defaults for `gcp.edition` and
`instanceType` must belong together.

`version` is the major version, such as `"17"`, and the cloud applies minor versions in the
maintenance window. Only RDS can turn minor upgrades off, and it then needs a full version
(`engineVersion` in `rds.aws.m.upbound.io_instances`, [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0)), so a user
who sets `maintenance.autoMinorVersionUpgrade: false` writes the full version, such as `"17.7"`, and
changes it to upgrade. A parameter group fits one major version, so a major change is rejected
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
Crossplane provider.

| Field                                  | AWS (RDS `Instance`)                                                        | GCP (Cloud SQL `DatabaseInstance`)                                                                                                       | OCI (`PsqlDbSystem`)                                                      | Azure (`FlexibleServer`)                                 |
| -------------------------------------- | --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------- | -------------------------------------------------------- |
| `version`                              | `engineVersion`; the full version with minor upgrades off                   | `databaseVersion: POSTGRES_17`                                                                                                           | `dbVersion: "17"`                                                         | `version: "17"`                                          |
| `instanceType`                         | `instanceClass: db.t4g.small`                                               | `settings.tier: db-custom-2-7680`                                                                                                        | `shape`; a `.Flex` shape without `oci.ocpus` and `oci.memoryGB` rejected  | `skuName: GP_Standard_D2ds_v5`                           |
| `storage.sizeGB`                       | `allocatedStorage`; below 20 rounded up to 20                               | `settings.diskSize`                                                                                                                      | ignored; storage grows on its own                                         | `storageMb`, rounded up to the next of its 12 sizes      |
| `storage.autoGrow`                     | on with `maxAllocatedStorage`                                               | `settings.diskAutoresize`                                                                                                                | always on; `false` rejected                                               | `autoGrowEnabled`                                        |
| `storage.maxSizeGB`                    | `maxAllocatedStorage`; a deployment default, since RDS needs a ceiling      | `settings.diskAutoresizeLimit`                                                                                                           | rejected                                                                  | rejected                                                 |
| `storage.iops`                         | `iops`                                                                      | `settings.dataDiskProvisionedIops`; rejected without Hyperdisk                                                                           | `storageDetails.iops`                                                     | rejected; `azure.storageTier` sets IOPS                  |
| `storage.throughputMBps`               | `storageThroughput`                                                         | `settings.dataDiskProvisionedThroughput`; rejected without Hyperdisk                                                                     | rejected                                                                  | rejected                                                 |
| `highAvailability`                     | `multiAz`                                                                   | `settings.availabilityType: REGIONAL`; with `backup.retentionDays: 0` rejected, as a regional instance needs backups                     | `instanceCount: 2`, regional storage                                      | `highAvailability.mode: ZoneRedundant`                   |
| `deletionProtection`                   | `deletionProtection`                                                        | `settings.deletionProtectionEnabled`                                                                                                     | platform blocks deletion                                                  | platform blocks deletion                                 |
| `maintenance.window`                   | `maintenanceWindow: sun:03:00-sun:03:30`                                    | `settings.maintenanceWindow` (`day: 7`, `hour: 3`)                                                                                       | `managementPolicy.maintenanceWindowStart: "SUN 03:00"`                    | `maintenanceWindow` (`dayOfWeek: 0`, `startHour: 3`)     |
| `maintenance.autoMinorVersionUpgrade`  | `autoMinorVersionUpgrade`                                                   | always on; `false` rejected                                                                                                              | always on; `false` rejected                                               | always on; `false` rejected                              |
| `maintenance.allowMajorVersionUpgrade` | `allowMajorVersionUpgrade`                                                  | `databaseVersion` change allowed                                                                                                         | `true` rejected until the provider supports it                            | `version` change allowed                                 |
| `backup.retentionDays`                 | `backupRetentionPeriod`                                                     | `settings.backupConfiguration.backupRetentionSettings.retainedBackups`, with `pointInTimeRecoveryEnabled: true`; 0 sets `enabled: false` | `managementPolicy.backupPolicy.retentionDays`, DAILY; 0 sets `kind: NONE` | `backupRetentionDays`; 0 rejected, backups are always on |
| `backup.pointInTimeDays`               | the same setting; above `retentionDays` rejected                            | `settings.backupConfiguration.transactionLogRetentionDays`                                                                               | `managementPolicy.pitrPolicy.restoreDays`, STANDARD                       | the same setting; above `retentionDays` rejected         |
| `backup.startHourUTC`                  | `backupWindow`, 30 minutes                                                  | `settings.backupConfiguration.startTime`                                                                                                 | `managementPolicy.backupPolicy.backupStart`                               | rejected                                                 |
| `parameters`                           | a `ParameterGroup`                                                          | `settings.databaseFlags`; keys outside its allowlist rejected                                                                            | a new `PsqlConfiguration` per change                                      | one `FlexibleServerConfiguration` each                   |
| `parameterApplyMethod`                 | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` | restarts when a flag needs it; `NextRestart` rejected                                                                                    | `NextRestart` rejected                                                    | restarts when a setting needs it; `NextRestart` rejected |
| `restore.fromInstance`                 | `restoreToPointInTime.sourceDbInstanceIdentifier`                           | `clone.sourceInstanceName`                                                                                                               | `source.sourceType: POINT_IN_TIME_DB_SYSTEM`, `source.dbSystemId`         | `createMode: PointInTimeRestore`, `sourceServerId`       |
| `restore.pointInTime`                  | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`        | `clone.pointInTime`                                                                                                                      | `source.timeToRestore`                                                    | `pointInTimeRestoreTimeInUtc`                            |
| `restore.fromBackup`                   | `snapshotIdentifier`                                                        | `restoreBackupContext` (`instanceId`, `backupRunId`)                                                                                     | `source.sourceType: BACKUP`, `source.backupId`                            | rejected; no backups outside point-in-time restore       |

Observing (rule 9) and the cloud-specific fields:

| Field                    | Maps To                                                                   |
| ------------------------ | ------------------------------------------------------------------------- |
| `observe`                | `managementPolicies: [Observe]` on every managed resource, on every cloud |
| `aws.id`                 | the console's Resource ID (`db-...`)                                      |
| `gcp.id`                 | the instance name                                                         |
| `oci.id`                 | the DB system's OCID                                                      |
| `azure.id`               | the server's resource ID; the function takes the name and resource group  |
| `aws.parameterGroupName` | `parameterGroupName`                                                      |
| `gcp.edition`            | `settings.edition`                                                        |
| `gcp.diskType`           | `settings.diskType`                                                       |
| `oci.ocpus`              | `instanceOcpuCount`                                                       |
| `oci.memoryGB`           | `instanceMemorySizeInGbs`                                                 |
| `azure.storageTier`      | `storageTier`                                                             |

Provider keys are from the CRDs of provider-upjet-aws v2.4.0, [provider-upjet-gcp
v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0),
[crossplane-provider-oci
v2.0.0](https://github.com/oracle/crossplane-provider-oci/releases/tag/v2.0.0) and
[provider-upjet-azure
v2.8.0](https://github.com/crossplane-contrib/provider-upjet-azure/releases/tag/v2.8.0). GCP's
`updateTrack` only delays minor upgrades.

## Platform Decisions

Network placement (subnets, security group or NSG, Azure private DNS zone), no public access, gp3
storage on AWS, encryption with the platform's data key, admin username and generated password,
workspace and Zone tags, port 5432, and a final backup when an instance is deleted, switched on per
deployment as today, on the clouds that take one (AWS, GCP). crossplane-provider-oci v2.0.0 has no
key field for OCI Database with PostgreSQL, so OCI uses the service's own key until the provider
adds one.

## Status

`storage.sizeGB` stays empty on OCI, which reports no size. `finalBackup` is the ID the final backup
will get; it stays empty on GCP, whose provider reports only `finalBackupDescription`, and on Azure,
which takes none. A deleted instance is restored with `restore.fromBackup` and the final backup's
ID, which the status loses with the instance, so it is taken from the cloud console.
`endpoint.host` is a hostname on AWS and Azure and an IP address on GCP and OCI. The running minor
version, the latest restorable time and the instance state are reported by only some providers
(provider-upjet-aws v2.4.0 and the versions in rule 7), so they go in the cloud blocks:

```yaml
status:
  aws:
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


## Users and Databases

`PostgreSQLUser` and `PostgreSQLDatabase` act inside PostgreSQL, through provider-sql, so they are
the same on every cloud. They keep today's fields; what changes is the version, `namespace` in
`instanceRef` (rule 11), defaults moving to the admission policy (rule 5), and `name` and
`instanceRef` becoming immutable. `extensions` creates an extension in that database; on Azure a
server must also allow it in `azure.extensions` ([Allow
extensions](https://learn.microsoft.com/en-us/azure/postgresql/extensions/how-to-allow-extensions)),
so every Azure instance allows the whole allowlist, and any database can create any
of them, as on the other clouds.

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

## Alternatives Considered

Designs that were weighed and rejected, each with the reason.

#### 1 Catalog of Fixed OCI Shapes

Allow only OCI's fixed shapes, so `instanceType` is one name on OCI as on the other
clouds.

**Rejected:** The platform would have to keep the catalog current, and a customer needing a custom
OCPU and memory combination could not have it.
