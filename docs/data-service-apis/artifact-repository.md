# ArtifactRepository

An artifact registry stores container images, Helm charts and other OCI artifacts, which deployments
pull and CI pushes.

One kind for users: `ArtifactRepository`, one application's image path, such as `shop/orders-api`,
with its tags. One kind shapes it: `ArtifactRepositoryClass`, a deployment's defaults and limits,
written by Entigo.

Built on ECR (AWS), Artifact Registry (GCP), OCIR (OCI) and Azure Container Registry (Azure), each
with a provider configuration kind of its own: `ArtifactRepositoryECRConfig`,
`ArtifactRepositoryArtifactRegistryConfig`, `ArtifactRepositoryOCIRConfig` and
`ArtifactRepositoryACRConfig`. Each stores every OCI artifact alike.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Notes](#notes)

## Kinds

The artifact registry is offered as two kinds, each with the manifest its owner writes.

### ArtifactRepository

The implementations define a repository differently: ECR and OCIR have one repository per image
path, an Artifact Registry repository holds many images, and an Azure registry holds repositories it
creates on the first push, with no resource of their own. The API's unit is the image path, because
an application owns its image path.

A user creates an `ArtifactRepository` with a manifest like the one below. A repository has no
setting specific to one implementation, so its manifest carries a provider configuration entry only
to observe an existing repository, and that entry holds only the repository's `id` (principle 9).
Observing fails on Azure, where a repository has no resource of its own:

```yaml
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
  className: standard                  # unset, the deployment's default class (principle 5)
  name: orders-api                     # immutable; defaults to the object name
  path: backend                        # immutable; the image path is shop/backend/orders-api
  deletionProtection: true
  immutableTags: false
  lifecycleRules:
  - tagPrefixes:
    - develop
    keepCount: 10
  - tagPatterns:
    - release-*
    keepCount: 20
  - untagged: true
    expireAfterDays: 7
```

*`ArtifactRepository` with common fields.*

The image path always starts with the namespace: `<namespace>/<path>/<name>`, or
`<namespace>/<name>` without `path`. A registry holds each path once, and the provider finds a
repository by its path alone (`aws_ecr_repository: config.NameAsIdentifier` in [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0)), so two
objects requesting one path would both manage it, and deleting either would delete the other's
images (the platform sets `forceDelete`). Namespace names are unique in a cluster, so the prefix
rules that out within one cluster. Clusters that push to one registry are not checked against each
other: their operator gives each its own registry, or keeps their namespace names distinct.

A lifecycle rule has one selector (`tagPrefixes`, `tagPatterns`, `untagged`, or none for every
image) and one bound (`keepCount` newest, or `expireAfterDays` since the push). `lifecycleRules`
and `immutableTags` take the class's defaults, so a repository whose spec is empty is still cleaned
up; `lifecycleRules` set on the object replace the class's rules.

An `ArtifactRepository` has no connection secret. The status shows `uri`, the image URI, with which
entry applied, where each effective value came from, and the applied implementation's own values
(principle 4):

```yaml
status:
  uri: 123456789012.dkr.ecr.eu-north-1.amazonaws.com/shop/backend/orders-api
  providerConfig:
    applied: aws.artifact.entigo.com/v1alpha1, ArtifactRepositoryECRConfig
  effective:
    immutableTags: { value: false, source: object }
    deletionProtection: { value: true, source: class/standard/enforced }
  provider:                            # the shape ArtifactRepositoryECRConfig's status defines
    arn: ...
```

*`ArtifactRepository` status on AWS.*

### ArtifactRepositoryClass

Entigo writes each deployment's classes, with the defaults a repository starts with (`template`)
and the values it cannot change (`enforced`) (principle 5):

```yaml
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepositoryClass
metadata:
  name: standard
  annotations:
    artifact.entigo.com/is-default-class: "true"
spec:
  template:
    immutableTags: true
    lifecycleRules:
    - untagged: true
      expireAfterDays: 7
    - keepCount: 100
    providerConfig:
      apiVersion: aws.artifact.entigo.com/v1alpha1
      kind: ArtifactRepositoryECRConfig
      spec: {}
  enforced:
    deletionProtection: true
```

*`ArtifactRepositoryClass` for ECR.*

## Mapping

The first table maps each common field to the implementation's provider. A setting the
implementation lacks shows as ignored, with the reason (principle 1):

| Field                | AWS                                 | GCP                                                                                                                                                                                                                                                    | OCI                                                                                                      | Azure                                                                                     |
| -------------------- | ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| the repository       | `Repository`                        | one `RegistryRepository` (`format: DOCKER`) per repository                                                                                                                                                                                             | `ContainerRepository`                                                                                    | a path in a platform-owned `Registry`; no resource of its own                             |
| `observe`            | `managementPolicies: [Observe]`     | `managementPolicies: [Observe]`                                                                                                                                                                                                                        | `managementPolicies: [Observe]`                                                                          | fails; an Azure repository has no resource of its own                                     |
| `deletionProtection` | platform blocks deletion            | platform blocks deletion                                                                                                                                                                                                                               | platform blocks deletion                                                                                 | platform blocks deletion                                                                  |
| `immutableTags`      | `imageTagMutability: IMMUTABLE`     | `dockerConfig.immutableTags`                                                                                                                                                                                                                           | `isImmutable`                                                                                            | ignored, so tags can be overwritten; tag locks are set per tag through the Azure CLI only |
| `lifecycleRules`     | `LifecyclePolicy`                   | `cleanupPolicies[]`. Rules that delete old images work. A rule that keeps the newest N works only for all images, not for some tags or for untagged images; a rule with `tagPatterns` or such a selector is skipped, and the images it covers are kept | ignored, so old images are not cleaned up; retention is console-only and counts days since the last pull | ignored, so old images are not cleaned up; only registry-wide untagged retention exists   |

*`ArtifactRepository` common fields and their provider fields.*

The second names the implementation's provider configuration kind and what its fields map to:

| Field | AWS                                 | GCP                                        | OCI                            | Azure                                                                         |
| ----- | ----------------------------------- | ------------------------------------------ | ------------------------------ | ----------------------------------------------------------------------------- |
| kind  | `ArtifactRepositoryECRConfig`       | `ArtifactRepositoryArtifactRegistryConfig` | `ArtifactRepositoryOCIRConfig` | `ArtifactRepositoryACRConfig`                                                 |
| `id`  | the repository name, the whole path | the repository ID                          | the repository's OCID          | accepted, but observing fails: an Azure repository has no resource of its own |

*Artifact repository provider configuration fields.*

**AWS:** ECR's `IMMUTABLE_WITH_EXCLUSION` and `MUTABLE_WITH_EXCLUSION` modes, which only ECR has,
are not offered. Images are scanned on push by ECR.

**GCP:** a separate Artifact Registry repository per application keeps immutability and cleanup per
application, as on ECR and OCIR. A registry takes a key only at creation. Images are scanned by
Artifact Registry's container scanning.

**OCI:** OCIR takes no customer key. Images are scanned by OCI Vulnerability Scanning Service, and
nodes pull through Oracle's kubelet credential provider (oke-credential-provider-for-ocir).

**Azure:** a registry per application would be one billed SKU each, so applications share a platform
registry and only path-scoped access is per application. A registry takes a key only at creation.
Images are scanned by Defender for Containers.

## Composed Resources

**Azure:** nothing is composed, since a repository there is a path in the platform's registry,
created by the first push, with no resource of its own.

The `ArtifactRepository` from [Kinds](#kinds) is composed into the resources below on the cloud it
runs on: the platform applies the entry of its class's kind and ignores the others. A comment names
the source of each value the object does not hold as written: `platform` for a value the platform
sets, and `deployment` for one the deployment sets. The object and its resources:

```yaml Object on AWS
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
  className: standard
  name: orders-api
  path: backend
  deletionProtection: true
  immutableTags: false
  lifecycleRules:
  - tagPrefixes:
    - develop
    keepCount: 10
  - tagPatterns:
    - release-*
    keepCount: 20
  - untagged: true
    expireAfterDays: 7
```

```yaml AWS resources
---
apiVersion: ecr.aws.m.upbound.io/v1beta1
kind: Repository
metadata:
  name: orders-api-7f3a2c      # name + UID hash
  namespace: shop
  annotations:
    crossplane.io/external-name: shop/backend/orders-api
spec:
  forProvider:
    region: eu-north-1         # deployment
    imageTagMutability: MUTABLE
    forceDelete: true          # platform
    encryptionConfiguration:   # platform
    - encryptionType: KMS
      kmsKey: arn:aws:kms:eu-north-1:111122223333:key/data
---
apiVersion: ecr.aws.m.upbound.io/v1beta1
kind: LifecyclePolicy
metadata:
  name: orders-api-7f3a2c
  namespace: shop
spec:
  forProvider:
    region: eu-north-1
    repositoryRef:
      name: orders-api-7f3a2c
    policy: |
      {"rules": [
        {"rulePriority": 1,
         "selection": {"tagStatus": "tagged", "tagPrefixList": ["develop"],
                       "countType": "imageCountMoreThan", "countNumber": 10},
         "action": {"type": "expire"}},
        {"rulePriority": 2,
         "selection": {"tagStatus": "tagged", "tagPatternList": ["release-*"],
                       "countType": "imageCountMoreThan", "countNumber": 20},
         "action": {"type": "expire"}},
        {"rulePriority": 3,
         "selection": {"tagStatus": "untagged",
                       "countType": "sinceImagePushed", "countUnit": "days", "countNumber": 7},
         "action": {"type": "expire"}}
      ]}
```

*`ArtifactRepository` on AWS and its composed resources.*

```yaml Object on GCP
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
  className: standard
  name: orders-api
  path: backend
  deletionProtection: true
  immutableTags: false
  lifecycleRules:
  - tagPrefixes:
    - develop
    keepCount: 10
  - tagPatterns:
    - release-*
    keepCount: 20
  - untagged: true
    expireAfterDays: 7
```

```yaml GCP resource
---
apiVersion: artifact.gcp.m.upbound.io/v1beta1
kind: RegistryRepository
metadata:
  name: orders-api-7f3a2c      # name + UID hash
  namespace: shop
  annotations:
    crossplane.io/external-name: shop-backend-orders-api
spec:
  forProvider:
    project: shop-project      # platform
    location: europe-north1    # deployment
    format: DOCKER             # platform
    dockerConfig:
      immutableTags: false
    cleanupPolicies:
    - id: untagged-7-days      # platform
      action: DELETE
      condition:
        tagState: UNTAGGED
        olderThan: 604800s     # 7 days
```

*`ArtifactRepository` on GCP and its composed resource.*

```yaml Object on OCI
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
  className: standard
  name: orders-api
  path: backend
  deletionProtection: true
  immutableTags: false
  lifecycleRules:
  - tagPrefixes:
    - develop
    keepCount: 10
  - tagPatterns:
    - release-*
    keepCount: 20
  - untagged: true
    expireAfterDays: 7
```

```yaml OCI resource
---
apiVersion: artifacts.oci.m.upbound.io/v1alpha1
kind: ContainerRepository
metadata:
  name: orders-api-7f3a2c      # name + UID hash
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # platform
    displayName: shop/backend/orders-api
    isImmutable: false
    isPublic: false            # platform
```

*`ArtifactRepository` on OCI and its composed resource.*

## Platform Decisions

Besides the settings it makes for every service
([Platform Decisions](common-design.md#platform-decisions)), the platform makes these
for an `ArtifactRepository` itself:

| Setting                | Platform's Choice                    |
| ---------------------- | ------------------------------------ |
| Vulnerability scanning | The cloud's own scanner              |
| Image pulls            | Keyless pulls by the cluster's nodes |

*`ArtifactRepository` settings chosen by the platform.*

## Notes

The design relies on these facts, which need no decision.

- Azure path-scoped access needs the registry in ABAC repository permissions mode, generally
  available since November 2025, which ignores the plain AcrPull and AcrPush roles.
- Deleting a repository deletes its images on OCI and Azure, as `forceDelete` does on ECR, so
  `deletionProtection` is the guard on every cloud.
