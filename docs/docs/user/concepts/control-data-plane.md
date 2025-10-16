---
sidebar_position: 4
---

# Control & Data Plane

Think of control and data plane separation like air traffic control and aircraft. The control plane is like air traffic control—it coordinates workloads, assigns resources, and manages configurations. The data plane is like the aircraft themselves—they carry out the actual work of running your applications. Just as planes can continue flying and land safely if they temporarily lose contact with the tower, your workloads continue running even if the control plane is temporarily unavailable.

Entigo Platform is strongly influenced by this concept:

- [Organizations](organization) implement a control plane for central workspace orchestration and governance. Your applications continue to work even when the central control plane is unavailable.
- [Workspaces](workspace) include both:
  - A local control plane to orchestrate workloads within the workspace
  - A data plane where your applications run.

Similar to Organizations, if the Workspace control plane is impacted, your applications continue to work, but making changes may be limited, including self-healing and autoscaling functionality.
