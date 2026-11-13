# Overview

The Portable Data Service APIs give each data service in
[`platform-apis`](https://github.com/entigolabs/platform-apis), from PostgreSQL to buckets, one form
that every cloud infralib deploys to can implement with its own managed service. One manifest then
works on AWS, GCP, OCI and Azure alike.

Today each API is built around one AWS service: its spec takes AWS values, such as an RDS instance
class, its status reports AWS identifiers, and the developer portal hard-codes AWS values in its
forms. infralib also deploys to GCP, OCI and Azure, and OCI is the next cloud to run the data
services. Implementing the design on a cloud and moving today's objects to it are follow-up work.

The rules the design follows are in [Design Rules](rules.md), and what changes for today's users is
in [API Changes](changes.md).

Status: a draft awaiting the agreement of the `platform-apis` maintainers, tracked in
[RMIT-44](https://entigo.atlassian.net/browse/RMIT-44).

## Pages

What each page of the design holds, in the order of the menu.

- [Design Rules](rules.md): the rules every data service API follows, and the alternatives weighed
  against them.
- One page per service, showing how it applies the rules: [PostgreSQL](postgresql.md),
  [MariaDB](mariadb.md), [Valkey](valkey.md), [Kafka](kafka.md), [RabbitMQ](rabbitmq.md),
  [Bucket](bucket.md) and [ArtifactRepository](artifact-repository.md). The site shows each in
  three tabs: its design, how each cloud implements it, and the draft schemas of its kinds from
  [schemas](schemas/). A page leaves out a section it has nothing for, and each section means the
  same on every page:
  - Spec: what a user writes to create the service's main kind. The comments note value ranges and
    what holds on one cloud only.
  - Status: what the status adds to the common fields, and the main kind's secret.
  - Other Kinds: the service's other kinds, such as its users.
  - Platform Decisions: what the platform sets that the user does not.
  - Ignored Settings: settings accepted on every cloud but without effect on some, and what that
    means there (rule 1).
  - Notes: facts the design relies on that need no decision.
  - Mapping: how each field maps to the managed service of each cloud, named by the fields of that
    cloud's Crossplane provider. Level is the field's support level (rule 1).
  - Cloud Limits: limits each cloud enforces itself, which the API does not check (rule 10). A
    value outside them is rejected by the cloud and shown as `CloudAccepted: False`. They are
    listed for reference, should validating them be decided later.
- [API Changes](changes.md): every change from today's APIs to the design, per kind.
- [Follow-Up Work](follow-up-work.md): work outside the design that it depends on or leads to.
- [Deferred Capabilities](deferred-capabilities.md): capabilities considered and omitted for now.
- [Open Points](open-points.md): questions still to decide, listed in the menu only while one is
  open.
