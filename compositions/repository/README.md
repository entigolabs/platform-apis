# Repository APIs

## Repository

### Templated objects
1. Repository - ecr.aws.m.upbound.io/v1beta2
2. LifecyclePolicy - ecr.aws.m.upbound.io/v1beta1 - only when lifecycle rules are configured

### Naming convention

The API should check if a conflicting repository with that name exists before creating the resource and fail with a clear error indicating a naming conflict.

### Lifecycle rules

`lifecycleRules` renders an ECR lifecycle policy that expires images. Every rule needs one selector
and one retention bound:

| Field             | Renders as                                       |
|-------------------|--------------------------------------------------|
| `tagPrefixes`     | `tagStatus: tagged` + `tagPrefixList`            |
| `tagPatterns`     | `tagStatus: tagged` + `tagPatternList`           |
| `untagged: true`  | `tagStatus: untagged`                            |
| no selector       | `tagStatus: any`                                 |
| `keepCount`       | `countType: imageCountMoreThan`                  |
| `expireAfterDays` | `countType: sinceImagePushed`, `countUnit: days` |

`rulePriority` follows the order of the list, the description is generated and the action is always
`expire`, the only action ECR supports. A rule without a selector matches every image, so AWS
requires it to be the last rule.

```yaml
spec:
  lifecycleRules:
    - tagPrefixes: [ "develop" ]
      keepCount: 10
    - untagged: true
      expireAfterDays: 7
    - expireAfterDays: 90
```

The default for all repositories comes from `lifecycleRules` in the `platform-apis-artifact`
EnvironmentConfig, which lets dev, test and prod clusters keep different retention. A repository
that sets `spec.lifecycleRules` replaces that default list entirely. With no rules configured in
either place no lifecycle policy is created and ECR keeps every image.

The policy is created only after the repository itself is ready, because AWS rejects a lifecycle
policy for a repository that does not exist yet.
