---
sidebar_position: 1
---
# Organization

Organizations in Entigo Platform provide centralized management and governance capabilities. Organizations enable platform teams to define company-specific platform configurations and policies while providing the means for centralized workspace management.

![](img/organization.png)

Organizations form the foundation of Entigo Platform's multi-tenancy architecture, providing isolation between companies or independent business units within a single company. Organizations support:

**Central identity management**: Federate authentication with your corporate SAML provider and manage user access across all workspaces from a single control point.

**Policies**: Set rules and define platform default behaviors that align with your organization's needs and compliance requirements.

**Workspace templating**: Simplify user experience and improve compliance with standardized workspace configurations that can be deployed consistently across your organization's workspaces.

**Reporting**: Gain comprehensive visibility into platform operations and usage patterns. Reporting capabilities span areas such as FinOps and cost management, vulnerability compliance tracking, and much more, enabling data-driven decision-making across your organization.

In addition to organization level isolation, Entigo Platform enables security isolation at:
- [Workspace](workspace) level, backed by dedicated [data plane](control-data-plane) services, usually deployed to a dedicated cloud account
- [Zone](zone) level, backed by a shared workspace, enabling cost-effective logical isolation
