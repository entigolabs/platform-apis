# RabbitMQ

RabbitMQ is an open-source message broker that routes messages between applications through
exchanges and queues.

Three kinds: `RabbitMQBroker`, one broker, a single node or a three-node cluster across zones;
`RabbitMQUser`, one application's login, with its permissions; and `RabbitMQVirtualHost`, an
isolated set of queues and exchanges in a broker.

Built on Amazon MQ (AWS), the only managed RabbitMQ among the four clouds, so the API follows
RabbitMQ's own concepts. GCP, OCI and Azure have none.

[API Changes](changes.md#rabbitmq) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Kinds

RabbitMQ is offered as three kinds, each with the manifest a user writes.

### RabbitMQBroker

A user creates a `RabbitMQBroker` with a manifest like the one below. Its comments note value ranges
and what holds on one cloud only:

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
  namespace: shop
spec:
  version: "4.2"                       # RabbitMQ's release line; the cloud manages the patch
  highAvailability: false              # true: a three-node cluster across zones
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
  parameters:                          # rabbitmq.conf settings, within the cloud's allowlist
    consumer_timeout: "1800000"
  secretName: orders-mq-login          # unset, orders-mq-credentials (principle 6)
  aws:                                 # also takes id (principle 9)
    instanceType: mq.m7g.medium        # the Amazon MQ host instance type
    publiclyAccessible: false          # set at creation only
```

*A `RabbitMQBroker` with its common fields and its `aws` block.*

The platform names the broker, the engine is always RabbitMQ, and Amazon MQ requires minor version
upgrades, so none of these is a field. `highAvailability` chooses the deployment mode, and
`parameters`, a map, is rendered into `rabbitmq.conf`.

The status adds `managementEndpoint` (the management console URL), and `status.aws` holds `id` and
`engineVersion`. The connection secret is `<broker>-credentials`, with `endpoint` (the AMQPS host),
`port` (5671), `username`, `password` and `managementEndpoint`.

### RabbitMQUser

`RabbitMQUser` acts inside RabbitMQ, so that each application has its own login instead of the
broker's admin login.

Amazon MQ does not manage RabbitMQ users through its own API; users, virtual hosts and permissions
are created through RabbitMQ's management API, with the broker's admin login ([Amazon MQ for
RabbitMQ broker
users](https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-basic-elements-user.html)).

A user receives the secret `<broker>-<user>` with the keys of the broker's secret except
`managementEndpoint`, and no management tags, so its login cannot open the management console.

Permissions follow RabbitMQ's own model: per virtual host, three regular expressions over queue and
exchange names, for `configure` (create and delete), `write` (publish) and `read` (consume).
Permissions are held in the user (principle 12).

An example:

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQUser
metadata:
  name: orders-app
  namespace: shop
spec:
  name: orders_app                     # the user's name in RabbitMQ; unset, metadata.name
  brokerRef:
    name: orders-mq
    namespace: shop                    # unset, this namespace; within the Zone (principle 11)
  permissions:
  - virtualHost: orders
    configure: "^orders\\."
    write: "^orders\\."
    read: "^orders\\."
  secretName: orders-app-login         # unset, orders-mq-orders-app (principle 6)
```

*A `RabbitMQUser` with permissions on one virtual host.*

### RabbitMQVirtualHost

`RabbitMQVirtualHost` acts inside RabbitMQ, through its management API, as `RabbitMQUser` does. A
virtual host is an isolated set of queues and exchanges inside the broker, comparable to a database
in PostgreSQL; deleting one deletes its queues and messages, so it has `deletionProtection`. A new
virtual host grants nothing until a user is given permissions on it.

An example:

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQVirtualHost
metadata:
  name: orders
  namespace: shop
spec:
  name: orders                         # the virtual host's name in RabbitMQ; unset, metadata.name
  brokerRef:
    name: orders-mq
    namespace: shop                    # unset, this namespace; within the Zone (principle 11)
  deletionProtection: true             # deleting it deletes its queues and messages
```

*A `RabbitMQVirtualHost` in a broker.*

## Mapping

The table names what each field maps to in the cloud's provider:

| Field                    | Maps To                                                                  |
| ------------------------ | ------------------------------------------------------------------------ |
| `observe`                | `managementPolicies: [Observe]` on every managed resource                |
| `version`                | `engineVersion`                                                          |
| `aws.instanceType`       | `hostInstanceType`                                                       |
| `highAvailability`       | `deploymentMode`: `CLUSTER_MULTI_AZ` or `SINGLE_INSTANCE`                |
| `deletionProtection`     | platform blocks deletion                                                 |
| `maintenance.window`     | `maintenanceWindowStartTime` (`dayOfWeek`, `timeOfDay`, `timeZone: UTC`) |
| `parameters`             | an Amazon MQ `Configuration` holding `rabbitmq.conf`                     |
| `aws.publiclyAccessible` | `publiclyAccessible`; `true` omits the security group                    |
| `aws.id`                 | the broker ID                                                            |

*Fields of `RabbitMQBroker` and their provider fields.*

**AWS:** `aws.publiclyAccessible` is set at creation only: Amazon MQ replaces a broker to change it
(`ForceNew` in [terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws)). The
platform sets `autoMinorVersionUpgrade: true`, which Amazon MQ requires from RabbitMQ 3.13, its
oldest version. Amazon MQ then applies patch versions in the maintenance window, so `version` holds
only the release line.

## Composed Resources

A comment names the source of each value the object does not hold as written: `platform` for a value
the platform sets, and `deployment` for one the deployment sets. The `RabbitMQBroker` from
[Kinds](#kinds) is composed into the resource below on AWS, the only cloud that offers it:

```yaml Object
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
  namespace: shop
spec:
  version: "4.2"
  highAvailability: false
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
  parameters:
    consumer_timeout: "1800000"
  secretName: orders-mq-login
  aws:
    instanceType: mq.m7g.medium
    publiclyAccessible: false
```

```yaml AWS resource
---
apiVersion: mq.aws.m.upbound.io/v1beta1
kind: Broker
metadata:
  name: orders-mq-7f3a2c       # name + UID hash
  namespace: shop
spec:
  forProvider:
    region: eu-north-1         # deployment
    brokerName: orders-mq-7f3a2c  # platform
    engineType: RabbitMQ       # platform
    engineVersion: "4.2"
    hostInstanceType: mq.m7g.medium
    deploymentMode: SINGLE_INSTANCE
    autoMinorVersionUpgrade: true  # platform
    maintenanceWindowStartTime:
      dayOfWeek: SUNDAY
      timeOfDay: "03:00"
      timeZone: UTC
    configuration:             # composed from parameters
      id: c-0123456789abcdef
      revision: 1
    publiclyAccessible: false
    subnetIds:                 # platform
    - subnet-0123456789abcdef0
    securityGroups:            # platform
    - sg-0123456789abcdef0
    encryptionOptions:         # platform
      kmsKeyId: arn:aws:kms:eu-north-1:111122223333:key/data
      useAwsOwnedKey: false
    user:                      # platform
    - username: admin
      passwordSecretRef:
        name: orders-mq-login
        key: password
```

*A `RabbitMQBroker` on AWS and its composed resource.*

## Platform Decisions

The platform makes these settings itself:

| Setting           | Platform's Choice                            |
| ----------------- | -------------------------------------------- |
| Network placement | The deployment's private network             |
| Public access     | None, unless `aws.publiclyAccessible` is set |
| Encryption        | The platform's data key                      |
| Admin login       | An admin user and a generated password       |
| Broker name       | Set by the platform                          |

*Settings the platform makes for a `RabbitMQBroker`.*

## Cloud Limits

The cloud enforces these limits itself, and the API does not check them (principle 10); a value
outside them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for
reference, should validating them be decided later. Each limit holds as of its source: the provider
release named, or the cloud's documentation as read in November 2026.

| Field                    | Cloud | Limit                | Source                                                  |
| ------------------------ | ----- | -------------------- | ------------------------------------------------------- |
| `highAvailability: true` | AWS   | not on `mq.t3.micro` | Amazon MQ documentation, RabbitMQ broker instance types |

*Limits AWS enforces on RabbitMQ.*

## Notes

The design relies on these facts, which need no decision.

- `parameters` keys: Amazon MQ lists the `rabbitmq.conf` keys it accepts in "Configurable values"
  (five general keys, plus pages for OAuth2, LDAP, TLS and resource limits); others are left to
  Amazon MQ to reject (principle 10).
