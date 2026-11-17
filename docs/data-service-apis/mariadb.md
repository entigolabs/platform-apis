# MariaDB

MariaDB is an open-source relational database that began as a fork of MySQL, used by applications
for their transactional data.

Three kinds for users: `MariaDBServer`, one database server with its standby;
`MariaDBUser`, one application's login on a server, with its permissions; and `MariaDBDatabase`, one
database on a server. Two kinds shape them: `MariaDBClass`, a deployment's defaults and limits,
written by Entigo, and `MariaDBParameters`, reusable MariaDB settings.

Built on Amazon RDS (AWS), with the provider configuration kind `MariaDBRDSConfig`. GCP, OCI and
Azure have no implementation: their MySQL services are not MariaDB (principle 7), and Azure Database
for MariaDB is retired.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)

## Kinds

MariaDB is offered as five kinds, each with the manifest its owner writes.

### MariaDBServer

A user creates a `MariaDBServer` with a manifest like the one below. It names its class and carries
a provider configuration entry for RDS (principle 2):

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBServer
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard                  # unset, the deployment's default class (principle 5)
  version: "11.4"                      # the full "11.4.10" with minor upgrades off
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100                     # autoGrow ceiling
    iops: 12000
    throughputMBps: 500
  highAvailability: false              # a standby in another zone
  deletionProtection: true
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  autoMinorVersionUpgrade: true
  allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7                   # backups kept; 0 disables them
    startHourUTC: 2
  parametersRef:
    name: orders-tuning                # a MariaDBParameters in this namespace
  parameters:                          # this server's own settings, merged last
    max_connections: "200"
  parameterApplyMethod: Immediate      # or NextRestart
  secretName: orders-db-admin          # unset, orders-db-dbadmin (principle 6)
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: MariaDBRDSConfig
    spec:
      instanceType: db.t4g.small       # the RDS instance class
      # parameterGroupName: orders-mariadb114  # an existing group, instead of parameters
      # fromBackup: orders-final       # an RDS snapshot ID, instead of restore
```

*`MariaDBServer` with common fields and an RDS entry.*

`restore` creates a server from another server's backups, at a point in time. A deleted server is
restored instead with its provider configuration's `fromBackup` and an RDS snapshot ID. Both are set
at creation only and cannot be combined:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBServer
metadata:
  name: orders-db-restored
  namespace: shop
spec:
  version: "11.4"
  restore:
    fromInstance: orders-db            # a MariaDBServer in this namespace
    pointInTime: "2026-11-01T10:00:00Z"
```

*`MariaDBServer` restored from another server's backups.*

`version` is the major version, which has two numbers, such as `"11.4"` or `"10.11"`, and RDS
applies minor versions in the maintenance window. A major change is a change in the first two
numbers.

`storage.sizeGB` is the size at creation and a floor afterwards: raising it grows the disk, and
lowering it is rejected. RDS may round it up and `autoGrow` can take the disk further, so the status
reports the actual size, and the platform never shrinks the disk back to `sizeGB`.

`highAvailability` is a portable feature: an object that requests it where its class does not
support it is refused when applied (principle 1).

The final snapshot's ID, which `fromBackup` needs, is lost with the server's status, so it is taken
from the AWS console. The status shows the common fields, which entry applied, where each effective
value came from, and the RDS values of `MariaDBRDSConfig` (principle 4):

```yaml
status:
  version: "11.4"
  endpoint:
    host: orders-db-7f3a2c.abc.eu-north-1.rds.amazonaws.com
    port: 3306
  connectionSecret: orders-db-admin
  providerConfig:
    applied: aws.database.entigo.com/v1alpha1, MariaDBRDSConfig
  effective:
    backup.retentionDays: { value: 7, source: object }
    deletionProtection: { value: true, source: class/standard/enforced }
    instanceType: { value: db.t4g.small, source: object/MariaDBRDSConfig }
  provider:                            # the shape MariaDBRDSConfig's status defines
    finalBackup: ...
    identifier: ...
    resourceId: ...
    latestRestorableTime: ...
    state: available
```

