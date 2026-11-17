# API Changes

Every change from the data service APIs on `main` of
[`platform-apis`](https://github.com/entigolabs/platform-apis) to this design, per kind. A field
not listed keeps its name and meaning.

## Contents

- [All Kinds](#all-kinds)
- [PostgreSQL](#postgresql)
- [MariaDB](#mariadb)
- [Valkey](#valkey)
- [Kafka](#kafka)
- [RabbitMQ](#rabbitmq)
- [Bucket](#bucket)
- [ArtifactRepository](#artifactrepository)

## All Kinds

Changes shared by all kinds, stated once here:

| Today                                                                              | Design                                                   | Reason                                                                            |
| ---------------------------------------------------------------------------------- | -------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `v1alpha1`                                                                         | `v1alpha2`                                               | Fields move, so the version changes                                               |
| XRD schema defaults, and deployment values the function applies on every reconcile | defaults written into the object once, at creation       | A changed default never changes a running resource (principle 5)                  |
| (none)                                                                             | `className`, naming a class of the kind                  | A deployment's defaults and limits come from a class (principle 5)                |
| (none)                                                                             | `providerConfigs`, one entry per implementation          | Settings of one implementation belong in its provider configuration (principle 2) |
| (none)                                                                             | `observe`, and `id` in a provider configuration entry    | Observing an existing resource (principle 9)                                      |
| (none)                                                                             | `secretName`                                             | Overrides the secret's name (principle 6)                                         |
| a reference without `namespace`                                                    | `namespace`, valid within the Zone                       | References cross namespaces within the Zone (principle 11)                        |
| a cloud's rejection only on the composed resource                                  | `CloudAccepted: False` and a warning event on the object | The refusal's cause shows on the object (principle 10)                            |
| AWS-only status fields, such as security groups, KMS keys and hosted zone IDs      | removed; the rest in `status.provider`                   | The status reports the resource's actual values (principle 4)                     |
| (none)                                                                             | `status.providerConfig`, `status.effective`              | The applied entry and each value's source (principle 4)                           |

*Changes shared by all kinds.*

## PostgreSQL

### PostgreSQLCluster

`PostgreSQLInstance` becomes `PostgreSQLCluster`:

| Today                                                                                                                   | Design                                                               | Reason                                                                                           |
| ----------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| kind `PostgreSQLInstance`                                                                                               | kind `PostgreSQLCluster`                                             | PostgreSQL's own term; a new kind, served beside today's, so objects move one at a time (principle 8) |
| `engineVersion`                                                                                                         | `version`                                                            | The major version; a full version only with minor upgrades off                                   |
| `instanceType`                                                                                                          | `instanceType` in the `PostgreSQLRDSConfig` entry                    | A size named by its product is a setting of one implementation (principle 2)                          |
| `allocatedStorage`                                                                                                      | `storage.sizeGB`                                                     | The size at creation and a floor afterwards                                                      |
| `iops`                                                                                                                  | `storage.iops`                                                       | Grouped with the other storage settings (principle 13)                                                |
| (none)                                                                                                                  | `storage.autoGrow`, `storage.maxSizeGB`, `storage.throughputMBps`    | New common settings                                                                              |
| `multiAZ`                                                                                                               | `highAvailability`                                                   | Named after the need, not the AWS feature                                                        |
| `maintenanceWindow` (RDS string)                                                                                        | `maintenanceWindow` (`day`, `hourUTC`)                               | One form everywhere                                                                              |
| `backupRetentionPeriod`                                                                                                 | `backup.retentionDays`                                               | Grouped under backup (principle 13)                                                                   |
| `backupWindow` (RDS string)                                                                                             | `backup.startHourUTC`                                                | One form everywhere                                                                              |
| (none)                                                                                                                  | `backup.pointInTimeDays`                                             | New common setting                                                                               |
| `parameterGroupName`                                                                                                    | `parameterGroupName` in the `PostgreSQLRDSConfig` entry              | An RDS concept (principle 2)                                                                          |
| `parameterGroupParameters` (a map with a reserved `applyMethod` key)                                                    | `parameters` and `parameterApplyMethod`                              | The apply method becomes its own field                                                           |
| (none)                                                                                                                  | `parametersRef`                                                      | Settings shared through a `PostgreSQLParameters`                                                 |
| `snapshotIdentifier`                                                                                                    | `fromBackup` in the `PostgreSQLRDSConfig` entry                      | A backup ID belongs to one implementation (principle 2)                                               |
| (none)                                                                                                                  | `restore.fromInstance`, `restore.pointInTime`                        | Restore from another cluster                                                                     |
| (none)                                                                                                                  | `className`                                                          | The deployment's defaults and limits come from a class (principle 5)                                  |
| (none)                                                                                                                  | `providerConfigs` entries of other kinds                             | Sizes and settings of other implementations (principle 2)                                             |
| (none)                                                                                                                  | `readReplicas`, `disasterRecovery`                                   | New portable features (principle 1)                                                                   |
| `status.endpoint.address`                                                                                               | `status.endpoint.host`                                               | The same word everywhere                                                                         |
| `status.dbInstanceIdentifier`, `status.resourceId`, `status.status`, `status.latestRestorableTime`                      | `status.provider`                                                    | Values of one implementation (principle 4)                                                            |
| `status.finalSnapshotIdentifier`                                                                                        | `status.provider.finalBackup`                                        | A value of one implementation (principle 4)                                                           |
| (none)                                                                                                                  | `status.providerConfig`, `status.effective`, `status.readerEndpoint` | The applied entry, each value's source, and where reads go (principles 4 and 6)                       |
| `status.hostedZoneId`, `status.vpcSecurityGroupIds`, `status.kmsKeyId`, `status.storageEncrypted`, `status.storageType` | removed                                                              | Settings the platform makes (principle 4)                                                             |
| `status.parameterGroupName`, `status.snapshotIdentifier`                                                                | removed                                                              | Echoes of the spec                                                                               |

*Changes from `PostgreSQLInstance` to `PostgreSQLCluster`.*

An example, today and in the design:

```yaml
---
apiVersion: database.entigo.com/v1alpha1
kind: PostgreSQLInstance
metadata:
  name: orders-db
spec:
  engineVersion: "17"
  instanceType: db.t4g.small
  allocatedStorage: 20
  multiAZ: false
  deletionProtection: true
  maintenanceWindow: "Sun:03:00-Sun:03:30"
  backupRetentionPeriod: 7
  backupWindow: "02:00-02:30"
  parameterGroupParameters:
    max_connections: "200"
    applyMethod: pending-reboot
```

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
spec:
  version: "17"                        # was engineVersion
  storage:
    sizeGB: 20                         # was allocatedStorage
  highAvailability: false              # was multiAZ
  deletionProtection: true
  maintenanceWindow:                   # now day and hourUTC
    day: Sun
    hourUTC: 3
  backup:
    retentionDays: 7                   # was backupRetentionPeriod
    startHourUTC: 2                    # was backupWindow
  parameters:                          # was parameterGroupParameters
    max_connections: "200"
  parameterApplyMethod: NextRestart    # was applyMethod: pending-reboot
  providerConfigs:                     # new
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      instanceType: db.t4g.small       # was instanceType
```

*`PostgreSQLInstance` today, `PostgreSQLCluster` in the design.*

### PostgreSQLUser

`PostgreSQLUser` keeps its fields; `instanceRef` becomes `clusterRef`, naming a `PostgreSQLCluster`,
and `name` and `clusterRef` become immutable.

### PostgreSQLDatabase

`PostgreSQLDatabase` keeps its fields; `instanceRef` becomes `clusterRef`, naming a
`PostgreSQLCluster`, and `name` and `clusterRef` become immutable.

## MariaDB

### MariaDBServer

`MariaDBInstance` becomes `MariaDBServer`:

| Today                                                                                                                            | Design                                                               | Reason                                                                                                            |
| -------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| kind `MariaDBInstance`                                                                                                           | kind `MariaDBServer`                                                 | MariaDB's own term for one server; a new kind, served beside today's, so objects move one at a time (principle 8) |
| `engineVersion`                                                                                                                  | `version`                                                            | The major version, such as `"11.4"`; a full version only with minor upgrades off                                  |
| `instanceType`                                                                                                                   | `instanceType` in the `MariaDBRDSConfig` entry                       | A size named by its product is a setting of one implementation (principle 2)                                      |
| `allocatedStorage`                                                                                                               | `storage.sizeGB`                                                     | The size at creation and a floor afterwards                                                                       |
| `iops`                                                                                                                           | `storage.iops`                                                       | Grouped with the other storage settings (principle 13)                                                            |
| (none)                                                                                                                           | `storage.autoGrow`, `storage.maxSizeGB`, `storage.throughputMBps`    | New common settings                                                                                               |
| `multiAZ`                                                                                                                        | `highAvailability`                                                   | Named after the need, not the AWS feature                                                                         |
| `maintenanceWindow` (RDS string)                                                                                                 | `maintenanceWindow` (`day`, `hourUTC`)                               | One form everywhere                                                                                               |
| `backupRetentionPeriod`                                                                                                          | `backup.retentionDays`                                               | Grouped under backup (principle 13)                                                                               |
| `backupWindow` (RDS string)                                                                                                      | `backup.startHourUTC`                                                | One form everywhere                                                                                               |
| (none)                                                                                                                           | `backup.pointInTimeDays`                                             | New common setting                                                                                                |
| `parameterGroupName`                                                                                                             | `parameterGroupName` in the `MariaDBRDSConfig` entry                 | An RDS concept (principle 2)                                                                                      |
| `parameterGroupParameters` (a map with a reserved `applyMethod` key)                                                             | `parameters` and `parameterApplyMethod`                              | The apply method becomes its own field                                                                            |
| (none)                                                                                                                           | `parametersRef`                                                      | Settings shared through a `MariaDBParameters`                                                                     |
| `snapshotIdentifier`                                                                                                             | `fromBackup` in the `MariaDBRDSConfig` entry                         | A backup ID belongs to one implementation (principle 2)                                                           |
| (none)                                                                                                                           | `restore.fromInstance`, `restore.pointInTime`                        | Restore from another server                                                                                       |
| (none)                                                                                                                           | `className`                                                          | The deployment's defaults and limits come from a class (principle 5)                                              |
| (none)                                                                                                                           | `readReplicas`, `disasterRecovery`                                   | New portable features (principle 1)                                                                               |
| `status.endpoint.address`                                                                                                        | `status.endpoint.host`                                               | The same word everywhere                                                                                          |
| `status.dbInstanceIdentifier`, `status.resourceId`, `status.status`, `status.latestRestorableTime`                               | `status.provider`                                                    | Values of one implementation (principle 4)                                                                        |
| `status.finalSnapshotIdentifier`                                                                                                 | `status.provider.finalBackup`                                        | A value of one implementation (principle 4)                                                                       |
| (none)                                                                                                                           | `status.providerConfig`, `status.effective`, `status.readerEndpoint` | The applied entry, each value's source, and the read address (principles 4 and 6)                                 |
| `status.endpoint.hostedZoneId`, `status.vpcSecurityGroupIds`, `status.kmsKeyId`, `status.storageEncrypted`, `status.storageType` | removed                                                              | Settings the platform makes (principle 4)                                                                         |
| `status.parameterGroupName`, `status.snapshotIdentifier`                                                                         | removed                                                              | Echoes of the spec                                                                                                |

*Changes from `MariaDBInstance` to `MariaDBServer`.*

An example, today and in the design:

```yaml
---
apiVersion: database.entigo.com/v1alpha1
kind: MariaDBInstance
metadata:
  name: orders-db
spec:
  engineVersion: "11.4"
  instanceType: db.t4g.small
  allocatedStorage: 20
  multiAZ: false
  deletionProtection: true
  maintenanceWindow: "Sun:03:00-Sun:03:30"
  backupRetentionPeriod: 7
  backupWindow: "02:00-02:30"
  parameterGroupParameters:
    max_connections: "200"
    applyMethod: pending-reboot
```

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBServer
metadata:
  name: orders-db
spec:
  version: "11.4"                      # was engineVersion
  storage:
    sizeGB: 20                         # was allocatedStorage
  highAvailability: false              # was multiAZ
  deletionProtection: true
  maintenanceWindow:                   # now day and hourUTC
    day: Sun
    hourUTC: 3
  backup:
    retentionDays: 7                   # was backupRetentionPeriod
    startHourUTC: 2                    # was backupWindow
  parameters:                          # was parameterGroupParameters
    max_connections: "200"
  parameterApplyMethod: NextRestart    # was applyMethod: pending-reboot
  providerConfigs:                     # new
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: MariaDBRDSConfig
    spec:
      instanceType: db.t4g.small       # was instanceType
```

*`MariaDBInstance` today, `MariaDBServer` in the design.*

### MariaDBUser

`MariaDBUser` changes as follows:

| Today                                                                                         | Design                                | Reason                                                                                                                                    |
| --------------------------------------------------------------------------------------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- |
| `instanceRef`                                                                                 | `serverRef`, naming a `MariaDBServer` | The parent's new kind (principle 8)                                                                                                       |
| `name` and `instanceRef`, changeable                                                          | `name` and `serverRef`, immutable     | The login is created once, on one server                                                                                                  |
| `databaseRef`, `table`, `privileges`                                                          | one entry of `databases`              | One login per application and instance (principle 12)                                                                                     |
| `databaseRef.namespace`, ignored today                                                        | honoured within the Zone              | References cross namespaces within the Zone (principle 11)                                                                                |
| user limits default to 20 queries and 10 updates an hour, 5 connections an hour and 2 at once | all default to 0, no limit            | Today's defaults are too low for a production application                                                                                 |
| `grant.users`                                                                                 | removed                               | The names never reached MariaDB: each one only created another copy of the user's own grant, and privileges applied only when one was set |

*Changes to `MariaDBUser`.*

An example, today and in the design:

```yaml
---
apiVersion: database.entigo.com/v1alpha1
kind: MariaDBUser
metadata:
  name: orders-app
spec:
  instanceRef:
    name: orders-db
  databaseRef:
    name: orders
  privileges:
  - SELECT
  - INSERT
```

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: MariaDBUser
metadata:
  name: orders-app
spec:
  serverRef:                           # was instanceRef
    name: orders-db
  databases:                           # was databaseRef and privileges
  - databaseRef:
      name: orders
    privileges:
    - SELECT
    - INSERT
```

*A `MariaDBUser`, today and in the design.*

### MariaDBDatabase

`MariaDBDatabase` keeps its fields; `instanceRef` becomes `serverRef`, naming a `MariaDBServer`,
and `name` and `serverRef` become immutable.

## Valkey

### ValkeyServer

`ValkeyInstance` becomes `ValkeyServer`:

| Today                                                           | Design                                                         | Reason                                                                                                                                  |
| --------------------------------------------------------------- | -------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| kind `ValkeyInstance`                                           | kind `ValkeyServer`                                            | One primary with its replicas, named as Valkey names it; a new kind, served beside today's, so objects move one at a time (principle 8) |
| `engineVersion`                                                 | `version`                                                      | Major and minor, a floor that only rises                                                                                                |
| `instanceType`                                                  | `instanceType` in the `ValkeyElastiCacheConfig` entry          | A size named by its product is a setting of one implementation (principle 2)                                                            |
| `numCacheClusters`                                              | `readReplicas`                                                 | Counts replicas only: `numCacheClusters` minus one                                                                                      |
| (none)                                                          | `highAvailability`                                             | New portable feature; needs at least one replica (principle 1)                                                                          |
| `maintenanceWindow` (ElastiCache string)                        | `maintenanceWindow` (`day`, `hourUTC`)                         | One form everywhere                                                                                                                     |
| `snapshotRetentionLimit`                                        | `backup.retentionDays`                                         | Grouped under backup (principle 13)                                                                                                     |
| `snapshotWindow`                                                | `backup.startHourUTC`                                          | One form everywhere                                                                                                                     |
| `parameterGroupName`                                            | `parameterGroupName` in the `ValkeyElastiCacheConfig` entry    | An ElastiCache concept (principle 2)                                                                                                    |
| `parameterGroupParameters`                                      | `parameters`                                                   | One map everywhere                                                                                                                      |
| (none)                                                          | `parametersRef`                                                | Settings shared through a `ValkeyParameters`                                                                                            |
| (none)                                                          | `fromBackup` in the `ValkeyElastiCacheConfig` entry            | Create a server from a snapshot, such as the final one                                                                                  |
| (none)                                                          | `className`                                                    | The deployment's defaults and limits come from a class (principle 5)                                                                    |
| (none)                                                          | `providerConfigs` entries of other kinds                       | Sizes and settings of other implementations (principle 2)                                                                               |
| `status.endpoint.address`                                       | `status.endpoint.host`                                         | The same word everywhere                                                                                                                |
| `status.multiAZenabled`                                         | `status.highAvailability`                                      | Named after the need                                                                                                                    |
| (none)                                                          | `status.readerEndpoint`, `status.readReplicas`                 | The read address, and the number of replicas                                                                                            |
| (none)                                                          | `status.providerConfig`, `status.effective`, `status.provider` | The applied entry, each value's source, and the implementation's own values (principle 4)                                               |
| `status.kmsKeyAlias`, `status.kmsKeyId`, `status.securityGroup` | removed                                                        | Settings the platform makes (principle 4)                                                                                               |
| `status.parameterGroupName`                                     | removed                                                        | An echo of the spec                                                                                                                     |
| secret keys `AUTH_TOKEN`, `PORT`, `credentials.json`            | `endpoint`, `port`, `username`, `password`, `readerEndpoint`   | The keys of principle 6                                                                                                                 |

*Changes from `ValkeyInstance` to `ValkeyServer`.*

An example, today and in the design:

```yaml
---
apiVersion: database.entigo.com/v1alpha1
kind: ValkeyInstance
metadata:
  name: orders-cache
spec:
  engineVersion: "8.1"
  instanceType: cache.t4g.small
  numCacheClusters: 2
  maintenanceWindow: "sun:05:00-sun:06:00"
  snapshotRetentionLimit: 7
  snapshotWindow: "03:00-05:00"
```

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: ValkeyServer
metadata:
  name: orders-cache
spec:
  version: "8.1"                       # was engineVersion
  readReplicas: 1                      # was numCacheClusters: 2
  highAvailability: true               # new
  maintenanceWindow:                   # now day and hourUTC
    day: Sun
    hourUTC: 5
  backup:
    retentionDays: 7                   # was snapshotRetentionLimit
    startHourUTC: 3                    # was snapshotWindow
  providerConfigs:                     # new
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: ValkeyElastiCacheConfig
    spec:
      instanceType: cache.t4g.small    # was instanceType
```

*`ValkeyInstance` today, `ValkeyServer` in the design.*

## Kafka

### KafkaCluster

Today's cluster-scoped `MSK` only observes an existing cluster; the namespaced `KafkaCluster`
creates one or observes one:

| Today                                    | Design                                                                                                          | Reason                                                                                                            |
| ---------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------- |
| kind `MSK`, cluster-scoped               | kind `KafkaCluster`, namespaced                                                                                 | Named after the engine (principle 8), shared within its Zone (principle 11)                                       |
| `clusterARN`                             | `observe: true` and `id` in the `KafkaMSKConfig` entry                                                          | Observing is one form for every kind (principle 9)                                                                |
| (none)                                   | `version`, `brokers`, `storage.sizeGB`, `highAvailability`, `deletionProtection`, `parameters`, `parametersRef` | Creating a cluster                                                                                                |
| (none)                                   | `className`, and a broker size in each `providerConfigs` entry                                                  | The deployment's defaults come from a class (principle 5), and a size belongs to one implementation (principle 2) |
| `status.brokers`, `status.brokersscram`  | `status.bootstrapServers`                                                                                       | One listener field everywhere                                                                                     |
| `status.arn`                             | `status.provider.arn`                                                                                           | A value of one implementation (principle 4)                                                                       |
| `status.providerConfig`, `status.region` | removed                                                                                                         | Internal to the platform                                                                                          |
| (none)                                   | `status.providerConfig`, `status.effective`                                                                     | The applied entry and each value's source (principle 4)                                                           |
| topics without an ACL open to every user | topics without an ACL closed                                                                                    | Access is granted per user (principle 12)                                                                         |

*Changes from `MSK` to `KafkaCluster`.*

An example, today and in the design:

```yaml
---
apiVersion: queue.entigo.com/v1alpha1
kind: MSK
metadata:
  name: events
spec:
  clusterARN: arn:aws:kafka:...
```

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaCluster
metadata:
  name: events
  namespace: platform                  # new: namespaced
spec:
  observe: true                        # new
  providerConfigs:                     # new
  - apiVersion: aws.queue.entigo.com/v1alpha1
    kind: KafkaMSKConfig
    spec:
      id: arn:aws:kafka:...            # was clusterARN
```

*`MSK` today, `KafkaCluster` in the design.*

### KafkaTopic

`Topic` becomes `KafkaTopic`:

| Today         | Design            | Reason                                          |
| ------------- | ----------------- | ----------------------------------------------- |
| kind `Topic`  | kind `KafkaTopic` | Named after the engine (principle 8)                 |
| `clusterName` | `clusterRef`      | A reference with a namespace (principle 11)          |
| `config`      | `parameters`      | The same word as on every other kind            |
| (none)        | `name`            | A Kafka name that a Kubernetes name cannot hold |

*Changes from `Topic` to `KafkaTopic`.*

An example, today and in the design:

```yaml
---
apiVersion: queue.entigo.com/v1alpha1
kind: Topic
metadata:
  name: orders
  namespace: shop
spec:
  clusterName: events
  partitions: 3
  config:
    retention.ms: "604800000"
```

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaTopic
metadata:
  name: orders
  namespace: shop
spec:
  clusterRef:                          # was clusterName
    name: events
    namespace: platform
  partitions: 3
  parameters:                          # was config
    retention.ms: "604800000"
```

*`Topic` today, `KafkaTopic` in the design.*

### KafkaUser

`KafkaUser` changes as follows:

| Today                                         | Design                                                                         | Reason                                                                             |
| --------------------------------------------- | ------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------- |
| `clusterName`                                 | `clusterRef`                                                                   | A reference with a namespace (principle 11)                                        |
| secret `<cluster>-<user>-k8s`, one JSON value | secret `<cluster>-<user>`: `endpoint`, `username`, `password`, `saslMechanism` | The keys of principle 6                                                            |
| `consumerGroups`, a list of names             | `consumerGroups` entries with `name` and `patternType`                         | A group can be matched by its prefix                                               |
| (none)                                        | `patternType` on each ACL                                                      | A topic can be matched by its prefix, so topics added at runtime need no new entry |

*Changes to `KafkaUser`.*

## RabbitMQ

### RabbitMQBroker

`RabbitMQBroker` changes as follows:

| Today                                                                             | Design                                                                                                                                             | Reason                                                                                       |
| --------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `engineVersion`                                                                   | `version`                                                                                                                                          | The release line; Amazon MQ manages the patch                                                |
| `instanceType`                                                                    | `instanceType` in the `RabbitMQAmazonMQConfig` entry                                                                                               | A size named by its product is a setting of one implementation (principle 2)                 |
| `deploymentMode`                                                                  | `highAvailability`                                                                                                                                 | Named after the need                                                                         |
| `maintenanceWindowStartTime`                                                      | `maintenanceWindow` (`day`, `hourUTC`)                                                                                                             | One form everywhere                                                                          |
| `configuration.data`                                                              | `parameters`                                                                                                                                       | A map the platform renders into `rabbitmq.conf`                                              |
| (none)                                                                            | `parametersRef`                                                                                                                                    | Settings shared through a `RabbitMQParameters`                                               |
| `configuration.description`                                                       | removed                                                                                                                                            | Internal to the platform                                                                     |
| `publiclyAccessible`                                                              | `publiclyAccessible` in the `RabbitMQAmazonMQConfig` entry                                                                                         | An Amazon MQ setting (principle 2)                                                           |
| `brokerName`, `engineType`, `autoMinorVersionUpgrade`                             | removed                                                                                                                                            | The platform names the broker, the engine is RabbitMQ, and Amazon MQ requires minor upgrades |
| (none)                                                                            | `deletionProtection`                                                                                                                               | The same guard as on every other kind                                                        |
| (none)                                                                            | `className`                                                                                                                                        | The deployment's defaults and limits come from a class (principle 5)                         |
| today's Amazon MQ status fields                                                   | `status.endpoint`, `status.managementEndpoint`, `status.highAvailability`, `status.version`; `status.provider.id`, `status.provider.engineVersion` | One form everywhere (principle 4)                                                            |
| (none)                                                                            | `status.providerConfig`, `status.effective`                                                                                                        | The applied entry and each value's source (principle 4)                                      |
| secrets `<broker>-credentials` (`username`, `password`) and `<broker>-connection` | one secret `<broker>-credentials`: `endpoint`, `port`, `username`, `password`, `managementEndpoint`                                                | The keys of principle 6                                                                      |

*Changes to `RabbitMQBroker`.*

An example, today and in the design:

```yaml
---
apiVersion: mq.entigo.com/v1alpha1
kind: RabbitMQBroker
metadata:
  name: orders-mq
spec:
  brokerName: orders-mq
  engineType: RabbitMQ
  engineVersion: "4.2"
  instanceType: mq.m7g.medium
  deploymentMode: SINGLE_INSTANCE
  autoMinorVersionUpgrade: true
  maintenanceWindowStartTime:
    dayOfWeek: SUNDAY
    timeOfDay: "03:00"
    timeZone: UTC
  configuration:
    data: consumer_timeout = 1800000
  publiclyAccessible: false
```

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
spec:
  version: "4.2"                       # was engineVersion
  highAvailability: false              # was deploymentMode
  maintenanceWindow:                   # was maintenanceWindowStartTime
    day: Sun
    hourUTC: 3
  parameters:                          # was configuration.data
    consumer_timeout: "1800000"
  providerConfigs:                     # new
  - apiVersion: aws.mq.entigo.com/v1alpha1
    kind: RabbitMQAmazonMQConfig
    spec:
      instanceType: mq.m7g.medium      # was instanceType
      publiclyAccessible: false        # was publiclyAccessible
# removed: brokerName, engineType, autoMinorVersionUpgrade
```

*A `RabbitMQBroker`, today and in the design.*

### RabbitMQUser

`RabbitMQUser` is new.

### RabbitMQVirtualHost

`RabbitMQVirtualHost` is new.

## Bucket

### Bucket

`S3Bucket` becomes `Bucket`:

| Today                                                        | Design                                                                        | Reason                                                                         |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| kind `S3Bucket`                                              | kind `Bucket`                                                                 | Named after the concept (principle 8)                                          |
| `enableVersioning`                                           | `versioning`                                                                  | The same word on every cloud                                                   |
| `createServiceAccount`, `serviceAccountName`                 | removed                                                                       | An application's identity is not part of the bucket                            |
| (none)                                                       | `deletionProtection`, `lifecycleRules`, `retention`, `cors`, `softDeleteDays` | New settings                                                                   |
| an IAM role, service account and IAM user with an access key | not created                                                                   | A bucket holds no credentials                                                  |
| a secret with `AWS_*` and `credentials.json` keys            | removed                                                                       | A bucket holds no credentials                                                  |
| `status.s3Url`, `status.s3Uri`                               | `status.url`, with `status.bucketName` and `status.region`                    | The same words on every cloud                                                  |
| encryption, public access and ACL status fields              | removed                                                                       | Settings the platform makes (principle 4)                                      |
| (none)                                                       | `className`, and `id` in a provider configuration entry to observe            | The kind's class (principle 5), and observing an existing bucket (principle 9) |

*Changes from `S3Bucket` to `Bucket`.*

Until applications have a cloud identity of their own, applications that read the secret keep
`S3Bucket`, which stays served next to `Bucket`.

An example, today and in the design:

```yaml
---
apiVersion: storage.entigo.com/v1alpha1
kind: S3Bucket
metadata:
  name: orders-files
spec:
  enableVersioning: true
  createServiceAccount: true
  serviceAccountName: orders-app
```

```yaml
---
apiVersion: storage.entigo.com/v1alpha2
kind: Bucket
metadata:
  name: orders-files
spec:
  versioning: true                     # was enableVersioning
  deletionProtection: true             # new
# removed: createServiceAccount, serviceAccountName
```

*`S3Bucket` today, `Bucket` in the design.*

## ArtifactRepository

### ArtifactRepository

`Repository` becomes `ArtifactRepository`:

| Today                           | Design                                                             | Reason                                                                                      |
| ------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| kind `Repository`               | kind `ArtifactRepository`                                          | Holds any OCI artifact, and named after the concept (principle 8)                           |
| `path`, free-form               | `path` under the namespace: `<namespace>/<path>/<name>`            | Namespaces keep paths apart in a cluster; image paths change                                |
| (none)                          | `immutableTags`                                                    | Today a deployment-wide setting, now a class default; ECR's two exclusion modes are dropped |
| `status.repositoryUri`          | `status.uri`                                                       | The same word everywhere                                                                    |
| deployment-wide lifecycle rules | `lifecycleRules`, defaulting from the class                        | Rules per repository, with the same fields (principle 5)                                    |
| (none)                          | `className`, and `id` in a provider configuration entry to observe | The kind's class (principle 5), and observing an existing repository (principle 9)          |
| (none)                          | `status.providerConfig`, `status.effective`, `status.provider`     | The applied entry, each value's source, and the implementation's own values (principle 4)   |

*Changes from `Repository` to `ArtifactRepository`.*

An example, today and in the design:

```yaml
---
apiVersion: artifact.entigo.com/v1alpha1
kind: Repository
metadata:
  name: orders-api
  namespace: shop
spec:
  path: backend
```

```yaml
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
  path: backend                        # now under the namespace: shop/backend/orders-api
  immutableTags: true                  # new
```

*`Repository` today, `ArtifactRepository` in the design.*
