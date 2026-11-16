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
- [Ignored Settings](#ignored-settings)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Kinds

Kafka is offered as three kinds, each shown below with a manifest a user writes.

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
  version: "3.9"                       # each cloud accepts the versions it offers
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

A cluster's size is its brokers times each broker's size. AWS and OCI take a broker size and count;
GCP takes the cluster's total vCPU and memory and chooses the brokers itself. The size is therefore
in each cloud's block (`aws.instanceType`, `oci.instanceType` with `oci.ocpus`, `gcp.vcpus` and
`gcp.memoryGB`), and `brokers` is ignored on GCP. No cloud offers a maintenance window.

The status adds `bootstrapServers`, the listener applications use (`bootstrapBrokersSaslScram` on
AWS, `bootstrap_address` on GCP, `kafkaBootstrapUrls` on OCI). `brokers` stays empty on GCP.

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

Created through the Kafka admin API on every cloud. `name` is the topic's Kafka name, which can hold
what a Kubernetes name cannot, such as `Orders` or `orders_v2`. Applications use that name, so it
cannot carry a hash like rule 9's cloud names, and two `KafkaTopic` objects on one cluster could
request the same topic and both manage it. The platform rejects the newer one instead, naming the
object that holds the name.

## Mapping

In the table below, Level is the field's support level (rule 1).

| Field              | Level                                | AWS (MSK provisioned `Cluster`)                             | GCP (`google_managed_kafka_cluster`), no provider resource                                                  | OCI (`KafkaCluster`)                                                                |
| ------------------ | ------------------------------------ | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `observe`          | Core                                 | `managementPolicies: [Observe]`                             | rejected until the provider has a Kafka resource                                                            | `managementPolicies: [Observe]`                                                     |
| `version`          | Core                                 | `kafkaVersion`                                              | `kafka_version`                                                                                             | `kafkaVersion`                                                                      |
| `brokers`          | Ignored on GCP                       | `numberOfBrokerNodes`                                       | ignored; chosen by the service                                                                              | `brokerShape.nodeCount`                                                             |
| `storage.sizeGB`   | Core                                 | `brokerNodeGroupInfo.storageInfo.ebsStorageInfo.volumeSize` | `broker_capacity_config.disk_size_gib`                                                                      | `brokerShape.storageSizeInGbs`                                                      |
| `highAvailability` | Core; `false` ignored on AWS and GCP | always; `false` ignored                                     | always three zones; `false` ignored                                                                         | `clusterType` `PRODUCTION` or `DEVELOPMENT`                                         |
| `parameters`       | Core                                 | an MSK Configuration                                        | Kafka admin API, for settings GCP marks `cluster-wide`; `read-only` ones take effect after a broker restart | `KafkaClusterConfig`; `sasl.*`, `ssl.*` and others the service ignores are rejected |

*Common fields of `KafkaCluster` on each cloud.*

The cloud-specific fields:

| Field              | Maps To                                                   |
| ------------------ | --------------------------------------------------------- |
| `aws.id`           | the cluster ARN                                           |
| `gcp.id`           | rejected; provider-upjet-gcp v3.1.0 has no Kafka resource |
| `oci.id`           | the cluster's OCID                                        |
| `aws.instanceType` | `brokerNodeGroupInfo.instanceType`                        |
| `oci.instanceType` | `brokerShape.nodeShape`                                   |
| `gcp.vcpus`        | `capacity_config.vcpu_count`                              |
| `gcp.memoryGB`     | `capacity_config.memory_bytes`, in bytes                  |
| `oci.ocpus`        | `brokerShape.ocpuCount`                                   |

*Cloud-specific fields of `KafkaCluster`.*

## Composed Resources

