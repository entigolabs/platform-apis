# Deferred Capabilities

Capabilities considered and omitted for now; the design is complete without them.

#### 1 Topic and group prefixes in `KafkaUser` ACLs

Each ACL and consumer group in a `KafkaUser` names one exact topic or group, as today, so an
application that adds topics at runtime, such as one per customer (`orders-acme`, `orders-globex`),
needs a new entry before it can use each one. Kafka's prefixed ACLs (every topic starting with
`orders-`) would cover them all; MSK and OCI run plain Kafka ACLs, and GCP's support is unverified.

#### 2 Per-instance extension allow list for PostgreSQL

On Azure a server allows an extension through its `azure.extensions` parameter before any database
can create it. The design sets it to the platform's whole allowlist on every Azure instance, so
databases behave as on the other clouds ([postgresql.md](postgresql.md)); a per-instance list would
allow a team to narrow it, at the cost of a second list to manage.

#### 3 MSK Serverless

MSK Serverless has a different capacity model (throughput limits, no brokers), IAM login only and no
Kafka ACLs, so no `KafkaUser` can work on it. Observing one is possible; creating one is not
designed.
