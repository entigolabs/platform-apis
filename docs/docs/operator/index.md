---
sidepar_position: 1
tiers:
  - Basic
  - Professional
---

import TierNotice from '/docs/components/TierNotice';

# Customize the platform

<TierNotice tiers={frontMatter.tiers} />

## Use private registry for platform deployment
Platform APIs depend on Crossplane providers and functions and are distributed as multi-platform OCI packages. To avoid depending on remote repository availability, it is recommended to use private OCI registry, for example, ECR, that is close to your infrastructure, to distribute the packages. 

```
docker buildx imagetools create --tag entigolabs/function-extra-resources:v0.1.0 xpkg.upbound.io/crossplane-contrib/function-extra-resources:v0.1.0
```

When using ```docker pull``` and ```docker push``` only multi-arch images are not copied and only images that match your computer CPU arhitecture are copied. 

