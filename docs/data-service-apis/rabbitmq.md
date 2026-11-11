# RabbitMQ

Applies the rules in [rules.md](rules.md). Can be installed on AWS only (rule 7).
Schemas: [rabbitmqbroker.yaml](schemas/rabbitmqbroker.yaml),
[rabbitmquser.yaml](schemas/rabbitmquser.yaml),
[rabbitmqvirtualhost.yaml](schemas/rabbitmqvirtualhost.yaml).

`RabbitMQBroker` is one RabbitMQ broker: a single node, or a three-node cluster across zones. Amazon
MQ is the only managed RabbitMQ among the four clouds, so its concepts come from RabbitMQ itself
and are checked against Amazon MQ.

## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Platform Decisions](#platform-decisions)
- [Connection Secret and Status](#connection-secret-and-status)
- [Users and Virtual Hosts](#users-and-virtual-hosts)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Spec

What a user writes to create a `RabbitMQBroker`. The comments note value ranges and what holds on
one cloud only.

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQBroker
metadata:
  name: orders-mq
  namespace: shop
spec:
  version: "4.2"                       # RabbitMQ's release line; the cloud manages the patch
  instanceType: mq.m7g.medium          # the cloud's own size name
  highAvailability: false              # true: a three-node cluster across zones
  deletionProtection: true
  maintenance:
    window:
      day: Sun
      hourUTC: 3
  parameters:                          # rabbitmq.conf settings, within the cloud's allowlist
    consumer_timeout: "1800000"
  aws:                                 # also takes id (rule 9)
    publiclyAccessible: false          # set at creation only
```

Fields of today's API that leave the spec: `brokerName` and `engineType` (the platform names the
broker, and the engine is always RabbitMQ), `autoMinorVersionUpgrade` (Amazon MQ requires it on),
and `deploymentMode` (replaced by `highAvailability`). `configuration`, a raw `rabbitmq.conf` text,
becomes the `parameters` map, which the platform renders into that file. `publiclyAccessible`, kept
from today's API, moves to the `aws` block (rule 2); Amazon MQ replaces a broker to change it
(`ForceNew` in [terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws)), so it
is set at creation only.

## Mapping

How each field maps to the managed service of each cloud, named by the fields of that cloud's
Crossplane provider.

| Field                    | AWS (Amazon MQ `Broker`)                                                 |
| ------------------------ | ------------------------------------------------------------------------ |
| `version`                | `engineVersion`                                                          |
| `instanceType`           | `hostInstanceType`                                                       |
| `highAvailability`       | `deploymentMode`: `CLUSTER_MULTI_AZ` or `SINGLE_INSTANCE`                |
| `deletionProtection`     | platform blocks deletion                                                 |
| `maintenance.window`     | `maintenanceWindowStartTime` (`dayOfWeek`, `timeOfDay`, `timeZone: UTC`) |
| `parameters`             | an Amazon MQ `Configuration` holding `rabbitmq.conf`                     |
| `aws.publiclyAccessible` | `publiclyAccessible`; `true` leaves out the security group               |
| `aws.id`                 | the broker ID                                                            |

## Platform Decisions

Network placement (subnets, security group), no public access unless `aws.publiclyAccessible`,
encryption with the platform's data key, the admin user and its generated password, the broker name,
and `autoMinorVersionUpgrade: true`, which Amazon MQ requires from RabbitMQ 3.13, its oldest
version; with it, Amazon MQ applies patch versions in the maintenance window, so `version` holds
only the release line.

## Connection Secret and Status

Secret `<broker>-credentials`, with `endpoint` (the AMQPS host), `port` (5671), `username`,
`password` and `managementEndpoint` (the management console URL). The status adds
`managementEndpoint`, and `status.aws` holds `id` and `engineVersion`.

## Users and Virtual Hosts

`RabbitMQUser` and `RabbitMQVirtualHost` act inside RabbitMQ, so that each application has its own
login instead of the broker's admin login. A virtual host is an isolated set of queues and exchanges
inside the broker, comparable to a database in PostgreSQL; deleting one deletes its queues and
messages, so it has `deletionProtection`. A user receives the secret `<broker>-<user>` with the keys
of the broker's secret except `managementEndpoint`, and no management tags, so its login cannot open
the management console.

Permissions follow RabbitMQ's own model: per virtual host, three regular expressions over queue and
exchange names, for `configure` (create and delete), `write` (publish) and `read` (consume). A new
virtual host grants nothing until a user is given permissions on it. Whether permissions stay in the
user or move to a separate grant object is an open point ([open-points.md](open-points.md)).

Amazon MQ does not manage RabbitMQ users through its own API; users, virtual hosts and permissions
are created through RabbitMQ's management API, with the broker's admin login ([Amazon MQ for
RabbitMQ broker
users](https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-basic-elements-user.html)).

```yaml
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQVirtualHost
metadata:
  name: orders
  namespace: shop
spec:
  brokerRef:
    name: orders-mq
---
apiVersion: mq.entigo.com/v1alpha2
kind: RabbitMQUser
metadata:
  name: orders-app
  namespace: shop
spec:
  brokerRef:
    name: orders-mq
  permissions:
  - virtualHost: orders
    configure: "^orders\\."
    write: "^orders\\."
    read: "^orders\\."
```

## Cloud Limits

Limits each cloud enforces itself, which the API does not check (rule 10); a value outside them is
rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference, should
validating them be decided later.

| Field                    | Cloud | Limit                | Source                                         |
| ------------------------ | ----- | -------------------- | ---------------------------------------------- |
| `highAvailability: true` | AWS   | not on `mq.t3.micro` | Amazon MQ docs, RabbitMQ broker instance types |

## Notes

Facts about this service that the design relies on and that need no decision.

- `parameters` keys: Amazon MQ lists the `rabbitmq.conf` keys it accepts in "Configurable values"
  (five general keys, plus pages for OAuth2, LDAP, TLS and resource limits); others are left to
  Amazon MQ to reject (rule 10).
