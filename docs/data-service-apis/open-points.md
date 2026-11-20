# Open Points

Questions to decide before implementation.

#### 1 How does a class change reach the objects that use it?

**Context.** A class's `template` is written into an object when it is created, and its `enforced`
values apply all the time ([Classes](common-design.md#classes)). The class name is the portable
contract that charts name, so a deployment changes a class in place rather than under a new name.

**Problem.** When Entigo adds or changes an `enforced` value, such as `highAvailability: true`, every
object of that class changes at once: fifty clusters gain a standby together, with no way to roll
the change out gradually.

**Options:**

| Option                   | Effect                                                                                                                                                                                                                          | Trade-offs                                                                                             |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Creation only            | A class applies when an object is created; an existing object changes only when its owner edits it                                                                                                                              | `enforced` holds only at creation, unless every update is checked against the class                    |
| Continuous               | Every class change applies to every object of the class at once                                                                                                                                                                 | One edit changes the whole deployment, with no gradual rollout                                         |
| Update policy per object | Each class change creates a numbered revision; an object with `classUpdatePolicy: Automatic` follows the newest, and one with `Manual`, the default, stays on its revision until moved, with a condition that shows a newer one | Classes keep stored revisions, as Crossplane's CompositionRevisions do, and every object gains a field |

*Ways a class change can reach existing objects.*
