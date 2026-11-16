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
- [Ignored Settings](#ignored-settings)
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
which an application needs to find the bucket. On OCI, `status.oci.retentionLockedFrom` is the date
a locked retention takes effect.

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

The table names, for every field, the field of each cloud's provider it maps to. Level is the
field's support level (rule 1).

| Field                | Level                  | AWS (S3)                                                                                                                                            | GCP (Cloud Storage)                                       | OCI (Object Storage)                                                                                                                                | Azure (Blob Storage)                                 |
| -------------------- | ---------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| the bucket           |                        | `Bucket`                                                                                                                                            | `Bucket`                                                  | `Bucket`                                                                                                                                            | one storage `Account` and one `Container` per bucket |
| `observe`            | Core                   | `managementPolicies: [Observe]`                                                                                                                     | `managementPolicies: [Observe]`                           | `managementPolicies: [Observe]`                                                                                                                     | `managementPolicies: [Observe]`                      |
| `versioning`         | Core                   | `BucketVersioning`                                                                                                                                  | `versioning.enabled`                                      | `versioning`: `Enabled`; `false` is `Disabled` on a new bucket and `Suspended` on one that had it on, which keeps old versions                      | `Account.blobProperties.versioningEnabled`           |
| `deletionProtection` | Core                   | platform blocks deletion                                                                                                                            | platform blocks deletion                                  | platform blocks deletion                                                                                                                            | platform blocks deletion                             |
| `lifecycleRules`     | Core                   | `BucketLifecycleConfiguration`                                                                                                                      | `lifecycleRule[]`                                         | `ObjectLifecyclePolicy`                                                                                                                             | `ManagementPolicy`                                   |
| `tier: Infrequent`   | Core                   | `STANDARD_IA`                                                                                                                                       | `NEARLINE`                                                | `INFREQUENT_ACCESS`                                                                                                                                 | `Cool`                                               |
| `tier: Archive`      | Core                   | `GLACIER`                                                                                                                                           | `ARCHIVE`                                                 | `ARCHIVE`                                                                                                                                           | `Archive`                                            |
| `retention`          | Core                   | Object Lock default retention, mode `COMPLIANCE` when `locked` and `GOVERNANCE` when not; needs `versioning`, and Object Lock stays on once enabled | `retentionPolicy.retentionPeriod`, in seconds; `isLocked` | `retentionRules[]`; `locked: true` sets `timeRuleLocked` 15 days ahead, since OCI rejects 14 days or less (tested in OC19), and the rule locks then | `ContainerImmutabilityPolicy`, `locked`              |
| `cors`               | Ignored on OCI         | `BucketCorsConfiguration`                                                                                                                           | `cors[]`                                                  | ignored; OCI allows every origin                                                                                                                    | `Account.blobProperties.corsRule[]`                  |
| `softDeleteDays`     | Ignored on AWS and OCI | ignored                                                                                                                                             | `softDeletePolicy.retentionDurationSeconds`, in seconds   | ignored                                                                                                                                             | `Account.blobProperties.deleteRetentionPolicy.days`  |

*Common fields of `Bucket` on each cloud.*

The cloud-specific fields:

| Field      | Maps To                                                                                   |
| ---------- | ----------------------------------------------------------------------------------------- |
| `aws.id`   | the bucket name                                                                           |
| `gcp.id`   | the bucket name                                                                           |
| `oci.id`   | the bucket name; the function adds `n/<namespace>/b/`                                     |
| `azure.id` | the container's resource ID; the function takes the account, container and resource group |

*Cloud-specific fields of `Bucket`.*

Azure puts versioning, lifecycle, CORS and soft delete on the storage account, so each `Bucket` has
its own account, with the account's keys turned off (`sharedAccessKeyEnabled: false`). Soft delete
exists only on GCP and Azure. S3 enables Object Lock on an existing bucket too, but only once
versioning is on, and never off again (`aws_s3_bucket_object_lock_configuration` in
[terraform-provider-aws](https://github.com/hashicorp/terraform-provider-aws)), so `versioning:
false` is refused on a bucket that ever had `retention`.

## Composed Resources

The platform composes the resources below from the `Bucket` in [Kinds](#kinds), on each cloud in
turn. A value commented `# set by the platform` comes from the deployment, not from the object. The
resources per cloud:

```yaml Object
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

```yaml AWS resources
---
apiVersion: s3.aws.m.upbound.io/v1beta1
kind: Bucket
metadata:
  name: orders-files-7f3a2c              # the object's name and a hash of its UID (rule 9)
  namespace: shop
spec:
  forProvider:
    region: eu-north-1                   # the deployment's region
    objectLockEnabled: true              # retention is set
    forceDestroy: false                  # set by the platform
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
    - id: rule-0                         # set by the platform
      status: Enabled
      filter:
      - prefix: logs/
      expiration:
      - days: 30
      noncurrentVersionExpiration:
      - noncurrentDays: 7
      transition:
      - days: 90
        storageClass: GLACIER            # tier: Archive
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
        mode: GOVERNANCE                 # COMPLIANCE when locked
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

```yaml GCP resources
---
apiVersion: storage.gcp.m.upbound.io/v1beta1
kind: Bucket
metadata:
  name: orders-files-7f3a2c              # the object's name and a hash of its UID (rule 9)
  namespace: shop
spec:
  forProvider:
    project: shop-project                # set by the platform
    location: EUROPE-NORTH1              # the deployment's region
    uniformBucketLevelAccess: true       # set by the platform
    publicAccessPrevention: enforced     # set by the platform
    forceDestroy: false                  # set by the platform
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
        storageClass: ARCHIVE            # tier: Archive
    retentionPolicy:
      retentionPeriod: 2592000           # 30 days, in seconds
      isLocked: false
    cors:
    - origin:
      - https://shop.example.com
      method:
      - GET
      - PUT
      maxAgeSeconds: 3600
    softDeletePolicy:
      retentionDurationSeconds: 604800   # 7 days, in seconds
```

*A `Bucket` on GCP and its composed resources.*

```yaml Object
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

```yaml OCI resources
---
apiVersion: objectstorage.oci.m.upbound.io/v1alpha1
kind: Bucket
metadata:
  name: orders-files-7f3a2c              # the object's name and a hash of its UID (rule 9)
  namespace: shop
spec:
  forProvider:
    compartmentId: ocid1.compartment.oc1..zone  # set by the platform
    namespace: tenancy-namespace         # set by the platform
    name: orders-files-7f3a2c
    accessType: NoPublicAccess           # set by the platform
    versioning: Enabled
    retentionRules:
    - displayName: retention             # set by the platform
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
    namespace: tenancy-namespace         # set by the platform
    bucketRef:
      name: orders-files-7f3a2c
    rules:
    - name: rule-0-expire                # set by the platform
      action: DELETE
      isEnabled: true
      target: objects
      timeAmount: "30"
      timeUnit: DAYS
      objectNameFilter:
      - inclusionPrefixes:
        - logs/
    - name: rule-0-noncurrent            # set by the platform
      action: DELETE
      isEnabled: true
      target: previous-object-versions
      timeAmount: "7"
      timeUnit: DAYS
      objectNameFilter:
      - inclusionPrefixes:
        - logs/
    - name: rule-0-archive               # set by the platform
      action: ARCHIVE                    # tier: Archive
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

```yaml Azure resources
---
apiVersion: storage.azure.m.upbound.io/v1beta1
kind: Account
metadata:
  name: ordersfiles7f3a2c                # the object's name and a hash, letters and digits only
  namespace: shop
spec:
  forProvider:
    location: northeurope                # the deployment's region
    resourceGroupName: zone-shop         # set by the platform
    accountKind: StorageV2               # set by the platform
    accountTier: Standard                # set by the platform
    accountReplicationType: ZRS          # set by the platform
    sharedAccessKeyEnabled: false        # set by the platform
    allowNestedItemsToBePublic: false    # set by the platform
    blobProperties:
      versioningEnabled: true
      corsRule:
      - allowedOrigins:
        - https://shop.example.com
        allowedMethods:
        - GET
        - PUT
        allowedHeaders:                  # set by the platform; Azure requires it
        - "*"
        exposedHeaders:                  # set by the platform; Azure requires it
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
    containerAccessType: private         # set by the platform
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
    - name: rule-0                       # set by the platform
      enabled: true
      filters:
        blobTypes:
        - blockBlob
        prefixMatch:
        - orders-files-7f3a2c/logs/      # the container name and the prefix
      actions:
        baseBlob:
          deleteAfterDaysSinceModificationGreaterThan: 30
          tierToArchiveAfterDaysSinceModificationGreaterThan: 90  # tier: Archive
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

| Setting        | Platform's Choice                                                       |
| -------------- | ----------------------------------------------------------------------- |
| Cloud name     | Set by the platform; globally unique on AWS, GCP and for Azure accounts |
| Public access  | Blocked                                                                 |
| Encryption     | The platform's data key                                                 |
| Access control | IAM only: S3 object ownership enforced, GCS uniform bucket-level access |

*Settings the platform makes for a `Bucket`.*

## Ignored Settings

These settings are accepted on every cloud but have no effect on some (rule 1). The table says what
that means there.

| Setting          | Ignored On | Effect                                                                             |
| ---------------- | ---------- | ---------------------------------------------------------------------------------- |
| `cors`           | OCI        | Browsers may call the bucket from every origin; requests still need authorization. |
| `softDeleteDays` | AWS, OCI   | Deleted objects cannot be recovered; versioning keeps replaced ones.               |

*Bucket settings that some clouds ignore.*

## Cloud Limits

Each cloud enforces these limits itself, and the API does not check them (rule 10); a value outside
them is rejected by the cloud and shown as `CloudAccepted: False`. They are listed for reference,
should validating them be decided later.

| Field            | Cloud | Limit         | Source                                                                               |
| ---------------- | ----- | ------------- | ------------------------------------------------------------------------------------ |
| `softDeleteDays` | GCP   | 0, or 7 to 90 | `softDeletePolicy.retentionDurationSeconds`, `Bucket` CRD, provider-upjet-gcp v3.1.0 |
| `softDeleteDays` | Azure | 1 to 365      | `deleteRetentionPolicy.days`, storage `Account` CRD, provider-upjet-azure v2.8.0     |

*Limits each cloud enforces on buckets.*

## Notes

The design relies on these facts, which need no decision.

- Deleting a bucket that holds objects fails on OCI and succeeds on Azure, taking the blobs with it;
  `deletionProtection` is the guard on every cloud.
