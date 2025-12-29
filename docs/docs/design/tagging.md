---
sidebar_position: 1
---

# Tagging

*Status*: draft

Entigo Platform makes use of resource tagging and lables to:
- set [Permissions Boundaries](permission-boundaries) on privilege delegation and
- instruct the platform behavioure with [Management Policies](management-policies). 

## Tag Inheritance 

Add a tag to a higher level resource and propagate it down to other managed objects. 

Example use-case is costs management. Zone can be used to isolate data and services between different products. If it is not necessary to monitor costs by each application component that makes up a product, Zone provides a convinient wrapper for aggregating product costs. Product team woult need to lable the Zone with product cost center identifier and it would be propagated to all resources deployed to the Zone. Since Zone is backed dedicated cloud provider resources, costs identifiers would be propagated to cloud resources, making it possible to use these through cloud cost management tools in addition to Entigo Platform FinOps Analytics.

TODO: Need to decide what tags are inherited and how to manage this list. 

Cloud Provider Native Support:

- AWS: Resource tag propagation varies by service (e.g., Auto Scaling Groups support it, some services don't)
- Azure: Uses Azure Policy to enforce tag inheritance from Resource Groups
- GCP: Limited native support; typically handled at IaC level

## Extra metadata
Add additional information to resources, like cost center, security classification, etc. 


## Related resources

- [Kubernetes Common Labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/)
- [ArgoCD Labels)[https://argo-cd.readthedocs.io/en/latest/user-guide/annotations-and-labels/]
- [How to implement SaaS tenant isolation with ABAC and AWS IAM)[https://aws.amazon.com/blogs/security/how-to-implement-saas-tenant-isolation-with-abac-and-aws-iam/]
- [SaaS tenant isolation with ABAC using AWS STS support for tags in JWT)[https://aws.amazon.com/blogs/security/saas-tenant-isolation-with-abac-using-aws-sts-support-for-tags-in-jwt/]
- [Build an end-to-end attribute-based access control strategy with AWS IAM Identity Center and Okta)[https://aws.amazon.com/blogs/security/build-an-end-to-end-attribute-based-access-control-strategy-with-aws-sso-and-okta/]
- [Crossplane: Observe Only Resources Design][https://github.com/crossplane/crossplane/blob/main/design/design-doc-observe-only-resources.md]
- [Support for Querying/Filtering for Import and Observe](https://github.com/crossplane/crossplane/issues/4141]
- [Best Practices for Tagging AWS Resources](https://docs.aws.amazon.com/whitepapers/latest/tagging-best-practices/tagging-best-practices.html]

