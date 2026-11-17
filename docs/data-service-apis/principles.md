# Model and Principles

Every data service API is built from the parts in Model and follows the principles after them.
[Common Design](common-design.md) describes the mechanisms that apply the principles.

## Model

Each data service has four parts, which the principles below rely on:

| Part            | Definition                                                                                       | Owned by       | Example                                  |
| --------------- | ------------------------------------------------------------------------------------------------ | -------------- | ---------------------------------------- |
| Core kind       | The resource a user requests, the same wherever it runs                                          | the user       | a `PostgreSQLCluster`                    |
| Implementation  | The provider of a core kind on one platform: a cloud's managed service, or software in a cluster | Entigo         | RDS, Cloud SQL, CloudNativePG            |
| Provider config | The settings that exist only in one implementation, in a kind that implementation defines        | the user       | a `PostgreSQLRDSConfig` with an RDS size |
| Class           | A deployment's choice of implementation, its defaults and the values a user cannot change        | Entigo         | a `PostgreSQLClass` named `standard`     |

*The parts of a data service.*

A user writes a core kind that names a class, and may add a provider config for each implementation
it can run on. Each deployment applies the provider config of its class's implementation, and the
implementation turns the result into running resources.

## Principles

Each principle states a rule every data service API follows, and the reason for it.

#### 1 The common spec states needs, not products

One manifest works wherever its kind is offered: it says what is needed, such as a version or a disk
size, not which product provides it. The product or software that provides it is the kind's
implementation: a cloud's managed service, such as RDS, or software in the cluster, such as
CloudNativePG. Each deployment chooses its implementation in a class (principle 5).

A need belongs in the common spec when two or more implementations can meet it, and a user can state
it without knowing which one runs underneath, such as a version, a disk size, high availability or
backups. A setting that only one implementation has goes into its provider config (principle 2).
This keeps the common spec from shrinking into a wrapper around provider configs: a manifest is only
as portable as its common fields.

A portable feature that an implementation cannot provide, such as a standby or a read replica, is
refused, never dropped silently: it is capacity a user counted on. A minor field that an
implementation cannot honour is ignored there, and that implementation's mapping says so. An
operation an implementation cannot perform fails, with the reason shown on the object.

#### 2 A setting of one implementation belongs in its provider config

Some settings exist in only one implementation, such as an RDS instance class or a Cloud SQL
edition. They go into a provider config: a block of settings whose kind belongs to that
implementation, such as `PostgreSQLRDSConfig` for RDS, so the core kind never lists any
implementation's settings.

Implementations differ: RDS has instance classes and parameter groups, Cloud SQL has editions, OCI
has flexible shapes. If the core kind held them all, every new implementation would change it
(principle 3).

Everything that is neither a common field nor in a provider config, such as network placement and
encryption keys, is decided by the platform.

#### 3 Adding an implementation never changes the core kinds

Implementing a kind on another cloud, on-premises or outside Entigo never changes the core kinds'
schemas, so manifests written before keep working.

The core schemas name no cloud and no vendor. A new implementation brings its own provider config
kind, what turns the core kind into its resources, and the classes that select it.

#### 4 The status reports the resource's actual values

The status reports the values the resource actually has, such as its version, disk size and address,
not the values the manifest requested.

The two can differ: a class adds its defaults and enforced values, an implementation applies minor
versions in its maintenance window, and a disk grows with `autoGrow`. Only the actual values tell a
user what they have. Values that exist in only one implementation, such as its resource IDs, are kept
apart from the common fields, as its settings are (principle 2).

#### 5 A class sets the defaults and limits of a deployment

Entigo describes how each deployment provides a kind in classes, and an object names its class;
defaults are written once, when the object is created.

A deployment's choices, such as its implementation, default sizes and the values no user may change,
are written once per deployment instead of in every manifest. Defaults written at creation stay in
the object, so a later change to a class never resizes or upgrades a running resource.

#### 6 Applications connect the same way on every cloud

An application reads its connection details in the same form wherever it runs, so it moves between
clouds without changes.

The connection details have the same keys on every implementation, and no key is named after a
cloud.

#### 7 A kind exists only where an implementation runs the same engine

A service is offered on a deployment only if an implementation there runs the same engine; no other
engine stands in for it.

A kind is installed on a deployment only if an implementation runs its engine there. On other
deployments the kind does not exist, and the portal does not show the service. A different engine is
never substituted: MySQL is not MariaDB.

#### 8 A kind's name says what it is and who owns it

A user-facing kind is named after its engine or concept, never a product; a provider config kind
names its product.

A product name ties a user-facing kind to one cloud: `S3Bucket` reads as Amazon S3 even where the
bucket is in Cloud Storage. A user-facing kind therefore takes the engine's own term, as
`PostgreSQLCluster` and `KafkaTopic` do, or, where no engine names it, the concept every cloud
shares, as `Bucket` and `ArtifactRepository` do. A provider config is specific to one implementation
by definition, so naming the product is accurate.

#### 9 A kind either creates its resource or observes an existing one

A kind either creates its resource or, with `observe: true`, only reads one that already exists.

This applies to every kind that represents a cloud resource, such as a database or a bucket. An
observed resource is never changed or deleted by the platform, so a resource created outside it can
be used without handing it over.

#### 10 The schema checks only what the API defines

The core schema checks the API's own rules only; each implementation checks its provider config,
and each cloud enforces its own value limits.

The core schema checks only what the API itself knows: field types, fields that cannot change,
fields that cannot be combined, and the API's own minimums, such as a disk that cannot shrink. It
does not copy the limits each cloud has for values, such as how many days of backups it keeps. Those
limits change, and a Kubernetes schema rule can never be loosened once released. The cloud rejects
such values itself, and each service's page lists the known limits under Cloud Limits.

#### 11 A reference crosses namespaces only within one Zone

An object may refer to one in another namespace, but only within the same Zone.

Creating a user gives power over the parent, so it stays within the user's boundary, which the Zone
already defines.

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
`restore: { fromInstance, pointInTime }` or one lifecycle rule. A subsystem is an area that classes
treat as a whole, such as `storage` and `backup`, as CloudNativePG, Crunchy and Zalando group them.
Any other field stays at the top level, such as `autoMinorVersionUpgrade`.