*`MariaDBServer` status on AWS.*

The admin login is in the secret `<instance>-dbadmin`, with the keys of principle 6.

### MariaDBClass

Entigo writes each deployment's classes, with the defaults a server starts with (`template`) and the
values it cannot change (`enforced`) (principle 5):

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBClass
metadata:
  name: standard
  annotations:
    database.entigo.com/is-default-class: "true"
spec:
  template:
    version: "11.4"
    storage:
      sizeGB: 50
      maxSizeGB: 500
    backup:
      retentionDays: 14
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: MariaDBRDSConfig
      spec:
        instanceType: db.t4g.medium
  enforced:
    deletionProtection: true
```

*`MariaDBClass` for RDS.*

On RDS, a class reports `HighAvailability` in `status.supportedFeatures`.

### MariaDBParameters

`MariaDBParameters` holds MariaDB settings that several servers or a class share. A class, a server
and a server's provider configuration may each reference one, and their settings merge key by key in
that order, followed by the server's own `parameters`:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBParameters
metadata:
  name: orders-tuning
  namespace: shop
spec:
  parameters:
    innodb_lock_wait_timeout: "120"
    long_query_time: "2"
```

*`MariaDBParameters` shared between a user's servers.*

A `MariaDBParameters` names no other, so references never chain. Settings RDS does not allow are
rejected by the cloud and reported as `CloudAccepted: False` (principle 10).

### MariaDBUser

`MariaDBUser` acts inside MariaDB, through provider-sql, so it is the same in every implementation.
Its `name` and `serverRef` are immutable, `serverRef` may name another namespace of the same Zone
(principle 11), and it has no class or provider configurations.

A user's databases and privileges are a list, `databases`, with one entry per database, so an
application that uses two databases on one server needs one login, as principle 12 requires. Each
entry becomes one provider-sql `Grant`, and each `databaseRef` takes `namespace` too (principle 11).
provider-sql writes the four secret keys principle 6 names for MariaDB users too
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
  serverRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (principle 11)
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
  secretName: orders-app-login         # unset, orders-db-orders-app (principle 6)
```

*`MariaDBUser` with privileges on two databases.*

### MariaDBDatabase

`MariaDBDatabase` acts inside MariaDB, through provider-sql, so it is the same in every
implementation. Its `name` and `serverRef` are immutable, `serverRef` may name another namespace of
the same Zone (principle 11), and it has no class or provider configurations.

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
  serverRef:
    name: orders-db
    namespace: shop                    # unset, this namespace; within the Zone (principle 11)
  defaultCharacterSet: utf8mb4
  defaultCollation: utf8mb4_unicode_ci
  deletionProtection: true
```

*`MariaDBDatabase` with its default character set.*

## Mapping

The first table names what each common field maps to in RDS. A setting RDS lacks shows as ignored,
with the reason (principle 1):

| Field                      | AWS                                                                         |
| -------------------------- | --------------------------------------------------------------------------- |
| `observe`                  | `managementPolicies: [Observe]`                                             |
| `version`                  | `engineVersion`; the full version with minor upgrades off                   |
| `storage.sizeGB`           | `allocatedStorage`; below 20 rounded up to 20                               |
| `storage.autoGrow`         | on with `maxAllocatedStorage`                                               |
| `storage.maxSizeGB`        | `maxAllocatedStorage`; a deployment default, since RDS needs a ceiling      |
| `storage.iops`             | `iops`                                                                      |
| `storage.throughputMBps`   | `storageThroughput`                                                         |
| `highAvailability`         | `multiAz`                                                                   |
| `deletionProtection`       | `deletionProtection`                                                        |
| `maintenanceWindow`        | `maintenanceWindow: sun:03:00-sun:03:30`                                    |
| `autoMinorVersionUpgrade`  | `autoMinorVersionUpgrade`                                                   |
| `allowMajorVersionUpgrade` | `allowMajorVersionUpgrade`                                                  |
| `backup.retentionDays`     | `backupRetentionPeriod`; 0 disables backups                                 |
| `backup.pointInTimeDays`   | the same setting; above `retentionDays` rejected                            |
| `backup.startHourUTC`      | `backupWindow`, 30 minutes                                                  |
| `parameters`               | a `ParameterGroup` of family `mariadb<major>`                               |
| `parametersRef`            | merged into `parameters`                                                    |
| `parameterApplyMethod`     | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` |
| `restore.fromInstance`     | `restoreToPointInTime.sourceDbInstanceIdentifier`                           |
| `restore.pointInTime`      | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`        |

