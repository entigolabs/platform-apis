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
- None. The design has no implementation of the kind there: the cloud has no managed service that
  fits it, or, on Kubernetes, no backend that runs in the cluster is designed yet.

| Service            | AWS      | GCP          | OCI      | Azure    | Kubernetes |
| ------------------ | -------- | ------------ | -------- | -------- | ---------- |
| PostgreSQL         | Designed | Designed     | Designed | Designed | Designed   |
| MariaDB            | Designed | None         | None     | None     | None       |
| Valkey             | Designed | Mapping only | Designed | None     | None       |
| Kafka              | Designed | Mapping only | Designed | None     | None       |
| RabbitMQ           | Designed | None         | None     | None     | None       |
| Bucket             | Designed | Designed     | Designed | Designed | None       |
| ArtifactRepository | Designed | Designed     | Designed | Designed | None       |

*Service design coverage per backend.*

The services:
