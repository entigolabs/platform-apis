# Common Design

Every data service API uses the mechanisms on this page, each of which applies a principle from
[Principles](principles.md).

## Naming

A user-facing kind is named after its engine or concept (principle 8), and the kinds around it take
their names from it:

| Kind                   | Name                      | Group                     | Example                                            |
| ---------------------- | ------------------------- | ------------------------- | -------------------------------------------------- |
| Class                  | `<Engine>Class`           | the core group            | `PostgreSQLClass` in `database.entigo.com`         |
| Parameters             | `<Engine>Parameters`      | the core group            | `PostgreSQLParameters` in `database.entigo.com`    |
| Provider configuration | `<Engine><Product>Config` | `<platform>.<core group>` | `PostgreSQLRDSConfig` in `aws.database.entigo.com` |

*Names of the kinds around a user-facing kind.*

The engine comes first, so the kinds sort together. The product is the implementation's own short
name, such as RDS, Cloud SQL or MSK; where it has none, the platform's name stands in for it, as in
`PostgreSQLOCIConfig` for OCI Database with PostgreSQL. An on-premises implementation is grouped by
the platform it runs on, `kubernetes`, with the product in the kind, as `PostgreSQLCNPGConfig` for
CloudNativePG. An implementation built outside Entigo uses its own domain, so names never collide.
Each provider configuration kind versions on its own.

## Installation

The `platform-apis` chart determines which kinds a deployment installs (principle 7). A deployment
passes only its cloud, and the chart installs each service's core kinds, its provider configuration
kinds and its classes where the service is implemented. A per-service switch leaves a service out,
never adds one that the deployment lacks. A manifest of a kind that is not installed is rejected by
the Kubernetes API server when it is applied, before any function runs.

## Classes

A class, such as a `PostgreSQLClass`, is cluster-scoped and written by Entigo for each deployment
(principle 5). It holds two parts of the object's spec, each with the common fields and one provider
configuration:

| Part       | Effect                                                                               |
| ---------- | ------------------------------------------------------------------------------------ |
| `template` | Defaults: written into the object when it is created, and the object may change them |
| `enforced` | Always applied; an object that sets a different value is refused when applied        |

*The two parts of a class.*

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
`HighAvailability`, `ReadReplicas` and `DisasterRecovery`. An object that requests a feature its
class does not report is refused when applied, with a message naming the class and the feature
(principle 1). The portal reads a class's supported features and hides the fields its class cannot
fulfil.

## Provider Configurations

An object carries its provider configurations (principle 2) in `providerConfigs`, a list with one
entry per kind. Each entry names its own `apiVersion` and `kind`, and holds the settings either
inline, in `spec`, or in a separate object of that kind, through `ref`. The list is keyed by
`apiVersion` and `kind`, so Helm, Argo CD and server-side apply merge it entry by entry, and one
manifest can carry settings for several implementations and be applied on any of them.

A deployment applies only the entry whose kind its class names and ignores the others; the status
lists which entry was applied and which were ignored, so a misspelled kind shows as nothing applied.
An object with no matching entry takes the class's provider configuration as it is.

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

Values that only make sense in one implementation, such as its resource IDs, go into
`status.provider`, whose shape the applied provider configuration kind defines.
`status.providerConfig` names the applied entry and the ignored ones, and `status.effective` shows
each effective value and where it came from: the class's `template`, the object, or the class's
`enforced`. If an implementation does not report a common field, that field stays empty there.

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
object holds the name, and `secretName` resolves it. It also allows a user to keep a name
its application already uses.

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

*An observed resource on AWS.*

The ID is one value the user copies from the cloud's console. The platform converts it into the form
the provider takes. An observed resource takes no other spec fields.

A resource can be observed only when its workspace tag (a label on GCP) names this cluster's
workspace, and only from a namespace of the Zone its Zone tag names, or from any Zone when it has no
Zone tag. Its owner sets the tags in the cloud. The platform reads them before it uses the resource,
and a resource that fails the check is refused with `CloudAccepted: False`. The same check allows a
disaster recovery workspace to locate the resource it replicates, from the workspaces the object's
`disasterRecovery` names.

