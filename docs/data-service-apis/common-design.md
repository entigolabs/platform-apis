# Common Design

Every data service API uses the mechanisms on this page, each of which applies a principle from
[Principles](principles.md).

## Naming

A core kind is named after its engine or concept (principle 8), and the kinds around it take
their names from it:

| Kind                   | Name                      | Group                     | Example                                            |
| ---------------------- | ------------------------- | ------------------------- | -------------------------------------------------- |
| Class                  | `<Engine>Class`           | the core group            | `PostgreSQLClass` in `database.entigo.com`         |
| Parameters             | `<Engine>Parameters`      | the core group            | `PostgreSQLParameters` in `database.entigo.com`    |
| Provider configuration | `<Engine><Product>Config` | `<platform>.<core group>` | `PostgreSQLRDSConfig` in `aws.database.entigo.com` |

*Related kind names, groups and examples.*

The kinds keep today's category groups: `database.`, `queue.`, `mq.`, `storage.` and
`artifact.entigo.com`. A group exists only where it serves a purpose, so no umbrella group gathers
them.

The engine comes first, so the kinds sort together. The product is the implementation's own short
name, such as RDS, Cloud SQL or MSK; where it has none, the platform's name stands in for it, as in
`PostgreSQLOCIConfig` for OCI Database with PostgreSQL. An on-premises implementation is grouped by
the platform it runs on, `kubernetes`, with the product in the kind, as `PostgreSQLCNPGConfig` for
CloudNativePG. An implementation built outside Entigo uses its own domain, so names never collide.
Each provider configuration kind is versioned independently.

## Installation

The `platform-apis` chart determines which kinds a deployment installs (principle 7). A deployment
passes only its cloud, and the chart installs each service's core kinds, its provider configuration
kinds and its classes where the service is implemented. A per-service switch leaves a service out,
never adds one that the deployment lacks. A manifest of a kind that is not installed is rejected by
the Kubernetes API server when it is applied, before any function runs.

## Versions

A kind that keeps its name from today's API, such as `PostgreSQLUser`, stays one CRD that serves
today's `v1alpha1` and the design's `v1alpha2`. Kubernetes stores one version, and a conversion
webhook translates between the two, so a manifest written for today's API still applies.

A renamed kind, such as `PostgreSQLCluster`, is a new kind beside today's `PostgreSQLInstance`,
since a conversion only translates between versions of one kind. Today's kind stays served, with its
function, until its last object has moved; client teams move their own objects, as follow-up work
describes.

## Classes

A class, such as a `PostgreSQLClass`, is cluster-scoped and written by Entigo for each deployment
(principle 5), and an object in any namespace may name any class of its kind; no Zone restricts the
classes its namespaces use. It holds two parts of the object's spec, each with the common fields and
one provider configuration:

| Part       | Effect                                                                                         |
| ---------- | ---------------------------------------------------------------------------------------------- |
| `template` | Defaults, written into the object when it is created; the object may change them               |
| `enforced` | Enforced values, always applied; an object that sets a different value is refused when applied |

*Class parts and their effects.*

The class's provider configuration kind selects the implementation, as `StorageClass.provisioner`
does, so a class has no separate field for it. A class whose provider configuration names a kind the
deployment does not install is refused when applied. A class name is the portable contract: every
deployment defines its own `standard`, and a chart that sets `className: standard` works wherever a
class of that name exists. Without `className`, the object takes the deployment's default class, the
one annotated `<group>/is-default-class: "true"`, as a default StorageClass is.

The effective spec is built in three layers, the last winning: the class's `template`, then the
object with its matching provider configuration entry, then the class's `enforced`. Engine
parameters merge key by key in the same order.

The defaults are written into the object by a `MutatingAdmissionPolicy`, built into Kubernetes from
version 1.36. It copies the class's `template` into the object when it is created and keeps the
values on updates. `enforced` applies all the time: it is policy, and an object that no longer
complies after a class tightens it reports that in a condition. Observed resources receive no
defaults.

Each class reports in its status which portable features its implementation supports, such as
`HighAvailability` and `ReadReplicas`. An object that requests a feature its class does not report
is refused when applied, with a message naming the class and the feature (principle 1). The portal
reads a class's supported features and hides the fields the class cannot fulfil.

## Provider Configurations

An object carries its provider configurations (principle 2) in `providerConfigs`, a list with one
entry per kind. Each entry names its own `apiVersion` and `kind`, and holds the settings either
inline, in `spec`, or in a separate object of that kind, through `ref`. The list is keyed by
`apiVersion` and `kind`, so Helm, Argo CD and server-side apply merge it entry by entry, and one
manifest can carry settings for several implementations and be applied on any of them.

A deployment applies only the entry whose kind its class names and ignores the others; the status
names the applied entry and the ignored ones, so a misspelled kind shows as no entry applied. An
object with no matching entry takes the class's provider configuration as it is.

Each provider configuration kind marks the settings its implementation cannot run without, such as a
machine size. An object for which neither its entry nor its class sets one is refused when applied,
with a message naming the setting. A deployment that wants every application to choose a setting,
such as its size, leaves it out of its class.

