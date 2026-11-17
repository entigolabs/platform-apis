# Overview

The Portable Data Service APIs give each data service in
[`platform-apis`](https://github.com/entigolabs/platform-apis), from PostgreSQL to buckets, one form
that every cloud infralib deploys to can implement with its own managed service. One manifest then
works on AWS, GCP, OCI and Azure alike.

```mermaid
flowchart LR
  object("<b>Data Service Object</b><br/>Describes<br/>cloud resource")
  function("<b>Platform Function</b><br/>Turns object into<br/>provider resources")
  provider("<b>Crossplane Provider</b><br/>Creates resource<br/>on cloud")
  resource[("<b>Cloud Resource</b><br/>&nbsp;<br/>&nbsp;")]
  status("<b>Status</b><br/>Reports resource's<br/>actual state")
  object --> function --> provider --> resource --> status
  class object user
  class function platform
  class provider platform
  class resource cloud
  class status app
```

*The path from a data service object to its resource on the deployment's cloud.*

Today each API is built around one AWS service: its spec takes AWS values, such as an RDS instance
class, its status reports AWS identifiers, and the developer portal hard-codes AWS values in its
forms. infralib also deploys to GCP, OCI and Azure, and OCI is the next cloud to run the data
services. Implementing the design on a cloud and moving today's objects to it are follow-up work.

The rules the design follows are in [Design Rules](rules.md), and what changes for today's users is
in [API Changes](changes.md).

Status: a draft awaiting the agreement of the `platform-apis` maintainers, tracked in
[RMIT-44](https://entigo.atlassian.net/browse/RMIT-44).

## The Model

Each data service has four parts, which the rules rely on:

| Part            | What it is                                                                                       | Owned by                  | Example                                  |
| --------------- | ------------------------------------------------------------------------------------------------ | ------------------------- | ---------------------------------------- |
| Core kind       | What a team asks for, the same wherever it runs                                                  | the team                  | a `PostgreSQLCluster`                    |
| Implementation  | What provides a core kind on one platform: a cloud's managed service, or software in the cluster | Entigo, or a third party  | RDS, Cloud SQL, CloudNativePG            |
| Provider config | The settings that exist only in one implementation, in a kind that implementation defines        | the team, or the operator | a `PostgreSQLRDSConfig` with an RDS size |
| Class           | A deployment's choice of implementation, its defaults and the values a team cannot change        | the deployment's operator | a `PostgreSQLClass` named `standard`     |

*The parts of a data service.*

A team writes a core kind that names a class, and may add a provider config for each implementation
it can run on. Each deployment applies the provider config of its class's implementation, and the
implementation turns the result into running resources.

## Pages

Each page of the design holds the following, in the order of the menu:

- [Design Rules](rules.md): the rules every data service API follows.
- One page per service, showing how it applies the rules: [PostgreSQL](postgresql.md),
  [MariaDB](mariadb.md), [Valkey](valkey.md), [Kafka](kafka.md), [RabbitMQ](rabbitmq.md),
  [Bucket](bucket.md) and [ArtifactRepository](artifact-repository.md). The site shows each in
  three tabs: its design, how each cloud implements it, and the draft
  schemas of its kinds from [schemas](schemas/).
- [API Changes](changes.md): every change from today's APIs to the design, per kind.
- [Follow-Up Work](follow-up-work.md): work outside the design that it depends on or leads to.
- [Deferred Capabilities](deferred-capabilities.md): capabilities considered and omitted for now.
- [Revisions](revisions.md): the design's revisions, newest first.
- Feedback, on the published page only: comments from reviewers, readable by everyone with access
  to the page.
