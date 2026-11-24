# Service Designs

Each data service has a design of its own, built on the mechanisms of
[Common Design](common-design.md) and limited to the service's own kinds and rules. A service's page
has three tabs: Design holds its kinds and their behaviour, Mapping their mapping to each backend,
and Schemas the draft schema of each kind.

The design covers each service on the clouds and on Kubernetes below, in one of three states:

- Designed. The design maps the backend and defines its provider configuration kind. The state
  describes design coverage, not a rollout.
- Mapping only. The design maps the cloud's managed service, but its kinds wait on support in the
  cloud's Crossplane provider.
- No implementation. The design has no implementation of the kind there: the cloud has no managed
  service that fits it, or, on Kubernetes, no backend that runs in the cluster is designed yet.

| Service            | AWS      | GCP               | OCI               | Azure             | Kubernetes        |
| ------------------ | -------- | ----------------- | ----------------- | ----------------- | ----------------- |
| PostgreSQL         | Designed | Designed          | Designed          | Designed          | Designed          |
| MariaDB            | Designed | No implementation | No implementation | No implementation | No implementation |
| Valkey             | Designed | Mapping only      | Designed          | No implementation | No implementation |
| Kafka              | Designed | Mapping only      | Designed          | No implementation | No implementation |
| RabbitMQ           | Designed | No implementation | No implementation | No implementation | No implementation |
| Bucket             | Designed | Designed          | Designed          | Designed          | No implementation |
| ArtifactRepository | Designed | Designed          | Designed          | Designed          | No implementation |

*Service design coverage per backend.*

The services:
