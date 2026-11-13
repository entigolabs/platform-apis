# Overview

The design of the Portable Data Service APIs, with its reasons.

## Context

[`platform-apis`](https://github.com/entigolabs/platform-apis) offers data services as Kubernetes
APIs: PostgreSQL, MariaDB, Valkey, Kafka, RabbitMQ, buckets and artifact repositories. Each API is
built around one AWS service. Its spec takes AWS values (an RDS instance class, an RDS maintenance
window string, a parameter group name), its status reports AWS identifiers, and its composition
function builds only AWS resources
([`functions/database/service/pg_instance.go`](https://github.com/entigolabs/platform-apis/blob/main/functions/database/service/pg_instance.go)).
The developer portal copies these schemas and hard-codes AWS instance types, engine versions and
maintenance windows in its forms.

infralib deploys to AWS, GCP, OCI and Azure. Currently, its [`platform-apis`
module](https://github.com/entigolabs/entigo-infralib/tree/main/modules/k8s/platform-apis) installs
`platform-apis` on AWS and OCI only, and the data services are implemented on AWS only; OCI is the
next cloud to implement them. A user requests a PostgreSQL database, and which cloud runs it is the
platform's concern, so each API needs one form that every infralib cloud can implement with its own
managed service, and that a later cloud can implement without changing it. This design covers all
four clouds; implementing it on a cloud is follow-up work.

The Zone API, implemented for AWS and OCI, is the precedent for a multi-cloud API in
`platform-apis`: it uses shared keys with cloud-native values (`instance-type`), prefixed keys for
settings of one cloud (`oci/ocpus`), and one implementation per cloud in
[`functions/tenancy`](https://github.com/entigolabs/platform-apis/tree/main/functions/tenancy).

## Pages

What each page of the design holds, in the order of the menu.

- [Design Rules](rules.md): the rules every data service API follows, and the alternatives weighed
  against them.
- One page per service, showing how it applies the rules: [PostgreSQL](postgresql.md),
  [MariaDB](mariadb.md), [Valkey](valkey.md), [Kafka](kafka.md), [RabbitMQ](rabbitmq.md),
  [Bucket](bucket.md) and [ArtifactRepository](artifact-repository.md).
- [Schemas](schemas/): the draft schema of every kind.
- [API Changes](changes.md): every change from today's APIs to the design, per kind.
- [Follow-Up Work](follow-up-work.md): work outside the design that it depends on or leads to.
- [Deferred Capabilities](deferred-capabilities.md): capabilities considered and omitted for now.
- [Open Points](open-points.md): questions still to decide, listed in the menu only while one is
  open.
