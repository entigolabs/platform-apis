# Bucket

Applies the rules in [rules.md](rules.md). Can be installed on every cloud (rule 7).
Schema: [bucket.yaml](schemas/bucket.yaml).

`Bucket` (today `S3Bucket`) is one object storage bucket, and only the bucket: who may use it is
decided by the user's own identity, not by the bucket.

## Contents

- [Spec](#spec)
- [Mapping](#mapping)
- [Platform Decisions](#platform-decisions)
- [Status](#status)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Spec

What a user writes to create a `Bucket`. The comments note value ranges and what holds on one cloud
only.

```yaml
---
apiVersion: storage.entigo.com/v1alpha2
kind: Bucket
metadata:
  name: orders-files
  namespace: shop
spec:
  # Common
  versioning: true
  deletionProtection: true
  lifecycleRules:
  - prefix: logs/                      # optional; the whole bucket without it
    expireAfterDays: 30                # delete objects this old
    deleteNoncurrentAfterDays: 7       # delete replaced versions this old
    transitions:
    - afterDays: 90
      tier: Archive                    # Infrequent or Archive
  retention:                           # objects cannot be changed or deleted for this long
    days: 30
    locked: false                      # true cannot be undone on any cloud
  cors:
  - origins:
    - https://shop.example.com
    methods:
    - GET
    - PUT
    maxAgeSeconds: 3600
  # Cloud-specific; every block also takes id (rule 9)
  gcp:
    softDeleteDays: 7
  azure:
    softDeleteDays: 7
```

`lifecycleRules`, `retention` and `cors` have no default. `tier` names the two cold tiers every
cloud has.

Today's `S3Bucket` also creates an IAM role, a service account annotated with it, and an IAM user
with an access key. Those are identities of the applications that use the bucket, not part of the
bucket, so they leave `Bucket`; an application's own identity is granted access to the buckets it
names (Follow-Up Work 3, Application identity, in [follow-up-work.md](follow-up-work.md)).

## Mapping

How each field maps to the managed service of each cloud, named by the fields of that cloud's
Crossplane provider. Level is the field's support level (rule 1).

| Field                | Level                     | AWS (S3)                                                                                                                                            | GCP (Cloud Storage)                                       | OCI (Object Storage)                                                                                                                                | Azure (Blob Storage)                                 |
| -------------------- | ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| the bucket           |                           | `Bucket`                                                                                                                                            | `Bucket`                                                  | `Bucket`                                                                                                                                            | one storage `Account` and one `Container` per bucket |
| `observe`            | Core                      | `managementPolicies: [Observe]`                                                                                                                     | `managementPolicies: [Observe]`                           | `managementPolicies: [Observe]`                                                                                                                     | `managementPolicies: [Observe]`                      |
| `versioning`         | Core                      | `BucketVersioning`                                                                                                                                  | `versioning.enabled`                                      | `versioning`: `Enabled`; `false` is `Disabled` on a new bucket and `Suspended` on one that had it on, which keeps old versions                      | `Account.blobProperties.versioningEnabled`           |
| `deletionProtection` | Core                      | platform blocks deletion                                                                                                                            | platform blocks deletion                                  | platform blocks deletion                                                                                                                            | platform blocks deletion                             |
| `lifecycleRules`     | Core                      | `BucketLifecycleConfiguration`                                                                                                                      | `lifecycleRule[]`                                         | `ObjectLifecyclePolicy`                                                                                                                             | `ManagementPolicy`                                   |
| `tier: Infrequent`   | Core                      | `STANDARD_IA`                                                                                                                                       | `NEARLINE`                                                | `INFREQUENT_ACCESS`                                                                                                                                 | `Cool`                                               |
| `tier: Archive`      | Core                      | `GLACIER`                                                                                                                                           | `ARCHIVE`                                                 | `ARCHIVE`                                                                                                                                           | `Archive`                                            |
| `retention`          | Core                      | Object Lock default retention, mode `COMPLIANCE` when `locked` and `GOVERNANCE` when not; needs `versioning`, and Object Lock stays on once enabled | `retentionPolicy.retentionPeriod`, in seconds; `isLocked` | `retentionRules[]`; `locked: true` sets `timeRuleLocked` 15 days ahead, since OCI rejects 14 days or less (tested in OC19), and the rule locks then | `ContainerImmutabilityPolicy`, `locked`              |
| `cors`               | Extended: AWS, GCP, Azure | `BucketCorsConfiguration`                                                                                                                           | `cors[]`                                                  | rejected                                                                                                                                            | `Account.blobProperties.corsRule[]`                  |

The cloud-specific fields:

| Field                  | Maps To                                                                                   |
| ---------------------- | ----------------------------------------------------------------------------------------- |
| `aws.id`               | the bucket name                                                                           |
| `gcp.id`               | the bucket name                                                                           |
| `oci.id`               | the bucket name; the function adds `n/<namespace>/b/`                                     |
| `azure.id`             | the container's resource ID; the function takes the account, container and resource group |
| `gcp.softDeleteDays`   | `softDeletePolicy.retentionDurationSeconds`, in seconds                                   |
| `azure.softDeleteDays` | `Account.blobProperties.deleteRetentionPolicy.days`                                       |

Azure puts versioning, lifecycle, CORS and soft delete on the storage account, so each `Bucket` has
its own account, with the account's keys turned off (`sharedAccessKeyEnabled: false`). Soft delete
exists only on GCP and Azure. S3 enables Object Lock on an existing bucket too, but only once
versioning is on, and never off again (`aws_s3_bucket_object_lock_configuration` in
[terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws)), so `versioning:
false` is refused on a bucket that ever had `retention`.

## Platform Decisions

The bucket's cloud name (globally unique on AWS, GCP and for Azure accounts), encryption with the
platform's data key, public access blocked, and access through IAM only (S3 object ownership
enforced, GCS uniform bucket-level access).

## Status

A `Bucket` has no secret: it holds no credentials. The status adds `url`, `bucketName` and `region`,
which an application needs to find the bucket. On OCI, `status.oci.retentionLockedFrom` is the date
a locked retention takes effect.

```yaml
status:
  aws:
    arn: ...
  gcp:
    selfLink: ...
  oci:
    namespace: ...
    retentionLockedFrom: ...
  azure:
    accountId: ...
    containerId: ...
```

## Cloud Limits

Limits each cloud enforces itself, which the API does not check (rule 10); a value outside them is
rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference, should
validating them be decided later.

| Field                  | Cloud | Limit         | Source                                                                               |
| ---------------------- | ----- | ------------- | ------------------------------------------------------------------------------------ |
| `gcp.softDeleteDays`   | GCP   | 0, or 7 to 90 | `softDeletePolicy.retentionDurationSeconds`, `Bucket` CRD, provider-upjet-gcp v3.1.0 |
| `azure.softDeleteDays` | Azure | 1 to 365      | `deleteRetentionPolicy.days`, storage `Account` CRD, provider-upjet-azure v2.8.0     |

## Notes

Facts about this service that the design relies on and that need no decision.

- Deleting a bucket that holds objects fails on OCI and succeeds on Azure, taking the blobs with it;
  `deletionProtection` is the guard on every cloud.
