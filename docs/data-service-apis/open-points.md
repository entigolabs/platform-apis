# Open Points

Questions to decide before implementation.

#### 1 Should a User's Permissions Be Defined in the User or in a Separate Grant?

The design adds `RabbitMQUser` and `RabbitMQVirtualHost` to RabbitMQ, so applications stop sharing
the broker's admin login. Today `KafkaUser` and `MariaDBUser` hold their own permissions, so whoever
writes the user decides what it reaches. A separate grant per service (`RabbitMQGrant`,
`KafkaGrant`, `MariaDBGrant`, `PostgreSQLGrant`, named after `RoleGrant` in `authz.entigo.com`)
would bind a user to a virtual host, topic or database with what it may do there, and would allow
the team owning that object grant another team's user access to it within the Zone (rule 11). The
choice changes the user kinds of all four services.

#### 2 Should Extended Fields Fail or Be Ignored on a Cloud That Lacks Them?

Some common fields exist on only some clouds, such as `storage.iops`, which Azure lacks. Today such
a field is Extended: on a cloud without it, the object reports `CloudAccepted: False` and nothing is
created, as Gateway API does with its Extended features. A manifest that uses it therefore works on
some clouds and fails on others. Dropping Extended would make these fields Ignored on the clouds
that lack them: the resource is created without the setting, so every manifest works on every
cloud, but what runs can differ from what was asked, such as a slower disk, and the Ignored level
would no longer mean that nothing is lost. The choice affects about 25 fields and values across six
schemas.
