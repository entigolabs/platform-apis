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

#### 2 What Happens to a Field on a Cloud That Lacks It?

Some common fields exist on only some clouds, such as `storage.iops`, which Azure lacks. Today such
a field is Extended: on a cloud without it, the object reports `CloudAccepted: False` and nothing is
created, as Gateway API does with its Extended features, so a manifest that uses it applies
everywhere but works only where the field is supported. Two other answers make every manifest work
everywhere:

- The cloud ignores the field and creates the resource anyway, with a warning condition naming
  what it ignored. The manifest keeps working, but the resource can differ from what was asked,
  such as a slower disk, and fields that protect data or change behaviour, such as
  `immutableTags` or `cors`, would need to stay Extended.
- The field moves into the cloud blocks (`aws.iops`, `gcp.iops`, `oci.iops`), which other clouds
  ignore by design (rule 2). Nothing is lost silently, since the value was written for that
  cloud, but it is written once per cloud.

The choice affects about 25 fields and values across six schemas.
