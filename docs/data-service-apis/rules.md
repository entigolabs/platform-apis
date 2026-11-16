# Design Rules

The rules every data service API follows.

## Rules

These rules apply to every data service API, and each service's page shows how it applies them.

#### 1 The common spec states needs, not cloud products

One manifest works on every cloud where its kind exists: it says what is needed, such as a version
or a disk size, not which cloud product provides it.

A setting that a cloud lacks is ignored there, and everything that exists on only one cloud goes
into a block for that cloud. An operation that a cloud cannot perform fails instead, with the reason
shown on the object.

The common spec holds needs such as a version, a size, high availability and backups. A field
belongs there when a user can choose it without knowing which cloud runs underneath. Where no fair
cloud-neutral value exists, the setting goes into the cloud blocks instead, as a machine size does,
since each cloud names its sizes differently.

The fields are checked against the managed services of every cloud infralib deploys to (AWS, GCP,
OCI and Azure), even where no Crossplane provider supports the service yet. As a result, a cloud
that receives a provider later finds its concepts already in the API.

Not every cloud supports every setting, so each field has a support level. The level can also apply
to a single value of a field, such as turning off a feature that one cloud always has on.

| Level   | Meaning                                                                                                                                                |
| ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Core    | Works on every cloud. Where a cloud cannot perform an operation the field asks for, the Level column says so, and the object reports the failure.      |
| Ignored | Accepted on every cloud, but has no effect on the clouds named in the field's description, which lack it. The resource is created without the setting. |

*Support levels of a field.*

In a schema, a field's description ends with `Ignored on` and the clouds that lack it; each
service's Mapping table names the same clouds in its Level column. A field without that mark is
Core.

#### 2 A setting of one cloud belongs in that cloud's block

Anything that exists on only one cloud goes into that cloud's block, such as `spec.aws`.

The blocks are `spec.aws`, `spec.gcp`, `spec.oci` and `spec.azure`. Each block has its own typed
fields, so the schema can check them. A deployment uses the block of the cloud it runs on and
ignores the others, so one manifest can carry settings for several clouds and be applied on any of
them. Only one block may hold the `id` of an existing resource to observe, since a resource exists
on one cloud.

A machine size is such a setting, named as the cloud names it, such as `db.t4g.small` in
`spec.aws.instanceType`. A size stated as CPU and memory would need the platform to pick the nearest
cloud size across fixed and flexible sizes, families and availability per region, on four clouds.
The size's deployment default (rule 5) spares users from knowing the cloud's names.

