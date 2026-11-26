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

The engine comes first, so the kinds sort together. The product is the backend's own short name,
such as RDS, Cloud SQL or MSK; where it has none, the platform's name stands in for it, as in
`PostgreSQLOCIConfig` for OCI Database with PostgreSQL. A backend that runs in the cluster is
grouped by the platform it runs on, `kubernetes`, with the product in the kind, as
`PostgreSQLCNPGConfig` for CloudNativePG. An implementation built outside Entigo uses its own
domain, so names never collide. Each provider configuration kind is versioned independently.

## Installation

The `platform-apis` chart determines which kinds a deployment installs (principle 7). A deployment
passes its cloud, and the chart installs each service's core kinds, its provider configuration kinds
and its classes where the service is implemented. A deployment may also add a backend that runs in
the Kubernetes cluster, such as CloudNativePG, beside its cloud's; a class then chooses between the
two, and a standalone object with entries for both is refused. A per-service switch leaves a service
out, never adds one the deployment lacks. The chart installs only on Kubernetes 1.36 or later, the
first version with the `MutatingAdmissionPolicy` built in that writes class defaults. A manifest of
a kind that is not installed is rejected by the Kubernetes API server when it is applied, before any
function runs.

## Versions

A kind that keeps its name from today's API, such as `PostgreSQLUser`, stays one CRD that serves
today's `v1alpha1` and the design's `v1alpha2`. Kubernetes stores one version, and a conversion
webhook translates between the two, so a manifest written for today's API still applies. A field
only one version has is kept in an annotation on the other version's form and restored from it on
the way back, so an object written through either version loses none of its fields.

An object of a kind that keeps its name keeps today's behaviour until its parent moves. Its parent
reference names today's kind in `kind`, such as `kind: PostgreSQLInstance`, which the conversion
writes from today's reference, and the object keeps today's Secret name, keys and defaults. Once the
parent has moved to the design's kind, the team removes `kind`, which can be removed once and never
added, and the object takes the design's behaviour. A kind whose Secret changes, `KafkaUser` or
`RabbitMQBroker`, keeps writing today's Secret beside the design's until the migration period ends,
so an application moves to the new Secret at any time within it.

A renamed kind, such as `PostgreSQLCluster`, is a new kind beside today's `PostgreSQLInstance`,
since a conversion only translates between versions of one kind. Today's kind stays served, with its
function, until its last object has moved; client teams move their own objects, as follow-up work
describes.

## Classes

A class, such as a `PostgreSQLClass`, is cluster-scoped and written by Entigo for each deployment
(principle 5), and an object in any namespace may name any class of its kind; no Zone restricts the
classes its namespaces use. It holds two parts of the object's spec, each with the common fields and
one provider configuration:

| Part       | Effect                                                                                                                           |
| ---------- | -------------------------------------------------------------------------------------------------------------------------------- |
| `template` | Defaults, written into the object when it is created; the object may change them                                                 |
| `enforced` | Enforced values, applied from creation and at each sync; an object that sets a different value is refused when created or synced |

*Class parts and their effects.*

The class's provider configuration kind selects the backend, as `StorageClass.provisioner` does, so
a class has no separate field for it. A class whose provider configuration names a kind the
deployment does not install is refused when applied. A class name is the portable contract: every
deployment defines its own `standard`, and a chart that sets `className: standard` works wherever a
class of that name exists. Without `className`, the object takes the deployment's default class, the
one annotated `<group>/is-default-class: "true"`, as a default StorageClass is; where none exists,
the object is standalone.

