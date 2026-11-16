# MariaDB

MariaDB is an open-source relational database that began as a fork of MySQL, used by applications
for their transactional data.

Three kinds: `MariaDBInstance`, one database server; `MariaDBUser`, one application's login on an
instance, with its permissions; and `MariaDBDatabase`, one database on an instance.

Built on Amazon RDS (AWS), so `aws` is the only cloud block. GCP, OCI and Azure have none: their
MySQL services are not MariaDB (rule 7), and Azure Database for MariaDB is retired.

[API Changes](changes.md#mariadb) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)

## Kinds

MariaDB is offered as three kinds, each shown below with a manifest a user writes.

### MariaDBInstance

A user creates a `MariaDBInstance` with a manifest like the one below. Its comments note value
ranges and what holds on one cloud only:

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
  secretName: orders-db-admin          # unset, orders-db-dbadmin (rule 6)
  aws:                                 # also takes id (rule 9)
    instanceType: db.t4g.small         # the RDS instance class
    # parameterGroupName: orders-mariadb114  # an existing group, instead of parameters
    # fromBackup: orders-final         # an RDS snapshot ID, instead of restore
```

*A `MariaDBInstance` with its common fields and its `aws` block.*

`restore` creates an instance from another instance's backups, at a point in time. A deleted
instance is restored instead with `aws.fromBackup` and an RDS snapshot ID. Both are set at creation
only and cannot be combined:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBInstance
metadata:
  name: orders-db-restored
  namespace: shop
spec:
  version: "11.4"
  restore:
    fromInstance: orders-db            # a MariaDBInstance in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
  aws:
    instanceType: db.t4g.small
```

*A `MariaDBInstance` created from another instance's backups.*

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

`aws.parameterGroupName` uses an existing RDS parameter group in place of `parameters`.

In the status, `aws.finalBackup` is the ID the final snapshot will receive. A deleted instance is
restored with `aws.fromBackup` and that ID, which the status loses with the instance, so it is taken
from the AWS console. The running minor version, the latest restorable time and the instance state
go in the `aws` block:

```yaml
status:
  aws:
    finalBackup: ...
    identifier: ...
    resourceId: ...
    engineVersionActual: "11.4.10"
    latestRestorableTime: ...
    status: available
```

*The `aws` block of a `MariaDBInstance` status.*

The admin login is in the secret `<instance>-dbadmin`, with the keys of rule 6.

### MariaDBUser

`MariaDBUser` and `MariaDBDatabase` act inside MariaDB, through provider-sql. Their `name` and
`instanceRef` are immutable, `instanceRef` may name another namespace of the same Zone (rule 11),
and their defaults are written by the admission policy (rule 5).

A user's databases and privileges are a list, `databases`, with one entry per database, so an
application that uses two databases on one instance needs one login, as rule 12 requires. Each entry
becomes one provider-sql `Grant`, and each `databaseRef` takes `namespace` too (rule 11).
provider-sql writes the four secret keys rule 6 names for MariaDB users too
([`pkg/clients/mysql/mysql.go`](https://github.com/crossplane-contrib/provider-sql/blob/v0.16.1/pkg/clients/mysql/mysql.go),
[provider-sql v0.16.1](https://github.com/crossplane-contrib/provider-sql/releases/tag/v0.16.1)).
The four limits on a user (`maxQueriesPerHour`, `maxUpdatesPerHour`, `maxConnectionsPerHour`,
`maxUserConnections`) default to 0, which MariaDB treats as no limit, as it does for any new user
([CREATE
USER](https://mariadb.com/docs/server/reference/sql-statements/account-management-sql-statements/create-user)).

An example:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBUser
metadata:
  name: orders-app
  namespace: shop
spec:
  name: orders_app                     # the user's name in MariaDB; unset, metadata.name
  instanceRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (rule 11)
  databases:                           # one entry per database
  - databaseRef:
      name: orders
    privileges:
    - SELECT
    - INSERT
    table: "*"                         # the default, every table
  - databaseRef:
      name: invoices
      namespace: billing
    privileges:
    - SELECT
  maxUserConnections: 20               # 0, the default, sets no limit of the user's own
  maxConnectionsPerHour: 0             # 0, the default, is no limit
  maxQueriesPerHour: 0
  maxUpdatesPerHour: 0
  secretName: orders-app-login         # unset, orders-db-orders-app (rule 6)
```

*A `MariaDBUser` with privileges on two databases.*

### MariaDBDatabase

An example:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBDatabase
metadata:
  name: orders
  namespace: shop
spec:
  name: orders                         # the database's name in MariaDB; unset, metadata.name
  instanceRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (rule 11)
  defaultCharacterSet: utf8mb4
  defaultCollation: utf8mb4_unicode_ci
  deletionProtection: true
```

*A `MariaDBDatabase` with its default character set.*

## Mapping

The table names, for every field, the field of each cloud's provider it maps to.

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
| `aws.fromBackup`                       | `snapshotIdentifier`                                                        |
| `aws.parameterGroupName`               | `parameterGroupName`                                                        |

*Fields of `MariaDBInstance` on Amazon RDS.*

Provider keys are from the CRDs of [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0).

## Platform Decisions

The platform makes these settings itself:

| Setting           | Platform's Choice                                                                 |
| ----------------- | --------------------------------------------------------------------------------- |
| Network placement | The deployment's subnets and a security group                                     |
| Public access     | None                                                                              |
| Encryption        | The platform's data key                                                           |
| Admin login       | An admin username and a generated password                                        |
| Storage           | gp3                                                                               |
| Port              | 3306                                                                              |
| Tags              | Workspace and Zone tags                                                           |
| Final backup      | A snapshot taken when an instance is deleted, switched on per deployment |

*Settings the platform makes for a `MariaDBInstance`.*

## Cloud Limits

Each cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later.

| Field                            | Cloud | Limit                     | Source                                                                     |
| -------------------------------- | ----- | ------------------------- | -------------------------------------------------------------------------- |
| `backup.retentionDays`           | AWS   | 0 to 35                   | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0     |
| `storage.iops`, `throughputMBps` | AWS   | only from 400 GiB of disk | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0 |

*Limits AWS enforces on MariaDB.*
