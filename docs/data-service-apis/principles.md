# Model & Principles

Every data service API is built from the parts in Model and follows the principles after them.

## Model

Each data service is built from four parts:

| Part                   | Definition                                                                                                       | Owned by | Example                                  |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------- | -------- | ---------------------------------------- |
| Core kind              | A portable API type a user writes, the same wherever it runs                                                     | the user | a `PostgreSQLCluster`                    |
| Implementation         | The managed service or in-cluster software that implements a core kind on one platform                           | Entigo   | RDS, Cloud SQL, CloudNativePG            |
| Provider configuration | The settings that exist only in one implementation, in a provider configuration kind that implementation defines | the user | a `PostgreSQLRDSConfig` with an RDS size |
| Class                  | A deployment's choice of implementation, its defaults and the values a user cannot change                        | Entigo   | a `PostgreSQLClass` named `standard`     |

*Data service parts and their owners.*

A user writes a core kind that names a class, and may add a provider configuration for each
implementation it can run on. Each deployment applies the provider configuration of its class's
implementation, and the implementation turns the result into running resources.

Two terms sound alike. A Crossplane provider, such as provider-upjet-aws, is the controller that
creates an implementation's cloud resources. A provider configuration kind, such as
`PostgreSQLRDSConfig`, is an API type of this design; it is not the `ProviderConfig` a Crossplane
provider uses for its credentials.

## Principles

Each principle states a rule every data service API follows, and the reason for it.

#### 1 The common spec holds requirements, not products

A manifest names requirements, such as a version or a disk size, and no product, so it applies on
every deployment that offers its kind.

The product or software that meets the requirements is the kind's implementation: a cloud's managed
service, such as RDS, or software in the cluster, such as CloudNativePG. Each deployment chooses its
implementation in a class (principle 5).

A requirement belongs in the common spec when two or more implementations can meet it, and a user
can set it without knowledge of the implementation, such as a version, a disk size, high
availability or backups. A setting that only one implementation has belongs in its provider
configuration (principle 2). This rule prevents the common spec from becoming a thin wrapper around
provider configurations: a manifest is only as portable as its common fields.

A portable feature that an implementation cannot provide, such as a standby or a read replica, is
refused, never dropped silently, because the user's applications rely on that capacity. A minor
field that an implementation cannot honour is ignored there. An operation an implementation cannot
perform fails, with the reason shown on the object. A need becomes a portable feature by the same
rule as a common field: once two or more implementations support it, in a revision of the design.

#### 2 A setting of one implementation belongs in its provider configuration

A setting that exists in only one implementation, such as an RDS instance class, belongs in that
implementation's provider configuration, never in the core kind.

A provider configuration is a block of settings whose kind belongs to one implementation, such as
`PostgreSQLRDSConfig` for RDS. Implementations differ: RDS has instance classes and parameter
groups, Cloud SQL has editions, OCI has flexible shapes. If the core kind held them all, every new
implementation would change it (principle 3).

Settings that are neither common fields nor provider configuration settings, such as network
placement and encryption keys, are decided by the platform.

#### 3 Adding an implementation never changes the core kinds

Implementing a kind on another cloud, on-premises or outside Entigo never changes the core kinds'
schemas, so existing manifests remain valid.

The core schemas name no cloud and no vendor. A new implementation brings its own provider
configuration kind, the composition logic that creates the core kind's resources, and the classes
that select it.

#### 4 The status reports the resource's actual values

The status reports the values the resource actually has, such as its version, disk size and address,
not the values the manifest requested.

The two can differ: a class adds its defaults and enforced values, an implementation applies minor
versions in its maintenance window, and a disk grows with `autoGrow`. A user can rely only on the
actual values. Values that exist in only one implementation, such as its resource IDs, are kept
apart from the common fields, as that implementation's settings are (principle 2).

#### 5 A class sets the defaults and limits of a deployment

Entigo defines each deployment's implementation, defaults and limits for a kind in classes, and an
object names its class; defaults are written into the object when it is created.

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

On other deployments the kind is not installed, and the portal does not show the service. A similar
engine is never substituted: MySQL is not MariaDB.

#### 8 A kind is named after its engine, and a provider configuration after its product

A core kind takes the name of its engine or concept, never of a product; a provider configuration
kind takes the name of its product.

A product name ties a core kind to one cloud: `S3Bucket` reads as Amazon S3 even where the bucket is
in Cloud Storage. A core kind therefore takes the engine's own term, as `PostgreSQLCluster` and
`KafkaTopic` do, or, where the engine has no term for it, the concept every cloud shares, as
`Bucket` and `ArtifactRepository` do. A provider configuration is specific to one implementation by
definition, so naming the product is accurate.

#### 9 A kind either creates its resource or observes an existing one

A kind either creates its resource or, with `observe: true`, only reads one that already exists.

This applies to every kind that represents a cloud resource, such as a database or a bucket. An
observed resource is never changed or deleted by the platform, so a resource created outside it is
used without transferring its ownership to the platform. An observed object takes ownership of its
resource when the user removes `observe`, once: the platform then manages the resource as one it
created.

#### 10 The core schema checks only the API's own rules

The core schema checks the API's own rules; each implementation checks its provider configuration,
and each cloud enforces its own value limits.

The core schema checks only the rules the API itself defines: field types, fields that cannot
change, fields that cannot be combined, and the API's own minimums, such as a disk that cannot
shrink. It does not copy the limits each cloud sets on values, such as the longest backup retention.
Those limits change, and a Kubernetes schema rule can never be loosened once released. The cloud
rejects such values itself.

#### 11 A reference crosses namespaces only within one Zone

An object may refer to one in another namespace, but only within the same Zone.

A user object grants access to its parent, so a reference stays within the boundary the Zone
already defines for its owner.

#### 12 A user object holds its own permissions

Each user object is one application's login on one instance, and holds every permission of that
login.

A user is one application's identity on one instance of the engine: its login, its Secret and its
permissions there, in the engine's own form, such as ACLs or privileges. It is not shared between
applications or instances, and human access is outside these kinds. A user's access is therefore
read from one object. A set of permissions shared by several users exists only where the engine has
one, such as PostgreSQL's roles. The platform adds no permission mechanism that no engine has.

#### 13 A field group is a unit or a subsystem

Fields are grouped only when they work as a unit or form a subsystem, never for tidiness.

A unit is a set of values without meaning on their own, such as
`maintenanceWindow: { day, hourUTC }`, `restore: { fromInstance, pointInTime }` or one lifecycle
rule. A subsystem is an area that classes treat as a whole, such as `storage` and `backup`, as
CloudNativePG, Crunchy and Zalando group them. Any other field stays at the top level, such as
`autoMinorVersionUpgrade`.
