# ArtifactRepository

An artifact registry stores container images, Helm charts and other OCI artifacts, which deployments
pull and CI pushes.

One kind: `ArtifactRepository`, one application's image path, such as `shop/orders-api`, with its
tags.

Built on ECR (AWS), Artifact Registry (GCP), OCIR (OCI) and Azure Container Registry (Azure), which
store every OCI artifact alike.

[API Changes](changes.md#artifactrepository) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Notes](#notes)

## Kinds

The artifact registry is offered as one kind, shown below with a manifest a user writes.

### ArtifactRepository

The clouds disagree on what a repository is: ECR and OCIR have one repository per image path, an
Artifact Registry repository holds many images, and an Azure registry holds repositories it creates
on the first push, with no resource of their own. The API's unit is the image path, because that is
what an application owns.

A user creates an `ArtifactRepository` with a manifest like the one below. Its comments note value
ranges and what holds on one cloud only:

```yaml
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
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
  # every cloud block takes id (rule 9); observing fails on Azure
```

*An `ArtifactRepository` with its common fields.*

The image path always starts with the namespace: `<namespace>/<path>/<name>`, or
`<namespace>/<name>` without `path`. A registry holds each path once, and the provider finds a
repository by its path alone (`aws_ecr_repository: config.NameAsIdentifier` in [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0)), so two
objects asking for one path would both manage it, and deleting either would delete the other's
images (the platform sets `forceDelete`). Namespace names are unique in a cluster, so the prefix
rules that out within one cluster. Clusters that push to one registry are not checked against each
other: whoever configures them gives each its own registry, or keeps their namespace names distinct.

A lifecycle rule has one selector (`tagPrefixes`, `tagPatterns`, `untagged`, or none for every
image) and one bound (`keepCount` newest, or `expireAfterDays` since the push). `lifecycleRules`
defaults per deployment, so a repository whose spec is empty is still cleaned up; set, it replaces
the default. `immutableTags` defaults per deployment. ECR's `IMMUTABLE_WITH_EXCLUSION` and
`MUTABLE_WITH_EXCLUSION` modes, which only ECR has, are not offered.

The status adds `uri`, the image URI, such as
`123456789012.dkr.ecr.eu-north-1.amazonaws.com/shop/backend/orders-api`. There is no connection
secret.

The cloud blocks of the status:

```yaml
status:
  aws:
    arn: ...
  gcp:
    id: ...
  oci:
    id: ...
  azure:
    registryId: ...
```

*The cloud blocks of an `ArtifactRepository` status.*

## Mapping

The table names what each field maps to in the cloud's provider. A setting the cloud lacks shows as
ignored, with the reason (rule 1), and a field of a cloud's block appears only for that cloud:

| Field                | AWS                                 | GCP                                                                                                                                                                                                                                                    | OCI                                                                                                      | Azure                                                                                     |
| -------------------- | ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| the repository       | `Repository`                        | one `RegistryRepository` (`format: DOCKER`) per repository                                                                                                                                                                                             | `ContainerRepository`                                                                                    | a path in a platform-owned `Registry`; no resource of its own                             |
| `observe`            | `managementPolicies: [Observe]`     | `managementPolicies: [Observe]`                                                                                                                                                                                                                        | `managementPolicies: [Observe]`                                                                          | fails; an Azure repository has no resource of its own                                     |
| `deletionProtection` | platform blocks deletion            | platform blocks deletion                                                                                                                                                                                                                               | platform blocks deletion                                                                                 | platform blocks deletion                                                                  |
| `immutableTags`      | `imageTagMutability: IMMUTABLE`     | `dockerConfig.immutableTags`                                                                                                                                                                                                                           | `isImmutable`                                                                                            | ignored, so tags can be overwritten; tag locks are set per tag through the Azure CLI only |
| `lifecycleRules`     | `LifecyclePolicy`                   | `cleanupPolicies[]`. Rules that delete old images work. A rule that keeps the newest N works only for all images, not for some tags or for untagged images; a rule with `tagPatterns` or such a selector is skipped, and the images it covers are kept | ignored, so old images are not cleaned up; retention is console-only and counts days since the last pull | ignored, so old images are not cleaned up; only registry-wide untagged retention exists   |
| `aws.id`             | the repository name, the whole path | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `gcp.id`             | ----------------------------------- | the repository ID                                                                                                                                                                                                                                      | -------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| `oci.id`             | ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | the repository's OCID                                                                                    | ----------------------------------------------------------------------------------------- |
| `azure.id`           | ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------- | accepted, but observing fails: an Azure repository has no resource of its own             |

*Fields of `ArtifactRepository` and their provider fields.*

**GCP:** a separate Artifact Registry repository per application keeps immutability and cleanup per
application, as on ECR and OCIR.

**Azure:** a registry per application would be one billed SKU each, so applications share a platform
registry and only path-scoped access is per application.

## Composed Resources

**Azure:** nothing is composed, since a repository there is a path in the platform's registry,
created by the first push, with no resource of its own.

The same `ArtifactRepository`, from [Kinds](#kinds), is composed into different resources on each
cloud: the platform uses the block of the cloud it runs on and ignores the others. A comment names
the source of each value the object does not hold as written: `platform` for a value the platform
sets, and `deployment` for one the deployment sets. The object and its resources:

```yaml Object
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
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

*An `ArtifactRepository` on AWS and its composed resources.*

```yaml Object
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
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

*An `ArtifactRepository` on GCP and its composed resource.*

```yaml Object
---
apiVersion: artifact.entigo.com/v1alpha2
kind: ArtifactRepository
metadata:
  name: orders-api
  namespace: shop
spec:
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

*An `ArtifactRepository` on OCI and its composed resource.*

## Platform Decisions

The platform makes these settings itself:

| Setting                | Platform's Choice                                                                                                                          |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Visibility             | Private                                                                                                                                    |
| Encryption             | The platform's data key on AWS; GCP and Azure take a key only at creation, and OCIR has none                                               |
| Vulnerability scanning | ECR scan on push, Artifact Registry's container scanning, OCI Vulnerability Scanning Service, Defender for Containers on Azure             |
| Image pulls            | Keyless pulls by the cluster's nodes; on OCI, through Oracle's kubelet credential provider (oke-credential-provider-for-ocir) on the nodes |

*Settings the platform makes for an `ArtifactRepository`.*

## Notes

The design relies on these facts, which need no decision.

- Azure path-scoped access needs the registry in ABAC repository permissions mode, generally
  available since November 2025, which ignores the plain AcrPull and AcrPush roles.
- Deleting a repository deletes its images on OCI and Azure, as `forceDelete` does on ECR, so
  `deletionProtection` is the guard on every cloud.
