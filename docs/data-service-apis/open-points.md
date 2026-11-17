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
