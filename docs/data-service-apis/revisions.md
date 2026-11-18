# Revisions

The design's revisions, newest first. A revision changes what the API is or does: a field, a
default, a rule, a mapping, a platform decision or a cloud limit. Changes to wording and layout are
not revisions. A change to the design adds its line to the open revision, or opens the next one once
the last is agreed.

#### 2 2026-11-18

Settings of one implementation move out of fixed cloud blocks into classes and provider
configurations, so a deployment sets its own defaults and limits, and a new cloud or an on-premises
implementation is added without changing the core kinds.

| Change                                                     | Details                                                                                                                                                                                                                                          |
| ---------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Fixed cloud blocks replaced<br>Changed, Common             | Settings of one implementation move from the fixed cloud blocks into an open list of provider configurations, one kind per implementation, so a new cloud or an on-premises implementation never changes the core kinds (principles 2, 3 and 8). |
| Classes for defaults and constraints<br>Added, Common      | Classes hold a deployment's defaults (`template`) and the values a user cannot change (`enforced`) (principle 5).                                                                                                                                |
| Defaults moved from infralib<br>Changed, Common            | A deployment's defaults move from values passed through infralib into classes (principle 5).                                                                                                                                                     |
| Unsupported features refused<br>Changed, Common            | An object that requests a portable feature its class does not support is refused instead of ignored (principle 1).                                                                                                                               |
| Entries checked by their implementation<br>Changed, Common | Provider configuration entries are checked by their implementation (principle 10).                                                                                                                                                               |
| CI checks against published schemas<br>Added, Common       | CI checks provider configuration entries against every implementation's published schema (principle 10).                                                                                                                                         |
| Field groups limited to units<br>Changed, Common           | Field groups exist only as units or subsystems (principle 13).                                                                                                                                                                                   |
| Tags on every created resource<br>Added, Common            | Every resource the platform creates carries workspace and Zone tags, on every service.                                                                                                                                                           |
| Implementation status moved<br>Changed, Common             | Values of one implementation move from `status.<cloud>` to `status.provider`, in the shape its provider configuration kind's status defines; on RDS, `status` becomes `state` (principle 4).                                                     |
| Applied entries and value sources<br>Added, Common         | `status.providerConfig`, the entries applied and ignored, and `status.effective`, each value's source (principle 4).                                                                                                                             |
| Version conversion for kept names<br>Added, Common         | A kind that keeps its name from today's API serves `v1alpha1` and `v1alpha2` through one stored version and a conversion webhook, so today's manifests still apply.                                                                              |
| Missing required settings refused<br>Added, Common         | An object for which neither its provider configuration entry nor its class sets a setting its implementation cannot run without, such as a machine size, is refused when applied.                                                                |
| Rule for new portable features<br>Added, Common            | A need becomes a portable feature once two or more implementations support it, in a revision of the design (principle 1).                                                                                                                        |
| Cluster kind renamed<br>Changed, PostgreSQL                | `PostgreSQLInstance` becomes `PostgreSQLCluster`; users and databases name it in `clusterRef`.                                                                                                                                                   |
| Class, parameters and provider kinds<br>Added, PostgreSQL  | `PostgreSQLClass`, `PostgreSQLParameters` and a provider configuration kind per implementation.                                                                                                                                                  |
| `vector` extension allowed<br>Added, PostgreSQL            | `PostgreSQLDatabase` allows the `vector` extension (pgvector), which today's API allows since release `v0.2.104`.                                                                                                                                |
| Server kind renamed<br>Changed, MariaDB                    | `MariaDBInstance` becomes `MariaDBServer`; users and databases name it in `serverRef`.                                                                                                                                                           |
| Class, parameters and RDS kind<br>Added, MariaDB           | `MariaDBClass`, `MariaDBParameters` and `MariaDBRDSConfig`.                                                                                                                                                                                      |
| Server kind and replicas renamed<br>Changed, Valkey        | `ValkeyInstance` becomes `ValkeyServer`, and `replicas` becomes `readReplicas`.                                                                                                                                                                  |
| Class, parameters and provider kinds<br>Added, Valkey      | `ValkeyClass`, `ValkeyParameters` and a provider configuration kind per implementation.                                                                                                                                                          |
| Class, parameters and provider kinds<br>Added, Kafka       | `KafkaClass`, `KafkaParameters` and a provider configuration kind per implementation for `KafkaCluster`.                                                                                                                                         |
| Topic defaults from the class<br>Added, Kafka              | A `KafkaTopic`'s defaults come from its cluster's `KafkaClass` (`topicTemplate`).                                                                                                                                                                |
| Prefix matching for ACLs and groups<br>Added, Kafka        | A `KafkaUser`'s ACLs and consumer groups match a name exactly or by its prefix (`patternType`).                                                                                                                                                  |
| Class, parameters and Amazon MQ kind<br>Added, RabbitMQ    | `RabbitMQClass`, `RabbitMQParameters` and `RabbitMQAmazonMQConfig` for `RabbitMQBroker`.                                                                                                                                                         |
| Maintenance window flattened<br>Changed, RabbitMQ          | `RabbitMQBroker`'s `maintenance.window` becomes `maintenanceWindow`.                                                                                                                                                                             |
| Class and provider kinds<br>Added, Bucket                  | `BucketClass` and a provider configuration kind per implementation for `Bucket`, which holds the `id` of a bucket to observe.                                                                                                                    |
| Class and provider kinds<br>Added, ArtifactRepository      | `ArtifactRepositoryClass` and a provider configuration kind per implementation for `ArtifactRepository`.                                                                                                                                         |
| Defaults from the class<br>Changed, ArtifactRepository     | `ArtifactRepository`'s `immutableTags` and `lifecycleRules` default from the class.                                                                                                                                                              |

*Revision 2 changes and their details.*

#### 1 2026-11-16

Each data service gets one form, with a fixed block of settings per cloud, so one manifest works on
AWS, GCP, OCI and Azure alike.
