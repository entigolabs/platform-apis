---
sidebar_position: 3
---

## Management policies
Entigo Platform can be used to manage the full lifecycle of a resource or just to observe the resource status, to enable central overview and governance. Management Policies are used to instruct the platform how to manage each resource through the `entigo.com/management-policy` label.

When the label is set to `full`, the resource is managed through the platform. This means that the platform is the source of truth for desired configuration and the resource will be provisioned accordingly to the Workspace. Resource status will be reported back from the Workspace to the Platform.

When the label is set to `observed`, the resource is considered as externally managed. For example, a user may be provisioning a database to the Workspace using GitOps tools. In that case, the Workspace is considered as the source of truth and the resource configuration spec and status are mirrored to the platform. The platform will treat the resource as read-only. To make changes to the resource, the user needs to use the same tools that were used to create the resource in the first place.

Syncer agent is responsible for labelling the resources, if the label is not already set:
- `full` - set the `entigo.com/management-policy=full` label on the managed resource in the Workspace
- `observed` - set the `entigo.com/management-policy=observed` label on the resource in the Platform. 

### Status matrix

| Platform                  | Workspace                  | Behaviour | Notes                                                                       |
| :------------------------ | :------------------------- | :-------- | :-------------------------------------------------------------------------- |
| any                       | management-policy=full     | full      | Workspace explicitly requests full management                               |
| any                       | management-policy=observed | observe   | Workspace explicitly requests observe only                                  |
| any                       | No label (steady-state)    | observe   | After migration: unlabeled resources default to observe                     |
| management-policy=full    | none (doesn't exist)       | full      | Platform provisions resource with full management                           |
| no label                  | none (doesn't exist)       | full      | Platform provisions resource with full management                           |
| management-policy=observed| none (doesn't exist)       | orphaned  | Platform expects resource but it doesn't exist; don't provision             |
| none (doesn't exist)      | none (doesn't exist)       | N/A       | No resource anywhere                                                        |

