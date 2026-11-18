# Revisions

The design's revisions, newest first. A revision changes what the API is or does: a field, a
default, a rule, a mapping, a platform decision or a cloud limit. Changes to wording and layout are
not revisions. A change to the design adds its line to the open revision, or opens the next one once
the last is agreed.

#### 2 in progress

| Type    | Area                    | Change                                                                                                                                                                                                                                           |
| ------- | ----------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Changed | Provider configurations | Settings of one implementation move from the fixed cloud blocks into an open list of provider configurations, one kind per implementation, so a new cloud or an on-premises implementation never changes the core kinds (principles 2, 3 and 8). |
| Added   | Classes                 | Classes hold a deployment's defaults (`template`) and the values a user cannot change (`enforced`) (principle 5).                                                                                                                                |
| Changed | Classes                 | A deployment's defaults move from values passed through infralib into classes (principle 5).                                                                                                                                                     |
| Changed | Portable features       | An object that requests a portable feature its class does not support is refused instead of ignored (principle 1).                                                                                                                               |
| Changed | Validation              | Provider configuration entries are checked by their implementation (principle 10).                                                                                                                                                               |
| Added   | Validation              | CI checks provider configuration entries against every implementation's published schema (principle 10).                                                                                                                                         |
| Changed | Field groups            | Field groups exist only as units or subsystems (principle 13).                                                                                                                                                                                   |
| Added   | Resource tags           | Every resource the platform creates carries workspace and Zone tags, on every service.                                                                                                                                                           |
| Changed | Status                  | Values of one implementation move from `status.<cloud>` to `status.provider`, in the shape its provider configuration kind's status defines; on RDS, `status` becomes `state` (principle 4).                                                     |
| Added   | Status                  | `status.providerConfig`, the entries applied and ignored, and `status.effective`, each value's source (principle 4).                                                                                                                             |
|         |                         |                                                                                                                                                                                                                                                  |
| Changed | PostgreSQL              | `PostgreSQLInstance` becomes `PostgreSQLCluster`; users and databases name it in `clusterRef`.                                                                                                                                                   |
| Added   | PostgreSQL              | `PostgreSQLClass`, `PostgreSQLParameters` and a provider configuration kind per implementation.                                                                                                                                                  |
| Added   | PostgreSQL              | `PostgreSQLDatabase` allows the `vector` extension (pgvector), which today's API allows since release `v0.2.104`.                                                                                                                                |
| Changed | MariaDB                 | `MariaDBInstance` becomes `MariaDBServer`; users and databases name it in `serverRef`.                                                                                                                                                           |
| Added   | MariaDB                 | `MariaDBClass`, `MariaDBParameters` and `MariaDBRDSConfig`.                                                                                                                                                                                      |
| Changed | Valkey                  | `ValkeyInstance` becomes `ValkeyServer`, and `replicas` becomes `readReplicas`.                                                                                                                                                                  |
| Added   | Valkey                  | `ValkeyClass`, `ValkeyParameters` and a provider configuration kind per implementation.                                                                                                                                                          |
| Added   | Kafka                   | `KafkaClass`, `KafkaParameters` and a provider configuration kind per implementation for `KafkaCluster`.                                                                                                                                         |
| Added   | Kafka                   | A `KafkaTopic`'s defaults come from its cluster's `KafkaClass` (`topicTemplate`).                                                                                                                                                                |
| Added   | Kafka                   | A `KafkaUser`'s ACLs and consumer groups match a name exactly or by its prefix (`patternType`).                                                                                                                                                  |
| Added   | RabbitMQ                | `RabbitMQClass`, `RabbitMQParameters` and `RabbitMQAmazonMQConfig` for `RabbitMQBroker`.                                                                                                                                                         |
| Changed | RabbitMQ                | `maintenance.window` becomes `maintenanceWindow`.                                                                                                                                                                                                |
| Added   | Bucket                  | `BucketClass` and a provider configuration kind per implementation for `Bucket`, which holds the `id` of a bucket to observe.                                                                                                                    |
| Added   | ArtifactRepository      | `ArtifactRepositoryClass` and a provider configuration kind per implementation for `ArtifactRepository`.                                                                                                                                         |
| Changed | ArtifactRepository      | `immutableTags` and `lifecycleRules` default from the class.                                                                                                                                                                                     |

*Revision 2 changes by type and area.*

#### 1 2026-11-16

Initial release.
