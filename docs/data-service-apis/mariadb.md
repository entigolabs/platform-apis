# MariaDB

Applies the rules in [rules.md](rules.md). Can be installed on AWS only (rule 7).
Schemas: [mariadbinstance.yaml](schemas/mariadbinstance.yaml),
[mariadbuser.yaml](schemas/mariadbuser.yaml),
[mariadbdatabase.yaml](schemas/mariadbdatabase.yaml).

`MariaDBInstance` is one database server on Amazon RDS. MariaDB runs on AWS only, so its only cloud
block is `aws`.

## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Platform Decisions](#platform-decisions)
- [Status](#status)
- [Users and Databases](#users-and-databases)
- [Cloud Limits](#cloud-limits)

## Spec

What a user writes to create a `MariaDBInstance`. The comments note value ranges and what holds on
one cloud only.

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBInstance
metadata:
  name: orders-db
  namespace: shop
spec:
  version: "11.4"                      # the full "11.4.10" with minor upgrades off
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
    retentionDays: 7                   # 0 disables backups
    startHourUTC: 2
  parameters:                          # MariaDB settings
    max_connections: "200"
  parameterApplyMethod: Immediate      # or NextRestart
  restore:                             # create time only
    fromInstance: orders-db-old        # a MariaDBInstance in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
    fromBackup: orders-final           # an RDS snapshot ID, in place of fromInstance
  aws:                                 # also takes id (rule 9)
    instanceType: db.t4g.small         # the RDS instance class
    parameterGroupName: orders-mariadb114  # not with parameters
```

`version` is the major version, which has two numbers, such as `"11.4"` or `"10.11"`, and RDS
applies minor versions in the maintenance window. RDS takes a version without its minor only while
minor upgrades are on (`engineVersion` in `rds.aws.m.upbound.io_instances`, provider-upjet-aws
v2.4.0), so a user who sets `maintenance.autoMinorVersionUpgrade: false` writes the full version,
such as `"11.4.10"`, and changes it to upgrade. A major change is a change in the first two numbers.
A parameter group applies to one major version, so a major change is rejected unless
`aws.parameterGroupName` changes or is removed in the same edit.

`storage.sizeGB` is the size at creation and a floor afterwards: raising it grows the disk, and
lowering it is rejected. The platform rounds it up to 20 GiB, the RDS minimum, and RDS itself rounds
an increase under 10% up to 10%. Rounding and `autoGrow` can take the disk past `sizeGB`, so the
status reports the actual size, and the platform never shrinks the disk back to `sizeGB`.

`aws.parameterGroupName`, kept from today's API, uses an existing RDS parameter group in place of
`parameters`.

## Mapping

How each field maps to the managed service of each cloud, named by the fields of that cloud's
Crossplane provider.

| Field                                  | RDS `Instance`                                                              |
| -------------------------------------- | --------------------------------------------------------------------------- |
| `observe`                              | `managementPolicies: [Observe]` on every managed resource                   |
| `aws.id`                               | the console's Resource ID (`db-...`)                                        |
| `version`                              | `engineVersion`; the full version with minor upgrades off                   |
| `aws.instanceType`                     | `instanceClass`                                                             |
| `storage.sizeGB`                       | `allocatedStorage`; below 20 rounded up to 20                               |
| `storage.autoGrow`                     | on with `maxAllocatedStorage`                                               |
| `storage.maxSizeGB`                    | `maxAllocatedStorage`; a deployment default, since RDS needs a ceiling      |
| `storage.iops`                         | `iops`                                                                      |
| `storage.throughputMBps`               | `storageThroughput`                                                         |
| `highAvailability`                     | `multiAz`                                                                   |
| `deletionProtection`                   | `deletionProtection`                                                        |
| `maintenance.window`                   | `maintenanceWindow: sun:03:00-sun:03:30`                                    |
| `maintenance.autoMinorVersionUpgrade`  | `autoMinorVersionUpgrade`                                                   |
| `maintenance.allowMajorVersionUpgrade` | `allowMajorVersionUpgrade`                                                  |
| `backup.retentionDays`                 | `backupRetentionPeriod`; 0 disables backups                                 |
| `backup.startHourUTC`                  | `backupWindow`, 30 minutes                                                  |
| `parameters`                           | a `ParameterGroup` of family `mariadb<major>`                               |
| `parameterApplyMethod`                 | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` |
| `restore.fromInstance`                 | `restoreToPointInTime.sourceDbInstanceIdentifier`                           |
| `restore.pointInTime`                  | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`        |
| `restore.fromBackup`                   | `snapshotIdentifier`                                                        |
| `aws.parameterGroupName`               | `parameterGroupName`                                                        |

Provider keys are from the CRDs of [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0).

## Platform Decisions

Network placement (subnets and security group), no public access, gp3 storage, encryption with the
platform's data key, admin username and generated password, workspace and Zone tags, port 3306, and
a final snapshot when an instance is deleted, switched on per deployment as today.

## Status

`finalBackup` is the ID the final snapshot will receive. A deleted instance is restored with
`restore.fromBackup` and that ID, which the status loses with the instance, so it is taken from the
AWS console. The running minor version, the latest restorable time and the instance state go in the
`aws` block:

```yaml
status:
  aws:
    identifier: ...
    resourceId: ...
    engineVersionActual: "11.4.10"
    latestRestorableTime: ...
    status: available
```



## Users and Databases

`MariaDBUser` and `MariaDBDatabase` act inside MariaDB, through provider-sql, and retain today's
fields. What changes is the version, `namespace` in `instanceRef` (rule 11), defaults moving to the
admission policy (rule 5), and `name` and the references becoming immutable.
`MariaDBUser.databaseRef.namespace`, which today's function ignores, works within the Zone (rule
11). provider-sql writes the four secret keys rule 6 names for MariaDB users too
([`pkg/clients/mysql/mysql.go`](https://github.com/crossplane-contrib/provider-sql/blob/v0.16.1/pkg/clients/mysql/mysql.go),
[provider-sql v0.16.1](https://github.com/crossplane-contrib/provider-sql/releases/tag/v0.16.1)).
The four limits on a user (`maxQueriesPerHour`, `maxUpdatesPerHour`, `maxConnectionsPerHour`,
`maxUserConnections`) default to 0, which MariaDB treats as no limit, as it does for any new user
([CREATE
USER](https://mariadb.com/docs/server/reference/sql-statements/account-management-sql-statements/create-user));
today's defaults of 20 queries and 10 updates an hour, 5 connections an hour and 2 at once are too
low for any production application.

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBUser
metadata:
  name: orders-app
  namespace: shop
spec:
  instanceRef:
    name: orders-db
  databaseRef:
    name: orders
  privileges:
  - SELECT
  - INSERT
  table: "*"
  maxUserConnections: 20                # 0, the default, is no limit
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBDatabase
metadata:
  name: orders
  namespace: shop
spec:
  instanceRef:
    name: orders-db
  defaultCharacterSet: utf8mb4
  deletionProtection: true
```

## Cloud Limits

Limits each cloud enforces itself, which the API does not check (rule 10); a value outside them is
rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference, should
validating them be decided later.

| Field                            | Cloud | Limit                     | Source                                                                     |
| -------------------------------- | ----- | ------------------------- | -------------------------------------------------------------------------- |
| `backup.retentionDays`           | AWS   | 0 to 35                   | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0     |
| `storage.iops`, `throughputMBps` | AWS   | only from 400 GiB of disk | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0 |
