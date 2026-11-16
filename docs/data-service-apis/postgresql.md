# PostgreSQL

PostgreSQL is an open-source relational database, used by applications for their transactional data.

Three kinds: `PostgreSQLInstance`, one database server; `PostgreSQLUser`, one application's login on
an instance, with its permissions; and `PostgreSQLDatabase`, one database on an instance.

Built on Amazon RDS (AWS), Cloud SQL (GCP), OCI Database with PostgreSQL (OCI) and Azure Database
for PostgreSQL flexible server (Azure).

[API Changes](changes.md#postgresql) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)

## Kinds

PostgreSQL is offered as three kinds, each with the manifest a user writes.

### PostgreSQLInstance

A user creates a `PostgreSQLInstance` with a manifest like the one below. Its comments note value
ranges and what holds on one cloud only:

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
  secretName: orders-db-admin          # unset, orders-db-dbadmin (rule 6)
  # Cloud-specific; every block also takes id (rule 9)
  aws:
    instanceType: db.t4g.small         # the RDS instance class
    # parameterGroupName: orders-pg17  # an existing group, instead of parameters
    # fromBackup: orders-final         # an RDS snapshot ID, instead of restore
  gcp:
    instanceType: db-custom-2-7680     # the Cloud SQL tier
    edition: ENTERPRISE                # or ENTERPRISE_PLUS
    diskType: PD_SSD                   # PD_HDD, HYPERDISK_BALANCED
    # fromBackup: orders-db/1234       # instance and backup run, instead of restore
  oci:
    instanceType: PostgreSQL.VM.Standard.E5.Flex  # the DB system shape
    ocpus: 2                           # required with a .Flex shape
    memoryGB: 32                       # required with a .Flex shape
    # fromBackup: ocid1...             # a backup OCID, instead of restore
  azure:
    instanceType: GP_Standard_D2ds_v5  # the flexible server SKU
    storageTier: P10                   # P4 to P80; sets the disk's IOPS
```

*A `PostgreSQLInstance` with its common fields and a block per cloud.*

`restore` creates an instance from another instance's backups, at a point in time. A deleted
instance is restored instead with its cloud block's `fromBackup`, such as `aws.fromBackup` with an
RDS snapshot ID. Both are set at creation only and cannot be combined:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLInstance
metadata:
  name: orders-db-restored
  namespace: shop
spec:
  version: "17"
  restore:
    fromInstance: orders-db            # a PostgreSQLInstance in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
  aws:
    instanceType: db.t4g.small
```

*A `PostgreSQLInstance` created from another instance's backups.*

`version` is the major version, such as `"17"`, and the cloud applies minor versions in the
maintenance window.

`storage.sizeGB` is the size at creation and a floor afterwards: raising it grows the disk, and
lowering it is rejected. The cloud may round it up and `autoGrow` can take the disk further, so the
status reports the actual size, and the platform never shrinks the disk back to `sizeGB`.

The kind is one database server. Read replicas belong to a cluster-shaped kind, if one is added.

The final backup's ID, which `fromBackup` needs, is lost with the instance's status, so it is taken
from the cloud console. The running minor version, the latest restorable time and the instance state
are reported by only some providers, so they go in the cloud blocks:

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

*The cloud blocks of a `PostgreSQLInstance` status.*

The admin login is in the secret `<instance>-dbadmin`, with the keys of rule 6.

### PostgreSQLUser

`PostgreSQLUser` acts inside PostgreSQL, through provider-sql, so it is the same on every cloud. Its
`name` and `instanceRef` are immutable, `instanceRef` may name another namespace of the same Zone
(rule 11), and its defaults are written by the admission policy (rule 5).

A set of permissions shared by several users is a role: a `PostgreSQLUser` with `login: false`,
which other users receive through `grant.roles`.

An example:

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
    namespace: shop                    # unset, this namespace; within the Zone (rule 11)
  login: true
  createDb: false
  createRole: false
  inherit: true
  grant:
    roles:
    - reporting
  secretName: orders-app-login         # unset, orders-db-orders-app (rule 6)
```

*A `PostgreSQLUser` that receives the `reporting` role.*

### PostgreSQLDatabase

`PostgreSQLDatabase` acts inside PostgreSQL, through provider-sql, so it is the same on every cloud.
Its `name` and `instanceRef` are immutable, `instanceRef` may name another namespace of the same
Zone (rule 11), and its defaults are written by the admission policy (rule 5).

`extensions` creates an extension in that database; on Azure a server must also allow it in
`azure.extensions` ([Allow
extensions](https://learn.microsoft.com/en-us/azure/postgresql/extensions/how-to-allow-extensions)),
so every Azure instance allows the whole allowlist, and any database can create any of them, as on
the other clouds.

An example:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLDatabase
metadata:
  name: orders
  namespace: shop
spec:
  name: orders                         # the database's name in PostgreSQL; unset, metadata.name
  instanceRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (rule 11)
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

*A `PostgreSQLDatabase` with the PostGIS extension in its own schema.*

## Mapping

The table names what each field maps to in the cloud's provider. A setting the cloud lacks shows as
ignored, with the reason (rule 1), and a field of a cloud's block appears only for that cloud:

| Field                                  | AWS                                                                         | GCP                                                                                                                                      | OCI                                                                                  | Azure                                                                                |
| -------------------------------------- | --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------ |
| `observe`                              | `managementPolicies: [Observe]`                                             | `managementPolicies: [Observe]`                                                                                                          | `managementPolicies: [Observe]`                                                      | `managementPolicies: [Observe]`                                                      |
| `version`                              | `engineVersion`; the full version with minor upgrades off                   | `databaseVersion: POSTGRES_17`; a full version ignored                                                                                   | `dbVersion: "17"`; a full version ignored                                            | `version: "17"`; a full version ignored                                              |
| `storage.sizeGB`                       | `allocatedStorage`; below 20 rounded up to 20                               | `settings.diskSize`                                                                                                                      | ignored; storage grows automatically                                                 | `storageMb`, rounded up to the next of its 12 sizes                                  |
| `storage.autoGrow`                     | on with `maxAllocatedStorage`                                               | `settings.diskAutoresize`                                                                                                                | always on; `false` ignored                                                           | `autoGrowEnabled`                                                                    |
| `storage.maxSizeGB`                    | `maxAllocatedStorage`; a deployment default, since RDS needs a ceiling      | `settings.diskAutoresizeLimit`                                                                                                           | ignored; storage grows without a ceiling                                             | ignored; growth has no ceiling                                                       |
| `storage.iops`                         | `iops`                                                                      | `settings.dataDiskProvisionedIops`; ignored without Hyperdisk                                                                            | `storageDetails.iops`                                                                | ignored; `azure.storageTier` sets IOPS                                               |
| `storage.throughputMBps`               | `storageThroughput`                                                         | `settings.dataDiskProvisionedThroughput`; ignored without Hyperdisk                                                                      | ignored; the disk keeps its default throughput                                       | ignored; the disk keeps its default throughput                                       |
| `highAvailability`                     | `multiAz`                                                                   | `settings.availabilityType: REGIONAL`; with `backup.retentionDays: 0` rejected, as a regional instance needs backups                     | `instanceCount: 2`, regional storage                                                 | `highAvailability.mode: ZoneRedundant`                                               |
| `deletionProtection`                   | `deletionProtection`                                                        | `settings.deletionProtectionEnabled`                                                                                                     | platform blocks deletion                                                             | platform blocks deletion                                                             |
| `maintenance.window`                   | `maintenanceWindow: sun:03:00-sun:03:30`                                    | `settings.maintenanceWindow` (`day: 7`, `hour: 3`)                                                                                       | `managementPolicy.maintenanceWindowStart: "SUN 03:00"`                               | `maintenanceWindow` (`dayOfWeek: 0`, `startHour: 3`)                                 |
| `maintenance.autoMinorVersionUpgrade`  | `autoMinorVersionUpgrade`                                                   | always on; `false` ignored, and minor versions are applied in the maintenance window                                                     | always on; `false` ignored, and minor versions are applied in the maintenance window | always on; `false` ignored, and minor versions are applied in the maintenance window |
| `maintenance.allowMajorVersionUpgrade` | `allowMajorVersionUpgrade`                                                  | `databaseVersion` change allowed                                                                                                         | a major upgrade fails until the provider supports it                                 | `version` change allowed                                                             |
| `backup.retentionDays`                 | `backupRetentionPeriod`                                                     | `settings.backupConfiguration.backupRetentionSettings.retainedBackups`, with `pointInTimeRecoveryEnabled: true`; 0 sets `enabled: false` | `managementPolicy.backupPolicy.retentionDays`, DAILY; 0 sets `kind: NONE`            | `backupRetentionDays`; 0 ignored, backups are always on                              |
| `backup.pointInTimeDays`               | the same setting; above `retentionDays` rejected                            | `settings.backupConfiguration.transactionLogRetentionDays`                                                                               | `managementPolicy.pitrPolicy.restoreDays`, STANDARD                                  | the same setting; above `retentionDays` rejected                                     |
| `backup.startHourUTC`                  | `backupWindow`, 30 minutes                                                  | `settings.backupConfiguration.startTime`                                                                                                 | `managementPolicy.backupPolicy.backupStart`                                          | ignored; Azure picks the hour                                                        |
| `parameters`                           | a `ParameterGroup`                                                          | `settings.databaseFlags`; keys outside its allowlist rejected                                                                            | a new `PsqlConfiguration` per change                                                 | one `FlexibleServerConfiguration` each                                               |
| `parameterApplyMethod`                 | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` | restarts when a flag needs it; `NextRestart` ignored                                                                                     | `NextRestart` ignored; a change that needs a restart restarts at once                | restarts when a setting needs it; `NextRestart` ignored                              |
| `restore.fromInstance`                 | `restoreToPointInTime.sourceDbInstanceIdentifier`                           | `clone.sourceInstanceName`                                                                                                               | `source.sourceType: POINT_IN_TIME_DB_SYSTEM`, `source.dbSystemId`                    | `createMode: PointInTimeRestore`, `sourceServerId`                                   |
| `restore.pointInTime`                  | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`        | `clone.pointInTime`                                                                                                                      | `source.timeToRestore`                                                               | `pointInTimeRestoreTimeInUtc`                                                        |
| `aws.id`                               | the console's Resource ID (`db-...`)                                        |                                                                                                                                          |                                                                                      |                                                                                      |
| `gcp.id`                               |                                                                             | the instance name                                                                                                                        |                                                                                      |                                                                                      |
| `oci.id`                               |                                                                             |                                                                                                                                          | the DB system's OCID                                                                 |                                                                                      |
| `azure.id`                             |                                                                             |                                                                                                                                          |                                                                                      | the server's resource ID; the function takes the name and resource group             |
| `aws.instanceType`                     | `instanceClass`                                                             |                                                                                                                                          |                                                                                      |                                                                                      |
| `gcp.instanceType`                     |                                                                             | `settings.tier`                                                                                                                          |                                                                                      |                                                                                      |
| `oci.instanceType`                     |                                                                             |                                                                                                                                          | `shape`; a `.Flex` shape without `oci.ocpus` and `oci.memoryGB` rejected             |                                                                                      |
| `azure.instanceType`                   |                                                                             |                                                                                                                                          |                                                                                      | `skuName`                                                                            |
| `aws.fromBackup`                       | `snapshotIdentifier`                                                        |                                                                                                                                          |                                                                                      |                                                                                      |
| `gcp.fromBackup`                       |                                                                             | `restoreBackupContext` (`instanceId`, `backupRunId`)                                                                                     |                                                                                      |                                                                                      |
| `oci.fromBackup`                       |                                                                             |                                                                                                                                          | `source.sourceType: BACKUP`, `source.backupId`                                       |                                                                                      |
| `aws.parameterGroupName`               | `parameterGroupName`                                                        |                                                                                                                                          |                                                                                      |                                                                                      |
| `gcp.edition`                          |                                                                             | `settings.edition`                                                                                                                       |                                                                                      |                                                                                      |
| `gcp.diskType`                         |                                                                             | `settings.diskType`                                                                                                                      |                                                                                      |                                                                                      |
| `oci.ocpus`                            |                                                                             |                                                                                                                                          | `instanceOcpuCount`                                                                  |                                                                                      |
| `oci.memoryGB`                         |                                                                             |                                                                                                                                          | `instanceMemorySizeInGbs`                                                            |                                                                                      |
| `azure.storageTier`                    |                                                                             |                                                                                                                                          |                                                                                      | `storageTier`                                                                        |

*Fields of `PostgreSQLInstance` and their provider fields.*

**AWS:** provider keys are from the CRDs of [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0). Only RDS can
turn minor upgrades off, and it then needs a full version (`engineVersion` in
`rds.aws.m.upbound.io_instances`), so a user who sets `maintenance.autoMinorVersionUpgrade: false`
writes the full version, such as `"17.7"`, and changes it to upgrade. A parameter group applies to
one major version, so a major change is rejected unless `aws.parameterGroupName` changes or is
removed in the same edit. `storage.sizeGB` is rounded up to at least 20 GiB, and RDS rounds an
increase under 10% up to 10%. `status.aws.finalBackup` is the ID the final snapshot will receive.
Storage is gp3.

**GCP:** provider keys are from the CRDs of [provider-upjet-gcp
v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0). `updateTrack`
only delays minor upgrades. The size belongs to an edition: `db-custom-*` to `ENTERPRISE`,
`db-perf-optimized-*` to `ENTERPRISE_PLUS`, which Cloud SQL selects by default from PostgreSQL 16.
The platform therefore always sets `gcp.edition`, and a deployment's defaults for `gcp.edition` and
`gcp.instanceType` must belong together. The provider reports only `finalBackupDescription`, and
`endpoint.host` is an IP address.

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

## Composed Resources

The `PostgreSQLInstance` from [Kinds](#kinds) is composed into the resources below on the cloud it
runs on: the platform uses that cloud's block and ignores the others. A comment names the source of
each value the object does not hold as written: `platform` for a value the platform sets, and
`deployment` for one the deployment sets. The object and its resources:

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLInstance
metadata:
  name: orders-db
  namespace: shop
spec:
  version: "17"
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100
    iops: 12000
    throughputMBps: 500
  highAvailability: false
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
    autoMinorVersionUpgrade: true
    allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7
    pointInTimeDays: 7
    startHourUTC: 2
  parameters:
    max_connections: "200"
  parameterApplyMethod: Immediate
  secretName: orders-db-admin
  aws:
    instanceType: db.t4g.small
  gcp:
    instanceType: db-custom-2-7680
    edition: ENTERPRISE
    diskType: PD_SSD
  oci:
    instanceType: PostgreSQL.VM.Standard.E5.Flex
    ocpus: 2
    memoryGB: 32
  azure:
    instanceType: GP_Standard_D2ds_v5
    storageTier: P10
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
    iops: 12000
    storageThroughput: 500
    storageType: gp3           # platform
    multiAz: false
    deletionProtection: true
    maintenanceWindow: sun:03:00-sun:03:30
    autoMinorVersionUpgrade: true
    allowMajorVersionUpgrade: false
    backupRetentionPeriod: 7
    backupWindow: 02:00-02:30
    parameterGroupName: orders-db-7f3a2c  # composed from parameters
    username: dbadmin          # platform
    autoGeneratePassword: true  # platform
    passwordSecretRef:
      name: orders-db-admin
      key: password
    dbSubnetGroupName: deployment-db  # platform
    vpcSecurityGroupIds:       # platform
    - sg-0123456789abcdef0
    publiclyAccessible: false  # platform
    storageEncrypted: true     # platform
    kmsKeyId: arn:aws:kms:eu-north-1:111122223333:key/data  # platform
```

*A `PostgreSQLInstance` on AWS and its composed resource.*

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLInstance
metadata:
  name: orders-db
  namespace: shop
spec:
  version: "17"
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100
    iops: 12000
    throughputMBps: 500
  highAvailability: false
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
    autoMinorVersionUpgrade: true
    allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7
    pointInTimeDays: 7
    startHourUTC: 2
  parameters:
    max_connections: "200"
  parameterApplyMethod: Immediate
  secretName: orders-db-admin
  aws:
    instanceType: db.t4g.small
  gcp:
    instanceType: db-custom-2-7680
    edition: ENTERPRISE
    diskType: PD_SSD
  oci:
    instanceType: PostgreSQL.VM.Standard.E5.Flex
    ocpus: 2
    memoryGB: 32
  azure:
    instanceType: GP_Standard_D2ds_v5
    storageTier: P10
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
      diskAutoresize: true
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

*A `PostgreSQLInstance` on GCP and its composed resource.*

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLInstance
metadata:
  name: orders-db
  namespace: shop
spec:
  version: "17"
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100
    iops: 12000
    throughputMBps: 500
  highAvailability: false
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
    autoMinorVersionUpgrade: true
    allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7
    pointInTimeDays: 7
    startHourUTC: 2
  parameters:
    max_connections: "200"
  parameterApplyMethod: Immediate
  secretName: orders-db-admin
  aws:
    instanceType: db.t4g.small
  gcp:
    instanceType: db-custom-2-7680
    edition: ENTERPRISE
    diskType: PD_SSD
  oci:
    instanceType: PostgreSQL.VM.Standard.E5.Flex
    ocpus: 2
    memoryGB: 32
  azure:
    instanceType: GP_Standard_D2ds_v5
    storageTier: P10
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
      iops: "12000"
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

*A `PostgreSQLInstance` on OCI and its composed resource.*

```yaml Object
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLInstance
metadata:
  name: orders-db
  namespace: shop
spec:
  version: "17"
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100
    iops: 12000
    throughputMBps: 500
  highAvailability: false
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
    autoMinorVersionUpgrade: true
    allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7
    pointInTimeDays: 7
    startHourUTC: 2
  parameters:
    max_connections: "200"
  parameterApplyMethod: Immediate
  secretName: orders-db-admin
  aws:
    instanceType: db.t4g.small
  gcp:
    instanceType: db-custom-2-7680
    edition: ENTERPRISE
    diskType: PD_SSD
  oci:
    instanceType: PostgreSQL.VM.Standard.E5.Flex
    ocpus: 2
    memoryGB: 32
  azure:
    instanceType: GP_Standard_D2ds_v5
    storageTier: P10
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
    autoGrowEnabled: true
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

*A `PostgreSQLInstance` on Azure and its composed resource.*

## Platform Decisions

The platform makes these settings itself:

| Setting           | Platform's Choice                                                                        |
| ----------------- | ---------------------------------------------------------------------------------------- |
| Network placement | The deployment's private network                                                         |
| Public access     | None                                                                                     |
| Encryption        | The platform's data key, where the cloud takes one                                       |
| Admin login       | An admin username and a generated password                                               |
| Port              | 5432                                                                                     |
| Tags              | Workspace and Zone tags                                                                  |
| Final backup      | Taken when an instance is deleted, switched on per deployment, where the cloud takes one |

*Settings the platform makes for a `PostgreSQLInstance`.*

## Cloud Limits

The cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later. Each limit holds as of its source: the provider release
named, or the cloud's documentation as read in November 2026.

| Field                                     | Cloud | Limit                                    | Source                                                                           |
| ----------------------------------------- | ----- | ---------------------------------------- | -------------------------------------------------------------------------------- |
| `backup.retentionDays`                    | AWS   | 0 to 35                                  | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0           |
| `backup.retentionDays`                    | Azure | 7 to 35                                  | `backupRetentionDays`, `FlexibleServer` CRD, provider-upjet-azure v2.8.0         |
| `backup.retentionDays`, `pointInTimeDays` | OCI   | up to 35                                 | OCI Database with PostgreSQL documentation, backups and point-in-time recovery   |
| `backup.pointInTimeDays`                  | GCP   | 1 to 7; 1 to 35 on `ENTERPRISE_PLUS`     | `transactionLogRetentionDays`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0 |
| `storage.sizeGB`                          | GCP   | at least 10; 20 on `HYPERDISK_BALANCED`  | `settings.diskSize`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0           |
| `storage.iops`, `throughputMBps`          | AWS   | only from 400 GiB of disk                | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0       |
| `storage.iops`                            | OCI   | steps of 75,000                          | `storageDetails.iops`, `PsqlDbSystem` CRD, crossplane-provider-oci v2.0.0        |
| `gcp.edition`                             | GCP   | `ENTERPRISE`, `ENTERPRISE_PLUS`          | `settings.edition`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0            |
| `gcp.diskType`                            | GCP   | `PD_SSD`, `PD_HDD`, `HYPERDISK_BALANCED` | `settings.diskType`, `DatabaseInstance` CRD, provider-upjet-gcp v3.1.0           |
| `azure.storageTier`                       | Azure | `P4` to `P80`                            | `storageTier`, `FlexibleServer` CRD, provider-upjet-azure v2.8.0                 |

*Limits the cloud enforces on PostgreSQL.*