The effective spec is built in four layers, the last winning: the implementation's defaults, then
the class's `template`, then the object with its matching provider configuration entry, then the
class's `enforced`. The implementation's defaults are fixed values its schema lists for each field,
such as `deletionProtection: true`; a setting without one, such as a disk size or a machine size,
comes from the class or the object, or the object is refused
([Provider Configurations](#provider-configurations)). A standalone object has only the
implementation's defaults and its own values. Engine parameters merge key by key in the same order,
with the class's `parametersRefs` beneath them all, each list in its own order: the class's list
stays in the class, and the platform applies it at every reconcile instead of copying it into the
object.

The defaults are written into the object by a `MutatingAdmissionPolicy`, built into Kubernetes from
version 1.36. It copies the class's `template` into the object when it is created and keeps the
values on updates. A default is written field by field, so it can break a rule that ties fields
together, such as a Valkey server's `highAvailability: true` with `readReplicas: 0`; the refusal
message then ends with a note that a value the object does not set comes from its class's template.
Observed resources receive no defaults.

An object keeps the class it was created with, and the class's values as they were then. Its
`className` is written at creation when the user leaves it out and a default class exists, so moving
the default-class annotation later moves no object, and it cannot be changed or removed afterwards;
a standalone object may add it once, as described below. The function records what it applied from
the class in the annotation `<group>/class-record`, which only the platform may write: the class's
name and generation, the `class-sync` value last applied, and the class's `enforced`,
`parametersRefs` and provider configuration. It keeps applying the recorded values when the class
changes, and the condition `ClassSynced: False` shows that the class has changed since. The record
is an annotation, not status, so it returns with the object from any backup or copy:

```yaml
---
apiVersion: database.entigo.com/v1alpha2
kind: PostgreSQLCluster
metadata:
  name: orders-db
  namespace: shop
  annotations:
    database.entigo.com/class-sync: "2026-11-20"   # written by the user: starts a sync
    database.entigo.com/class-record: |            # written by the platform only
      {
        "class": "standard",
        "generation": 4,
        "sync": "2026-11-20",
        "enforced": {
          "highAvailability": true,
          "backup": {"retentionDays": 14}
        },
        "parametersRefs": [{"name": "pg-baseline", "namespace": "platform-apis"}],
        "providerConfig": {
          "apiVersion": "aws.database.entigo.com/v1alpha1",
          "kind": "PostgreSQLRDSConfig",
          "spec": {"instanceType": "db.t4g.medium"}
        }
      }
spec:
  className: standard
```

*A `PostgreSQLCluster` with its class record after a sync.*

A user applies the class's current values with the annotation `<group>/class-sync`: each new value
of it, such as a date, starts one sync, which replaces the record. Admission compares every change to
an object with its recorded `enforced` and refuses one that sets a locked field to a different
value, naming the field; at creation and at a sync it compares with the class's current `enforced`
instead, so a class change never blocks an unrelated edit. An object whose record is missing keeps
its resources as they are, reports `ClassSynced: False` with the record missing, and waits for a
sync; it never falls back to the class's current values. A deleted class stops nothing: the object
keeps applying its recorded values, reports `ClassSynced: False` with the class missing, and a sync
waits until a class of that name exists again. The `template` is neither applied again nor
recorded, since its values are the object's own after creation.

An object without a class is standalone. Its backend is the one whose provider configuration kind
the deployment installs and one of its entries names; an object with no such entry, or with entries
for two installed backends, is refused when applied, with a message naming the kinds. Its entry sets
every setting the backend cannot run without, the implementation's defaults fill the rest, and it
has no `template`, enforced values or class record. A standalone object joins a class once, by
adding `className`. Admission refuses a class whose provider configuration names another backend
than the one the object runs on, since moving a running resource to another backend would replace
it. Adding the class applies nothing: the object reports `ClassSynced: False`, and its first sync
applies and records the class's values, refusing any that conflict with the class's `enforced`.

An object that requests a portable feature, such as `HighAvailability` or `ReadReplicas`, that its
backend does not support is refused when applied, by its implementation's own validation policy,
with a message naming the backend and the feature (principle 1).

## Provider Configurations

An object carries its provider configurations (principle 2) in `providerConfigs`, a list with one
entry per kind. Each entry names its own `apiVersion` and `kind`, and holds the settings either
inline, in `spec`, or in a separate object of that kind, through `ref`. The list is keyed by
`apiVersion` and `kind`, so Helm, Argo CD and server-side apply merge it entry by entry, and one
manifest can carry settings for several backends and be applied on any of them.

A deployment applies only the entry whose group and kind its class names, or, for a standalone
object, the entry of an installed backend, whatever the entry's version, and ignores the others. An
entry of an older version is converted to the version the class uses, so a class that moves to a
newer version still applies the entries written for the older one, and an object holds one entry per
group and kind; the status names the applied entry and the ignored ones, so a misspelled kind shows
as no entry applied. An object with no matching entry takes the class's provider configuration as it
is.

Each provider configuration kind marks the settings its backend cannot run without, such as a
machine size. An object for which neither its entry nor its class sets one is refused when applied,
with a message naming the setting. A deployment that wants every application to choose a setting,
such as its size, leaves it out of its class.

A machine size is a provider configuration setting, named as the product names it, such as
`db.m7g.large` in a `PostgreSQLRDSConfig` and `db-custom-2-8192` in a `PostgreSQLCloudSQLConfig`. A
size stated as CPU and memory would need the platform to pick the nearest size across fixed and
flexible sizes, families and availability per region. A class's default size spares users from
knowing the names.

## Status

The status (principle 4) shows the common fields as the backend actually runs them, the usual
`conditions`, the address applications connect to, and the name of the connection secret. The
address is `endpoint: { host, port }` where the service has one host, or the service's own form
where it has several.

Values that exist in only one backend, such as its resource IDs, go into `status.provider`, whose
shape the applied provider configuration kind defines. `status.providerConfig` names the applied
entry and the ignored ones, and `status.effective` shows each effective value and where it came
from: the implementation's default, the class's `template`, the object, or the class's `enforced`.
If a backend does not report a common field, that field stays empty there.

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
name their application already uses. An object that takes ownership of a resource today's API
created takes over the Secret of the same name that today's object owns, instead of reporting it, so
a migration needs no `secretName`.

## Observed Resources

To observe an existing resource (principle 9), the user sets `observe: true` and gives the
resource's ID in the provider configuration entry of the backend that holds it:

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
and a resource that fails the check is refused with `CloudAccepted: False`. A resource without a
Zone tag is read only: no kind inside it, such as a user or a database, is created on it. The
parent's status records whether the resource has a Zone tag, and the function of a kind inside it
reads that status and refuses with a condition instead of creating anything.

To take ownership of the resource, the user removes `observe` and sets the common fields in the same
edit; `id` stays in the entry. `observe` can be removed once and never set again after creation, and
only when the resource's Zone tag names the object's Zone. A field the user leaves unset at takeover
gets no class default and is not sent, so the resource keeps its value, which the status shows with
the source `cloud`. From then on the platform changes the resource to match the object, and deleting
the object deletes the resource, as for a resource the platform created.

The check holds only where users can create the platform's kinds and nothing else, so it has a
prerequisite: kcp serves no Crossplane provider kinds, and a Zone's Argo CD project and roles allow
only the platform's and Kubernetes' own groups. Today's Argo CD project does not yet limit itself to
an allowlist of groups. A user who could create a Crossplane provider's resource directly could
observe anything without the check, so the platform refuses `observe: true` until the allowlist is
in place.

## Resource Names

When the platform creates a resource, its name in the cloud includes a hash of the object's unique
ID. As a result, two objects with the same name in different namespaces never share one cloud
resource. A name that applications use, such as a Kafka topic or an image path, keeps the user's
name instead, and the platform rejects a second object that requests a name already held.

## Platform Decisions

The platform makes the settings below itself, for every service that has them, so no field sets them
(principle 2):

| Setting            | Platform's Choice                                               |
| ------------------ | --------------------------------------------------------------- |
| Network placement  | The deployment's private network                                |
| Public access      | None, unless a backend's provider configuration allows it       |
| Encryption at rest | The platform's data key, where the backend takes one            |
| Tags               | Workspace and Zone tags, on every resource the platform creates |

*Service-wide settings chosen by the platform.*

## References

A kind that exists inside another, such as a database user or a Kafka topic, names its parent in a
reference, such as `clusterRef`, `serverRef` or `brokerRef`, as `{ name, namespace }`. Without
`namespace`, the parent is in the same namespace. Such a kind has no class and no provider
configurations: it acts inside the engine, which is the same everywhere, and follows its parent's
backend. Its defaults come from its parent's class, where that class defines them, such as a
`KafkaClass`'s `topicTemplate` (principle 5). Admission cannot look up the parent, so the function
applies them when it first reconciles the object and keeps them, and the object's status shows the
values applied.

A reference to another namespace is followed only when both namespaces carry the same
`tenancy.entigo.com/zone` label (principle 11). Admission sees only the object's own namespace, so
the function checks the label: for a parent in another Zone it creates nothing and reports the
refusal in a condition.

## Validation

Provider configuration entries are opaque to the core schema (principle 10). Each implementation
checks the entry that applies on its deployment with a `ValidatingAdmissionPolicy`, and publishes
its kind's schema, so that CI checks every entry of a manifest, including those for other
deployments, before it is applied anywhere.

When the cloud rejects a value, the error appears only on the Crossplane provider's own object, not
on the user's. The platform therefore also shows it on the user's object, as the condition
`CloudAccepted: False` and as a warning event. An operation that the running backend cannot perform,
such as an upgrade its service does not offer, is reported the same way, without reaching the cloud.

## Deletion

Deletion protection is enforced by a `ValidatingAdmissionPolicy`, which refuses to delete an object
whose `deletionProtection` is true. It guards against accidental deletion: the user sets it to false
before deleting the object, so a class sets it in `template`, never in `enforced`.

Protection also covers the composed resource that holds the data, such as a DB system: deleting it
would delete the data while the object stays protected, and Crossplane would then create an empty
one in its place. While protection is on, the platform labels that resource, and a second policy
refuses to delete it for anyone but the garbage collector, which deletes it with its object.
Crossplane is not exempt, so a composition that stops composing the resource cannot delete it
either.

A namespace deleted while it holds a protected object therefore stays in deletion: the namespace
controller cannot delete the object or its data, but it does delete the object's Secrets, and the
terminating namespace accepts no new ones. Only switching protection off ends it, and that deletes
the data; a backup the cloud keeps after deletion is the way back.

Every kind is deleted in the foreground (`defaultCompositeDeletePolicy: Foreground`, as today's
PostgreSQL and MariaDB kinds are): an object stays in deletion until its composed resources are
gone, so a resource the cloud refuses to delete keeps its object, with the cloud's error on that
resource. An object that holds others is deleted only once it is empty (principle 14). Each kind
inside another, such as a user, a database, a topic or a virtual host, creates a Crossplane `Usage`
of its parent, which blocks the parent's deletion until the kind inside is gone; a `Usage` also
orders the objects inside an engine, such as a grant before the database it names. When a namespace
or an application is deleted as a whole, the kinds inside therefore go first and the parent last. A
kind inside another also holds a `Usage` of the Secret and the provider configuration it logs in
with, so they outlive it when everything is deleted at once. A bucket that holds objects is refused
by its cloud, except on Azure, whose implementation needs its own guard.
