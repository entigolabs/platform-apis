# Related Work

The design draws on the projects in Prior Art, and was chosen over the options in Alternatives. Rein
Remmel first collected both lists in
[Provider Configs and Classes for Portable Data Service APIs](https://claude.ai/artifact/XEQLcGTK3Co7dwuygecC7M).

## Prior Art

Each project below solved part of the same problem, and the design takes one idea from each:

| Project                                                                                                                                                           | Idea Taken                                                                                                                                                    |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Kubernetes in-tree to CSI migration](https://kubernetes.io/blog/2021/12/10/storage-in-tree-to-csi-migration-status-update/)                                      | Implementation fields stay out of the core kinds from the start (principle 3); Kubernetes needed years to remove its own                                      |
| [StorageClass](https://kubernetes.io/docs/concepts/storage/storage-classes/) and [IngressClass](https://kubernetes.io/docs/concepts/services-networking/ingress/) | An object names a class by a name that every deployment defines for itself                                                                                    |
| [Gateway API](https://gateway-api.sigs.k8s.io/): GatewayClass and [GEP-1867](https://gateway-api.sigs.k8s.io/geps/gep-1867/)                                      | Parameters Entigo owns in the class, with each object's own settings applied over them                                                                        |
| [Gateway API GEP-713](https://gateway-api.sigs.k8s.io/geps/gep-713/), policy attachment                                                                           | Two levels of class values: defaults the object may change (`template`) and values it may not (`enforced`)                                                    |
| [Gateway API GEP-2162](https://gateway-api.sigs.k8s.io/geps/gep-2162/), supported features                                                                        | A class that reports the features its implementation supports (`supportedFeatures`)                                                                           |
| [kube-scheduler configuration](https://kubernetes.io/docs/reference/scheduling/config/), `pluginConfig`                                                           | A list of typed entries in which each component reads only its own (`providerConfigs`)                                                                        |
| [Crossplane](https://docs.crossplane.io/latest/) composition function `input`                                                                                     | An embedded object with its own `apiVersion` and `kind`, checked by the component that reads it; `platform-apis` already uses the form                        |
| [Cluster API](https://cluster-api.sigs.k8s.io/)                                                                                                                   | A core API whose implementations ship separately in groups of their own, on-premises ones included; its template kind per kind is the shape the design avoids |
| [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)                          | List-type maps, so entries merge by `apiVersion` and `kind`, and unions with a discriminator                                                                  |
| [LimitRange](https://kubernetes.io/docs/concepts/policy/limit-range/)                                                                                             | Bounds on values per scope, the form allowed values in a class would take                                                                                     |
| [CloudNativePG](https://cloudnative-pg.io/documentation/current/postgresql_conf/)                                                                                 | Engine parameters as a map the user sets, while the operator keeps a fixed set to itself                                                                      |
| [KubeVirt instancetypes](https://kubevirt.io/user-guide/user_workloads/instancetypes/) and [Karpenter NodePools](https://karpenter.sh/docs/concepts/nodepools/)   | Portable size names and sizing by CPU and memory, both examined and not adopted (Machine Sizes, below)                                                        |

*Projects the design draws on.*

## Alternatives

Each option below was considered for one part of the design and dropped for the reason given.

### Settings of One Implementation

The design holds them in provider configurations (principle 2). The options it was chosen over:

| Alternative                                                                                 | Used By                                                                                                    | Reason                                                                                                                               |
| ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| Fixed blocks per cloud: `spec.aws`, `spec.gcp` and so on                                    | Revision 1 of this design; Kubernetes in-tree volume sources (`awsElasticBlockStore`, `gcePersistentDisk`) | Each new implementation adds a block to every kind, which principle 3 forbids; Kubernetes' own blocks took years to retire.          |
| A single reference to an object of the implementation: `parametersRef`, `infrastructureRef` | Cluster API, Gateway API, Crossplane `providerConfigRef`                                                   | It names one kind only, so a manifest cannot serve several deployments; each entry keeps it as its `ref` form.                       |
| A policy object attached to the object                                                      | Gateway API policy attachment (GEP-713)                                                                    | A manifest with a policy kind that a deployment lacks fails to apply there, and one object's settings spread across several objects. |
| A string map: `parameters: map[string]string`                                               | CSI `StorageClass.parameters`, Dapr component `metadata`                                                   | Keys carry no type, version or owner, and the keys of two implementations can collide.                                               |
| Domain-prefixed annotations                                                                 | `service.beta.kubernetes.io/aws-load-balancer-*`                                                           | Untyped and undiscoverable; acceptable only as an escape hatch.                                                                      |
| Entries keyed by `kind` alone                                                               | (none)                                                                                                     | Two owners can choose one kind name; the API group carries the owner's domain.                                                       |
| A region in the object's spec                                                               | (none)                                                                                                     | Region names differ per cloud, and placement is the platform's decision (principle 2).                                               |

*Alternatives for the settings of one implementation.*

### Machine Sizes

The design names a size as the product names it, in its provider configuration
([Provider Configurations](common-design.md#provider-configurations)). The options it was chosen
over:

| Alternative                                                 | Used By                                                | Reason                                                                                                                     |
| ----------------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------- |
| Portable size names mapped per cloud, such as `general-2x8` | KubeVirt instancetypes, Aiven and Crunchy Bridge plans | A mapping for every size of every cloud to maintain, and names that users find in no cloud's documentation.                |
| CPU and memory, resolved to the nearest size                | Karpenter `NodePool` requirements                      | Needs each cloud's size catalogue per region; natural only where the implementation sizes this way, as CloudNativePG does. |
| `instanceType` as a common field                            | Today's `PostgreSQLInstance`                           | No value is valid in two implementations, so a manifest for several deployments cannot hold one.                           |

*Alternatives for machine sizes.*

### Classes

The design holds a deployment's defaults and limits in classes (principle 5). The options it was
chosen over:

| Alternative                                                                                | Used By                                                   | Reason                                                                                                   |
| ------------------------------------------------------------------------------------------ | --------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| One `EnvironmentConfig` per service family, with the fields a user may set decided in code | Today's `platform-apis`                                   | One profile per cluster, and a change to the fields a user may set needs a code change.                  |
| A class with defaults and a `locked` list of field names                                   | (none)                                                    | It separates each value from its lock; `enforced` keeps them together.                                   |
| Enforced values that silently win over the object                                          | Gateway API `overrides`                                   | A user's value has no effect and nothing says why; refusing the object makes the conflict visible.       |
| A list of provider configurations in the class, as in the object                           | (none)                                                    | A class serves one deployment, so it has one implementation.                                             |
| A `spec.provider` field naming the class's implementation                                  | `StorageClass.provisioner`, `GatewayClass.controllerName` | The provider configuration's `apiVersion` and `kind` already name the implementation.                    |
| A template kind per kind, such as `PostgreSQLTemplate`                                     | Cluster API `AWSMachineTemplate`                          | Twice the kinds; a class's `template` reuses the object's own types.                                     |
| `defaultProviderConfigs` and `overrideProviderConfigs` fields                              | Gateway API `defaults` and `overrides`                    | `template` already holds the defaults, and "override" leaves open which level wins.                      |
| Support listed in field descriptions, such as "Ignored on AWS, OCI"                        | Revision 1 of this design                                 | It lists clouds in the core schema again; classes report their supported features instead (principle 1). |

*Alternatives for classes.*
