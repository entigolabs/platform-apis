# Model and Principles

How every data service API is built, and the principles each follows.

## Model

Three roles take part. A user is a team that runs its applications in a Zone and writes data service
objects there. An operator runs an infralib deployment: its cluster and the cloud account behind it.
An implementer builds an implementation: Entigo for the clouds infralib deploys to, or anyone else
for their own environment, such as a customer's platform team.

Each data service has four parts, which the principles below rely on:

| Part            | What it is                                                                                       | Owned by                  | Example                                  |
| --------------- | ------------------------------------------------------------------------------------------------ | ------------------------- | ---------------------------------------- |
| Core kind       | What a user asks for, the same wherever it runs                                                  | the user                  | a `PostgreSQLCluster`                    |
| Implementation  | What provides a core kind on one platform: a cloud's managed service, or software in the cluster | an implementer            | RDS, Cloud SQL, CloudNativePG            |
| Provider config | The settings that exist only in one implementation, in a kind that implementation defines        | the user, or the operator | a `PostgreSQLRDSConfig` with an RDS size |
| Class           | A deployment's choice of implementation, its defaults and the values a user cannot change        | the operator              | a `PostgreSQLClass` named `standard`     |

*The parts of a data service.*

A user writes a core kind that names a class, and may add a provider config for each implementation
it can run on. Each deployment applies the provider config of its class's implementation, and the
implementation turns the result into running resources.

## Principles

These principles apply to every data service API, and each service's page shows how it applies
them.

#### 1 The common spec states needs, not products

One manifest works wherever its kind is offered: it says what is needed, such as a version or a disk
size, not which product provides it. What provides it is the kind's implementation: a cloud's
managed service, such as RDS, or software in the cluster, such as CloudNativePG. Each deployment
chooses its implementation in a class (principle 5).

A need belongs in the common spec when two or more implementations can meet it, and a user can state
it without knowing which one runs underneath, such as a version, a disk size, high availability or
backups. What only one implementation has goes into its provider config (principle 2). This keeps
the common spec from shrinking into a wrapper around provider configs: a manifest is only as
portable as its common fields.

Each class reports in its status which portable features its implementation supports, such as
`HighAvailability`, `ReadReplicas` and `DisasterRecovery`. An object that asks for a feature its
class does not report is refused when applied, with a message naming the class and the feature: a
missing standby or replica is capacity a user counted on, so it is never dropped silently. A minor
field that an implementation cannot honour is ignored there, and that implementation's mapping says
so. An operation an implementation cannot perform fails, with the reason shown on the object.

The portal reads a class's supported features and hides the fields its class cannot fulfil.

#### 2 A setting of one implementation belongs in its provider config

Some settings exist in only one implementation, such as an RDS instance class or a Cloud SQL
edition. They go into a provider config: a block of settings whose kind belongs to that
implementation, such as `PostgreSQLRDSConfig` for RDS, so the core kind never lists any
implementation's settings.

Implementations differ: RDS has instance classes and parameter groups, Cloud SQL has editions, OCI
has flexible shapes. If the core kind held them all, every new implementation would change it (rule
3).

An object carries its provider configs in `providerConfigs`, a list with one entry per kind. Each
entry names its own `apiVersion` and `kind`, and holds the settings either inline, in `spec`, or in
a separate object of that kind, through `ref`. The list is keyed by `apiVersion` and `kind`, so
Helm, Argo CD and server-side apply merge it entry by entry, and one manifest can carry settings for
several implementations and be applied on any of them.

A deployment applies only the entry whose kind its class names (principle 5) and ignores the others;
the status lists which entry was applied and which were ignored, so a misspelled kind shows as
nothing applied. An object with no matching entry takes the class's provider config as it is.

A machine size is such a setting, named as the product names it, such as `db.m7g.large` in a
`PostgreSQLRDSConfig` and `db-custom-2-8192` in a `PostgreSQLCloudSQLConfig`. A size stated as CPU
and memory would need the platform to pick the nearest size across fixed and flexible sizes,
families and availability per region. A class's default size spares users from knowing the names.

Everything that is neither a common field nor in a provider config, such as network placement and
encryption keys, is decided by the platform.

#### 3 Adding an implementation never changes the core kinds

Implementing a kind on another cloud, on-premises or by another implementer never changes the core
kinds' schemas, so manifests written before keep working.

The core schemas name no cloud and no vendor. A new implementation brings its own provider config
kind, what turns the core kind into its resources, and the classes that select it.

#### 4 The status reports what actually runs

The status shows what is running, not what the manifest asked for.

The status shows the common fields as the implementation actually runs them, the usual
`conditions`, the address applications connect to, and the name of the connection secret. The
address is `endpoint: { host, port }` where the service has one host, or the service's own form
where it has several.