*`MariaDBServer` common fields and their provider fields.*

The second names the provider configuration kind and what its fields map to:

| Field                | AWS                                  |
| -------------------- | ------------------------------------ |
| kind                 | `MariaDBRDSConfig`                   |
| `id`                 | the console's Resource ID (`db-...`) |
| `instanceType`       | `instanceClass`                      |
| `fromBackup`         | `snapshotIdentifier`                 |
| `parameterGroupName` | `parameterGroupName`                 |

*`MariaDBRDSConfig` fields.*

**AWS:** provider keys are from the CRDs of [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0). RDS takes a
version without its minor only while minor upgrades are on (`engineVersion` in
`rds.aws.m.upbound.io_instances`), so a user who sets `autoMinorVersionUpgrade: false` writes the
full version, such as `"11.4.10"`, and changes it to upgrade. A parameter group applies to one major
version, so a major change is rejected unless `parameterGroupName` changes or is removed in the same
edit. `storage.sizeGB` is rounded up to at least 20 GiB, and RDS rounds an increase under 10% up to
10%. `status.provider.finalBackup` is the ID the final snapshot will receive. Storage is gp3.

## Composed Resources

The `MariaDBServer` from [Kinds](#kinds) is composed into the resource below on AWS, the only cloud
that offers it. A comment names the source of each value the object does not hold as written:
`platform` for a value the platform sets, and `deployment` for one the deployment sets. The object
and its resource:

```yaml Object on AWS
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBServer
metadata:
  name: orders-db
  namespace: shop
spec:
  className: standard
  version: "11.4"
  storage:
    sizeGB: 20
    autoGrow: true
    maxSizeGB: 100
    iops: 12000
    throughputMBps: 500
  highAvailability: false
  deletionProtection: true
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  autoMinorVersionUpgrade: true
  allowMajorVersionUpgrade: false
  backup:
    retentionDays: 7
    startHourUTC: 2
  parametersRef:
    name: orders-tuning
  parameters:
    max_connections: "200"
  parameterApplyMethod: Immediate
  secretName: orders-db-admin
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: MariaDBRDSConfig
    spec:
      instanceType: db.t4g.small
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
    engine: mariadb
    engineVersion: "11.4"
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
    parameterGroupName: orders-db-7f3a2c  # composed from parametersRef and parameters
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

*`MariaDBServer` on AWS and its composed resource.*

## Platform Decisions

Besides the settings it makes for every service
([Platform Decisions](common-design.md#platform-decisions)), the platform makes these
for a `MariaDBServer` itself:

| Setting      | Platform's Choice                                                     |
| ------------ | --------------------------------------------------------------------- |
| Admin login  | An admin username and a generated password                            |
| Port         | 3306                                                                  |
| Final backup | A snapshot taken when a server is deleted, switched on per deployment |

*`MariaDBServer` settings chosen by the platform.*

## Cloud Limits

The cloud enforces these limits itself, and the API does not check them (principle 10); a value
outside them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for
reference, should validating them be decided later. Each limit holds as of its source: the provider
release named, or the cloud's documentation as read in November 2026.

| Field                            | Cloud | Limit                     | Source                                                                     |
| -------------------------------- | ----- | ------------------------- | -------------------------------------------------------------------------- |
| `backup.retentionDays`           | AWS   | 0 to 35                   | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0     |
| `storage.iops`, `throughputMBps` | AWS   | only from 400 GiB of disk | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0 |

*MariaDB field limits and their sources.*