The check holds only because users can create the platform's kinds and nothing else: kcp serves no
provider kinds, and a Zone's Argo CD project and roles allow only the platform's and Kubernetes' own
groups. A user who could create a provider's resource directly could observe anything without the
check.

## Resource Names

When the platform creates a resource, its name in the cloud includes a hash of the object's unique
ID. As a result, two objects with the same name in different namespaces never share one cloud
resource. A name that applications use, such as a Kafka topic or an image path, keeps the user's
name instead, and the platform rejects a second object that requests a name already held.

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

## Read Replicas

`readReplicas: N` requests N read-only copies of a database, identical to the primary: the same
size, the same engine parameters and the same class layers. Applications send reads to them through
the secret's `readerEndpoint` key. Replicas of another size, such as one large replica for
reporting, exist only in some implementations, so they are written in the provider configuration
instead of the count, each with its own size and, where the implementation allows it, its own engine
parameters.

The rules for replicas:

- **The count or the layout, not both.** Where an implementation offers a replica layout, an object
  that sets `readReplicas` and a layout in its applied provider configuration is refused, so the
  number of replicas is never ambiguous.
- **Scaling down is predictable.** Where each replica is a resource of its own, replicas are named
  `<instance>-ro-<index>`, and scaling down removes the highest index first.
- **The reader endpoint follows the implementation.** Where it has a balanced read endpoint,
  `readerEndpoint` holds it; elsewhere it lists the replicas' hosts.
- **A replica runs the primary's major version.** A replica copies the primary through the engine's
  own replication, which needs the same major version; a copy on a newer version belongs to upgrades
  and migration.

## Disaster Recovery

`disasterRecovery` keeps a copy of a database in another Entigo workspace, usually in another
region, that can become the primary when the primary's region is lost. The same manifest is applied
in both workspaces, and each takes its role from `primary`, which names one of `workspaces`. With
`workspaces: [prod, prod-dr]` and `primary: prod`:

1. In `prod`, the platform creates the primary as usual.
2. In `prod-dr`, that workspace's own platform creates a replica of it in its own region. The DR
   workspace owns the replica, so losing the primary's region does not also take down the platform
   that must promote it.
3. To fail over, the user changes `primary` to `prod-dr` in Git, and `prod-dr`'s platform promotes
   its replica.

The manifest names no region: each workspace's class determines where its copy runs. A replica that
only waits should be small, so a class in the DR workspace can hold a `standby` provider
configuration, applied only while that workspace is not primary; on promotion, the platform resizes
the replica to the object's own size, which adds that time to the recovery. Where recovery time
matters more than cost, the class leaves `standby` empty.

The rules for disaster recovery:

- **The source is found by tags.** The DR platform cannot read the primary's resource, and cloud
  names differ between clusters, so the primary platform tags the resource with the workspace, the
  Zone and a DR group (`<namespace>/<name>`), and the DR platform finds it with the tag check of
  [Observed Resources](#observed-resources).
- **Credentials survive the outage.** The primary platform keeps the generated passwords in a store
  that replicates across regions, such as AWS Secrets Manager replica secrets, which the DR side
  reads through External Secrets. An encrypted replica also needs a key in the DR region.
- **The old primary is fenced.** When the old primary's workspace returns, its platform reads from
  Git that it is no longer primary, stops writes, and rebuilds the database as a replica, before
  anything else.
- **Each workspace's secret holds its local endpoint,** so applications in the DR region connect
  locally.
- **The feature gate applies.** A class that cannot replicate across regions does not report
  `DisasterRecovery`, so a manifest that uses it fails when applied, not during an outage.

## Validation

Provider configuration entries are opaque to the core schema (principle 10). Each implementation
checks the entry that applies on its deployment with a `ValidatingAdmissionPolicy`, and publishes
its kind's schema, so that CI checks every entry of a manifest, including those for other
deployments, before it is applied anywhere.

When the cloud rejects a value, the error appears only on the provider's own object, not on the
user's. The platform therefore also shows it on the user's object, as the condition
`CloudAccepted: False` and as a warning event. An operation that the running implementation cannot
perform, such as an upgrade its service does not offer, is reported the same way, without reaching
the cloud.

Deletion protection is enforced by a `ValidatingAdmissionPolicy`, which refuses to delete an object
whose `deletionProtection` is true.
