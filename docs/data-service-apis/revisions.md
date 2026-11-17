# Revisions

The design's revisions, newest first. A revision changes what the API is or does: a field, a
default, a rule, a mapping, a platform decision or a cloud limit. Changes to wording and layout are
not revisions. A change to the design adds its line to the open revision, or opens the next one once
the last is agreed.

#### 2 in progress

- Settings of one implementation move from the fixed cloud blocks into an open list of provider
  configs, one kind per implementation, so a new cloud or an on-premises implementation never
  changes the core kinds (principles 2, 3 and 8). From Rein Remmel's proposal.
- Classes hold a deployment's defaults (`template`) and the values a user cannot
  change (`enforced`), replacing defaults passed through infralib (principle 5).
- An object that requests a portable feature its class does not support is refused instead of
  ignored (principle 1).
- Provider config entries are checked by their implementation, and by CI against every
  implementation's published schema (principle 10).
- Field groups exist only as units or subsystems (principle 13).
- PostgreSQL: `PostgreSQLInstance` becomes `PostgreSQLCluster`, with `PostgreSQLClass`,
  `PostgreSQLParameters` and a provider config kind per implementation; users and databases name it
  in `clusterRef`.
- MariaDB: `MariaDBInstance` becomes `MariaDBServer`, with `MariaDBClass`, `MariaDBParameters` and
  `MariaDBRDSConfig`, read replicas and disaster recovery; users and databases name it in
  `serverRef`.
- Valkey: `ValkeyInstance` becomes `ValkeyServer`, with `ValkeyClass`, `ValkeyParameters` and a
  provider config kind per implementation; `replicas` becomes `readReplicas`.
- PostgreSQL gains read replicas (`readReplicas`) and disaster recovery across workspaces
  (`disasterRecovery`).

#### 1 2026-11-16

- Initial release.
