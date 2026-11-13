# Design Rules

The rules every data service API follows, and the alternatives weighed against them.

## Rules

These rules apply to every data service API, and each service's page shows how it applies them. The
draft schemas are in [schemas/](schemas/).

#### 1 The common spec states needs, not cloud products

One manifest works on every cloud where its kind exists: it says what is needed, such as a version
or a disk size, not which cloud product provides it.

A setting that a cloud lacks is ignored there, as `storage.iops` is on Azure, and everything
cloud-specific belongs in the cloud blocks (rule 2). An operation that a cloud cannot perform, such
as a major version upgrade on OCI, fails instead (rule 10).

The common spec holds needs such as a version, a size, high availability and backups. A field
belongs there when a user can choose it without knowing which cloud runs underneath. Where no fair
cloud-neutral value exists, the setting goes into the cloud blocks instead: a size, for example, is
`aws.instanceType` on AWS and `oci.instanceType` with `oci.ocpus` on OCI.

The fields are checked against the managed services of every cloud infralib deploys to (AWS, GCP,
OCI and Azure), even where no Crossplane provider supports the service yet. As a result, a cloud
that receives a provider later finds its concepts already in the API.

Not every cloud supports every setting, so each field has a support level. The level can also apply
to a single value of a field, such as `storage.autoGrow: false`, which OCI ignores because its
storage always grows.

| Level   | Meaning                                                                                                                                                |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Core    | Works on every cloud.                                                                                                                                  |
| Ignored | Accepted on every cloud, but has no effect on the clouds named in the field's description, which lack it. The resource is created without the setting. |

In a schema, a field's description ends with `Ignored on` and the clouds that lack it; each
service's Mapping table names the same clouds in its Level column. A field without that mark is
Core.

#### 2 A setting of one cloud belongs in that cloud's block

Anything that exists on only one cloud goes into that cloud's block, such as `spec.aws`.

The blocks are `spec.aws`, `spec.gcp`, `spec.oci` and `spec.azure`. For example, the name of an
existing RDS parameter group goes into `spec.aws`. Each block has its own typed fields, so the
schema can check them. A deployment uses the block of the cloud it runs on and ignores the others,
so one manifest can carry settings for several clouds and be applied on any of them. Only one block
may hold an `id` for observing (rule 9), since a resource exists on one cloud.

Everything that is neither a common field nor in a cloud's block, such as network placement and
encryption keys, is decided by the platform.

#### 3 A new cloud adds its blocks and never changes common fields

Adding a cloud only adds its blocks, so manifests written before keep working.

When a cloud's implementation is added, the schema gains that cloud's blocks, `spec.<cloud>` and
`status.<cloud>`; the common fields never change.

#### 4 The status reports what the cloud actually runs

The status shows what is running on the cloud, not what the manifest asked for.

The status shows the common fields as the cloud actually runs them, the usual `conditions`, the
address applications connect to, and the name of the connection secret. The address is
`endpoint: { host, port }` where the service has one host, or the service's own form, such as
`bootstrapServers` for Kafka.

Values that only make sense on one cloud, such as its resource IDs, go into `status.<cloud>`. If a
cloud's provider does not report a common field, that field stays empty on that cloud, and the
service's page says so.

#### 5 Defaults are written once, when the object is created

A field left out gets its default once, at creation, and the platform never changes it afterwards.

Most fields are optional, and their defaults are of two kinds:

| Kind               | Set By                                                 | Example                       |
| ------------------ | ------------------------------------------------------ | ----------------------------- |
| Fixed              | the platform, the same on every deployment             | `deletionProtection: true`    |
| Deployment default | the deployment's operator, per cloud, through infralib | `version`, `aws.instanceType` |

Safety settings, such as deletion protection, are fixed. Cost, size and availability are
deployment defaults, because only the operator can judge what suits their cloud and budget. In a
schema, the field's description ends with `Default: fixed` or `Default: per deployment`.

The defaults are written into the object by a `MutatingAdmissionPolicy`, which is built into
Kubernetes from version 1.36. It fills them in when the object is created and keeps them on
updates, so a default is applied only once: if the operator later changes a default, running
databases are not resized or upgraded. Defaults were tested to survive later updates from the
portal and from Argo CD. Observed resources (rule 9) receive no defaults.

Deletion protection works as today: a `ValidatingAdmissionPolicy` refuses to delete an object
whose `deletionProtection` is true.

#### 6 Each credential is one Secret with the same keys on every cloud

Applications read every credential from one Secret with the same keys, `endpoint`, `port`,
`username` and `password`, on every cloud.

Each credential is one Kubernetes Secret in the object's namespace, with the same keys on every
cloud: `endpoint`, `port`, `username` and `password`. `port` is omitted where `endpoint` already
contains it, as in a Kafka bootstrap list. A service that needs more adds its own keys, such as
`readerEndpoint` for Valkey. No key is named after a cloud, and no key holds the others as JSON.

The secrets are named `<instance>-dbadmin` for a database's admin, `<instance>-credentials` for
other services, and `<instance>-<user>` for a user object, as today. Every kind with a secret takes
an optional `secretName` to override that name. The platform never overwrites a Secret that another
object or person owns: joined names can collide, as a user named `dbadmin` with the admin's secret,
or instance `a-b` with user `c` and instance `a` with user `b-c`, so the second object reports which
object holds the name, and `secretName` resolves it. It also allows a team to use a name its
application already expects.

