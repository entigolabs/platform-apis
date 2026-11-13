# Kafka

Three kinds: `KafkaCluster`, `KafkaUser` (an identity with credentials and ACLs) and `KafkaTopic`.
Their concepts are checked against Amazon MSK provisioned (AWS), Managed Service for Apache Kafka
(GCP) and OCI Streaming with Apache Kafka (OCI). GCP's cluster, topic and ACL resources are in the
GA Terraform `google` provider, and [provider-upjet-gcp
v3.1.0](https://github.com/crossplane-contrib/provider-upjet-gcp/releases/tag/v3.1.0) has none of
them yet. Azure Event Hubs does not count as Kafka: it has no Kafka ACLs, no topic creation through
the Kafka admin API and no broker settings.

## Contents

- [KafkaCluster](#kafkacluster)
- [KafkaUser](#kafkauser)
- [KafkaTopic](#kafkatopic)
- [Platform Decisions](#platform-decisions)
- [Ignored Settings](#ignored-settings)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## KafkaCluster

The Kafka cluster itself: its version, brokers, storage and broker settings.

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

A cluster's size is its brokers times each broker's size. AWS and OCI take a broker size and count;
GCP takes the cluster's total vCPU and memory and chooses the brokers itself. The size is therefore
in each cloud's block (`aws.instanceType`, `oci.instanceType` with `oci.ocpus`, `gcp.vcpus` and
`gcp.memoryGB`), and `brokers` is ignored on GCP. No cloud offers a maintenance window.

In the table below, Level is the field's support level (rule 1).

| Field              | Level                                | AWS (MSK provisioned `Cluster`)                             | GCP (`google_managed_kafka_cluster`), no provider resource                                                  | OCI (`KafkaCluster`)                                                                |
| ------------------ | ------------------------------------ | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| `observe`          | Core                                 | `managementPolicies: [Observe]`                             | rejected until the provider has a Kafka resource                                                            | `managementPolicies: [Observe]`                                                     |
| `version`          | Core                                 | `kafkaVersion`                                              | `kafka_version`                                                                                             | `kafkaVersion`                                                                      |
| `brokers`          | Ignored on GCP                       | `numberOfBrokerNodes`                                       | ignored; chosen by the service                                                                              | `brokerShape.nodeCount`                                                             |
| `storage.sizeGB`   | Core                                 | `brokerNodeGroupInfo.storageInfo.ebsStorageInfo.volumeSize` | `broker_capacity_config.disk_size_gib`                                                                      | `brokerShape.storageSizeInGbs`                                                      |
| `highAvailability` | Core; `false` ignored on AWS and GCP | always; `false` ignored                                     | always three zones; `false` ignored                                                                         | `clusterType` `PRODUCTION` or `DEVELOPMENT`                                         |
| `parameters`       | Core                                 | an MSK Configuration                                        | Kafka admin API, for settings GCP marks `cluster-wide`; `read-only` ones take effect after a broker restart | `KafkaClusterConfig`; `sasl.*`, `ssl.*` and others the service ignores are rejected |

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

The status adds `bootstrapServers`, the listener applications use (`bootstrapBrokersSaslScram` on
AWS, `bootstrap_address` on GCP, `kafkaBootstrapUrls` on OCI). `brokers` stays empty on GCP.

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

## KafkaUser

An identity that applications use to log in to the cluster, with the ACLs it is granted.

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaUser
metadata:
  name: orders-app
  namespace: shop
spec:
  clusterRef:                          # required (rule 11)
    name: orders-kafka
  consumerGroups:
  - orders
  acls:
  - topic: orders
    operation: Read
```

| Cloud | Identity                                           | Login                                   |
| ----- | -------------------------------------------------- | --------------------------------------- |
| AWS   | a SCRAM secret in Secrets Manager (`AmazonMSK_*`)  | SASL SCRAM-SHA-512                      |
| GCP   | a service account with a key                       | SASL PLAIN: the account's email and key |
| OCI   | provider-kafka `User`, through the Kafka admin API | SASL SCRAM-SHA-512                      |

ACLs are Kafka ACLs on every cloud: provider-kafka on AWS and OCI,
[`google_managed_kafka_acl`](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/managed_kafka_acl)
or the Kafka admin API on GCP.

Secret `<cluster>-<user>`: `endpoint` (the bootstrap servers, as `host:port` pairs, so no `port`),
`username`, `password` and `saslMechanism` (`SCRAM-SHA-512` or `PLAIN`), since the mechanism
differs by cloud.

## KafkaTopic

A topic in the cluster, created through the Kafka admin API on every cloud.

```yaml
---
apiVersion: queue.entigo.com/v1alpha2
kind: KafkaTopic
metadata:
  name: orders
  namespace: shop
spec:
  clusterRef:                          # required (rule 11)
    name: orders-kafka
  name: orders_v2                      # the Kafka name; unset, metadata.name
  partitions: 3
  replicationFactor: 3                 # at most the broker count
  parameters:
    retention.ms: "604800000"
```

Created through the Kafka admin API on every cloud. `name` is the topic's Kafka name, which can hold
what a Kubernetes name cannot, such as `Orders` or `orders_v2`. Applications use that name, so it
cannot carry a hash like rule 9's cloud names, and two `KafkaTopic` objects on one cluster could
request the same topic and both manage it. The platform rejects the newer one instead, naming the
object that holds the name.

## Platform Decisions

Network placement, TLS, the login mechanism, encryption with the platform's data key (AWS, GCP; OCI
has no customer key), the admin login provider-kafka uses (IAM on AWS, the superuser's Vault secret
on OCI), and deny by default: a topic or group is reachable only by users with an ACL on it and by
the admin. MSK and OCI set `allow.everyone.if.no.acl.found=false` in the cluster configuration, with
the admin as a superuser; GCP needs explicit ALLOW ACLs for the admin on every resource.

## Ignored Settings

Settings this service accepts on every cloud but that have no effect on some (rule 1), and what that
means there.

| Setting                   | Ignored On | Effect                                |
| ------------------------- | ---------- | ------------------------------------- |
| `brokers`                 | GCP        | GCP chooses the broker count.         |
| `highAvailability: false` | AWS, GCP   | The cluster runs across zones anyway. |

## Cloud Limits

Limits each cloud enforces itself, which the API does not check (rule 10); a value outside them is
rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference, should
validating them be decided later.

| Field            | Cloud | Limit                          | Source                                                                      |
| ---------------- | ----- | ------------------------------ | --------------------------------------------------------------------------- |
| `version`        | all   | the versions each cloud offers | each cloud's docs                                                           |
| `brokers`        | AWS   | a multiple of the zone count   | `numberOfBrokerNodes`, MSK `Cluster` CRD, provider-upjet-aws v2.4.0         |
| `brokers`        | OCI   | 1 to 30                        | `brokerShape.nodeCount`, `KafkaCluster` CRD, crossplane-provider-oci v2.0.0 |
| `storage.sizeGB` | GCP   | at least 100                   | `disk_size_gib`, `google_managed_kafka_cluster` docs                        |
| `storage.sizeGB` | OCI   | at least 50                    | `brokerShape.storageSizeInGbs`, `KafkaCluster` CRD                          |
| `gcp.vcpus`      | GCP   | at least 3                     | `vcpu_count`, `google_managed_kafka_cluster` docs                           |
| `gcp.memoryGB`   | GCP   | 1 to 8 per vCPU                | `memory_bytes`, `google_managed_kafka_cluster` docs                         |

## Notes

Facts about this service that the design relies on and that need no decision.

- MSK Express brokers have no storage to size, so `storage.sizeGB` would be rejected with an
  `express.*` instance type.
- GCP: users are IAM service accounts, and a service account key in a secret is the only static
  credential; workload identity would need no secret but cloud-specific client code.
- Unverified: OCI SCRAM users through the Kafka admin API (Oracle documents only the superuser).
