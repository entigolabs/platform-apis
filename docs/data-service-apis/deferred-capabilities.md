# Deferred Capabilities

The capabilities below were considered and omitted for now; the design is complete without them.

#### 1 Per-cluster extension allowlist for PostgreSQL, deferred as a second list to manage

On Azure a server allows an extension through its `azure.extensions` parameter before any database
can create it. The design sets it to the platform's whole allowlist on every Azure cluster, so
databases behave as on the other clouds ([postgresql.md](postgresql.md)); a per-cluster list would
allow a user to narrow it, at the cost of a second list to manage.

#### 2 MSK Serverless support, deferred since no `KafkaUser` can work on it

MSK Serverless has a different capacity model (throughput limits, no brokers), IAM login only and no
Kafka ACLs, so no `KafkaUser` can work on it. Observing one is possible; creating one is not
designed.

#### 3 Read replicas for PostgreSQL and MariaDB

A `PostgreSQLCluster` or `MariaDBServer` keeps read-only copies of itself, requested as a count,
`readReplicas`, or, where the implementation allows it, as replicas of their own sizes in its
provider configuration.

Valkey keeps the read replicas it has today. The design as last written, with its rules, mappings,
schemas and the open point on the reader endpoint in user secrets, is at commit
[`3c74fd5`](https://github.com/entigolabs/platform-apis/tree/3c74fd58e082e1cc17c96b5d08e6aa2d1a73f2a1/docs/data-service-apis).

#### 4 Disaster recovery across workspaces

A database keeps a copy in another Entigo workspace, usually in another region, which becomes the
primary when the user changes `primary` in its `disasterRecovery`; a class in the copy's workspace
can hold a smaller `standby` provider configuration.

The design as last written, with its rules, mappings, schemas and its open points on fencing the old
primary, the default size of the copy and the pairing of workspaces, is at commit
[`3c74fd5`](https://github.com/entigolabs/platform-apis/tree/3c74fd58e082e1cc17c96b5d08e6aa2d1a73f2a1/docs/data-service-apis).

#### 5 Credential rotation on request

A field such as `spec.rotation` replaces a credential when its value changes, so a leaked or aged
credential can be retired; today a credential changes only when its object is recreated. A user
kind keeps two logins and alternates between them: a rotation moves the Secret to the other login,
and the previous one keeps working until the next rotation, so applications that reread the Secret
within one rotation never lose their connection. A GCP `KafkaUser` alternates its service account's
two keys. The admin login and a Valkey password have a single login each, so they change at once,
and an application connected with the old value reconnects.
