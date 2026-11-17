# Deferred Capabilities

Capabilities considered and omitted for now; the design is complete without them.

#### 1 Per-instance extension allow list for PostgreSQL

On Azure a server allows an extension through its `azure.extensions` parameter before any database
can create it. The design sets it to the platform's whole allowlist on every Azure instance, so
databases behave as on the other clouds ([postgresql.md](postgresql.md)); a per-instance list would
allow a team to narrow it, at the cost of a second list to manage.

#### 2 MSK Serverless support

MSK Serverless has a different capacity model (throughput limits, no brokers), IAM login only and no
Kafka ACLs, so no `KafkaUser` can work on it. Observing one is possible; creating one is not
designed.
