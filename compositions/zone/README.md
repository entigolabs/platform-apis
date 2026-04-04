# Zones for managing isolated resources in a Kubernetes cluster

Zones combine multiple technologies to enforce resource and privilege isolation in a shared Kubernetes cluster. It is not designed to be a full multi-tenancy solution.


## Improvement opportunities:
1. Infralib managed Subnet objects should be labeled by their intended workload type. At the moment it is difficult to look up subnets and distinguish between compute and database subnets.
2. Launch template AMI should be configurable by the platform provider


## Development

To render the composition generated resources:

```
crossplane render examples/zone.yaml apis/zone-composition.yaml examples/functions.yaml --xrd=apis/zone-definition.yaml --required-resources=examples/required-resources.yaml
```

---

## Kyverno Policies

The zone composition ships several Kyverno policies that enforce tenancy rules at admission time:

| Policy | Kind | Description |
|---|---|---|
| `platform-apis-zone-namespace-pod-security` | ValidatingPolicy | Rejects namespaces with `privileged` pod security enforce/warn level |
| `platform-apis-namespace-add-missing-zone-label` | MutatingPolicy | Auto-assigns a zone label to namespaces that do not carry one |
| `platform-apis-zone-namespace-contributor-deny` | ValidatingPolicy | Blocks contributors from creating, updating, or deleting namespaces |
| `platform-apis-zone-namespace-maintainer-deny` | ValidatingPolicy | Blocks maintainers from creating/updating namespaces with the `infralib` zone label |
| `platform-apis-zone-maintainer-infralib-zone-deny` | ValidatingPolicy | Blocks maintainers from creating or updating the Zone named `infralib` |
| `platform-apis-zone-deletion-check-namespaces` | ValidatingPolicy | Blocks deletion of a Zone that still has namespaces attached |
| `platform-apis-zone-namespace-ownership` | ValidatingPolicy | Enforces namespace ownership: zones cannot claim namespaces labeled to another zone or without any zone label |
| `generate-namespace-from-argocd-app` | GeneratingPolicy | Generates a destination namespace when an ArgoCD Application is created (excludes `infralib` project) |

### Running Static Policy Tests

Static policy tests live in `test/zone_kyverno_policies_test.go`. They use the `static-common/kyverno` library to simulate admission requests offline — no running cluster required.

```bash
cd compositions/zone/test
go test -v -run TestKyvernoPolicies ./...
```

Each policy has its own test function (`testNamespacePodSecurity`, `testContributorDeny`, etc.) with table-driven cases marked `pass` or `fail`. Test names match the e2e test names exactly so failures can be traced across both layers.

### Running E2E Policy Tests

E2E policy tests live in `test/tests/k8s_zone_kyverno_test.go`. They exercise the real admission webhooks in the cluster.

Role-based tests (`ContributorDeny`, `MaintainerNamespaceDeny`, `MaintainerInfralibZoneDeny`) require AWS IAM credentials with the appropriate permissions:

| Env var | Role |
|---|---|
| `CONTRIBUTOR_AWS_ACCESS_KEY_ID` / `CONTRIBUTOR_AWS_SECRET_ACCESS_KEY` | IAM identity in the `contributor` group |
| `MAINTAINER_AWS_ACCESS_KEY_ID` / `MAINTAINER_AWS_SECRET_ACCESS_KEY` | IAM identity in the `maintainer` group |

If these are not set, the role-based tests are skipped with an uppercase log message. See `test/tests/GUIDE.md` for details on writing and extending these tests.
