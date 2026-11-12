# Open Points

Questions to decide before implementation.

#### 1 Should Extended fields fail or be ignored on a cloud that lacks them?

Some common fields exist on only some clouds, such as `storage.iops`, which Azure lacks. Today such
a field is Extended: on a cloud without it, the object reports `CloudAccepted: False` and nothing is
created, as Gateway API does with its Extended features. A manifest that uses it therefore works on
some clouds and fails on others. Dropping Extended would make these fields Ignored on the clouds
that lack them: the resource is created without the setting, so every manifest works on every
cloud, but what runs can differ from what was asked, such as a slower disk, and the Ignored level
would no longer mean that nothing is lost. The choice affects about 25 fields and values across six
schemas.
