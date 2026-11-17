# Service Designs

Each data service has a design of its own, built on the mechanisms of
[Common Design](common-design.md) and limited to the service's own kinds and rules. A service's page
has three tabs: Design holds its kinds and their behaviour, Cloud Mapping their mapping to each
cloud's managed service, and Schemas the draft schema of each kind.

The design covers each service on the clouds below. Designed means the design maps the cloud's
managed service and defines its provider configuration kind; it describes the design's coverage, not
a rollout. Mapping only means the service is mapped, but its kinds wait on support in the cloud's
Crossplane provider. No implementation means the cloud has no managed service that fits the kind.

| Service            | AWS      | GCP               | OCI               | Azure             |
| ------------------ | -------- | ----------------- | ----------------- | ----------------- |
| PostgreSQL         | Designed | Designed          | Designed          | Designed          |
| MariaDB            | Designed | No implementation | No implementation | No implementation |
| Valkey             | Designed | Mapping only      | Designed          | No implementation |
| Kafka              | Designed | Mapping only      | Designed          | No implementation |
| RabbitMQ           | Designed | No implementation | No implementation | No implementation |
| Bucket             | Designed | Designed          | Designed          | Designed          |
| ArtifactRepository | Designed | Designed          | Designed          | Designed          |

*Service design coverage per cloud.*

The services:
