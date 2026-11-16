# Open Points

Questions to decide before implementation.

#### 1 Which API group do the kinds take, and how do existing objects move to it?

**Context.** Most kinds keep their group and name in the design: `PostgreSQLInstance` is
`database.entigo.com/v1alpha1` today and `database.entigo.com/v1alpha2` in the design, in one of
today's category groups (`database.`, `queue.`, `mq.`, `storage.` and `artifact.entigo.com`).
Kubernetes stores the objects of a group and kind in a single version; any other version it serves
is a conversion of that one.

**Problem.** Today's API and the design cannot run side by side for these kinds, so their objects
cannot move from one to the other gradually: each kind switches at once. The category groups also do
not present the design as one family.

**Options:**

| Option                                                         | Migration                                                                        | Cost                                                                                |
| -------------------------------------------------------------- | -------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Same groups, cutover                                           | One switch for the whole platform, by a conversion webhook or a one-time rewrite | Proving per kind that no cloud resource changes; no way back                        |
| One new group, `data.entigo.com`                               | Each object moves once, at its user's pace, through `observe` and adoption       | Today's kinds stay served, and their functions run, until the last object has moved |
| A new group per category, `database.data.entigo.com` and so on | As with one new group                                                            | As with one new group, with longer names                                            |

*Ways to move today's objects to the design.*
