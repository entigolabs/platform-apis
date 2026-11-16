# Revisions

The design's revisions, newest first. A revision changes what the API is or does: a field, a
default, a rule, a mapping, a platform decision or a cloud limit. Changes to wording and layout are
not revisions. A change to the design adds its line to the open revision, or opens the next one once
the last is agreed.

#### 3 2026-11-16

- An operation a cloud cannot perform fails, with the reason on the object (rule 1).
- The schema holds a block for every cloud before its implementation exists, and implementing a
  cloud never changes the common fields (rule 3).
- Applications connect the same way on every cloud (rule 6).
- The `platform-apis` chart installs each service's API only on the clouds that run it (rule 7).
- A name that applications use, such as a Kafka topic or an image path, keeps the user's name, and a
  second object asking for it is rejected (rule 9).
- An observed resource takes no field other than its cloud block's `id` (rule 9).
- `brokerRef` joins the references that may name another namespace of the Zone (rule 11).
- `ValkeyInstance` gains `aws.fromBackup`.
- `MariaDBUser` loses `grant.users`, which never reached MariaDB.

#### 2 2026-11-12

- A setting a cloud lacks is ignored there instead of failing (rule 1).
- Backup IDs move into the cloud blocks, as `<cloud>.fromBackup` (rule 2).
- A user is one application's identity on one instance, and holds its own permissions; separate
  grant objects are deferred (rule 12).
- `MariaDBUser` holds privileges on several databases, as `databases`.

#### 1 2026-11-11

- The first design: common fields that state needs, a block per cloud with its sizes, a support
  level for every field, and `observe` on every kind that represents a cloud resource.