#### 7 A kind exists only where a cloud runs the same engine

A service is offered on a cloud only if that cloud runs the same engine as a managed service; no
other engine stands in for it.

An API is installed on a cloud only if that cloud has a managed service running the same engine,
and its Crossplane provider can create it. On other clouds the kind does not exist, and the portal
does not show the service. A different engine is never substituted: MySQL is not MariaDB.

| Service  | AWS         | GCP                                    | OCI                             | Azure          |
| -------- | ----------- | -------------------------------------- | ------------------------------- | -------------- |
| MariaDB  | RDS         | none                                   | none                            | none (retired) |
| Valkey   | ElastiCache | Memorystore, but no password login yet | OCI Cache                       | none           |
| Kafka    | MSK         | Managed Kafka, but no provider support | OCI Streaming with Apache Kafka | none           |
| RabbitMQ | Amazon MQ   | none                                   | none                            | none           |

PostgreSQL, buckets and artifact repositories exist on every cloud.

#### 8 A kind is named after the engine or concept, never a cloud product

Kinds are named for what they are, such as `Bucket`, never after a product such as S3.

Four kinds are renamed: `S3Bucket` becomes `Bucket`, `MSK` becomes `KafkaCluster`, `Repository`
becomes `ArtifactRepository`, and `Topic` becomes `KafkaTopic`.

#### 9 A kind either creates its resource or observes an existing one

A kind either creates its cloud resource or, with `observe: true`, only reads one that already
exists.

This applies to every kind that represents a cloud resource, such as a database instance or a
bucket. To observe, the user sets `observe: true` and gives the resource's ID in the cloud's block:

```yaml
spec:
  observe: true
  aws:
    id: db-ABC123DEFGHIJ               # copied from the RDS console's Resource ID
```

The ID is one value the user copies from the cloud's console, and each service's page says which
one. The platform turns it into whatever the provider needs. An observed resource is never changed
or deleted by the platform, and takes no other spec fields.

A resource can be observed only when its workspace tag (a label on GCP) names this cluster's
workspace, and only from a namespace of the Zone its Zone tag names, or from any Zone when it has no
Zone tag. Its owner sets the tags in the cloud. The platform reads them before it uses the resource,
and a resource that fails the check is refused with `CloudAccepted: False`.

The check holds only because teams can create the platform's kinds and nothing else: kcp serves no
provider kinds, and a Zone's Argo CD project and roles allow only the platform's and Kubernetes' own
groups. A team that could create a provider's resource directly could observe anything without the
check.

When the platform creates a resource, its name in the cloud includes a hash of the object's
unique ID. As a result, two objects with the same name in different namespaces never share one
cloud resource.

#### 10 The schema checks only what the API defines

The schema checks the API's own rules only; each cloud enforces its own value limits.

The schema checks only what the API itself knows: field types, fields that cannot change, fields
that cannot be combined, and the API's own minimums, such as a disk that cannot shrink. It does not
copy the limits each cloud has for values, such as how many days of backups it keeps. Those limits
change, and a Kubernetes schema rule can never be loosened once released. The cloud rejects such
values itself, and each service's page lists the known limits under Cloud Limits.

When the cloud rejects a value, the error appears only on the provider's own object, not on the
user's. The platform therefore also shows it on the user's object, as the condition `CloudAccepted:
False` and as a warning event. An operation that the running cloud cannot perform, such as a major
version upgrade on OCI or observing a repository on Azure, is reported the same way, without
reaching the cloud.

#### 11 A reference crosses namespaces only within one Zone

An object may refer to one in another namespace, but only within the same Zone.

A kind that exists inside another, such as a database user or a Kafka topic, names its parent in
`instanceRef` or `clusterRef`, as `{ name, namespace }`. Without `namespace`, the parent is in the
same namespace.

A reference may point to another namespace only within the same Zone, meaning both namespaces carry
the same `tenancy.entigo.com/zone` label. Creating a user gives power over the parent, so it stays
within the team's boundary, which the Zone already defines.

#### 12 A user object holds its own permissions

Each user object is one application's login on one instance, and holds everything that login may do.

A user is one application's identity on one instance of the engine: its login, its Secret and its
permissions there. It is not shared between applications or instances; access for people is not
covered by these kinds. `KafkaUser` holds its ACLs, `MariaDBUser` its privileges and `RabbitMQUser`
its permissions per virtual host, so a user's access is read from one object. A set of permissions
shared by several users exists only where the engine has one, as roles: a `PostgreSQLUser` with
`login: false` is a role, and `grant.roles` grants it. The platform adds no permission mechanism
that no engine has.

## Alternatives Considered

Designs that were weighed and rejected, each with the reason.

#### 1 Users write every field

The user writes every field, including each cloud's `instanceType`.

**Rejected:** Every user then needs to know the cloud's size names and versions, which is the cloud
knowledge infralib exists to abstract. Deployment defaults keep the same schema and allow a user to
write only what they care about.

#### 2 Platform-picked sizes

The user states CPU and memory, or a named size, and the platform selects the nearest
cloud size.

**Rejected:** Picking has to handle fixed and flexible sizes, burstable and dedicated families, and
availability per region, on four clouds. Deployment defaults solve the same need with no picking
logic.