Values that only make sense in one implementation, such as its resource IDs, go into
`status.provider`, whose shape the applied provider config kind defines. `status.providerConfig`
names the applied entry and the ignored ones, and `status.effective` shows each effective value and
where it came from: the class's `template`, the object, or the class's `enforced` (principle 5). If
an implementation does not report a common field, that field stays empty there, and its mapping says
so.

#### 5 A class sets the defaults and limits of a deployment

Each deployment's operator describes how it provides a kind in classes, and an object names its
class; defaults are written once, when the object is created.

A class, such as a `PostgreSQLClass`, is cluster-scoped and owned by the deployment's operator. It
holds two parts of the object's spec, each with the common fields and one provider config:

| Part       | Effect                                                                               |
| ---------- | ------------------------------------------------------------------------------------ |
| `template` | Defaults: written into the object when it is created, and the object may change them |
| `enforced` | Always applied; an object that sets a different value is refused when applied        |

*The two parts of a class.*

The class's provider config kind selects the implementation, as `StorageClass.provisioner` does, so
a class has no separate field for it. A class whose provider config names a kind the deployment does
not install is refused when applied. A class name is the portable contract: every deployment defines
its own `standard`, and a chart that says `className: standard` works wherever a class of that name
exists. Without `className`, the object takes the deployment's default class.

The effective spec is built in three layers, the last winning: the class's `template`, then the
object with its matching provider config entry, then the class's `enforced`. Engine parameters merge
key by key in the same order.

The defaults are written into the object by a `MutatingAdmissionPolicy`, built into Kubernetes from
version 1.36. It copies the class's `template` into the object when it is created and keeps the
values on updates, so a later change to a class never resizes or upgrades a running resource.
`enforced` applies all the time: it is policy, and an object that no longer complies after a class
tightens it reports that in a condition. Observed resources receive no defaults.

Deletion protection is enforced by a `ValidatingAdmissionPolicy`, which refuses to delete an object
whose `deletionProtection` is true.

#### 6 Applications connect the same way on every cloud

An application reads its connection details in the same form wherever it runs, so it moves between
clouds without changes.

Each credential is one Kubernetes Secret in the object's namespace, with the same keys everywhere:
`endpoint`, `port`, `username` and `password`. `port` is omitted where `endpoint` already lists
hosts with their ports. A service that needs more adds its own keys, such as `readerEndpoint` for
read replicas. No key is named after a cloud, and no key holds the others as JSON.

The secrets are named `<instance>-dbadmin` for a database's admin, `<instance>-credentials` for
other services, and `<instance>-<user>` for a user object. Every kind with a secret takes an
optional `secretName` to override that name. The platform never overwrites a Secret that another
object or person owns: joined names can collide, as a user named `dbadmin` with the admin's secret,
or instance `a-b` with user `c` and instance `a` with user `b-c`, so the second object reports which
object holds the name, and `secretName` resolves it. It also allows a user to use a name its
application already expects.

#### 7 A kind exists only where an implementation runs the same engine

A service is offered on a deployment only if an implementation there runs the same engine; no other
engine stands in for it.

A kind is installed on a deployment only if an implementation runs its engine there. On other
deployments the kind does not exist, and the portal does not show the service. A different engine is
never substituted: MySQL is not MariaDB.

The `platform-apis` chart holds this knowledge: a deployment passes only its cloud, and the chart
installs each service's core kinds, its provider config kinds and its classes where the service is
implemented. A per-service switch lets an operator leave a service out, never add one that the
deployment lacks. A manifest of a kind that is not installed is rejected by the Kubernetes API
server when it is applied, before any function runs.

#### 8 A kind's name says what it is and who owns it

A user-facing kind is named after its engine or concept, never a product; a provider config kind
names its product.

A product name ties a user-facing kind to one cloud: `S3Bucket` reads as Amazon S3 even where the
bucket is in Cloud Storage. A user-facing kind therefore takes the engine's own term, as
`PostgreSQLCluster` and `KafkaTopic` do, or, where no engine names it, the concept every cloud
shares, as `Bucket` and `ArtifactRepository` do.

The kinds around it follow from that name:

| Kind            | Name                      | Group                     | Example                                            |
| --------------- | ------------------------- | ------------------------- | -------------------------------------------------- |
| Class           | `<Engine>Class`           | the core group            | `PostgreSQLClass` in `database.entigo.com`         |
| Parameters      | `<Engine>Parameters`      | the core group            | `PostgreSQLParameters` in `database.entigo.com`    |
| Provider config | `<Engine><Product>Config` | `<platform>.<core group>` | `PostgreSQLRDSConfig` in `aws.database.entigo.com` |

