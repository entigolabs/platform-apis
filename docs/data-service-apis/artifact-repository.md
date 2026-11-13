# ArtifactRepository

`ArtifactRepository` (today `Repository`) is one application's repository of OCI artifacts: one
image path, such as `shop/orders-api`, with its tags. It holds container images, Helm charts and any
other OCI artifact, which all four registries store alike. The clouds disagree on what a repository
is: ECR and OCIR have one repository per image path, an Artifact Registry repository holds many
images, and an Azure registry holds repositories it creates on the first push, with no resource of
their own. The API's unit is the image path, because that is what an application owns.

## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Status](#status)
- [Platform Decisions](#platform-decisions)
- [Ignored Settings](#ignored-settings)
- [Notes](#notes)

## Spec

What a user writes to create an `ArtifactRepository`. The comments note value ranges and what holds
on one cloud only.

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
  # every cloud block takes id (rule 9), except azure
```

The image path always starts with the namespace: `<namespace>/<path>/<name>`, or
`<namespace>/<name>` without `path`. A registry holds each path once, and the provider finds a
repository by its path alone (`aws_ecr_repository: config.NameAsIdentifier` in [provider-upjet-aws
v2.4.0](https://github.com/crossplane-contrib/provider-upjet-aws/releases/tag/v2.4.0)), so two
objects asking for one path would both manage it, and deleting either would delete the other's
images (today's function sets `forceDelete`). Namespace names are unique in a cluster, so the prefix
rules that out within one cluster. Clusters that push to one registry are not checked against each
other: whoever configures them gives each its own registry, or keeps their namespace names distinct.
Today's free-form `path` is not retained, so moving from `Repository` changes image paths.

A lifecycle rule has one selector (`tagPrefixes`, `tagPatterns`, `untagged`, or none for every
image) and one bound (`keepCount` newest, or `expireAfterDays` since the push), as today.
`lifecycleRules` defaults per deployment, as today's deployment-wide rules do, so a repository whose
spec is empty is still cleaned up; set, it replaces them. `immutableTags` defaults per deployment
and replaces today's deployment-wide `imageTagMutability`; its `IMMUTABLE_WITH_EXCLUSION` and
`MUTABLE_WITH_EXCLUSION` modes, which only ECR has, are dropped.

## Mapping

How each field maps to the managed service of each cloud, named by the fields of that cloud's
Crossplane provider. Level is the field's support level (rule 1).

| Field                | Level                    | AWS (ECR)                       | GCP (Artifact Registry)                                                                                                                                                                                             | OCI (OCIR)                                                             | Azure (ACR)                                                   |
| -------------------- | ------------------------ | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | ------------------------------------------------------------- |
| the repository       |                          | `Repository`                    | one `RegistryRepository` (`format: DOCKER`) per repository                                                                                                                                                          | `ContainerRepository`                                                  | a path in a platform-owned `Registry`; no resource of its own |
| `observe`            | Core; fails on Azure     | `managementPolicies: [Observe]` | `managementPolicies: [Observe]`                                                                                                                                                                                     | `managementPolicies: [Observe]`                                        | fails; an Azure repository has no resource of its own         |
| `deletionProtection` | Core                     | platform blocks deletion        | platform blocks deletion                                                                                                                                                                                            | platform blocks deletion                                               | platform blocks deletion                                      |
| `immutableTags`      | Ignored on Azure         | `imageTagMutability: IMMUTABLE` | `dockerConfig.immutableTags`                                                                                                                                                                                        | `isImmutable`                                                          | ignored; tag locks are set per tag through the Azure CLI only |
| `lifecycleRules`     | Ignored on OCI and Azure | `LifecyclePolicy`               | `cleanupPolicies[]`. Rules that delete old images work. A rule that keeps the newest N works only for all images, not for some tags or for untagged images; a rule with `tagPatterns` or such a selector is skipped | ignored; retention is console-only and counts days since the last pull | ignored; only registry-wide untagged retention exists         |

The cloud-specific fields:

| Field      | Maps To                                                  |
| ---------- | -------------------------------------------------------- |
| `aws.id`   | the repository name, the whole path                      |
| `gcp.id`   | the repository ID                                        |
| `oci.id`   | the repository's OCID                                    |
| `azure.id` | rejected; an Azure repository has no resource of its own |

A separate Artifact Registry repository per application on GCP keeps immutability and cleanup per
application, as on ECR and OCIR. On Azure, a registry per application would be one billed SKU each,
so applications share a platform registry and only path-scoped access is per application.

## Status

The status adds `uri`, the image URI, such as
`123456789012.dkr.ecr.eu-north-1.amazonaws.com/shop/backend/orders-api`. There is no connection
secret.

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

## Platform Decisions

Private repositories, encryption (the platform's data key on AWS; GCP and Azure take a key only at
creation, and OCIR has none), vulnerability scanning (ECR scan on push, Artifact Registry's
container scanning, OCI Vulnerability Scanning Service, Defender for Containers on Azure), and
keyless image pulls by the cluster's nodes, which on OCI needs Oracle's kubelet credential provider
(oke-credential-provider-for-ocir) on the nodes.

## Ignored Settings

Settings this service accepts on every cloud but that have no effect on some (rule 1), and what that
means there.

| Setting                                                       | Ignored On | Effect                                                       |
| ------------------------------------------------------------- | ---------- | ------------------------------------------------------------ |
| `immutableTags`                                               | Azure      | Tags can be overwritten.                                     |
| `lifecycleRules`                                              | OCI, Azure | Old images are not cleaned up.                               |
| A rule with `tagPatterns`, or with `keepCount` and a selector | GCP        | The whole rule is skipped, so the images it covers are kept. |

## Notes

Facts about this service that the design relies on and that need no decision.

- Azure path-scoped access needs the registry in ABAC repository permissions mode, generally
  available since November 2025, which ignores the plain AcrPull and AcrPush roles.
- Deleting a repository deletes its images on OCI and Azure, as `forceDelete` does on ECR, so
  `deletionProtection` is the guard on every cloud.
