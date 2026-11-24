# MariaDB

MariaDB is an open-source relational database, a fork of MySQL, that applications use for their
transactional data.

Three kinds for users: `MariaDBServer`, one database server with its standby; `MariaDBUser`, one
application's login on a server, with its permissions; and `MariaDBDatabase`, one database on a
server. Two kinds shape them: `MariaDBClass`, a deployment's defaults and limits, written by Entigo,
and `MariaDBParameters`, reusable MariaDB settings.

Built on Amazon RDS (AWS), with the provider configuration kind `MariaDBRDSConfig`. GCP, OCI and
Azure have no backend: their MySQL services are not MariaDB (principle 7), and Azure Database for
MariaDB is retired.

## Kinds

MariaDB is offered as five kinds, each with the manifest its owner writes.

### MariaDBClass

Entigo writes each deployment's classes, with the defaults a server starts with, in `template`, and
the values it cannot change, in `enforced` (principle 5):

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
    deletionProtection: true
    version: "11.4"
    storage:
      sizeGB: 50
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: MariaDBRDSConfig
      spec:
        instanceType: db.t4g.medium
        maxSizeGB: 500
  enforced:                            # teams cannot change these
    backup:
      retentionDays: 7
    providerConfig:
      apiVersion: aws.database.entigo.com/v1alpha1
      kind: MariaDBRDSConfig
      spec:
        maintenanceWindow:
          day: Sun
          hourUTC: 3
```

*`MariaDBClass` for RDS.*

RDS supports `HighAvailability`.

### MariaDBParameters

`MariaDBParameters` holds MariaDB settings that several servers or a class share. A class and a
server each list the ones they use in `parametersRefs`. The settings merge key by key, a later value
winning: the class's list in order, then the server's, then the server's own `parameters`, so a team
can put settings all its servers share beneath one server's own tuning:

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
  className: standard                  # unset, the default class; without one, standalone
  version: "11.4"                      # the major version
  storage:
    sizeGB: 20
  highAvailability: false              # a standby in another zone
  deletionProtection: true
  backup:
    retentionDays: 7                   # backups kept; 0 disables them
  parametersRefs:
  - name: orders-tuning                # MariaDBParameters objects in this namespace, in order
  parameters:                          # this server's own settings, merged last
    max_connections: "200"
  secretName: orders-db-admin          # unset, orders-db-dbadmin (principle 6)
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: MariaDBRDSConfig
    spec:
      instanceType: db.t4g.small       # the RDS instance class
      maxSizeGB: 100                   # the storage autoscaling ceiling
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      # minorVersion: "11.4.10"        # stay on this minor
      # parameterGroupName: orders-mariadb114  # an existing group, instead of parameters
      # fromBackup: orders-final       # an RDS snapshot ID, instead of restore
```

*`MariaDBServer` with common fields and an RDS entry.*

`restore` creates a server from another server's backups, at a point in time. A deleted server is
restored instead with its provider configuration's `fromBackup` and the final snapshot's ID. The
status loses that ID with the server, so it is taken from the AWS console. `restore` and
`fromBackup` are set at creation only and cannot be combined:

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

`version` is the major version, which has two numbers, such as `"11.4"` or `"10.11"`. Raising it
upgrades the server, and RDS applies minor versions in the maintenance window its provider
configuration sets. A major change is a change in the first two numbers.

Settings that backends run differently, such as disk growth, provisioned IOPS, the maintenance
window, the minor version and the backup hour, are in the backend's provider configuration
(principle 1).

`storage.sizeGB` is the size at creation and a floor afterwards: raising it grows the disk, and a
value below the real size is ignored, so a rejected increase can still be corrected. RDS may round
it up and storage autoscaling can take the disk further, so the status reports the actual size, and
the platform never shrinks the disk back to `sizeGB`.

`highAvailability` is a portable feature: an object that requests it where its backend does not
support it is refused when applied (principle 1).

The status shows the common fields, which entry applied, where each effective value came from, and
the RDS values of `MariaDBRDSConfig` (principle 4):

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
    backup.retentionDays: { value: 7, source: enforced }
    deletionProtection: { value: true, source: object }
    instanceType: { value: db.t4g.small, source: object/MariaDBRDSConfig }
  provider:                            # the shape MariaDBRDSConfig's status defines
    finalBackup: ...
    identifier: ...
    resourceId: ...
    engineVersionActual: ...
    latestRestorableTime: ...
    state: available
