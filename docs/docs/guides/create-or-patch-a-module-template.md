---
sidebar_position: 1
---

# 1. Module Template overview
**Module Templates** define the configuration specifications for Infralib Agent Modules. They serve as blueprints for infrastructure components.

The system utilizes a hierarchical inheritance model involving three levels.

**Entigo Workspace Module Templates:** Are default, predefined templates.

**Organization Workspace Module Templates:** Are shared across all child resources.

**Resource Workspace Module Templates:** Are specific to a single resource environment.

This hierarchy allows users to create new templates or patch (override) existing ones at different levels of granularity.

# 2. Scope and Precedence
The configuration applied to an Infralib Agent Module is determined by the location where the Module Template is defined. The resolution order is from most specific to most general:

**Resource Workspace:** Templates defined here have the highest priority. They apply only to the specific resource workspace.

**Organization Workspace:** Templates defined here apply to all child resource workspaces within the organization.

**Entigo Workspace (Default):** These are the base read-only templates provided by the platform.

### Resolution Logic
The Infralib-Operator configures/creates Infralib Agent Modules on Module Templates basis.
When time to configure/create Module, the Infralib-Operator searches for Module Templates in mentioned above locations.

- If a template with the same name exists in a parent scope (e.g., Organization or Entigo), the local template acts as a patch.
- If no matching name is found upstream, the local template is treated as a new definition.

# 3. Creating and Patching Templates.
Module Templates are managed via standard Kubernetes manifests. The system 'chooses' between creating a new Module Template and patching an existing one based on the `metadata.name`.

## 3.1. Patching Strategy
To modify the behavior of a default module (e.g., `eks` or `argocd`), create a Module Template in your workspace with the **exact same name** as the target module.

- **Merge Behavior:** You only need to define the fields you wish to override in the spec.
- **Inheritance:** Fields not specified in your local template will inherit values from the upstream (Organization or Entigo) template.

## 3.2. Creating New Module Templates
To introduce a custom module template that does not exist in the standard library, create a Module Template with a unique name.

# 4. Standard Library (Predefined Templates)
The following modules are currently available in the Entigo workspace:
```
argocd
aws-alb
aws-storageclass
cluster-autoscaler
crossplane
crossplane-aws
crossplane-sql
crossplane-system
eks
entigo-portal-agent
external-dns
external-secrets
grafana
istio-base
istio-gateway
istio-system
karpenter
karpenter-node-role
kms
kyverno
metrics-server
platform-apis
prometheus
promtail
rbac-bindings
route53
vpc
wireguard
```

# 5. Apply Module Template
To apply a Module Template, create a manifest in your Organization or Resource workspace.

## 5.1. Example Manifest
The following example demonstrates how to define or patch a template.

**Note:** UI-specific fields (`displayName`, `hidden`, `protected`) are optional for backend operations but affect how the module appears in the Portal.

```yaml
apiVersion: infraliboperator.entigo.com/v1alpha1
kind: ModuleTemplate
metadata:
  # The name determines if this acts as a patch or a new module template.
  # Use an existing name to patch, or a unique name for new module templates.
  name: example-module
spec:
  # The name of the infralib agent module source in the registry
  source: module-source
  # The Terraform step the infralib agent module belongs to
  step: terraform-step
  # Inputs are defined as a multiline string block.
  inputs: |-
    inputField: inputValue
  # --- UI Configuration (Optional) ---
  displayName: Example Module
  hidden: false
  protected: false
```

# 6. Deployment
Once the ModuleTemplate resource is applied, it will be automatically detected by the Infralib-Operator. The configuration will take effect during the next infrastructure deployment cycle (Infralib Agent Run Job execution)