This follows the Zone API, which uses shared keys with cloud-native values and prefixed keys for one
cloud's settings
([`functions/tenancy`](https://github.com/entigolabs/platform-apis/tree/main/functions/tenancy)).

Everything that is neither a common field nor in a cloud's block, such as network placement and
encryption keys, is decided by the platform.

#### 3 Implementing a cloud never changes the common fields

Implementing the design on another cloud never changes the common fields, so manifests written
before keep working.

The schema holds a block for every cloud infralib deploys to, `spec.<cloud>` and `status.<cloud>`,
even before that cloud's implementation exists. Implementing a cloud gives its block a working
mapping; the common fields stay as they are.

#### 4 The status reports what the cloud actually runs

The status shows what is running on the cloud, not what the manifest asked for.

The status shows the common fields as the cloud actually runs them, the usual `conditions`, the
address applications connect to, and the name of the connection secret. The address is `endpoint: {
host, port }` where the service has one host, or the service's own form where it has several.

Values that only make sense on one cloud, such as its resource IDs, go into `status.<cloud>`. If a
cloud's provider does not report a common field, that field stays empty on that cloud, and the
service's page says so.

#### 5 Defaults are written once, when the object is created

A field left out gets its default once, at creation, and the platform never changes it afterwards.

Most fields are optional, and their defaults are of two kinds:

| Kind               | Set By                                                 | Example                       |
| ------------------ | ------------------------------------------------------ | ----------------------------- |
| Fixed              | the platform, the same on every deployment             | deletion protection           |
| Deployment default | the deployment's operator, per cloud, through infralib | version, machine size         |

*The two kinds of default.*

Safety settings, such as deletion protection, are fixed. Cost, size and availability are
deployment defaults, because only the operator can judge what suits their cloud and budget. In a
schema, the field's description ends with `Default: fixed` or `Default: per deployment`.

The defaults are written into the object by a `MutatingAdmissionPolicy`, which is built into
Kubernetes from version 1.36. It fills them in when the object is created and keeps them on
updates, so a default is applied only once: if the operator later changes a default, running
databases are not resized or upgraded. Defaults were tested to survive later updates from the
portal and from Argo CD. Observed resources receive no defaults.

Deletion protection is enforced by a `ValidatingAdmissionPolicy`, which refuses to delete an object
whose `deletionProtection` is true.

#### 6 Applications connect the same way on every cloud

An application reads its connection details in the same form on every cloud, so it moves between
clouds without changes.

Each credential is one Kubernetes Secret in the object's namespace, with the same keys on every
cloud: `endpoint`, `port`, `username` and `password`. `port` is omitted where `endpoint` already
lists hosts with their ports. A service that needs more adds its own keys, such as a reader
endpoint. No key is named after a cloud, and no key holds the others as JSON.

The secrets are named `<instance>-dbadmin` for a database's admin, `<instance>-credentials` for
other services, and `<instance>-<user>` for a user object. Every kind with a secret takes an
optional `secretName` to override that name. The platform never overwrites a Secret that another
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

The `platform-apis` chart holds this knowledge: a deployment passes only its cloud, and the chart
installs each service's API where that service is implemented and its engine runs. A per-service
switch lets an operator leave a service out, never add one that the cloud lacks. A manifest of a
kind that is not installed is rejected by the Kubernetes API server when it is applied, before any
function runs.

#### 8 A kind is named after the engine or concept, never a cloud product

A kind's name means the same on every cloud: it names the engine or the concept, never one cloud's
product.

A product name ties a kind to one cloud: `S3Bucket` reads as Amazon S3 even where the bucket is in
Cloud Storage, and a user would have to know which cloud runs underneath to recognise it. A kind
therefore takes the engine's own name, as `PostgreSQLInstance` and `KafkaTopic` do, or, where no
engine names it, the concept every cloud shares, as `Bucket` and `ArtifactRepository` do.

#### 9 A kind either creates its resource or observes an existing one

A kind either creates its cloud resource or, with `observe: true`, only reads one that already
exists.

This applies to every kind that represents a cloud resource, such as a database instance or a
bucket. To observe, the user sets `observe: true` and gives the resource's ID in the cloud's block:

```yaml
spec:
  observe: true
  aws:
    id: ...                            # the resource's ID, copied from the cloud console
```

*An observed resource on AWS.*

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

When the platform creates a resource, its name in the cloud includes a hash of the object's unique
ID. As a result, two objects with the same name in different namespaces never share one cloud
resource. A name that applications use, such as a Kafka topic or an image path, keeps the user's
name instead, and the platform rejects a second object that asks for a name already held.

#### 10 The schema checks only what the API defines

The schema checks the API's own rules only; each cloud enforces its own value limits.

The schema checks only what the API itself knows: field types, fields that cannot change, fields
that cannot be combined, and the API's own minimums, such as a disk that cannot shrink. It does not
copy the limits each cloud has for values, such as how many days of backups it keeps. Those limits
change, and a Kubernetes schema rule can never be loosened once released. The cloud rejects such
values itself, and each service's page lists the known limits under Cloud Limits.

When the cloud rejects a value, the error appears only on the provider's own object, not on the
user's. The platform therefore also shows it on the user's object, as the condition `CloudAccepted:
False` and as a warning event. An operation that the running cloud cannot perform, such as an
upgrade its service does not offer, is reported the same way, without reaching the cloud. Defaults
are not checks: the admission policy that writes them runs per deployment, so a default may take
that cloud's limit into account.

#### 11 A reference crosses namespaces only within one Zone

An object may refer to one in another namespace, but only within the same Zone.

A kind that exists inside another, such as a database user or a Kafka topic, names its parent in a
reference, `instanceRef`, `clusterRef` or `brokerRef`, as `{ name, namespace }`. Without
`namespace`, the parent is in the same namespace.

A reference may point to another namespace only within the same Zone, meaning both namespaces carry
the same `tenancy.entigo.com/zone` label. Creating a user gives power over the parent, so it stays
within the team's boundary, which the Zone already defines.

#### 12 A user object holds its own permissions

Each user object is one application's login on one instance, and holds everything that login may do.

A user is one application's identity on one instance of the engine: its login, its Secret and its
permissions there, in whatever form the engine has them, such as ACLs or privileges. It is not
shared between applications or instances; access for people is not covered by these kinds. A user's
access is therefore read from one object. A set of permissions shared by several users exists only
where the engine has one, such as PostgreSQL's roles. The platform adds no permission mechanism that
no engine has.
