# Follow-Up Work

Each item below is work outside the design that the design depends on or leads to.

#### 1 Implement the design on each cloud

Each cloud gets its own implementation of the design, built on its managed services and its
Crossplane provider.

The following points are known so far:

- A floor (`sizeGB`, Valkey's `version`) is set in `initProvider`, which Crossplane uses at creation
  only, and in `forProvider` only when the spec requests more than the resource has, with
  `LateInitialize` omitted; a lower value is ignored, with a condition saying so. Sending an older
  value back would shrink a GCP disk, replace an Azure server, or be refused by upjet as a Valkey
  downgrade, which blocks every later change.
- Azure deletes a storage account with its blobs, without checking that it is empty, so the Azure
  implementation needs its own way to refuse deleting a `Bucket` that still holds objects.
- The function records the `fromBackup` and `id` it resolved at creation, from an inline entry or a
  `ref`, and refuses a later difference with a condition: a changed `ref`, or a referenced object
  deleted and created again, passes admission, since the policy cannot read the referenced object.
- provider-kafka finds a topic by name only, so the duplicate-name check of `KafkaTopic` must run in
  the function.
- An observed resource's admin login, which kinds such as `KafkaUser` need in order to act inside
  it, comes from where the cloud keeps one: provider-kafka's IAM role on MSK, the RDS-managed master
  secret where the instance has one, and the superuser's Vault secret on OCI Kafka. Where the cloud
  keeps none, the implementation chooses how the login is supplied.
- `RabbitMQUser` and `RabbitMQVirtualHost` need a Crossplane provider for RabbitMQ's management API,
  and none was found. The Terraform `rabbitmq` provider covers users, virtual hosts and permissions
  and could be wrapped with upjet.
- The `platform-apis` chart takes the deployment's cloud and installs each service where it is
  implemented (principle 7). Today infralib's per-cloud values switch services off, such as
  [values-oracle.yaml](https://github.com/entigolabs/entigo-infralib/blob/main/modules/k8s/platform-apis/values-oracle.yaml)
  in the `platform-apis` module; once the chart holds the support matrix, those values keep only an
  opt-out.
- Each implementation is a Composition for the core kind, labelled with its provider configuration
  kind, such as `data.entigo.com/provider-config: PostgreSQLRDSConfig.aws.database.entigo.com`. The
  admission policy that writes a class's defaults (principle 5) also writes the object's composition
  selector with that label, and Crossplane picks the Composition by it: a composite resource without
  a Composition reference takes one whose labels match its selector (checked in Crossplane v2.4.3).

#### 2 Migrate today's objects to the design

Client teams move their own objects from today's API to the design, without losing their cloud
resources, over a migration period of 6 to 12 months on AWS. Today's renamed kinds stay served until
their last object has moved, and the kinds that keep their names convert between versions
([Versions](common-design.md#versions)).

Application identity, and whether it joins this migration or a later one, is still to be decided
([Open Point 2](open-points.md)). Until then, an application that reads a bucket's keys keeps
today's `S3Bucket`, and database, cache and broker users keep passwords in connection Secrets.

The differences are listed in [API Changes](changes.md). The teams get four aids:

- Manifest converter. A command rewrites an old manifest into the new one, renaming the kind where
  needed and moving fields into the provider configuration entry; teams run it on their
  repositories.
- Move path. For each kind, a documented sequence moves an object without touching its cloud
  resource: the new object observes the resource by its ID and takes ownership of it by removing
  `observe`, then the old object is deleted without deleting the resource. Before the migration
  starts, today's functions add the workspace and Zone tags to the resources they manage, so the new
  object passes the tag check. A parent moves first, then the team removes `kind` from its users'
  parent reference ([Versions](common-design.md#versions)).
- Deprecation signal. Every object of a renamed kind reports a status condition that names the new
  kind and links to the steps, also shown in Argo CD and the portal.
- Migration guide. One page per kind, built from API Changes, states what changes, what to check
  and the converter command.

The following points of the migration itself are known so far:

- The renamed and changed kinds need new XRDs. `MSK` is cluster-scoped today, while `KafkaCluster`
  is namespaced and shared within its Zone (principle 11).
- Kafka clusters switch to deny by default. Today's clusters are open, so applications that use
  topics without ACLs, such as FleetComplete's, lose access at the switch, which is agreed with that
  team first.

#### 3 Give each application a cloud identity

Each application gets a cloud identity of its own, through which it reaches buckets, artifact
repositories, databases, caches and brokers.

The identity belongs to the application, not to the resource it uses. The following points are
known so far:

- Portal applications currently reach buckets only through the keys of `S3Bucket`, because `WebApp`
  sets no service account, so their pods run as `default`. `WebApp` needs a service account of its
  own before these applications move.
- The way the identity and its grants are created is still to be decided
  ([Open Point 2](open-points.md)).
- For pushing to an artifact repository, CI currently uses credentials configured outside the API.
  OCIR accepts a user's auth token or an instance principal through Oracle's
  docker-credential-ocir.

Sources:
[`functions/workload/service/web_app.go`](https://github.com/entigolabs/platform-apis/blob/main/functions/workload/service/web_app.go),
[crossplane-provider-oci v2.0.0](https://github.com/oracle/crossplane-provider-oci/releases/tag/v2.0.0).

#### 4 Limit Zone Argo CD projects to an allowlist of kinds

A Zone's Argo CD project allows only the kinds the Zone's role permits.

Today the AppProject blocks `*.m.upbound.io` but allows every other namespaced kind, so a namespaced
provider outside it, such as provider-sql or provider-kubernetes, can be synced past the platform's
functions. Its `namespaceResourceWhitelist` should list the groups the Zone's role allows, so a
provider installed later is blocked by default. The `observe` check in
[Common Design](common-design.md#observed-resources) depends on this allowlist, and the platform
refuses `observe: true` until it is in place.

#### 5 Show cloud rejections in Argo CD health

Argo CD shows an object whose cloud rejected it as `Degraded`, with the cloud's message.

Its health check for `*.entigo.com` kinds marks an object `Degraded` only on `Synced` or
`LastAsyncOperation` false. Until it also reads `CloudAccepted` (principle 10), a value the cloud
rejects shows in Argo CD as `Progressing`, "Provisioning ...", with no message.

Sources:
[`modules/k8s/argocd/values.yaml`](https://github.com/entigolabs/entigo-infralib/blob/main/modules/k8s/argocd/values.yaml)
in [entigo-infralib](https://github.com/entigolabs/entigo-infralib).
