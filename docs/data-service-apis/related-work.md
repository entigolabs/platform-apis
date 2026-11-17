# Related Work

The design draws on the projects in Prior Art, and was chosen over the options in Alternatives. Both
come from Rein Remmel's proposal,
[Provider Configs and Classes for Portable Data Service APIs](https://claude.ai/artifact/XEQLcGTK3Co7dwuygecC7M).

## Prior Art

Each project below solved a part of the same problem, and the design takes one idea from each:

| Project                                                                                                                                                                                                          | Idea Taken                                                                                                                                                     |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Kubernetes in-tree to CSI migration](https://kubernetes.io/blog/2021/12/10/storage-in-tree-to-csi-migration-status-update/)                                                                                     | The warning: cloud-specific fields in a core schema took several years to move out                                                                             |
| [StorageClass](https://kubernetes.io/docs/concepts/storage/storage-classes/) and [IngressClass](https://kubernetes.io/docs/concepts/services-networking/ingress/)                                                | A portable object names a class; each cluster defines that class name for its own implementation, and the class names one implementation                       |
| [Gateway API](https://gateway-api.sigs.k8s.io/): GatewayClass and [GEP-1867](https://gateway-api.sigs.k8s.io/geps/gep-1867/)                                                                                     | Class parameters owned by the cluster's operator, with parameters per Gateway layered on top                                                                   |
| [Gateway API GEP-713](https://gateway-api.sigs.k8s.io/geps/gep-713/), policy attachment                                                                                                                          | The split between defaults a lower level may change and overrides it may not, here `template` and `enforced`                                                   |
| [Gateway API GEP-2162](https://gateway-api.sigs.k8s.io/geps/gep-2162/), supported features                                                                                                                       | Implementations report their supported features in the class's status, here `supportedFeatures`                                                                |
| [kube-scheduler configuration](https://kubernetes.io/docs/reference/scheduling/config/), `pluginConfig`                                                                                                          | A list of typed entries, each consumed only by the component it names                                                                                          |
| [Crossplane](https://docs.crossplane.io/latest/) composition function `input`                                                                                                                                    | An inline object with its own `apiVersion` and `kind`, validated by the function that consumes it, already used in `platform-apis`                             |
| [Cluster API](https://cluster-api.sigs.k8s.io/)                                                                                                                                                                  | One core API with providers shipped independently in their own groups, on-premises ones included; also the counter-example of a template kind per kind         |
| [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)                                                                         | Unions with a discriminator, and list-type maps for lists merged by key                                                                                        |
| [LimitRange](https://kubernetes.io/docs/concepts/policy/limit-range/)                                                                                                                                            | Defaults with bounds per namespace, the model for allowed values in a class, should they be added                                                              |
| [CloudNativePG](https://cloudnative-pg.io/documentation/current/postgresql_conf/)                                                                                                                                | Engine parameters a user may set, with a fixed list the operator keeps                                                                                         |
| [CloudNativePG replica clusters](https://cloudnative-pg.io/documentation/current/replica_cluster/), distributed topology                                                                                         | One manifest applied to several clusters, each taking its role from the named primary, with switchover by changing one field: the model for `disasterRecovery` |
| [AlloyDB read pools](https://cloud.google.com/alloydb/docs/instance-read-pool-scale) and [Aurora Auto Scaling](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/Aurora.Integrating.AutoScaling.html) | Read scaling as a count of identical replicas                                                                                                                  |
| [Aurora Global Database](https://docs.aws.amazon.com/AmazonRDS/latest/AuroraUserGuide/aurora-global-database.html)                                                                                               | Secondary clusters without instances as a small standby, with managed switchover and failover                                                                  |
| [AWS Secrets Manager replica secrets](https://docs.aws.amazon.com/secretsmanager/latest/userguide/create-manage-multi-region-secrets.html)                                                                       | Credentials readable in the disaster recovery region during a regional outage                                                                                  |
| [KubeVirt instancetypes](https://kubevirt.io/user-guide/user_workloads/instancetypes/) and [Karpenter NodePools](https://karpenter.sh/docs/concepts/nodepools/)                                                  | Portable size names and sizing by requirements, both considered and dropped for managed services                                                               |

*Projects the design draws on.*

## Alternatives

Each option below was considered for one part of the design and dropped for the reason given.

### Settings of One Implementation

The design holds them in provider configurations (principle 2). The options it was chosen over:

| Alternative                                                                               | Used By                                                                                                    | Reason                                                                                                                                                    |
| ----------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Fixed blocks per cloud: `spec.aws`, `spec.gcp` and so on                                  | Revision 1 of this design; Kubernetes in-tree volume sources (`awsElasticBlockStore`, `gcePersistentDisk`) | A closed list: every new implementation changes every kind (principle 3). Kubernetes spent years moving its volume sources out to CSI.                    |
| Only a reference to an object of the implementation: `parametersRef`, `infrastructureRef` | Cluster API, Gateway API, Crossplane `providerConfigRef`                                                   | One reference names one kind, so a manifest cannot carry settings for several implementations. Kept inside each entry as its `ref` form.                  |
| A separate policy object attached to the object                                           | Gateway API policy attachment (GEP-713)                                                                    | A manifest holding a policy of one implementation fails to apply where that kind is not installed, and one object's settings spread over several objects. |
| A string map: `parameters: map[string]string`                                             | CSI `StorageClass.parameters`, Dapr component `metadata`                                                   | No types, no versions and no owner; keys of different implementations can collide.                                                                        |
| Domain-prefixed annotations                                                               | `service.beta.kubernetes.io/aws-load-balancer-*`                                                           | Untyped and not discoverable; usable as an escape hatch, not as the main mechanism.                                                                       |
| Entries identified by `kind` alone                                                        | (none)                                                                                                     | Kinds of different owners collide; the API group carries the owner's domain.                                                                              |

*Alternatives for the settings of one implementation.*

### Machine Sizes

The design names a size as the product names it, in its provider configuration
([Provider Configurations](common-design.md#provider-configurations)). The options it was chosen
over:

| Alternative                                                 | Used By                                                | Reason                                                                                                                                 |
| ----------------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------- |
| Portable size names mapped per cloud, such as `general-2x8` | KubeVirt instancetypes, Aiven and Crunchy Bridge plans | Too many sizes to keep a mapping for, and users learn names that match no cloud's documentation.                                       |
| CPU and memory, resolved to the nearest size                | Karpenter `NodePool` requirements                      | Needs the full size catalogue of every cloud and region. Natural only where it is the implementation's own form, as for CloudNativePG. |
| `instanceType` as a common field                            | Today's `PostgreSQLInstance`                           | One value cannot be valid in two implementations, so a manifest for several deployments cannot set it.                                 |

*Alternatives for machine sizes.*

### Classes

The design holds a deployment's defaults and limits in classes (principle 5). The options it was
chosen over:

| Alternative                                                                                     | Used By                                                   | Reason                                                                                         |
| ----------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| One `EnvironmentConfig` per service family, with the fields a user may override decided in code | Today's `platform-apis`                                   | One profile per cluster, and changing the fields a user may override needs a code change.      |
| A class with defaults and a `locked` list of field names                                        | (none)                                                    | Values and locks live in two places; `enforced` holds the value and the lock together.         |
| Enforced values that silently win over the object                                               | Gateway API `overrides`                                   | The user cannot tell why a value has no effect; the object is refused when applied instead.    |
| A list of provider configurations in the class, as in the object                                | (none)                                                    | A class belongs to one deployment, where several provider configurations mean nothing.         |
| A `spec.provider` field naming the class's implementation                                       | `StorageClass.provisioner`, `GatewayClass.controllerName` | Repeats the provider configuration's own `apiVersion` and `kind`.                              |
| A template kind per kind, such as `PostgreSQLTemplate`                                          | Cluster API `AWSMachineTemplate`                          | Doubles the number of kinds; a class's `template` reuses the object's own types.               |
| `defaultProviderConfigs` and `overrideProviderConfigs` fields                                   | Gateway API `defaults` and `overrides`                    | `template` already holds the defaults, and "override" leaves open which level overrides which. |

*Alternatives for classes.*

### Replicas and Disaster Recovery

The design requests replicas as a count, and keeps a disaster recovery copy in another workspace
([Read Replicas](common-design.md#read-replicas),
[Disaster Recovery](common-design.md#disaster-recovery)). The options it was chosen over:

| Alternative                                                                      | Used By                                                                                                          | Reason                                                                                                                                                                  |
| -------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A separate object per replica, naming its primary in `replicaOf`                 | RDS and Cloud SQL resource models                                                                                | Replicas belong to the same user and application as their primary, and a count is simpler; the platform's function turns the count into one cloud resource per replica. |
| A `replicas` list in the primary, each item with its own provider configurations | (none)                                                                                                           | Provider configuration lists nest inside a list, which server-side apply merges poorly.                                                                                 |
| Read pools with sizes in the common spec                                         | AlloyDB read pools                                                                                               | Not portable: OCI and CloudNativePG cannot run replicas of different sizes (principle 1). Kept as a replica layout in the provider configuration.                       |
| A `PostgreSQLReplica` kind                                                       | (none)                                                                                                           | Repeats nearly every field of `PostgreSQLCluster`.                                                                                                                      |
| A replica on a newer major version                                               | [RDS Blue/Green Deployments](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/blue-green-deployments.html) | Uses logical replication, a different mechanism with its own limits; it belongs to upgrades and migration.                                                              |
| The primary's platform creates the disaster recovery replica                     | (none)                                                                                                           | Lost together with the primary's region, so no platform remains to promote the replica.                                                                                 |
| A region in the object's spec                                                    | (none)                                                                                                           | Region names belong to one cloud, and network placement is the platform's decision (principle 2).                                                                       |
| The disaster recovery source's cloud ID in the manifest                          | (none)                                                                                                           | It belongs to one cloud and differs per workspace; the disaster recovery platform finds the source by its tags.                                                         |
| Support listed in field descriptions, such as "Ignored on AWS, OCI"              | Revision 1 of this design                                                                                        | A closed list of clouds again; classes report their supported features instead (principle 1).                                                                           |

*Alternatives for replicas and disaster recovery.*
