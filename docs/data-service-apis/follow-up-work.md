# Follow-Up Work

Work outside the design that it depends on or leads to.

#### 1 Per-cloud implementation

The implementation of the design on each cloud. Known so far:

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

#### 2 Migration from current APIs

The path from today's kinds to the design. Known so far:

- The renamed kinds need new XRDs. `MSK` is cluster-scoped today, while `KafkaCluster` is
  namespaced and shared within its Zone (rule 11).
- Some secrets change form: Valkey's `AUTH_TOKEN`, `PORT` and `credentials.json` keys; the bucket's
  `AWS_*` and `credentials.json` keys, which leave `Bucket` together with its access (see
  [bucket.md](bucket.md)); and the Kafka user secret's name (`<cluster>-<user>-k8s`) and single
  JSON value.
- Kafka clusters switch to deny by default. Today's clusters are open, so applications that use
  topics without ACLs, such as FleetComplete's, lose access at the switch, which is therefore agreed
  with that team first.

#### 3 Application identity

An application's cloud identity, through which applications gain access to buckets, artifact
repositories and other cloud services. The identity belongs to the application, not to the
resource it uses. Known so far:

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

#### 4 Zone allow list for Argo CD

A Zone's AppProject blocks `*.m.upbound.io` but allows every other namespaced kind, so a namespaced
provider outside it, such as provider-sql or provider-kubernetes, can be synced past the platform's
functions. Its `namespaceResourceWhitelist` should list the groups the Zone's role allows, so a
provider installed later is blocked by default.

#### 5 Cloud rejections in Argo CD health

Argo CD's health check for `*.entigo.com` kinds
([`modules/k8s/argocd/values.yaml`](https://github.com/entigolabs/entigo-infralib/blob/main/modules/k8s/argocd/values.yaml)
in [entigo-infralib](https://github.com/entigolabs/entigo-infralib)) marks an object `Degraded` only
on `Synced` or `LastAsyncOperation` false. Until it also reads `CloudAccepted` (rule 10), a value
the cloud rejects shows in Argo CD as `Progressing`, "Provisioning ...", with no message.
