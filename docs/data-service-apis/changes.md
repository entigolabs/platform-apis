# Changes from Current APIs

Every change from the data service APIs on `main` of
[`platform-apis`](https://github.com/entigolabs/platform-apis) to this design, per kind. A field
not listed keeps its name and meaning.

## Contents

- [Every Kind](#every-kind)
- [PostgreSQL](#postgresql)
- [MariaDB](#mariadb)
- [Valkey](#valkey)
- [Kafka](#kafka)
- [RabbitMQ](#rabbitmq)
- [Bucket](#bucket)
- [ArtifactRepository](#artifactrepository)

## Every Kind

Changes shared by all kinds, stated once here.

- Every kind moves from `v1alpha1` to `v1alpha2`.
- Defaults are written into the object once, at creation, by a `MutatingAdmissionPolicy`, and never
  change a running resource afterwards (rule 5).
- A kind that represents a cloud resource gains `observe` and one block per cloud, `aws`, `gcp`,
  `oci` and `azure`, each with an `id` for observing (rules 2 and 9). Cloud-specific settings move
  into these blocks.
- Every kind with a connection secret gains `secretName` to override the secret's name (rule 6).
- A reference gains `namespace`, valid within the same Zone (rule 11).
- A cloud rejection or an operation the cloud cannot perform shows on the object as the condition
  `CloudAccepted: False` and a warning event (rule 10).
- AWS-only status fields, such as security groups, KMS keys and hosted zone IDs, are removed; what
  remains cloud-specific moves into `status.<cloud>` (rule 4).

## PostgreSQL

`PostgreSQLInstance` changes as follows.

| Today                                                                | Design                                                                        | Reason                                                         |
| -------------------------------------------------------------------- | ----------------------------------------------------------------------------- | -------------------------------------------------------------- |
| `engineVersion`                                                      | `version`                                                                     | The major version; a full version only with minor upgrades off |
| `instanceType`                                                       | `aws.instanceType`                                                            | A cloud's own size name belongs in its block (rule 2)          |
| `allocatedStorage`                                                   | `storage.sizeGB`                                                              | The size at creation and a floor afterwards                    |
| `iops`                                                               | `storage.iops`                                                                | Grouped with the other storage settings                        |
| (none)                                                               | `storage.autoGrow`, `storage.maxSizeGB`, `storage.throughputMBps`             | New common settings                                            |
| `multiAZ`                                                            | `highAvailability`                                                            | Named after the need, not the AWS feature                      |
| `maintenanceWindow` (RDS string)                                     | `maintenance.window` (`day`, `hourUTC`)                                       | One form for every cloud                                       |
| `autoMinorVersionUpgrade`, `allowMajorVersionUpgrade`                | `maintenance.autoMinorVersionUpgrade`, `maintenance.allowMajorVersionUpgrade` | Grouped under maintenance                                      |
| `backupRetentionPeriod`                                              | `backup.retentionDays`                                                        | Grouped under backup                                           |
| `backupWindow` (RDS string)                                          | `backup.startHourUTC`                                                         | One form for every cloud                                       |
| (none)                                                               | `backup.pointInTimeDays`                                                      | New common setting                                             |
| `parameterGroupName`                                                 | `aws.parameterGroupName`                                                      | An RDS concept (rule 2)                                        |
| `parameterGroupParameters` (a map with a reserved `applyMethod` key) | `parameters` and `parameterApplyMethod`                                       | The apply method becomes its own field                         |
| `snapshotIdentifier`                                                 | `aws.fromBackup`                                                              | A backup ID is a cloud value (rule 2)                          |
| (none)                                                               | `restore.fromInstance`, `restore.pointInTime`                                 | Restore from another instance                                  |
| (none)                                                               | `gcp`, `oci` and `azure` settings                                             | Sizes and settings of those clouds                             |

In the status, `endpoint.address` becomes `endpoint.host`; `dbInstanceIdentifier`, `resourceId`,
`status`, `latestRestorableTime` and `finalSnapshotIdentifier` move into `status.aws`, the last as
`finalBackup`; `hostedZoneId`, `vpcSecurityGroupIds`, `kmsKeyId`, `storageEncrypted`,
`storageType` and the echoed `parameterGroupName` and `snapshotIdentifier` are removed. The common
fields are reported as the cloud runs them.

`PostgreSQLUser` and `PostgreSQLDatabase` keep their fields; `name` and `instanceRef` become
immutable.

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
kind: PostgreSQLInstance
metadata:
  name: orders-db
spec:
  version: "17"                        # was engineVersion
  storage:
    sizeGB: 20                         # was allocatedStorage
  highAvailability: false              # was multiAZ
  deletionProtection: true
  maintenance:
    window:                            # was maintenanceWindow
      day: Sun
      hourUTC: 3
  backup:
    retentionDays: 7                   # was backupRetentionPeriod
    startHourUTC: 2                    # was backupWindow
  parameters:                          # was parameterGroupParameters
    max_connections: "200"
  parameterApplyMethod: NextRestart    # was applyMethod: pending-reboot
  aws:
    instanceType: db.t4g.small         # was instanceType
```

## MariaDB

`MariaDBInstance` changes as `PostgreSQLInstance` does, on AWS only, without
`backup.pointInTimeDays`.

`MariaDBUser` changes as follows.

| Today                                                                                         | Design                     | Reason                                                    |
| --------------------------------------------------------------------------------------------- | -------------------------- | --------------------------------------------------------- |
| `databaseRef`, `table`, `privileges`                                                          | one entry of `databases`   | One login per application and instance (rule 12)          |
| `databaseRef.namespace`, ignored today                                                        | honoured within the Zone   | References cross namespaces within the Zone (rule 11)     |
| user limits default to 20 queries and 10 updates an hour, 5 connections an hour and 2 at once | all default to 0, no limit | Today's defaults are too low for a production application |

`MariaDBDatabase` keeps its fields.

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
  instanceRef:
    name: orders-db
  databases:                           # was databaseRef and privileges
  - databaseRef:
      name: orders
    privileges:
    - SELECT
    - INSERT
```

## Valkey

`ValkeyInstance` changes as follows.

| Today                      | Design                                 | Reason                                                |
| -------------------------- | -------------------------------------- | ----------------------------------------------------- |
| `engineVersion`            | `version`                              | Major and minor, a floor that only rises              |
| `instanceType`             | `aws.instanceType`, `gcp.instanceType` | A cloud's own node type belongs in its block (rule 2) |
| `numCacheClusters`         | `replicas`                             | Counts replicas only: `numCacheClusters` minus one    |
| (none)                     | `highAvailability`                     | New common setting; needs at least one replica        |
| `maintenanceWindow`        | `maintenance.window`                   | One form for every cloud                              |
| `autoMinorVersionUpgrade`  | `maintenance.autoMinorVersionUpgrade`  | Grouped under maintenance                             |
| `snapshotRetentionLimit`   | `backup.retentionDays`                 | Grouped under backup                                  |
| `snapshotWindow`           | `backup.startHourUTC`                  | One form for every cloud                              |
| `parameterGroupName`       | `aws.parameterGroupName`               | An ElastiCache concept (rule 2)                       |
| `parameterGroupParameters` | `parameters`                           | One map for every cloud                               |

In the status, `endpoint.address` becomes `endpoint.host`, `multiAZenabled` becomes
`highAvailability`, and `readerEndpoint` and `replicas` are added; `kmsKeyAlias`, `kmsKeyId`,
`securityGroup` and `parameterGroupName` are removed.

The secret's `AUTH_TOKEN`, `PORT` and `credentials.json` keys become `endpoint`, `port`,
`username`, `password` and `readerEndpoint` (rule 6).

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
kind: ValkeyInstance
metadata:
  name: orders-cache
spec:
  version: "8.1"                       # was engineVersion
  replicas: 1                          # was numCacheClusters: 2
  highAvailability: true               # new
  maintenance:
    window:                            # was maintenanceWindow
      day: Sun
      hourUTC: 5
  backup:
    retentionDays: 7                   # was snapshotRetentionLimit
    startHourUTC: 3                    # was snapshotWindow
  aws:
    instanceType: cache.t4g.small      # was instanceType
```

## Kafka

Today's cluster-scoped `MSK` only observes an existing cluster; the namespaced `KafkaCluster`
creates one or observes one.

| Today                                    | Design                                                                                | Reason                                                            |
| ---------------------------------------- | ------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| kind `MSK`, cluster-scoped               | kind `KafkaCluster`, namespaced                                                       | Named after the engine (rule 8), shared within its Zone (rule 11) |
| `clusterARN`                             | `observe: true` and `aws.id`                                                          | Observing is one form for every kind (rule 9)                     |
| (none)                                   | `version`, `brokers`, `storage.sizeGB`, `highAvailability`, `parameters`, cloud sizes | Creating a cluster                                                |
| `status.brokers`, `status.brokersscram`  | `status.bootstrapServers`                                                             | One listener field for every cloud                                |
| `status.arn`                             | `status.aws.arn`                                                                      | A cloud ID (rule 4)                                               |
| `status.providerConfig`, `status.region` | removed                                                                               | Internal to the platform                                          |

| Today                                        | Design                  | Reason                                          |
| -------------------------------------------- | ----------------------- | ----------------------------------------------- |
| kind `Topic`                                 | kind `KafkaTopic`       | Named after the engine (rule 8)                 |
| `Topic.clusterName`, `KafkaUser.clusterName` | `clusterRef`            | A reference with a namespace (rule 11)          |
| `Topic.config`                               | `KafkaTopic.parameters` | The same word as on every other kind            |
| (none)                                       | `KafkaTopic.name`       | A Kafka name that a Kubernetes name cannot hold |

The user secret `<cluster>-<user>-k8s`, with one JSON value, becomes `<cluster>-<user>` with the
keys `endpoint`, `username`, `password` and `saslMechanism`. Clusters deny access to topics
without ACLs, where today they allow everyone.

An example, today and in the design:

```yaml
---
apiVersion: queue.entigo.com/v1alpha1
kind: MSK
metadata:
  name: events
spec:
  clusterARN: arn:aws:kafka:...
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
kind: KafkaCluster                     # was MSK
metadata:
  name: events
  namespace: platform                  # new: namespaced
spec:
  observe: true                        # new
  aws:
    id: arn:aws:kafka:...              # was clusterARN
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaTopic                       # was Topic
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

## RabbitMQ

`RabbitMQBroker` changes as follows, and `RabbitMQUser` and `RabbitMQVirtualHost` are new.

| Today                                                 | Design                   | Reason                                                                                       |
| ----------------------------------------------------- | ------------------------ | -------------------------------------------------------------------------------------------- |
| `engineVersion`                                       | `version`                | The release line; Amazon MQ manages the patch                                                |
| `instanceType`                                        | `aws.instanceType`       | A cloud's own size name (rule 2)                                                             |
| `deploymentMode`                                      | `highAvailability`       | Named after the need                                                                         |
| `maintenanceWindowStartTime`                          | `maintenance.window`     | One form for every cloud                                                                     |
| `configuration.data`                                  | `parameters`             | A map the platform renders into `rabbitmq.conf`                                              |
| `configuration.description`                           | removed                  | Internal to the platform                                                                     |
| `publiclyAccessible`                                  | `aws.publiclyAccessible` | An Amazon MQ setting (rule 2)                                                                |
| `brokerName`, `engineType`, `autoMinorVersionUpgrade` | removed                  | The platform names the broker, the engine is RabbitMQ, and Amazon MQ requires minor upgrades |

In the status, today's Amazon MQ fields are replaced by `endpoint`, `managementEndpoint`,
`highAvailability` and `version`, with `status.aws.id` and `status.aws.engineVersion`. The two
secrets `<broker>-credentials` (`username`, `password`) and `<broker>-connection` become one,
`<broker>-credentials`, with `endpoint`, `port`, `username`, `password` and `managementEndpoint`.

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
  maintenance:
    window:                            # was maintenanceWindowStartTime
      day: Sun
      hourUTC: 3
  parameters:                          # was configuration.data
    consumer_timeout: "1800000"
  aws:
    instanceType: mq.m7g.medium        # was instanceType
    publiclyAccessible: false          # was publiclyAccessible
# removed: brokerName, engineType, autoMinorVersionUpgrade
```

## Bucket

| Today                                        | Design                                                                                                    | Reason                                                                 |
| -------------------------------------------- | --------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| kind `S3Bucket`                              | kind `Bucket`                                                                                             | Named after the concept (rule 8)                                       |
| `enableVersioning`                           | `versioning`                                                                                              | The same word on every cloud                                           |
| `createServiceAccount`, `serviceAccountName` | removed                                                                                                   | An application's identity is not part of the bucket (Follow-Up Work 3) |
| (none)                                       | `deletionProtection`, `lifecycleRules`, `retention`, `cors`, `gcp.softDeleteDays`, `azure.softDeleteDays` | New settings                                                           |

The IAM role, service account and IAM user with an access key that `S3Bucket` creates are no
longer created, and the secret with its `AWS_*` and `credentials.json` keys is removed: a bucket
holds no credentials. In the status, `s3Url` and `s3Uri` become `url`, with `bucketName` and
`region`; the encryption, public access and ACL fields are removed.

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
kind: Bucket                           # was S3Bucket
metadata:
  name: orders-files
spec:
  versioning: true                     # was enableVersioning
  deletionProtection: true             # new
# removed: createServiceAccount, serviceAccountName
```

## ArtifactRepository

| Today                  | Design                                                  | Reason                                                       |
| ---------------------- | ------------------------------------------------------- | ------------------------------------------------------------ |
| kind `Repository`      | kind `ArtifactRepository`                               | Holds any OCI artifact, and named after the concept (rule 8) |
| `path`, free-form      | `path` under the namespace: `<namespace>/<path>/<name>` | Namespaces keep paths apart in a cluster; image paths change |
| (none)                 | `immutableTags`                                         | Today a deployment-wide setting                              |
| `status.repositoryUri` | `status.uri`                                            | The same word on every cloud                                 |

`lifecycleRules` keeps its fields and now defaults per deployment, replacing today's
deployment-wide rules.

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
kind: ArtifactRepository               # was Repository
metadata:
  name: orders-api
  namespace: shop
spec:
  path: backend                        # now under the namespace: shop/backend/orders-api
  immutableTags: true                  # new
```