```

*`MariaDBServer` status on AWS.*

The connection secret `<instance>-dbadmin` holds the admin login, with the keys of principle 6.

### MariaDBUser

`MariaDBUser` acts inside MariaDB, through provider-sql, so it is the same in every backend. Its
`name` and `serverRef` are immutable, `serverRef` may name another namespace of the same Zone
(principle 11), and it has no class or provider configurations.

A user's databases and privileges are a list, `databases`, with one entry per database, so an
application that uses two databases on one server needs one login, as principle 12 requires. Each
entry becomes one provider-sql `Grant`, and each `databaseRef` takes `namespace` too (principle 11).
provider-sql v0.16.1 writes the four secret keys principle 6 names for MariaDB users too. The four
limits on a user (`maxQueriesPerHour`, `maxUpdatesPerHour`, `maxConnectionsPerHour`,
`maxUserConnections`) default to 0, which MariaDB treats as no limit, as it does for any new user.

Sources:
[`pkg/clients/mysql/mysql.go`](https://github.com/crossplane-contrib/provider-sql/blob/v0.16.1/pkg/clients/mysql/mysql.go),
[provider-sql v0.16.1](https://github.com/crossplane-contrib/provider-sql/releases/tag/v0.16.1),
[CREATE USER](https://mariadb.com/docs/server/reference/sql-statements/account-management-sql-statements/create-user).

A user creates a `MariaDBUser` with a manifest like the one below:

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

`MariaDBDatabase` acts inside MariaDB, through provider-sql, so it is the same in every backend. Its
`name` and `serverRef` are immutable, `serverRef` may name another namespace of the same Zone
(principle 11), and it has no class or provider configurations.

A user creates a `MariaDBDatabase` with a manifest like the one below:

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

## Fields

The first table maps each common field to RDS. A setting RDS lacks shows as ignored, with the reason
(principle 1):

| Field                  | AWS                                                                                                                       |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| `observe`              | `managementPolicies: [Observe]`                                                                                           |
| `version`              | `engineVersion`; with `minorVersion`, that full version; raising the major sets `allowMajorVersionUpgrade` for the change |
| `storage.sizeGB`       | `allocatedStorage`; below 20 rounded up to 20                                                                             |
| `highAvailability`     | `multiAz`                                                                                                                 |
| `deletionProtection`   | `deletionProtection`                                                                                                      |
| `backup.retentionDays` | `backupRetentionPeriod`; 0 disables backups                                                                               |
| `parameters`           | a `ParameterGroup` of family `mariadb<major>`                                                                             |
| `parametersRefs`       | merged into `parameters`                                                                                                  |
| `restore.fromInstance` | `restoreToPointInTime.sourceDbInstanceIdentifier`                                                                         |
| `restore.pointInTime`  | `restoreToPointInTime.restoreTime`; unset, `useLatestRestorableTime`                                                      |

*`MariaDBServer` common fields and their provider fields.*

The second names the provider configuration kind and what its fields map to:

| Field                  | AWS                                                                         |
| ---------------------- | --------------------------------------------------------------------------- |
| kind                   | `MariaDBRDSConfig`                                                          |
| `id`                   | the console's Resource ID (`db-...`)                                        |
| `instanceType`         | `instanceClass`                                                             |
| `fromBackup`           | `snapshotIdentifier`                                                        |
| `parameterGroupName`   | `parameterGroupName`                                                        |
| `minorVersion`         | `engineVersion` with that full version, `autoMinorVersionUpgrade: false`    |
| `maxSizeGB`            | `maxAllocatedStorage`                                                       |
| `iops`                 | `iops`                                                                      |
| `throughputMBps`       | `storageThroughput`                                                         |
| `maintenanceWindow`    | `maintenanceWindow: sun:03:00-sun:03:30`                                    |
| `backupStartHourUTC`   | `backupWindow`, 30 minutes                                                  |
| `parameterApplyMethod` | `ParameterGroup` `parameter[].applyMethod`: `immediate` or `pending-reboot` |

*`MariaDBRDSConfig` fields.*

**AWS:** provider keys are from the CRDs of
[provider-upjet-aws v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0).
Only `minorVersion` turns minor upgrades off: it sets the full version in `engineVersion`, such as
`"11.4.10"`, and the user changes it to upgrade. A parameter group applies to one major version, so
a major change is rejected unless `parameterGroupName` changes or is removed in the same edit.
`storage.sizeGB` is rounded up to at least 20 GiB, and RDS rounds an increase under 10% up to 10%.
`status.provider.finalBackup` is the ID the final snapshot will receive. Storage is gp3.

## Provider Configurations

Each backend's provider configuration kind holds the settings of that backend, with every field it
takes; commented fields are optional, alternatives, or apply only with `observe`. The settings sit
either in a separate object, named by an entry's `ref`, or inline in an entry's `spec`, as each pair
shows, and they override the class's settings for the same kind field by field (principle 2):

```yaml Separate object
---
apiVersion: aws.database.entigo.com/v1alpha1
kind: MariaDBRDSConfig
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
  # minorVersion: "11.4.10"        # stay on this minor; unset, minor upgrades apply
  # iops: 12000                    # from 400 GiB of disk
  # throughputMBps: 500            # from 400 GiB of disk
  # parameterApplyMethod: NextRestart  # unset, Immediate
  # parameterGroupName: orders-mariadb114  # an existing group, instead of parameters
  # fromBackup: orders-final       # an RDS snapshot ID, at creation only, instead of restore
  # id: orders-db                  # an existing instance's ID, with observe