A machine size is a provider configuration setting, named as the product names it, such as
`db.m7g.large` in a `PostgreSQLRDSConfig` and `db-custom-2-8192` in a `PostgreSQLCloudSQLConfig`. A
size stated as CPU and memory would need the platform to pick the nearest size across fixed and
flexible sizes, families and availability per region. A class's default size spares users from
knowing the names.

## Status

The status (principle 4) shows the common fields as the implementation actually runs them, the usual
`conditions`, the address applications connect to, and the name of the connection secret. The
address is `endpoint: { host, port }` where the service has one host, or the service's own form
where it has several.

Values that exist in only one implementation, such as its resource IDs, go into `status.provider`,
whose shape the applied provider configuration kind defines. `status.providerConfig` names the
applied entry and the ignored ones, and `status.effective` shows each effective value and where it
came from: the class's `template`, the object, or the class's `enforced`. If an implementation does
not report a common field, that field stays empty there.

## Connection Secrets

Each credential is one Kubernetes Secret in the object's namespace, with the same keys everywhere
(principle 6): `endpoint`, `port`, `username` and `password`. `port` is omitted where `endpoint`
already lists hosts with their ports. A service that needs more adds its own keys, such as
`readerEndpoint` for read replicas. No key is named after a cloud, and no key holds the others as
JSON.

The secrets are named `<instance>-dbadmin` for a database's admin, `<instance>-credentials` for
other services, and `<instance>-<user>` for a user object. Every kind with a secret takes an
optional `secretName` to override that name. The platform never overwrites a Secret that another
object or person owns: joined names can collide, as a user named `dbadmin` with the admin's secret,
or instance `a-b` with user `c` and instance `a` with user `b-c`, so the second object reports which
object holds the name, and `secretName` resolves it. `secretName` also lets a user keep a Secret
name their application already uses.

## Observed Resources

To observe an existing resource (principle 9), the user sets `observe: true` and gives the
resource's ID in the provider configuration entry of the implementation that holds it:

```yaml
spec:
  observe: true
  providerConfigs:
  - apiVersion: aws.database.entigo.com/v1alpha1
    kind: PostgreSQLRDSConfig
    spec:
      id: ...                          # the resource's ID, copied from the cloud console
```

*Observed resource on AWS.*

The ID is one value the user copies from the cloud's console. The platform converts it into the form
the Crossplane provider takes. An observed resource takes no other spec fields.

A resource can be observed only when its workspace tag (a label on GCP) names this cluster's
workspace, and only from a namespace of the Zone its Zone tag names, or from any Zone when it has no
Zone tag. Its owner sets the tags in the cloud. The platform reads them before it uses the resource,
and a resource that fails the check is refused with `CloudAccepted: False`.

The check holds only where users can create the platform's kinds and nothing else, so it has a
prerequisite: kcp serves no Crossplane provider kinds, and a Zone's Argo CD project and roles allow
only the platform's and Kubernetes' own groups. Today's Argo CD project does not yet limit itself to
an allowlist of groups. A user who could create a Crossplane provider's resource directly could
observe anything without the check.

## Resource Names

When the platform creates a resource, its name in the cloud includes a hash of the object's unique
ID. As a result, two objects with the same name in different namespaces never share one cloud
resource. A name that applications use, such as a Kafka topic or an image path, keeps the user's
name instead, and the platform rejects a second object that requests a name already held.

## Platform Decisions

The platform makes the settings below itself, for every service that has them, so no field sets them
(principle 2):

| Setting            | Platform's Choice                                                 |
| ------------------ | ----------------------------------------------------------------- |
| Network placement  | The deployment's private network                                  |
| Public access      | None, unless an implementation's provider configuration allows it |
| Encryption at rest | The platform's data key, where the implementation takes one       |
| Tags               | Workspace and Zone tags, on every resource the platform creates   |

*Service-wide settings chosen by the platform.*

## References

A kind that exists inside another, such as a database user or a Kafka topic, names its parent in a
reference, such as `clusterRef`, `serverRef` or `brokerRef`, as `{ name, namespace }`. Without
`namespace`, the parent is in the same namespace. Such a kind has no class and no provider
configurations: it acts inside the engine, which is the same everywhere, and follows its parent's
implementation. Its defaults come from its parent's class, where that class defines them, such as
a `KafkaClass`'s `topicTemplate`, and are written into it at creation like a class's `template`
(principle 5).

A reference to another namespace is accepted only when both namespaces carry the same
`tenancy.entigo.com/zone` label (principle 11).

## Validation

Provider configuration entries are opaque to the core schema (principle 10). Each implementation
checks the entry that applies on its deployment with a `ValidatingAdmissionPolicy`, and publishes
its kind's schema, so that CI checks every entry of a manifest, including those for other
deployments, before it is applied anywhere.

When the cloud rejects a value, the error appears only on the Crossplane provider's own object, not
on the user's. The platform therefore also shows it on the user's object, as the condition
`CloudAccepted: False` and as a warning event. An operation that the running implementation cannot
perform, such as an upgrade its service does not offer, is reported the same way, without reaching
the cloud.

Deletion protection is enforced by a `ValidatingAdmissionPolicy`, which refuses to delete an object
whose `deletionProtection` is true.
