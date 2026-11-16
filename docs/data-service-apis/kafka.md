# Kafka

Apache Kafka is a distributed event log that applications publish streams of records to and consume
them from.

Three kinds: `KafkaCluster`, the cluster itself; `KafkaUser`, one application's login, with its
ACLs; and `KafkaTopic`, one topic in a cluster.

Built on Amazon MSK provisioned (AWS) and OCI Streaming with Apache Kafka (OCI). GCP's Managed
Service for Apache Kafka is mapped, but the kinds exist on GCP only once its Crossplane provider
supports the service (rule 7). Azure has none: Event Hubs speaks the Kafka protocol but has no Kafka
ACLs, no topic creation through the Kafka admin API and no broker settings.

[API Changes](changes.md#kafka) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Kinds

Kafka is offered as three kinds, each with the manifest a user writes.

### KafkaCluster

A user sets a `KafkaCluster`'s version, brokers, storage and broker settings in a manifest like the
one below. Its comments note value ranges and what holds on one cloud only:

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaCluster
metadata:
  name: orders-kafka
  namespace: shop
spec:
  # Common
  version: "4.0"                       # each cloud accepts the versions it offers
  brokers: 3
  storage:
    sizeGB: 100                        # per broker
  highAvailability: true               # survive the loss of one zone
  deletionProtection: true
  parameters:                          # broker settings, within each cloud's allowlist
    auto.create.topics.enable: "false"
  # Cloud-specific; every block also takes id (rule 9)
  aws:
    instanceType: kafka.m7g.large      # the broker instance type
  gcp:
    vcpus: 3                           # GCP sizes the whole cluster by vCPU and memory
    memoryGB: 12                       # within GCP's range per vCPU
  oci:
    instanceType: VM.Standard.E5.Flex  # the broker shape
    ocpus: 2                           # per broker, required with a .Flex shape
```

*A `KafkaCluster` with its common fields and a block per cloud.*

A cluster's size is its brokers times each broker's size. The clouds size them differently, so the
size is in each cloud's block (rule 2). No cloud offers a maintenance window.

The status adds `bootstrapServers`, the listener applications use.

The cloud blocks of the status:

```yaml
status:
  aws:
    arn: ...
    kafkaVersion: ...
  gcp:
    id: ...
  oci:
    id: ...
    state: ACTIVE
```

*The cloud blocks of a `KafkaCluster` status.*

A cluster has no secret of its own: applications log in with a `KafkaUser`'s secret.

### KafkaUser

An identity that applications use to log in to the cluster, with the ACLs it is granted.

An example:

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaUser
metadata:
  name: orders-app
  namespace: shop
spec:
  clusterRef:                          # required
    name: orders-kafka
    namespace: shop                    # unset, this namespace; within the Zone (rule 11)
  consumerGroups:
  - orders
  acls:
  - topic: orders
    operation: Read
  secretName: orders-app-login         # unset, orders-kafka-orders-app (rule 6)
```

*A `KafkaUser` with one consumer group and a read ACL.*

| Cloud | Identity                                           | Login                                   |
| ----- | -------------------------------------------------- | --------------------------------------- |
| AWS   | a SCRAM secret in Secrets Manager (`AmazonMSK_*`)  | SASL SCRAM-SHA-512                      |
| GCP   | a service account with a key                       | SASL PLAIN: the account's email and key |
| OCI   | provider-kafka `User`, through the Kafka admin API | SASL SCRAM-SHA-512                      |

*A `KafkaUser`'s identity and login on each cloud.*

ACLs are Kafka ACLs on every cloud: provider-kafka on AWS and OCI,
[`google_managed_kafka_acl`](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/managed_kafka_acl)
or the Kafka admin API on GCP.

Secret `<cluster>-<user>`: `endpoint` (the bootstrap servers, as `host:port` pairs, so no `port`),
`username`, `password` and `saslMechanism` (`SCRAM-SHA-512` or `PLAIN`), since the mechanism
differs by cloud.

### KafkaTopic

A topic in the cluster, created through the Kafka admin API on every cloud.

An example:

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaTopic
metadata:
  name: orders
  namespace: shop
spec:
  clusterRef:                          # required
    name: orders-kafka
    namespace: shop                    # unset, this namespace; within the Zone (rule 11)
  name: orders_v2                      # the Kafka name; unset, metadata.name
  partitions: 3
  replicationFactor: 3                 # at most the broker count
  parameters:
    retention.ms: "604800000"
```

*A `KafkaTopic` whose Kafka name differs from its object name.*

`name` is the topic's Kafka name, which can hold what a Kubernetes name cannot, such as `Orders` or
`orders_v2`. Applications use that name, so it cannot carry a hash like rule 9's cloud names, and
two `KafkaTopic` objects on one cluster could request the same topic and both manage it. The
platform rejects the newer one instead, naming the object that holds the name.

## Mapping

The table names what each field maps to in the cloud's provider. A setting the cloud lacks shows as
ignored, with the reason (rule 1), and a field of a cloud's block appears only for that cloud:

| Field              | AWS                                                         | GCP                                                                                                         | OCI                                                                                 |
| ------------------ | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `observe`          | `managementPolicies: [Observe]`                             | rejected until the provider has a Kafka resource                                                            | `managementPolicies: [Observe]`                                                     |
| `version`          | `kafkaVersion`                                              | `kafka_version`                                                                                             | `kafkaVersion`                                                                      |
| `brokers`          | `numberOfBrokerNodes`                                       | ignored; chosen by the service                                                                              | `brokerShape.nodeCount`                                                             |
| `storage.sizeGB`   | `brokerNodeGroupInfo.storageInfo.ebsStorageInfo.volumeSize` | `broker_capacity_config.disk_size_gib`                                                                      | `brokerShape.storageSizeInGbs`                                                      |
| `highAvailability` | always; `false` ignored                                     | always three zones; `false` ignored                                                                         | `clusterType` `PRODUCTION` or `DEVELOPMENT`                                         |
| `parameters`       | an MSK Configuration                                        | Kafka admin API, for settings GCP marks `cluster-wide`; `read-only` ones take effect after a broker restart | `KafkaClusterConfig`; `sasl.*`, `ssl.*` and others the service ignores are rejected |
| `aws.id`           | the cluster ARN                                             |                                                                                                             |                                                                                     |
| `gcp.id`           |                                                             | rejected; provider-upjet-gcp v3.1.0 has no Kafka resource                                                   |                                                                                     |
| `oci.id`           |                                                             |                                                                                                             | the cluster's OCID                                                                  |
| `aws.instanceType` | `brokerNodeGroupInfo.instanceType`                          |                                                                                                             |                                                                                     |
| `oci.instanceType` |                                                             |                                                                                                             | `brokerShape.nodeShape`                                                             |
| `gcp.vcpus`        |                                                             | `capacity_config.vcpu_count`                                                                                |                                                                                     |
| `gcp.memoryGB`     |                                                             | `capacity_config.memory_bytes`, in bytes                                                                    |                                                                                     |
| `oci.ocpus`        |                                                             |                                                                                                             | `brokerShape.ocpuCount`                                                             |

*Fields of `KafkaCluster` and their provider fields.*

**AWS:** the size is the broker type, `aws.instanceType`, times `brokers`. `bootstrapServers` is
`bootstrapBrokersSaslScram`. provider-kafka logs in with IAM. `allow.everyone.if.no.acl.found` is
`false`, with the admin as a superuser.

**GCP:** the mapping names fields of the Terraform resource `google_managed_kafka_cluster`, since no
Crossplane provider has one yet. GCP takes the cluster's total vCPU and memory, `gcp.vcpus` and
`gcp.memoryGB`, and chooses the brokers itself, so `brokers` is ignored and stays empty in the
status. `bootstrapServers` is `bootstrap_address`. The admin needs explicit ALLOW ACLs on every
resource.

**OCI:** the size is the broker shape, `oci.instanceType` with `oci.ocpus`, times `brokers`.
`bootstrapServers` is `kafkaBootstrapUrls`. Data at rest uses the service's own key, since OCI takes
no customer key. provider-kafka logs in with the superuser's Vault secret.
`allow.everyone.if.no.acl.found` is `false`, with the admin as a superuser.

## Composed Resources

**GCP:** nothing is composed, since its provider has no Managed Kafka resource.

The `KafkaCluster` from [Kinds](#kinds) is composed into the resources below on the cloud it runs
on: the platform uses that cloud's block and ignores the others. A comment names the source of each
value the object does not hold as written: `platform` for a value the platform sets, and
`deployment` for one the deployment sets. The object and its resources:

```yaml Object
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaCluster
metadata:
  name: orders-kafka
  namespace: shop
spec:
  version: "4.0"
  brokers: 3
  storage:
    sizeGB: 100
  highAvailability: true
  deletionProtection: true
  parameters:
    auto.create.topics.enable: "false"
  aws:
    instanceType: kafka.m7g.large
  gcp:
    vcpus: 3
    memoryGB: 12
  oci:
    instanceType: VM.Standard.E5.Flex
    ocpus: 2
```

```yaml AWS resource
---
apiVersion: kafka.aws.m.upbound.io/v1beta1
kind: Cluster
metadata:
  name: orders-kafka-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    region: eu-north-1         # deployment
    clusterName: orders-kafka-7f3a2c
    kafkaVersion: "4.0.x"
    numberOfBrokerNodes: 3
    brokerNodeGroupInfo:
      instanceType: kafka.m7g.large
      storageInfo:
        ebsStorageInfo:
          volumeSize: 100
      clientSubnets:           # platform
      - subnet-0123456789abcdef0
      - subnet-0123456789abcdef1
      - subnet-0123456789abcdef2
      securityGroups:          # platform
      - sg-0123456789abcdef0
    configurationInfo:         # composed from parameters
      arn: arn:aws:kafka:eu-north-1:111122223333:configuration/orders-kafka-7f3a2c
      revision: 1
    clientAuthentication:      # platform
      sasl:
        scram: true
    encryptionInfo:            # platform
      encryptionAtRestKmsKeyArn: arn:aws:kms:eu-north-1:111122223333:key/data
```

*A `KafkaCluster` on AWS and its composed resource.*

```yaml Object
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaCluster
metadata:
  name: orders-kafka
  namespace: shop
spec:
  version: "4.0"
  brokers: 3
  storage:
    sizeGB: 100
  highAvailability: true
  deletionProtection: true
  parameters:
    auto.create.topics.enable: "false"
  aws:
    instanceType: kafka.m7g.large
  gcp:
    vcpus: 3
    memoryGB: 12
  oci:
    instanceType: VM.Standard.E5.Flex
    ocpus: 2
```

```yaml OCI resource
---
apiVersion: managedkafka.oci.m.upbound.io/v1alpha1
kind: KafkaCluster
metadata:
  name: orders-kafka-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # platform
    displayName: orders-kafka-7f3a2c
    kafkaVersion: "4.0.0"
    clusterType: PRODUCTION
    coordinationType: KRAFT    # platform
    brokerShape:
    - nodeCount: 3
      nodeShape: VM.Standard.E5.Flex
      ocpuCount: 2
      storageSizeInGbs: 100
    clusterConfigId: ocid1.kafkaclusterconfig.oc1..orders  # composed from parameters
    clusterConfigVersion: 1
    accessSubnets:             # platform
    - subnets:
      - ocid1.subnet.oc1..deployment
```

*A `KafkaCluster` on OCI and its composed resource.*

## Platform Decisions

The platform makes these settings itself:

| Setting           | Platform's Choice                                                                                 |
| ----------------- | ------------------------------------------------------------------------------------------------- |
| Network placement | The deployment's private network                                                                  |
| Encryption        | TLS in transit; at rest, the platform's data key where the cloud takes one                        |
| Login mechanism   | Chosen by the platform per cloud, and named in each user's secret                                 |
| Admin login       | The one provider-kafka uses                                                                       |
| Access            | Denied by default: a topic or group is reachable only by users with an ACL on it and by the admin |

*Settings the platform makes for a `KafkaCluster`.*

## Cloud Limits

The cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later. Each limit holds as of its source: the provider release
named, or the cloud's documentation as read in November 2026.

| Field            | Cloud | Limit                         | Source                                                                             |
| ---------------- | ----- | ----------------------------- | ---------------------------------------------------------------------------------- |
| `version`        | all   | the versions the cloud offers | the cloud's documentation                                                          |
| `brokers`        | AWS   | a multiple of the zone count  | `numberOfBrokerNodes`, MSK `Cluster` CRD, provider-upjet-aws v2.4.0                |
| `brokers`        | OCI   | 1 to 30                       | `brokerShape.nodeCount`, `KafkaCluster` CRD, crossplane-provider-oci v2.0.0        |
| `storage.sizeGB` | GCP   | at least 100                  | `disk_size_gib`, `google_managed_kafka_cluster` documentation                      |
| `storage.sizeGB` | OCI   | at least 50                   | `brokerShape.storageSizeInGbs`, `KafkaCluster` CRD, crossplane-provider-oci v2.0.0 |
| `gcp.vcpus`      | GCP   | at least 3                    | `vcpu_count`, `google_managed_kafka_cluster` documentation                         |
| `gcp.memoryGB`   | GCP   | 1 to 8 per vCPU               | `memory_bytes`, `google_managed_kafka_cluster` documentation                       |

*Limits the cloud enforces on Kafka.*

## Notes

The design relies on these facts, which need no decision.

- GCP: the cluster, topic and ACL resources are in the GA Terraform `google` provider, and
  [provider-upjet-gcp
  v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0) has none of
  them yet.
- MSK Express brokers have no storage to size, so `storage.sizeGB` would be rejected with an
  `express.*` instance type.
- GCP: users are IAM service accounts, and a service account key in a secret is the only static
  credential; workload identity would need no secret but cloud-specific client code.
- Unverified: OCI SCRAM users through the Kafka admin API (Oracle documents only the superuser).