*Names of the kinds around a user-facing kind.*

A provider config is specific to one implementation by definition, so naming the product is
accurate. The engine comes first, so the kinds sort together. An on-premises implementation is
grouped by the platform it runs on, `kubernetes`, with the product in the kind, as
`PostgreSQLCNPGConfig` for CloudNativePG. An implementer other than Entigo uses its own domain, so
names never collide. Each provider config kind versions on its own.

#### 9 A kind either creates its resource or observes an existing one

A kind either creates its resource or, with `observe: true`, only reads one that already exists.

This applies to every kind that represents a cloud resource, such as a database or a bucket. To
observe, the user sets `observe: true` and gives the resource's ID in the provider config entry of
the implementation that holds it:

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

The ID is one value the user copies from the cloud's console, and each service's page says which
one. The platform turns it into whatever the provider needs. An observed resource is never changed
or deleted by the platform, and takes no other spec fields.

A resource can be observed only when its workspace tag (a label on GCP) names this cluster's
workspace, and only from a namespace of the Zone its Zone tag names, or from any Zone when it has no
Zone tag. Its owner sets the tags in the cloud. The platform reads them before it uses the resource,
and a resource that fails the check is refused with `CloudAccepted: False`. The same check lets a
disaster recovery workspace find the resource it replicates, from the workspaces the object's
`disasterRecovery` names.

The check holds only because users can create the platform's kinds and nothing else: kcp serves no
provider kinds, and a Zone's Argo CD project and roles allow only the platform's and Kubernetes' own
groups. A user who could create a provider's resource directly could observe anything without the
check.

When the platform creates a resource, its name in the cloud includes a hash of the object's unique
ID. As a result, two objects with the same name in different namespaces never share one cloud
resource. A name that applications use, such as a Kafka topic or an image path, keeps the user's
name instead, and the platform rejects a second object that asks for a name already held.

#### 10 The schema checks only what the API defines

The core schema checks the API's own rules only; each implementation checks its provider config,
and each cloud enforces its own value limits.

The core schema checks only what the API itself knows: field types, fields that cannot change,
fields that cannot be combined, and the API's own minimums, such as a disk that cannot shrink. It
does not copy the limits each cloud has for values, such as how many days of backups it keeps. Those
limits change, and a Kubernetes schema rule can never be loosened once released. The cloud rejects
such values itself, and each service's page lists the known limits under Cloud Limits.

Provider config entries are opaque to the core schema. Each implementation checks the entry that
applies on its deployment with a `ValidatingAdmissionPolicy`, and publishes its kind's schema, so
that CI checks every entry of a manifest, including those for other deployments, before it is
applied anywhere.

When the cloud rejects a value, the error appears only on the provider's own object, not on the
user's. The platform therefore also shows it on the user's object, as the condition `CloudAccepted:
False` and as a warning event. An operation that the running implementation cannot perform, such as
an upgrade its service does not offer, is reported the same way, without reaching the cloud.

#### 11 A reference crosses namespaces only within one Zone

An object may refer to one in another namespace, but only within the same Zone.

A kind that exists inside another, such as a database user or a Kafka topic, names its parent in a
reference, such as `clusterRef`, `serverRef` or `brokerRef`, as `{ name, namespace }`. Without
`namespace`, the parent is in the same namespace. Such a kind has no class and no provider configs:
it acts inside the engine, which is the same everywhere, and follows its parent's implementation.

A reference may point to another namespace only within the same Zone, meaning both namespaces carry
the same `tenancy.entigo.com/zone` label. Creating a user gives power over the parent, so it stays
within the user's boundary, which the Zone already defines.

#### 12 A user object holds its own permissions

Each user object is one application's login on one instance, and holds everything that login may do.

A user is one application's identity on one instance of the engine: its login, its Secret and its
permissions there, in whatever form the engine has them, such as ACLs or privileges. It is not
shared between applications or instances; access for people is not covered by these kinds. A user's
access is therefore read from one object. A set of permissions shared by several users exists only
where the engine has one, such as PostgreSQL's roles. The platform adds no permission mechanism that
no engine has.

#### 13 A field group is a unit or a subsystem

Fields are grouped only when they work as a unit or form a subsystem, never for tidiness.

A unit is a set of values that mean nothing apart, such as `maintenanceWindow: { day, hourUTC }`,
`restore: { fromInstance, pointInTime }` or one lifecycle rule. A subsystem is an area that
operators and classes treat as a whole, such as `storage` and `backup`, as CloudNativePG, Crunchy
and Zalando group them. Any other field stays at the top level, such as `autoMinorVersionUpgrade`.
