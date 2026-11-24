# RabbitMQ

RabbitMQ is an open-source message broker that routes messages between applications through
exchanges and queues.

Three kinds for users: `RabbitMQBroker`, one broker, a single node or a three-node cluster across
zones; `RabbitMQUser`, one application's login, with its permissions; and `RabbitMQVirtualHost`, an
isolated set of queues and exchanges in a broker. Two kinds shape them: `RabbitMQClass`, a
deployment's defaults and limits, written by Entigo, and `RabbitMQParameters`, reusable
`rabbitmq.conf` settings.

Built on Amazon MQ (AWS), with the provider configuration kind `RabbitMQAmazonMQConfig`. GCP, OCI
and Azure have no managed RabbitMQ. The API follows RabbitMQ's own concepts.

## Kinds

RabbitMQ is offered as five kinds, each with the manifest its owner writes.

### RabbitMQClass

Entigo writes each deployment's classes, with the defaults a broker starts with (`template`) and the
values it cannot change (`enforced`) (principle 5):

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQClass
metadata:
  name: standard
  annotations:
    mq.entigo.com/is-default-class: "true"
spec:
  template:
    deletionProtection: true
    version: "4.2"
    providerConfig:
      apiVersion: aws.mq.entigo.com/v1alpha1
      kind: RabbitMQAmazonMQConfig
      spec:
        instanceType: mq.m7g.medium
  enforced:                            # teams cannot change these
    maintenanceWindow:
      day: Sun
      hourUTC: 3
```

*`RabbitMQClass` for Amazon MQ.*

Amazon MQ supports `HighAvailability`.

### RabbitMQParameters

`RabbitMQParameters` holds `rabbitmq.conf` settings that several brokers or a class share. A class
and a broker each list the ones they use in `parametersRefs`. The settings merge key by key, a later
value winning: the class's list in order, then the broker's, then the broker's own `parameters`, so
a team can put settings all its brokers share beneath one broker's own tuning:

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQParameters
metadata:
  name: orders-mq-tuning
  namespace: shop
spec:
  parameters:
    consumer_timeout: "1800000"
```

*`RabbitMQParameters` shared between a user's brokers.*

A `RabbitMQParameters` names no other, so references never chain. Settings Amazon MQ does not
accept are rejected by the cloud and reported as `CloudAccepted: False` (principle 10).

### RabbitMQBroker

A user creates a `RabbitMQBroker` with a manifest like the one below. It names its class and
carries a provider configuration entry for Amazon MQ (principle 2):

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
  namespace: shop
spec:
  className: standard                  # unset, the default class; without one, standalone
  version: "4.2"                       # the release line; the backend manages the patch
  highAvailability: false              # true: a three-node cluster across zones
  deletionProtection: true
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  parametersRefs:
  - name: orders-mq-tuning             # RabbitMQParameters objects in this namespace, in order
  parameters:                          # this broker's own rabbitmq.conf settings, merged last
    consumer_timeout: "1800000"
  secretName: orders-mq-login          # unset, orders-mq-credentials (principle 6)
  providerConfigs:
  - apiVersion: aws.mq.entigo.com/v1alpha1
    kind: RabbitMQAmazonMQConfig
    spec:
      instanceType: mq.m7g.medium      # the Amazon MQ host instance type
      publiclyAccessible: false        # set at creation only
```

*`RabbitMQBroker` with common fields and an Amazon MQ entry.*

The platform names the broker, the engine is always RabbitMQ, and Amazon MQ requires minor version
upgrades, so none of these is a field. `parameters`, a map, is rendered into `rabbitmq.conf`.

`highAvailability` chooses the deployment mode. It is a portable feature: an object that requests it
where its backend does not support it is refused when applied (principle 1).

The status shows the common fields, `managementEndpoint`, the management console URL, which entry
applied, where each effective value came from, and Amazon MQ's own values (principle 4):

```yaml
status:
  version: "4.2"
  highAvailability: false
  endpoint:
    host: b-0123.mq.eu-north-1.amazonaws.com
    port: 5671
  managementEndpoint: https://b-0123.mq.eu-north-1.amazonaws.com
  connectionSecret: orders-mq-login
  providerConfig:
    applied: aws.mq.entigo.com/v1alpha1, RabbitMQAmazonMQConfig
  effective:
    deletionProtection: { value: true, source: object }
    instanceType: { value: mq.m7g.medium, source: object/RabbitMQAmazonMQConfig }
  provider:                            # the shape RabbitMQAmazonMQConfig's status defines
    id: b-0123
    engineVersion: 4.2.1
