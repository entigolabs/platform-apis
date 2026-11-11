# Open Points

Questions to decide. Each leaves a security or data-loss risk while open.

#### 1 Where Do a User's Permissions Live: In the User, or in a Separate Grant?

RabbitMQ gets `RabbitMQUser` and `RabbitMQVirtualHost`, so applications stop sharing the broker's
admin login. Today `KafkaUser` and `MariaDBUser` hold their own permissions, so whoever writes the
user decides what it reaches. A separate grant per service (`RabbitMQGrant`, `KafkaGrant`,
`MariaDBGrant`, `PostgreSQLGrant`, named after `RoleGrant` in `authz.entigo.com`) would bind a user
to a virtual host, topic or database with what it may do there, and would let the team owning that
object grant another team's user access to it within the Zone (rule 11). The choice changes the user
kinds of all four services.
