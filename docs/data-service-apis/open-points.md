# Open Points

Questions to decide before implementation.

#### 1 Which API group do the kinds take, and how do existing objects move to it?

**Context.** Most user-facing kinds take new names, such as `PostgreSQLCluster` beside today's
`PostgreSQLInstance` and `Bucket` beside `S3Bucket`, and are served beside today's kinds in today's
category groups (`database.`, `queue.`, `mq.`, `storage.` and `artifact.entigo.com`). Six kinds keep
their names: `RabbitMQBroker`, `KafkaUser`, `PostgreSQLUser`, `PostgreSQLDatabase`, `MariaDBUser`
and `MariaDBDatabase`. Kubernetes stores the objects of a group and kind in a single version; any
other version it serves is a conversion of that one.

**Problem.** An object of a renamed kind can move by itself, but the steps of a move, from today's
object to the new one without touching the cloud resource, are not designed. The six kinds that keep
their names cannot run in both forms side by side, so each switches at once. The category groups
also do not present the design as one family.

**Options:**

| Option                                                         | Migration                                                                                                                                                                                 | Cost                                                                                                                |
| -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Same groups, renamed kinds beside today's                      | Each object of a renamed kind moves at its user's pace, through `observe` and adoption; the six kinds that keep their names switch at once, by a conversion webhook or a one-time rewrite | Proving for the six kinds that no cloud resource changes; today's kinds stay served until the last object has moved |
| One new group, `data.entigo.com`                               | Every object moves at its user's pace, the six kinds included                                                                                                                             | Today's kinds stay served, and their functions run, until the last object has moved                                 |
| A new group per category, `database.data.entigo.com` and so on | As with one new group                                                                                                                                                                     | As with one new group, with longer names                                                                            |

*Ways to move today's objects to the design.*

#### 2 How are credentials rotated?

**Context.** The platform generates every credential: an instance's admin password, the password of
each database, RabbitMQ and Kafka user, and a Valkey cache's password; a `KafkaUser` on GCP has a
service account key instead. Applications read it from one Secret with the same keys on every cloud
(principle 6).

**Problem.** Nothing in the API replaces a credential, so a leaked or aged one stays valid. Once a
credential is replaced, an application keeps the old value until it reads the Secret again, so how
the switch happens decides whether applications lose their connection.

**Options:**

| Option                  | Rotation                                                                                                                  | Cost                                                                                        |
| ----------------------- | ------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| None in the API         | A credential changes only when its object is recreated, or by hand in the cloud                                           | Nothing to build; no answer to a leaked credential                                          |
| On request, at once     | A field such as `spec.rotation`: changing it writes a new credential into the same Secret, and the old one stops working  | Applications fail until they read the Secret again                                          |
| On request, overlapping | The platform keeps two logins per user and moves the Secret to the other one; the old login works until the next rotation | No downtime, but twice the logins, and only for users inside an engine, not its admin login |

*Ways to rotate a credential.*

#### 3 How do users see a resource's health?

**Context.** A resource's status reports what the cloud runs, such as its version, size and
endpoint. Its metrics, such as CPU, storage used and connections, live in each cloud's monitoring
service, to which platform users usually have no access. Infralib deploys Prometheus, Grafana and
Mimir in every cluster.

**Problem.** A user cannot see whether a resource is healthy. A disk that fills up to `maxSizeGB`,
exhausted connections or a failed backup show only when the application breaks.

**Options:**

| Option                     | Health                                                                                                                              | Cost                                                                                                                                  |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| None in the platform       | Users ask the platform team, who read the cloud's console                                                                           | Nothing to build; problems surface as outages                                                                                         |
| Console link in the status | `status.provider.consoleURL` opens the resource in its cloud's console                                                              | Little to build; useful only to users with access to the cloud                                                                        |
| Metrics in the cluster     | The platform exports each resource's cloud metrics into Prometheus, labelled by object and namespace, with standard alerts per kind | An exporter per cloud (CloudWatch, Cloud Monitoring, OCI Monitoring, Azure Monitor); users see health in the Grafana they already use |

*Ways users see a resource's health.*

#### 4 Can a class require a provider configuration entry?

**Context.** An object with no provider configuration entry of its class's kind takes the class's
provider configuration as it is
([Provider Configurations](common-design.md#provider-configurations)).

**Problem.** A setting chosen per application, such as a machine size, then falls back to the
class's value without notice. A deployment where every application must choose its own size has no
way to require the entry.

**Options:**

| Option                       | Effect                                                                                                                    | Cost                                                                                          |
| ---------------------------- | ------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Always fall back             | An object without an entry runs with the class's provider configuration                                                   | Nothing to build; a forgotten entry goes unnoticed                                            |
| A class can require an entry | A class marks its provider configuration as required, and an object without an entry of that kind is refused when applied | A manifest for several deployments carries an entry for each implementation that requires one |

*Ways to treat an object without an entry.*

#### 5 Can a Zone restrict the classes its namespaces use?

**Context.** Classes are cluster-scoped, and an object in any namespace may name any class of its
kind ([Classes](common-design.md#classes)).

**Problem.** A Zone for a test environment can request the deployment's largest class, and nothing
limits the classes per Zone.

**Options:**

| Option                   | Effect                                                                                                                                           | Cost                                                 |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------- |
| No restriction           | Every Zone may use every class                                                                                                                   | Nothing to build; cost control stays outside the API |
| A Zone lists its classes | The Zone names the classes its namespaces may use, and an admission policy refuses others, as Gateway API's `allowedRoutes` restricts attachment | A list per Zone to keep current as classes are added |
| A class lists its Zones  | The class names the Zones that may use it                                                                                                        | Entigo's classes change whenever a Zone is added     |

*Ways to restrict the classes per Zone.*

#### 6 Which further needs become portable features?

**Context.** Classes report two portable features, `HighAvailability` and `ReadReplicas`, and an
object that requests one its class does not report is refused (principle 1).

**Problem.** Other needs, such as replica autoscaling, exist in some implementations only. Without a
portable feature for them, they can only be settings of one implementation, and a manifest cannot
state them portably.

**Options:**

| Option                                     | Effect                                                                                                                 | Cost                                                    |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| The two features only                      | Every other need is a setting of one implementation                                                                    | No portable form for needs such as autoscaling          |
| A feature once two implementations meet it | A need becomes a portable feature when two or more implementations support it, as principle 1 states for common fields | A design change, and a revision, for each feature added |

*Ways to add portable features.*
