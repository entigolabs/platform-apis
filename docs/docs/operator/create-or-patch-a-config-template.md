---
sidebar_position: 1
---
# Create/Patch a Config Template

# 1. Config Template overview
The **Config Template** resource defines the configuration specification for the **Infralib Agent Run Job** (infrastructure deployment).

The system utilizes a hierarchical inheritance model involving three levels.

1.  **Global Config Template:** The base, read-only template provided by the platform.
2.  **Organization Workspace Config Template:** A shared template applicable to all resources within an organization.
3.  **Resource Workspace Config Template:** A specific template tailored to a single resource environment.

**Note:** Multiple Config Templates may exist at each level, but only one can be active at a time. The active template is identified by the label:
`infraliboperator.entigo.com/active-template: "true"`

# 2. Scope and Precedence
The configuration applied to an Infralib Agent Run Job is determined by the scope in which the Config Template is defined. The Infralib Operator resolves configuration using a **specific-to-general** precedence order:

1.  **Resource Workspace (Highest Priority):** Applies strictly to the specific resource.
2.  **Organization Workspace:** Applies to all child resources within the organization.
3.  **Global Config (Lowest Priority):** The fallback default configuration.

### Resolution Logic
When initializing a new Job, the Infralib Operator searches for a Config Template with the `active-template: "true"` label. The resolution follows this logic:

- **Patching (Inheritance):** If the active template includes the `spec.parentConfig` field, the Operator looks for a template with that name in the parent scope. The local template is then applied as a **patch** over the parent configuration.
- **New Definition:** If `spec.parentConfig` is omitted, the local template is treated as a standalone definition.

# 3. Creating and Patching Templates
Config Templates are managed via standard Kubernetes manifests. Depending on your requirements, you can either create a new configuration from scratch or patch an existing one.

## 3.1. Patching Strategy (Inheritance)
To modify the behavior of a parent Config Template (e.g., to override specific organization settings for a single resource):

1.  Create a Config Template in your workspace.
2.  Set the `spec.parentConfig` field to match the **name** of the upstream template you wish to inherit from.

**Behavior:**
- **Merge:** You only need to define the specific fields you wish to override.
- **Inheritance:** Any fields not specified in your local template will automatically inherit values from the parent (Organization or Global) template.

## 3.2. Creating New Config Template
To introduce a fully custom configuration that does not inherit from upstream defaults, omit the `spec.parentConfig` field entirely.

# 4. Apply Config Template
To apply a Config Template, create a manifest in your Organization or Resource workspace.

## 4.1. Example Manifest
The following example demonstrates how to define or patch a template.

```yaml
apiVersion: infraliboperator.entigo.com/v1alpha1
kind: ConfigTemplate
metadata:
  labels:
    # Identifies this template as the currently active configuration
    infraliboperator.entigo.com/active-template: "true"
  name: config-example
spec:
  # Optional: Inherit from a parent config. Remove this line to create a root config.
  parentConfig: org-default-config
  # Notification configuration settings
  notifications:
  - api:
      key: <<.callback_key>>
      url: http://infralib-operator.default.svc.cluster.local:8082
    # Message types to subscribe to
    message_types:
      - progress
    name: portal
  # Workspace identifier prefix
  prefix: <<.resource_workspace_slug>>
  # Source definition for the Infralib registry
  sources:
  - force_version: false
    url: <<.git_url>>
    version: <<.version>>
  # Terraform execution steps (Strict order of execution)
  steps:
  - name: net
  - name: infra
  - name: apps
  # List of Infralib Agent Modules to exclude from execution
  disabledModules:
    - example-module

```

# 5. Deployment
Once the ConfigTemplate resource is applied, it will be automatically detected by the Infralib-Operator. The configuration will take effect during the next infrastructure deployment cycle (Infralib Agent Run Job execution)