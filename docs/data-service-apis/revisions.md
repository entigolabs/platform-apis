# Revisions

The design's revisions, newest first. A revision changes what the API is or does: a field, a
default, a rule, a mapping, a platform decision or a cloud limit. Changes to wording and layout are
not revisions. A change to the design adds its line to the open revision, or opens the next one once
the last is agreed.

#### 2 in progress

| Area                    | Change                                                                                                                                                                                                                                                                                                                |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Provider configurations | Settings of one implementation move from the fixed cloud blocks into an open list of provider configurations, one kind per implementation, so a new cloud or an on-premises implementation never changes the core kinds (principles 2, 3 and 8).                                                                      |
| Classes                 | Classes hold a deployment's defaults (`template`) and the values a user cannot change (`enforced`), replacing defaults passed through infralib (principle 5).                                                                                                                                                         |
| Portable features       | An object that requests a portable feature its class does not support is refused instead of ignored (principle 1).                                                                                                                                                                                                    |
| Validation              | Provider configuration entries are checked by their implementation, and by CI against every implementation's published schema (principle 10).                                                                                                                                                                         |
| Field groups            | Field groups exist only as units or subsystems (principle 13).                                                                                                                                                                                                                                                        |
| Resource tags           | Every resource the platform creates carries workspace and Zone tags, on every service.                                                                                                                                                                                                                                |
|                         |                                                                                                                                                                                                                                                                                                                       |
| PostgreSQL              | `PostgreSQLInstance` becomes `PostgreSQLCluster`, with `PostgreSQLClass`, `PostgreSQLParameters` and a provider configuration kind per implementation; users and databases name it in `clusterRef`. `PostgreSQLDatabase` allows the `vector` extension (pgvector), which today's API allows since release `v0.2.104`. |
| MariaDB                 | `MariaDBInstance` becomes `MariaDBServer`, with `MariaDBClass`, `MariaDBParameters` and `MariaDBRDSConfig`; users and databases name it in `serverRef`.                                                                                                                                                               |
| Valkey                  | `ValkeyInstance` becomes `ValkeyServer`, with `ValkeyClass`, `ValkeyParameters` and a provider configuration kind per implementation; `replicas` becomes `readReplicas`.                                                                                                                                              |
| Kafka                   | `KafkaCluster` gains `KafkaClass`, `KafkaParameters` and a provider configuration kind per implementation; a `KafkaTopic`'s defaults come from its cluster's `KafkaClass` (`topicTemplate`); and a `KafkaUser`'s ACLs and consumer groups match a name exactly or by its prefix (`patternType`).                      |
| RabbitMQ                | `RabbitMQBroker` gains `RabbitMQClass`, `RabbitMQParameters` and `RabbitMQAmazonMQConfig`; `maintenance.window` becomes `maintenanceWindow`.                                                                                                                                                                          |
| Bucket                  | `Bucket` gains `BucketClass` and a provider configuration kind per implementation, which holds the `id` of a bucket to observe.                                                                                                                                                                                       |
| ArtifactRepository      | `ArtifactRepository` gains `ArtifactRepositoryClass` and a provider configuration kind per implementation; `immutableTags` and `lifecycleRules` default from the class.                                                                                                                                               |

*Revision 2 changes by area.*

#### 1 2026-11-16

Initial release.
