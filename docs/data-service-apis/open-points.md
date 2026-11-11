# Open Points

Questions to decide before implementation.

#### 1 Where Do a User's Permissions Live: In the User, or in a Separate Grant?

RabbitMQ gets `RabbitMQUser` and `RabbitMQVirtualHost`, so applications stop sharing the broker's
admin login. Today `KafkaUser` and `MariaDBUser` hold their own permissions, so whoever writes the
user decides what it reaches. A separate grant per service (`RabbitMQGrant`, `KafkaGrant`,
`MariaDBGrant`, `PostgreSQLGrant`, named after `RoleGrant` in `authz.entigo.com`) would bind a user
to a virtual host, topic or database with what it may do there, and would let the team owning that
object grant another team's user access to it within the Zone (rule 11). The choice changes the user
kinds of all four services.

#### 2 Should One Manifest Work on Every Cloud, or Only Apply There?

A field that some clouds lack, such as `storage.iops` on Azure, is Extended today: written once in
the common spec, it works where the cloud supports it and reports `CloudAccepted: False` elsewhere,
as Gateway API does with its Extended features. A manifest therefore applies on every cloud but
works everywhere only if it avoids Extended fields. Dropping Extended would move every such field
into the cloud blocks (`aws.iops`, `gcp.iops`, `oci.iops`), which other clouds ignore by design
(rule 2), so any manifest works everywhere; the cost is writing a value once per cloud. The choice
changes about 25 fields and values across six schemas.
