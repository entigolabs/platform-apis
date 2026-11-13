# Overview

The Portable Data Service APIs give each data service in
[`platform-apis`](https://github.com/entigolabs/platform-apis), from PostgreSQL to buckets, one form
that every cloud infralib deploys to can implement with its own managed service. One manifest then
works on AWS, GCP, OCI and Azure alike.

Today each API is built around one AWS service: its spec takes AWS values, such as an RDS instance
class, its status reports AWS identifiers, and the developer portal hard-codes AWS values in its
forms. infralib also deploys to GCP, OCI and Azure, and OCI is the next cloud to run the data
services. Implementing the design on a cloud and moving today's objects to it are follow-up work.

Status: a draft awaiting the agreement of the `platform-apis` maintainers, tracked in
[RMIT-44](https://entigo.atlassian.net/browse/RMIT-44).

## Key Decisions

Each decision is one of the [Design Rules](rules.md):

- A manifest states what is needed, such as a version or a size. A setting that exists on only one
  cloud goes into that cloud's block (rules 1 and 2).
- A cloud that lacks a setting ignores it; an operation it cannot perform fails, with the reason on
  the object (rules 1 and 10).
- A service is offered only where a cloud runs the same engine, so MariaDB and RabbitMQ run on AWS
  only (rule 7).
- Defaults are written once, at creation, so changing a default never alters a running resource
  (rule 5).
- Every credential is one Secret with the same keys on every cloud (rule 6).
- A kind either creates its resource or observes one that already exists (rule 9).
- A user object is one application's login on one instance, with its own permissions (rule 12).

The split into common fields and cloud blocks follows the Zone API, which uses shared keys with
cloud-native values and prefixed keys for one cloud's settings
([`functions/tenancy`](https://github.com/entigolabs/platform-apis/tree/main/functions/tenancy)).
What changes for today's users is listed in [API Changes](changes.md).

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
