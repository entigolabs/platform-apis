# Overview

The design of the Portable Data Service APIs, with its reasons.

Status: a draft awaiting the agreement of the `platform-apis` maintainers, tracked in
[RMIT-44](https://entigo.atlassian.net/browse/RMIT-44).

## Summary

- One manifest works on every cloud where its kind exists. It states what is needed, such as a
  version or a size, and settings that exist on only one cloud go into that cloud's block
  ([rule 1](rules.md#1-the-common-spec-states-needs-not-cloud-products), [rule 2](rules.md#2-a-setting-of-one-cloud-belongs-in-that-clouds-block)).
- A cloud that lacks a setting ignores it; an operation it cannot perform fails, with the reason on
  the object ([rule 10](rules.md#10-the-schema-checks-only-what-the-api-defines)).
- A service is offered only where a cloud runs the same engine: MariaDB and RabbitMQ run on AWS
  only ([rule 7](rules.md#7-a-kind-exists-only-where-a-cloud-runs-the-same-engine)).
- Defaults are written once, at creation, so changing a default never alters a running resource
  ([rule 5](rules.md#5-defaults-are-written-once-when-the-object-is-created)).
- Applications read every credential from one Secret with the same keys on every cloud
  ([rule 6](rules.md#6-each-credential-is-one-secret-with-the-same-keys-on-every-cloud)).
- A kind either creates its resource or observes one that already exists ([rule 9](rules.md#9-a-kind-either-creates-its-resource-or-observes-an-existing-one)).
- A user object is one application's login on one instance, with its own permissions
  ([rule 12](rules.md#12-a-user-object-holds-its-own-permissions)).

What changes for today's users is listed in [API Changes](changes.md).

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
  [Bucket](bucket.md) and [ArtifactRepository](artifact-repository.md). The site shows each in
  three tabs: its design, how each cloud implements it (Mapping and Cloud Limits), and the draft
  schemas of its kinds from [schemas](schemas/).
- [API Changes](changes.md): every change from today's APIs to the design, per kind.
- [Follow-Up Work](follow-up-work.md): work outside the design that it depends on or leads to.
- [Deferred Capabilities](deferred-capabilities.md): capabilities considered and omitted for now.
- [Open Points](open-points.md): questions still to decide, listed in the menu only while one is
  open.
