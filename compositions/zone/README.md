# Zones for managing isolated resources in a Kubernetes cluster

Zones combine multiple technologies to enforce resource and privilage isolation in a shared Kubernetes cluster. It is not designed to be a full multi-tenancy solution. 


## Improvement opportunities:
1. Infralib managed Subnet objects should be labeled by their intended workload type. At the moment it is difficult to look up subnest and distinguish between compute and database subnets.
2. launch template ami should be configurable by the platform provider


## Development

To render the composition generated resources:

```
crossplane render examples/zone.yaml apis/zone-composition.yaml examples/functions.yaml --xrd=apis/zone-definition.yaml --required-resources=examples/required-resources.yaml
```