The platform composes the resources below from the `KafkaCluster` in [Kinds](#kinds), on each cloud
in turn. A value commented `# set by the platform` comes from the deployment, not from the object.
GCP has no pair: its provider has no Managed Kafka resource. The resources per cloud:

```yaml Object
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaCluster
metadata:
  name: orders-kafka
  namespace: shop
spec:
  # Common
  version: "3.9"                       # each cloud accepts the versions it offers
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
```

```yaml AWS resources
---
apiVersion: kafka.aws.m.upbound.io/v1beta1
kind: Cluster
metadata:
  name: orders-kafka-7f3a2c              # the object's name and a hash of its UID (rule 9)
  namespace: shop
spec:
  forProvider:
    region: eu-north-1                   # the deployment's region
    clusterName: orders-kafka-7f3a2c
    kafkaVersion: "3.9"
    numberOfBrokerNodes: 3
    brokerNodeGroupInfo:
      instanceType: kafka.m7g.large
      storageInfo:
        ebsStorageInfo:
          volumeSize: 100
      clientSubnets:                     # set by the platform
      - subnet-0123456789abcdef0
      - subnet-0123456789abcdef1
      - subnet-0123456789abcdef2
      securityGroups:                    # set by the platform
      - sg-0123456789abcdef0
    configurationInfo:                   # a composed Configuration holding parameters
      arn: arn:aws:kafka:eu-north-1:111122223333:configuration/orders-kafka-7f3a2c
      revision: 1
    clientAuthentication:                # set by the platform
      sasl:
        scram: true
    encryptionInfo:                      # set by the platform
      encryptionAtRestKmsKeyArn: arn:aws:kms:eu-north-1:111122223333:key/data
```

*A `KafkaCluster` on AWS and its composed resources.*

```yaml Object
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaCluster
metadata:
  name: orders-kafka
  namespace: shop
spec:
  # Common
  version: "3.9"                       # each cloud accepts the versions it offers
  brokers: 3
  storage:
    sizeGB: 100                        # per broker
  highAvailability: true               # survive the loss of one zone
  deletionProtection: true
  parameters:                          # broker settings, within each cloud's allowlist
    auto.create.topics.enable: "false"
  # Cloud-specific; every block also takes id (rule 9)
  oci:
    instanceType: VM.Standard.E5.Flex  # the broker shape
    ocpus: 2                           # per broker, required with a .Flex shape
```

```yaml OCI resources
---
apiVersion: managedkafka.oci.m.upbound.io/v1alpha1
kind: KafkaCluster
metadata:
  name: orders-kafka-7f3a2c              # the object's name and a hash of its UID (rule 9)
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # set by the platform
    displayName: orders-kafka-7f3a2c
    kafkaVersion: "3.9"
    clusterType: PRODUCTION              # DEVELOPMENT without highAvailability
    coordinationType: ZOOKEEPER          # set by the platform: ZOOKEEPER for Kafka 3, KRAFT from 4.0
    brokerShape:
    - nodeCount: 3
      nodeShape: VM.Standard.E5.Flex
      ocpuCount: 2
      storageSizeInGbs: 100
    clusterConfigId: ocid1.kafkaclusterconfig.oc1..orders  # a composed KafkaClusterConfig holding parameters
    clusterConfigVersion: 1
    accessSubnets:                       # set by the platform
    - subnets:
      - ocid1.subnet.oc1..deployment
```

*A `KafkaCluster` on OCI and its composed resources.*

## Platform Decisions

The platform makes these settings itself:

| Setting           | Platform's Choice                                                                                                                                                                                                                                       |
| ----------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Network placement | The deployment's network                                                                                                                                                                                                                                |
| Encryption        | TLS in transit; at rest, the platform's data key on AWS and GCP, while OCI has no customer key                                                                                                                                                          |
| Login mechanism   | Chosen by the platform per cloud, and named in each user's secret                                                                                                                                                                                       |
| Admin login       | The one provider-kafka uses: IAM on AWS, the superuser's Vault secret on OCI                                                                                                                                                                            |
| Access            | Denied by default: a topic or group is reachable only by users with an ACL on it and by the admin. MSK and OCI set `allow.everyone.if.no.acl.found=false`, with the admin as a superuser; GCP needs explicit ALLOW ACLs for the admin on every resource |

*Settings the platform makes for a `KafkaCluster`.*

## Ignored Settings

These settings are accepted on every cloud but have no effect on some (rule 1). The table says what
that means there.

| Setting                   | Ignored On | Effect                                |
| ------------------------- | ---------- | ------------------------------------- |
| `brokers`                 | GCP        | GCP chooses the broker count.         |
| `highAvailability: false` | AWS, GCP   | The cluster runs across zones anyway. |

*Kafka settings that some clouds ignore.*

## Cloud Limits

Each cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later.

| Field            | Cloud | Limit                          | Source                                                                      |
| ---------------- | ----- | ------------------------------ | --------------------------------------------------------------------------- |
| `version`        | all   | the versions each cloud offers | each cloud's docs                                                           |
| `brokers`        | AWS   | a multiple of the zone count   | `numberOfBrokerNodes`, MSK `Cluster` CRD, provider-upjet-aws v2.4.0         |
| `brokers`        | OCI   | 1 to 30                        | `brokerShape.nodeCount`, `KafkaCluster` CRD, crossplane-provider-oci v2.0.0 |
| `storage.sizeGB` | GCP   | at least 100                   | `disk_size_gib`, `google_managed_kafka_cluster` docs                        |
| `storage.sizeGB` | OCI   | at least 50                    | `brokerShape.storageSizeInGbs`, `KafkaCluster` CRD                          |
| `gcp.vcpus`      | GCP   | at least 3                     | `vcpu_count`, `google_managed_kafka_cluster` docs                           |
| `gcp.memoryGB`   | GCP   | 1 to 8 per vCPU                | `memory_bytes`, `google_managed_kafka_cluster` docs                         |

*Limits each cloud enforces on Kafka.*

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
