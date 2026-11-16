# Bucket

Object storage keeps files as objects in buckets, which applications read and write over HTTP.

One kind: `Bucket`, one bucket.

Built on S3 (AWS), Cloud Storage (GCP), Object Storage (OCI) and Blob Storage (Azure).

[API Changes](changes.md#bucket) lists what changes from today's API.

## Contents

- [Kinds](#kinds)
- [Mapping](#mapping)
- [Composed Resources](#composed-resources)
- [Platform Decisions](#platform-decisions)
- [Cloud Limits](#cloud-limits)
- [Notes](#notes)

## Kinds

Object storage is offered as one kind, shown below with a manifest a user writes.

### Bucket

A `Bucket` is only the bucket: who may use it is decided by the user's own identity, not by the
bucket.

A user creates a `Bucket` with a manifest like the one below. Its comments note value ranges and
what holds on one cloud only:

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
  softDeleteDays: 7                    # ignored on AWS and OCI
  # Cloud-specific: every block takes id (rule 9)
```

*A `Bucket` with its common fields.*

`lifecycleRules`, `retention` and `cors` have no default. `tier` names the two cold tiers every
cloud has.

A `Bucket` creates no identity: no IAM role, service account or access key. Those belong to the
applications that use the bucket; an application's own identity is granted access to the buckets it
names ([Cloud identity for applications](follow-up-work.md#3-cloud-identity-for-applications), in
Follow-Up Work).

A `Bucket` has no secret: it holds no credentials. The status adds `url`, `bucketName` and `region`,
which an application needs to find the bucket.

The cloud blocks of the status:

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

*The cloud blocks of a `Bucket` status.*

## Mapping

The table names what each field maps to in the cloud's provider. A setting the cloud lacks shows as
ignored, with the reason (rule 1), and a field of a cloud's block appears only for that cloud:

| Field                | AWS                                                                                                                                                 | GCP                                                       | OCI                                                                                                                                                 | Azure                                                                                     |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| the bucket           | `Bucket`                                                                                                                                            | `Bucket`                                                  | `Bucket`                                                                                                                                            | one storage `Account` and one `Container` per bucket                                      |
| `observe`            | `managementPolicies: [Observe]`                                                                                                                     | `managementPolicies: [Observe]`                           | `managementPolicies: [Observe]`                                                                                                                     | `managementPolicies: [Observe]`                                                           |
| `versioning`         | `BucketVersioning`                                                                                                                                  | `versioning.enabled`                                      | `versioning`: `Enabled`; `false` is `Disabled` on a new bucket and `Suspended` on one that had it on, which keeps old versions                      | `Account.blobProperties.versioningEnabled`                                                |
| `deletionProtection` | platform blocks deletion                                                                                                                            | platform blocks deletion                                  | platform blocks deletion                                                                                                                            | platform blocks deletion                                                                  |
| `lifecycleRules`     | `BucketLifecycleConfiguration`                                                                                                                      | `lifecycleRule[]`                                         | `ObjectLifecyclePolicy`                                                                                                                             | `ManagementPolicy`                                                                        |
| `tier: Infrequent`   | `STANDARD_IA`                                                                                                                                       | `NEARLINE`                                                | `INFREQUENT_ACCESS`                                                                                                                                 | `Cool`                                                                                    |
| `tier: Archive`      | `GLACIER`                                                                                                                                           | `ARCHIVE`                                                 | `ARCHIVE`                                                                                                                                           | `Archive`                                                                                 |
| `retention`          | Object Lock default retention, mode `COMPLIANCE` when `locked` and `GOVERNANCE` when not; needs `versioning`, and Object Lock stays on once enabled | `retentionPolicy.retentionPeriod`, in seconds; `isLocked` | `retentionRules[]`; `locked: true` sets `timeRuleLocked` 15 days ahead, since OCI rejects 14 days or less (tested in OC19), and the rule locks then | `ContainerImmutabilityPolicy`, `locked`                                                   |
| `cors`               | `BucketCorsConfiguration`                                                                                                                           | `cors[]`                                                  | ignored; OCI allows every origin, though requests still need authorization                                                                          | `Account.blobProperties.corsRule[]`                                                       |
| `softDeleteDays`     | ignored; deleted objects cannot be recovered, though versioning keeps replaced ones                                                                 | `softDeletePolicy.retentionDurationSeconds`, in seconds   | ignored; deleted objects cannot be recovered, though versioning keeps replaced ones                                                                 | `Account.blobProperties.deleteRetentionPolicy.days`                                       |
| `aws.id`             | the bucket name                                                                                                                                     |                                                           |                                                                                                                                                     |                                                                                           |
| `gcp.id`             |                                                                                                                                                     | the bucket name                                           |                                                                                                                                                     |                                                                                           |
| `oci.id`             |                                                                                                                                                     |                                                           | the bucket name; the function adds `n/<namespace>/b/`                                                                                               |                                                                                           |
| `azure.id`           |                                                                                                                                                     |                                                           |                                                                                                                                                     | the container's resource ID; the function takes the account, container and resource group |

*Fields of `Bucket` and their provider fields.*

**AWS:** S3 enables Object Lock on an existing bucket too, but only once versioning is on, and never
off again (`aws_s3_bucket_object_lock_configuration` in
[terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws)), so `versioning:
false` is refused on a bucket that ever had `retention`. Bucket names are globally unique, and S3
object ownership is enforced.

**GCP:** bucket names are globally unique, and uniform bucket-level access is on.

**OCI:** `status.oci.retentionLockedFrom` is the date a locked retention takes effect.

**Azure:** versioning, lifecycle, CORS and soft delete belong to the storage account, so each
`Bucket` has its own account, with the account's keys turned off (`sharedAccessKeyEnabled: false`).
Storage account names are globally unique.

## Composed Resources

The `Bucket` from [Kinds](#kinds) is composed into the resources below on the cloud it runs on: the
platform uses that cloud's block and ignores the others. A comment names the source of each value
the object does not hold as written: `platform` for a value the platform sets, and `deployment` for
one the deployment sets. The object and its resources:

```yaml Object
---
apiVersion: storage.entigo.com/v1alpha2
kind: Bucket
metadata:
  name: orders-files
  namespace: shop
spec:
  versioning: true
  deletionProtection: true
  lifecycleRules:
  - prefix: logs/
    expireAfterDays: 30
    deleteNoncurrentAfterDays: 7
    transitions:
    - afterDays: 90
      tier: Archive
  retention:
    days: 30
    locked: false
  cors:
  - origins:
    - https://shop.example.com
    methods:
    - GET
    - PUT
    maxAgeSeconds: 3600
  softDeleteDays: 7
```

```yaml AWS resources
---
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: Bucket
metadata:
  name: orders-files-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    region: eu-north-1         # deployment
    objectLockEnabled: true
    forceDestroy: false        # platform
---
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: BucketVersioning
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    region: eu-north-1
    bucketRef:
      name: orders-files-7f3a2c
    versioningConfiguration:
      status: Enabled
---
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: BucketLifecycleConfiguration
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    region: eu-north-1
    bucketRef:
      name: orders-files-7f3a2c
    rule:
    - id: rule-0               # platform
      status: Enabled
      filter:
      - prefix: logs/
      expiration:
      - days: 30
      noncurrentVersionExpiration:
      - noncurrentDays: 7
      transition:
      - days: 90
        storageClass: GLACIER
---
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: BucketObjectLockConfiguration
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    region: eu-north-1
    bucketRef:
      name: orders-files-7f3a2c
    rule:
      defaultRetention:
        days: 30
        mode: GOVERNANCE
---
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: BucketCorsConfiguration
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    region: eu-north-1
    bucketRef:
      name: orders-files-7f3a2c
    corsRule:
    - allowedOrigins:
      - https://shop.example.com
      allowedMethods:
      - GET
      - PUT
      maxAgeSeconds: 3600
```

*A `Bucket` on AWS and its composed resources.*

```yaml Object
---
apiVersion: storage.entigo.com/v1alpha2
kind: Bucket
metadata:
  name: orders-files
  namespace: shop
spec:
  versioning: true
  deletionProtection: true
  lifecycleRules:
  - prefix: logs/
    expireAfterDays: 30
    deleteNoncurrentAfterDays: 7
    transitions:
    - afterDays: 90
      tier: Archive
  retention:
    days: 30
    locked: false
  cors:
  - origins:
    - https://shop.example.com
    methods:
    - GET
    - PUT
    maxAgeSeconds: 3600
  softDeleteDays: 7
```

```yaml GCP resource
---
apiVersion: storage.gcp.m.upbound.io/v1beta1
kind: Bucket
metadata:
  name: orders-files-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    project: shop-project      # platform
    location: EUROPE-NORTH1    # deployment
    uniformBucketLevelAccess: true  # platform
    publicAccessPrevention: enforced  # platform
    forceDestroy: false        # platform
    versioning:
      enabled: true
    lifecycleRule:
    - condition:
        matchesPrefix:
        - logs/
        age: 30
      action:
        type: Delete
    - condition:
        matchesPrefix:
        - logs/
        daysSinceNoncurrentTime: 7
      action:
        type: Delete
    - condition:
        matchesPrefix:
        - logs/
        age: 90
      action:
        type: SetStorageClass
        storageClass: ARCHIVE
    retentionPolicy:
      retentionPeriod: 2592000  # 30 days
      isLocked: false
    cors:
    - origin:
      - https://shop.example.com
      method:
      - GET
      - PUT
      maxAgeSeconds: 3600
    softDeletePolicy:
      retentionDurationSeconds: 604800  # 7 days
```

*A `Bucket` on GCP and its composed resource.*

```yaml Object
---
apiVersion: storage.entigo.com/v1alpha2
kind: Bucket
metadata:
  name: orders-files
  namespace: shop
spec:
  versioning: true
  deletionProtection: true
  lifecycleRules:
  - prefix: logs/
    expireAfterDays: 30
    deleteNoncurrentAfterDays: 7
    transitions:
    - afterDays: 90
      tier: Archive
  retention:
    days: 30
    locked: false
  cors:
  - origins:
    - https://shop.example.com
    methods:
    - GET
    - PUT
    maxAgeSeconds: 3600
  softDeleteDays: 7
```

```yaml OCI resources
---
apiVersion: objectstorage.oci.m.upbound.io/v1alpha1
kind: Bucket
metadata:
  name: orders-files-7f3a2c    # name + UID hash
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # platform
    namespace: tenancy-namespace  # platform
    name: orders-files-7f3a2c
    accessType: NoPublicAccess  # platform
    versioning: Enabled
    retentionRules:
    - displayName: retention   # platform
      duration:
      - timeAmount: "30"
        timeUnit: DAYS
---
apiVersion: objectstorage.oci.m.upbound.io/v1alpha1
kind: ObjectLifecyclePolicy
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    namespace: tenancy-namespace  # platform
    bucketRef:
      name: orders-files-7f3a2c
    rules:
    - name: rule-0-expire      # platform
      action: DELETE
      isEnabled: true
      target: objects
      timeAmount: "30"
      timeUnit: DAYS
      objectNameFilter:
      - inclusionPrefixes:
        - logs/
    - name: rule-0-noncurrent  # platform
      action: DELETE
      isEnabled: true
      target: previous-object-versions
      timeAmount: "7"
      timeUnit: DAYS
      objectNameFilter:
      - inclusionPrefixes:
        - logs/
    - name: rule-0-archive     # platform
      action: ARCHIVE
      isEnabled: true
      target: objects
      timeAmount: "90"
      timeUnit: DAYS
      objectNameFilter:
      - inclusionPrefixes:
        - logs/
```

*A `Bucket` on OCI and its composed resources.*

```yaml Object
---
apiVersion: storage.entigo.com/v1alpha2
kind: Bucket
metadata:
  name: orders-files
  namespace: shop
spec:
  versioning: true
  deletionProtection: true
  lifecycleRules:
  - prefix: logs/
    expireAfterDays: 30
    deleteNoncurrentAfterDays: 7
    transitions:
    - afterDays: 90
      tier: Archive
  retention:
    days: 30
    locked: false
  cors:
  - origins:
    - https://shop.example.com
    methods:
    - GET
    - PUT
    maxAgeSeconds: 3600
  softDeleteDays: 7
```

```yaml Azure resources
---
apiVersion: storage.azure.m.upbound.io/v1beta1
kind: Account
metadata:
  name: ordersfiles7f3a2c      # name + UID hash, alphanumeric
  namespace: shop
spec:
  forProvider:
    location: northeurope      # deployment
    resourceGroupName: zone-shop  # platform
    accountKind: StorageV2     # platform
    accountTier: Standard      # platform
    accountReplicationType: ZRS  # platform
    sharedAccessKeyEnabled: false  # platform
    allowNestedItemsToBePublic: false  # platform
    blobProperties:
      versioningEnabled: true
      corsRule:
      - allowedOrigins:
        - https://shop.example.com
        allowedMethods:
        - GET
        - PUT
        allowedHeaders:        # platform
        - "*"
        exposedHeaders:        # platform
        - "*"
        maxAgeInSeconds: 3600
      deleteRetentionPolicy:
        days: 7
---
apiVersion: storage.azure.m.upbound.io/v1beta1
kind: Container
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    storageAccountIdRef:
      name: ordersfiles7f3a2c
    containerAccessType: private  # platform
---
apiVersion: storage.azure.m.upbound.io/v1beta1
kind: ManagementPolicy
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    storageAccountIdRef:
      name: ordersfiles7f3a2c
    rule:
    - name: rule-0             # platform
      enabled: true
      filters:
        blobTypes:
        - blockBlob
        prefixMatch:
        - orders-files-7f3a2c/logs/
      actions:
        baseBlob:
          deleteAfterDaysSinceModificationGreaterThan: 30
          tierToArchiveAfterDaysSinceModificationGreaterThan: 90
        version:
          deleteAfterDaysSinceCreation: 7
---
apiVersion: storage.azure.m.upbound.io/v1beta1
kind: ContainerImmutabilityPolicy
metadata:
  name: orders-files-7f3a2c
  namespace: shop
spec:
  forProvider:
    storageContainerResourceManagerIdRef:
      name: orders-files-7f3a2c
    immutabilityPeriodInDays: 30
    locked: false
```

*A `Bucket` on Azure and its composed resources.*

## Platform Decisions

The platform makes these settings itself:

| Setting        | Platform's Choice                                       |
| -------------- | ------------------------------------------------------- |
| Cloud name     | Set by the platform, unique where the cloud requires it |
| Public access  | Blocked                                                 |
| Encryption     | The platform's data key                                 |
| Access control | Cloud IAM only                                          |

*Settings the platform makes for a `Bucket`.*

## Cloud Limits

The cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later. Each limit holds as of its source: the provider release
named, or the cloud's documentation as read in November 2026.

| Field            | Cloud | Limit         | Source                                                                               |
| ---------------- | ----- | ------------- | ------------------------------------------------------------------------------------ |
| `softDeleteDays` | GCP   | 0, or 7 to 90 | `softDeletePolicy.retentionDurationSeconds`, `Bucket` CRD, provider-upjet-gcp v3.1.0 |
| `softDeleteDays` | Azure | 1 to 365      | `deleteRetentionPolicy.days`, storage `Account` CRD, provider-upjet-azure v2.8.0     |

*Limits the cloud enforces on buckets.*

## Notes

The design relies on these facts, which need no decision.

- Deleting a bucket that holds objects fails on OCI and succeeds on Azure, taking the blobs with it;
  `deletionProtection` is the guard on every cloud.
