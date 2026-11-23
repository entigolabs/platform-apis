# Open Points

Questions to decide before implementation.

#### 1 How are class defaults written on clusters older than Kubernetes 1.36?

**Context.** A `MutatingAdmissionPolicy` writes a class's `template` into each object when it is
created ([Classes](common-design.md#classes)). It is stable and on by default from Kubernetes 1.36;
in 1.34 and 1.35 it is a beta API that is off unless the cluster enables it.

**Problem.** A deployment whose cluster runs an older version gets no defaults from its classes, so
an object that relies on its class for a required setting is refused. Some clients may still run
such versions when the design rolls out.

**Options:**

| Option                   | Effect                                                                | Trade-offs                                                                                    |
| ------------------------ | --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Kubernetes 1.36 required | The chart installs only on 1.36 or later                              | A client on an older version cannot use the design until it upgrades                          |
| Mutating webhook         | A webhook served by the platform writes the defaults on every version | The platform runs, secures and keeps available one more webhook, which every create waits for |
| Both, by version         | The policy on 1.36 or later, the webhook below it                     | Two ways to write the same defaults, which must give the same result                          |

*Ways to write class defaults on older clusters.*

#### 2 How does an application get its identity and its grants?

**Context.** An application reaches a service with its workload identity where the implementation
supports it (principle 15). A `Bucket` creates no service account or key
([Bucket](bucket.md)), and a user object, such as `PostgreSQLUser` or `KafkaUser`, creates a login
with a password in a connection Secret ([Connection Secrets](common-design.md#connection-secrets)).
Today's `S3Bucket` creates an IAM role, keys and optionally a service account per bucket.

**Problem.** An application that uses two buckets needs one identity holding both grants: on AWS a
service account takes only one EKS Pod Identity role, and on Azure only one `client-id` annotation,
so an identity owned by each bucket breaks the second one. Each cloud also ties a database or broker
login to an identity differently: an RDS user granted `rds_iam` can no longer log in with a password,
a Cloud SQL login is named after the service account, an Azure login is created with
`pgaadauth_create_principal`, and MSK takes an IAM identity's permissions from IAM policies instead
of Kafka ACLs. Tokens last 15 to 60 minutes, so the application's client library or pool must fetch
new ones.

The choice also decides the migrations client teams go through, which take 6 to 12 months each: one
migration that includes identity starts later, while a migration now followed by one for identity
moves each application twice. A kind added later needs no change to existing manifests; a field
added to `Bucket` now and replaced later does. Migration jobs that the platform runs for a team,
performing the move path of each object, would lower the cost of every migration, whichever is
chosen.

**Options:**

| Option                         | Effect                                                                                                                                                                      | Trade-offs                                                                                                |
| ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `access` on each kind          | A bucket or user object lists the service accounts that may use it; one identity per service account, owned apart from the buckets, holds every grant                       | The platform keeps an identity alive while any object lists its service account; every kind gains a field |
| Identity kind                  | A kind in `workload.entigo.com`, such as `WorkloadIdentity`, names one service account and the buckets, databases and brokers it uses; it owns the identity and every grant | One more kind; added later, it changes no existing manifest                                               |
| Identity per bucket, as today  | A bucket creates its own role and service account                                                                                                                           | An application using two buckets needs two service accounts                                               |
| Static credentials until later | Buckets keep keys through `S3Bucket`, and user objects keep passwords; identity comes in a later revision                                                                   | Applications keep credentials that can leak and must be rotated, and move again when identity comes       |

*Ways to give an application its identity and grants.*

Known prerequisites, whichever option is chosen: EKS Pod Identity needs the
`eks-pod-identity-agent` add-on, which today's clusters lack, and does not run on Fargate; an AWS SDK
prefers environment keys and an IRSA role over Pod Identity, so an application drops both before the
keys are deleted; a grant on a bucket encrypted with the platform's data key also needs that key;
and on GCP, clusters in one project with the same namespace and service account share a grant.
