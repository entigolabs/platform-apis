# Overview

The design of the Portable Data Service APIs, with its reasons.

Status: a draft awaiting the agreement of the `platform-apis` maintainers, tracked in
[RMIT-44](https://entigo.atlassian.net/browse/RMIT-44).

## Summary

[`platform-apis`](https://github.com/entigolabs/platform-apis) offers data services, such as
PostgreSQL and Kafka, as Kubernetes APIs that work only on AWS. This design gives each API one form
that every cloud infralib deploys to can implement with its own managed service, so that one
manifest works on AWS, GCP, OCI and Azure alike.

## Motivation

`platform-apis` offers PostgreSQL, MariaDB, Valkey, Kafka, RabbitMQ, buckets and artifact
repositories. Each API is built around one AWS service: its spec takes AWS values, such as an RDS
instance class or a parameter group name, its status reports AWS identifiers, and its function
builds only AWS resources. The developer portal copies these schemas and hard-codes AWS values in
its forms.

infralib deploys to AWS, GCP, OCI and Azure, and OCI is the next cloud to run the data services. A
user asks for a PostgreSQL database; which cloud runs it is the platform's concern.

### Goals

- One manifest works on every cloud where its service exists.
- A cloud added later implements the APIs without changing them.
- Applications connect the same way on every cloud.

### Non-Goals

- Implementing the design on a cloud, and moving today's objects to it. Both are
  [Follow-Up Work](follow-up-work.md).
- Access for people. A user object is an application's login, not a person's.

## Proposal

The key decisions, grouped by who they affect. Each is one of the [Design Rules](rules.md).

### For Users

- A manifest states what is needed, such as a version or a size. A setting that exists on only one
  cloud goes into that cloud's block (rules 1 and 2).
- A kind either creates its resource or observes one that already exists (rule 9).
- A service is offered only where a cloud runs the same engine, so MariaDB and RabbitMQ run on AWS
  only (rule 7).

### On the Platform

- A cloud that lacks a setting ignores it; an operation it cannot perform fails, with the reason on
  the object (rules 1 and 10).
- Defaults are written once, at creation, so changing a default never alters a running resource
  (rule 5).

### For Applications

- Every credential is one Secret with the same keys on every cloud (rule 6).
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