```

```yaml AWS entry
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBServer
metadata:
  name: orders-db
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: MariaDBRDSConfig
    spec:
      instanceType: db.t4g.small       # the RDS instance class
      maxSizeGB: 100                   # the storage autoscaling ceiling
      maintenanceWindow:
        day: Sun
        hourUTC: 3
      backupStartHourUTC: 2            # unset, RDS picks the hour
      # minorVersion: "11.4.10"        # stay on this minor; unset, minor upgrades apply
      # iops: 12000                    # from 400 GiB of disk
      # throughputMBps: 500            # from 400 GiB of disk
      # parameterApplyMethod: NextRestart  # unset, Immediate
      # parameterGroupName: orders-mariadb114  # an existing group, instead of parameters
      # fromBackup: orders-final       # an RDS snapshot ID, at creation only, instead of restore
      # id: orders-db                  # an existing instance's ID, with observe
```

*`MariaDBRDSConfig`: a separate object, and the same settings in an entry.*

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
    kind: MariaDBRDSConfig
    spec:
      instanceType: db.t4g.small
      maxSizeGB: 100
      maintenanceWindow:
        day: Sun
        hourUTC: 3
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
    engine: mariadb
    engineVersion: "11.4"
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
    parameterGroupName: orders-db-7f3a2c  # composed from parametersRefs and parameters
    username: dbadmin          # platform
    manageMasterUserPassword: true  # platform; the admin Secret is read from the RDS-managed secret
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
([Platform Decisions](common-design.md#platform-decisions)), the platform makes these for a
`MariaDBServer` itself:

| Setting      | Platform's Choice                                                                                                                                                |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Admin login  | An admin username and a generated password; on RDS, the password RDS manages, read into the admin Secret, so taking ownership of an instance changes no password |
| Port         | 3306                                                                                                                                                             |
| Final backup | A snapshot taken when a server is deleted, switched on per deployment                                                                                            |

*`MariaDBServer` settings chosen by the platform.*

## Limits

The backend enforces these limits itself, and the API does not check them (principle 10); a value
outside them is rejected by the backend and shown as `CloudAccepted: False`. They are listed for
reference, should validating them be decided later. Each limit holds as of its source: the provider
release named, or the cloud's documentation as read in November 2026.

| Field                    | Backend | Limit                     | Source                                                                     |
| ------------------------ | ------- | ------------------------- | -------------------------------------------------------------------------- |
| `backup.retentionDays`   | AWS     | 0 to 35                   | `backupRetentionPeriod`, RDS `Instance` CRD, provider-upjet-aws v2.4.0     |
| `iops`, `throughputMBps` | AWS     | only from 400 GiB of disk | `iops`, `storageThroughput`, RDS `Instance` CRD, provider-upjet-aws v2.4.0 |

*MariaDB field limits and their sources.*
