# Follow-Up Work

Work outside the design that it depends on or leads to.

#### 1 Implementation on each cloud

Each cloud gets its own implementation of the design, built on its managed services and its
Crossplane provider.

Known so far:

- A floor (`sizeGB`, Valkey's `version`) is set in `initProvider`, which Crossplane uses at
  creation only, and in `forProvider` only when the spec requests more than the resource has, with
  `LateInitialize` omitted. Sending an older value back would shrink a GCP disk, replace an Azure
  server, or be refused by upjet as a Valkey downgrade, which blocks every later change.
- provider-kafka finds a topic by name only, so the duplicate-name check of `KafkaTopic` must run in
  the function.
- An observed resource's admin login, which kinds such as `KafkaUser` need in order to act inside
  it, comes from where the cloud keeps one: provider-kafka's IAM role on MSK, the RDS-managed master
  secret where the instance has one, and the superuser's Vault secret on OCI Kafka. Where the cloud
  keeps none, the implementation chooses how the login is supplied.
- No Crossplane provider for RabbitMQ's management API was found, which `RabbitMQUser` and
  `RabbitMQVirtualHost` need. The Terraform `rabbitmq` provider covers users, virtual hosts and
  permissions and could be wrapped with upjet.
- The `platform-apis` chart takes the deployment's cloud and installs each service where it is
  implemented (principle 7). Today infralib's per-cloud values switch services off, such as
  [values-oracle.yaml](https://github.com/entigolabs/entigo-infralib/blob/main/modules/k8s/platform-apis/values-oracle.yaml)
  in the `platform-apis` module; once the chart holds the support matrix, those values keep only an
  opt-out.
- Each implementation is a Composition for the core kind, labelled with its provider config kind,
  such as `data.entigo.com/provider-config: PostgreSQLRDSConfig.aws.database.entigo.com`. The
  admission policy that writes a class's defaults (principle 5) also writes the object's composition
  selector with that label, and Crossplane picks the Composition by it: a composite resource without
  a Composition reference takes one whose labels match its selector (checked in Crossplane v2.4.3).

#### 2 Migration from current APIs

Existing objects of today's kinds move to the design without losing their cloud resources.

The differences are listed in [API Changes](changes.md). Known so far for the migration itself:

- The renamed and changed kinds need new XRDs. `MSK` is cluster-scoped today, while `KafkaCluster`
  is namespaced and shared within its Zone (principle 11).
- Kafka clusters switch to deny by default. Today's clusters are open, so applications that use
  topics without ACLs, such as FleetComplete's, lose access at the switch, which is therefore agreed
  with that team first.

#### 3 Cloud identity for applications

Each application gets a cloud identity of its own, through which it reaches buckets, artifact
repositories and other cloud services.

The identity belongs to the application, not to the resource it uses: a [Bucket](bucket.md) creates
no identity, and [API Changes](changes.md#bucket) keeps `S3Bucket` served until applications have
one. Known so far:

- Portal applications currently reach buckets only through the keys of `S3Bucket`, because `WebApp`
  sets no service account
  ([`functions/workload/service/web_app.go`](https://github.com/entigolabs/platform-apis/blob/main/functions/workload/service/web_app.go)),
  so they keep `S3Bucket` until application identity is available.
- For buckets, the identity is a role or managed identity per service account on AWS and Azure, the
  service account itself on GCP and OCI, or a key pair. On OCI, [crossplane-provider-oci
  v2.0.0](https://github.com/oracle/crossplane-provider-oci/releases/tag/v2.0.0) writes a
  `CustomerSecretKey`'s secret only to its plain status (`status.atProvider.key`).
- For pushing to an artifact repository, CI currently uses credentials configured outside the API.
  OCIR accepts a user's auth token or an instance principal through Oracle's
  docker-credential-ocir.

#### 4 Allow list of kinds in Zone Argo CD projects

A Zone's Argo CD project allows only the kinds the Zone's role permits.

Today the AppProject blocks `*.m.upbound.io` but allows every other namespaced kind, so a namespaced
provider outside it, such as provider-sql or provider-kubernetes, can be synced past the platform's
functions. Its `namespaceResourceWhitelist` should list the groups the Zone's role allows, so a
provider installed later is blocked by default.

#### 5 Cloud rejections in Argo CD health

Argo CD shows an object whose cloud rejected it as `Degraded`, with the cloud's message.

Its health check for `*.entigo.com` kinds
([`modules/k8s/argocd/values.yaml`](https://github.com/entigolabs/entigo-infralib/blob/main/modules/k8s/argocd/values.yaml)
in [entigo-infralib](https://github.com/entigolabs/entigo-infralib)) marks an object `Degraded` only
on `Synced` or `LastAsyncOperation` false. Until it also reads `CloudAccepted` (principle 10), a
value the cloud rejects shows in Argo CD as `Progressing`, "Provisioning ...", with no message.