```

*`RabbitMQBroker` status on AWS.*

The connection secret is `<broker>-credentials`, with `endpoint` (the AMQPS host), `port` (5671),
`username`, `password` and `managementEndpoint`.

### RabbitMQUser

`RabbitMQUser` acts inside RabbitMQ and gives each application its own login instead of the broker's
admin login. It has no class or provider configurations: it follows its broker's backend.

Amazon MQ does not manage RabbitMQ users through its own API; users, virtual hosts and permissions
are created through RabbitMQ's management API, with the broker's admin login ([Amazon MQ for
RabbitMQ broker
users](https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-basic-elements-user.html)).

A user receives the secret `<broker>-<user>`, with the keys of the broker's secret except
`managementEndpoint`. A user has no management tags, so its login cannot open the management
console.

Permissions follow RabbitMQ's own model: per virtual host, three regular expressions over queue and
exchange names, for `configure` (create and delete), `write` (publish) and `read` (consume).
Permissions are held in the user (principle 12).

A user creates a `RabbitMQUser` with a manifest like the one below:

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

*`RabbitMQUser` with permissions on one virtual host.*

### RabbitMQVirtualHost

`RabbitMQVirtualHost` acts inside RabbitMQ, through its management API, and has no class or provider
configurations. A virtual host is an isolated set of queues and exchanges inside the broker;
deleting one deletes its queues and messages, so it has `deletionProtection`. A new virtual host
grants nothing until a user is given permissions on it.

A user creates a `RabbitMQVirtualHost` with a manifest like the one below:

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

*`RabbitMQVirtualHost` in a broker.*

## Fields

The first table maps each common field to Amazon MQ:

| Field                | AWS                                                                      |
| -------------------- | ------------------------------------------------------------------------ |
| `observe`            | `managementPolicies: [Observe]` on every managed resource                |
| `version`            | `engineVersion`                                                          |
| `highAvailability`   | `deploymentMode`: `CLUSTER_MULTI_AZ` or `SINGLE_INSTANCE`                |
| `deletionProtection` | platform blocks deletion                                                 |
| `maintenanceWindow`  | `maintenanceWindowStartTime` (`dayOfWeek`, `timeOfDay`, `timeZone: UTC`) |
| `parameters`         | an Amazon MQ `Configuration` holding `rabbitmq.conf`                     |
| `parametersRefs`     | merged into `parameters`                                                 |

*`RabbitMQBroker` common fields and their provider fields.*

The second names the provider configuration kind and what its fields map to:

| Field                | AWS                                                   |
| -------------------- | ----------------------------------------------------- |
| kind                 | `RabbitMQAmazonMQConfig`                              |
| `id`                 | the broker ID                                         |
| `instanceType`       | `hostInstanceType`                                    |
| `publiclyAccessible` | `publiclyAccessible`; `true` omits the security group |

*`RabbitMQAmazonMQConfig` fields.*

**AWS:** `publiclyAccessible` is set at creation only: Amazon MQ replaces a broker to change it
(`ForceNew` in [terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws)). The
platform sets `autoMinorVersionUpgrade: true`, which Amazon MQ requires from RabbitMQ 3.13, its
oldest version. Amazon MQ then applies patch versions in the maintenance window, so `version` holds
only the release line.

## Provider Configurations

Each backend's provider configuration kind holds the settings of that backend, with every field it
takes; commented fields are alternatives or apply only with `observe`. The settings sit either in a
separate object, named by an entry's `ref`, or inline in an entry's `spec`, as each pair shows, and
they override the class's settings for the same kind field by field (principle 2):

```yaml Separate object
---
apiVersion: aws.mq.entigo.com/v1alpha1
kind: RabbitMQAmazonMQConfig
metadata:
  name: orders-mq-aws
  namespace: shop
spec:
  instanceType: mq.m7g.medium      # the Amazon MQ host instance type
  publiclyAccessible: false        # set at creation only
  # id: b-1234abcd-56ef-78ab-90cd-ef1234567890  # an existing broker's ID, with observe
```

```yaml AWS entry
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
  namespace: shop
spec:
  providerConfigs:
  - apiVersion: aws.mq.entigo.com/v1alpha1
    kind: RabbitMQAmazonMQConfig
    spec:
      instanceType: mq.m7g.medium      # the Amazon MQ host instance type
      publiclyAccessible: false        # set at creation only
      # id: b-1234abcd-56ef-78ab-90cd-ef1234567890  # an existing broker's ID, with observe
```

*`RabbitMQAmazonMQConfig`: a separate object, and the same settings in an entry.*

## Composed Resources

The `RabbitMQBroker` from [Kinds](#kinds) is composed into the resource below on AWS, the only cloud
that offers it. A comment names the source of each value the object does not hold as written:
`platform` for a value the platform sets, and `deployment` for one the deployment sets. The object
and its resource:

```yaml Object on AWS
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
  namespace: shop
spec:
  className: standard
  version: "4.2"
  highAvailability: false
  deletionProtection: true
  maintenanceWindow:
    day: Sun
    hourUTC: 3
  parameters:
    consumer_timeout: "1800000"
  secretName: orders-mq-login
  providerConfigs:
  - apiVersion: aws.mq.entigo.com/v1alpha1
    kind: RabbitMQAmazonMQConfig
    spec:
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

*`RabbitMQBroker` on AWS and its composed resource.*

## Platform Decisions

Besides the settings it makes for every service
([Platform Decisions](common-design.md#platform-decisions)), the platform makes these
for a `RabbitMQBroker` itself:

| Setting     | Platform's Choice                      |
| ----------- | -------------------------------------- |
| Admin login | An admin user and a generated password |

*`RabbitMQBroker` settings chosen by the platform.*

## Limits

The backend enforces these limits itself, and the API does not check them (principle 10); a value
outside them is rejected by the backend and shown as `CloudAccepted: False`. They are listed for
reference, should validating them be decided later. Each limit holds as of its source: the provider
release named, or the cloud's documentation as read in November 2026.

| Field                    | Backend | Limit                | Source                                                  |
| ------------------------ | ------- | -------------------- | ------------------------------------------------------- |
| `highAvailability: true` | AWS     | not on `mq.t3.micro` | Amazon MQ documentation, RabbitMQ broker instance types |

*RabbitMQ field limits and their sources.*

## Notes

The design relies on these facts, which need no decision.

- `parameters` keys: Amazon MQ lists the `rabbitmq.conf` keys it accepts in "Configurable values"
  (five general keys, plus pages for OAuth2, LDAP, TLS and resource limits); others are left to
  Amazon MQ to reject (principle 10).
