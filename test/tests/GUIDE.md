# Writing End-to-End Integration Tests

This guide explains how to write, structure, and register a new end-to-end test suite in `test/tests/`.

End-to-end tests run against a **live AWS Kubernetes cluster** with Crossplane, ArgoCD, and all platform-apis packages installed. They cannot be run locally — they are triggered by CI, which copies the Go test files into the `entigolabs/entigo-infralib` repository and runs them from there.

---

## Overview of the Flow

```
CI builds Helm charts  →  prepare-infralib-branch copies test files
→  infralib pipeline runs go test  →  tests connect to cluster
→  ArgoCD deploys CRs  →  Crossplane reconciles  →  assertions pass/fail
```

The entry point is `TestK8sPlatformApisAWSBiz` in `k8s_unit_basic_test.go`. It:
1. Waits for all Crossplane packages (Functions + Configurations) to be Healthy+Installed.
2. Sets up zone infrastructure (required base for all suites).
3. Dispatches active suites in parallel.

---

## File Layout for a New Suite

Adding a suite named `myresource` requires creating/modifying these files:

```
test/tests/
├── helm/platform-apis-test-myresource/
│   ├── Chart.yaml
│   └── templates/
│       └── myresource_test.yaml        # the CRs to deploy
├── templates/
│   └── myresource_test_application.yaml  # ArgoCD Application + Namespace
├── k8s_myresource_test.go              # CRUD test logic
└── constants_test.go                   # add your constants here
```

Plus workflow updates in `.github/workflows/` (covered at the end).

---

## Step 1 — Create the Helm Chart

**`test/tests/helm/platform-apis-test-myresource/Chart.yaml`**

```yaml
apiVersion: v2
name: platform-apis-test-myresource
description: Test chart for MyResource
type: application
version: 0.0.1
```

**`test/tests/helm/platform-apis-test-myresource/templates/myresource_test.yaml`**

Define the CRs that will be deployed into the cluster. Use `{{ .Release.Namespace }}` for the namespace — ArgoCD sets this from the Application's destination.

```yaml
apiVersion: mygroup.entigo.com/v1alpha1
kind: MyResource
metadata:
  name: test-myresource-minimal
  namespace: {{ .Release.Namespace }}
spec: {}
---
apiVersion: mygroup.entigo.com/v1alpha1
kind: MyResource
metadata:
  name: test-myresource-custom
  namespace: {{ .Release.Namespace }}
spec:
  someField: someValue
  deletionProtection: false
```

**Conventions:**
- Always include a minimal CR (empty `spec: {}`) to verify defaults.
- Include a custom CR to test non-default field propagation.
- Set `deletionProtection: false` on the custom CR so cleanup does not require patching.
- The minimal CR should get `deletionProtection: true` by default (if the resource supports it), so you can test the deletion rejection webhook.

---

## Step 2 — Create the ArgoCD Application Template

**`test/tests/templates/myresource_test_application.yaml`**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: test-myresource
  labels:
    tenancy.entigo.com/zone: a
    pod-security.kubernetes.io/enforce: baseline
    pod-security.kubernetes.io/warn: baseline
---
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: test-myresource
  namespace: argocd-biz
spec:
  destination:
    server: https://kubernetes.default.svc
    namespace: test-myresource
  project: a
  source:
    chart: platform-apis-test-myresource
    repoURL: ghcr.io/entigolabs
    targetRevision: '*.*.*-0'
    helm:
      releaseName: myresource
      values: |
        targetRevision: '*.*.*-0'
  syncPolicy:
    syncOptions:
      - RespectIgnoreDifferences=true
```

**Notes:**
- Namespace must have `tenancy.entigo.com/zone: a` to be picked up by the zone tenancy rules.
- `targetRevision: '*.*.*-0'` matches pre-release chart versions built by CI.
- Add `ignoreDifferences` on fields that Crossplane or controllers mutate after creation (e.g. `spec.deletionProtection` populated by defaulting webhook):

```yaml
  ignoreDifferences:
    - group: mygroup.entigo.com
      kind: MyResource
      jsonPointers:
        - /spec/deletionProtection
```

---

## Step 3 — Add Constants

Add all resource names and kind strings to `constants_test.go`:

```go
// ── MyResource ────────────────────────────────────────────────────────────────

MyResourceNamespaceName   = "test-myresource"
MyResourceApplicationName = "test-myresource"

MyResourceMinimalName = "test-myresource-minimal"
MyResourceCustomName  = "test-myresource-custom"

MyResourceKind        = "myresources.mygroup.entigo.com"
MyProviderResourceKind = "managedresource.provider.aws.m.upbound.io"
```

**Kind strings** follow the Crossplane convention:
- XR composite: `<plural>.<group>` (e.g. `s3buckets.storage.entigo.com`)
- Provider-managed resource: `<kind>.<group>` (e.g. `bucket.s3.aws.m.upbound.io`)

Find the exact strings with:
```bash
kubectl get crds | grep myresource
```

---

## Step 4 — Write the Test File

Create `test/tests/k8s_myresource_test.go`. All tests are in `package test`.

### Structure

Every suite file follows the same four-section layout:

```
Orchestrator   — deploy ArgoCD app, defer cleanup, run sub-tests sequentially
  sub-tests    — one function per resource (or logical group)
    Create     — waitSyncedAndReady
    SubResources — wait for provider-managed resources
    Read       — assert spec/status fields
    Update     — patch XR, verify propagation
    DeleteProtection — testDeletionRejected (if applicable)
Cleanup        — disable protection, delete composites, delete namespace
```

### Minimal Example

```go
package test

import (
    "testing"
    "time"

    terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
    "github.com/stretchr/testify/require"
)

// ── Orchestrator ──────────────────────────────────────────────────────────────

func testMyResource(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
    ns := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, MyResourceNamespaceName)
    defer cleanupMyResource(t, cluster, argocd)

    applyFile(t, cluster, "./templates/myresource_test_application.yaml")
    syncWithRetry(t, argocd, MyResourceApplicationName)

    t.Run("Minimal", func(t *testing.T) { testMinimalMyResource(t, ns) })
    if t.Failed() {
        return
    }
    t.Run("Custom", func(t *testing.T) { testCustomMyResource(t, ns) })
}

// ── Minimal ───────────────────────────────────────────────────────────────────

func testMinimalMyResource(t *testing.T, ns *terrak8s.KubectlOptions) {
    t.Helper()

    // Create: wait for composite to be Synced+Ready
    waitSyncedAndReady(t, ns, MyResourceKind, MyResourceMinimalName, 60, 10*time.Second)
    if t.Failed() {
        return
    }

    // Sub-resources: provider-managed resource must be created and ready
    t.Run("SubResources", func(t *testing.T) {
        t.Run("ProviderResource", func(t *testing.T) {
            t.Parallel()
            waitSyncedAndReadyByLabel(t, ns, MyProviderResourceKind, MyResourceMinimalName, 60, 10*time.Second)
        })
    })
    if t.Failed() {
        return
    }

    // Read: status fields populated by function
    require.NotEmpty(t, getField(t, ns, MyResourceKind, MyResourceMinimalName, ".status.someOutput"),
        "status.someOutput should be populated")

    // Delete protection: minimal CR defaults to deletionProtection=true
    testDeletionRejected(t, ns, MyResourceKind, MyResourceMinimalName)

    // Update: patch spec field, verify it propagates to provider resource
    patchResource(t, ns, MyResourceKind, MyResourceMinimalName, `{"spec":{"someField":"newValue"}}`)
    provName, err := getFirstByLabel(t, ns, MyProviderResourceKind, MyResourceMinimalName)
    require.NoError(t, err)
    waitFieldEquals(t, ns, MyProviderResourceKind, provName, ".spec.forProvider.someField", "newValue", 30, 10*time.Second)
}

// ── Custom ────────────────────────────────────────────────────────────────────

func testCustomMyResource(t *testing.T, ns *terrak8s.KubectlOptions) {
    t.Helper()

    waitSyncedAndReady(t, ns, MyResourceKind, MyResourceCustomName, 60, 10*time.Second)
    if t.Failed() {
        return
    }

    provName, err := getFirstByLabel(t, ns, MyProviderResourceKind, MyResourceCustomName)
    require.NoError(t, err)

    // Read: custom spec fields reflected on provider resource
    require.Equal(t, "someValue", getField(t, ns, MyProviderResourceKind, provName, ".spec.forProvider.someField"))
}

// ── Cleanup ───────────────────────────────────────────────────────────────────

func cleanupMyResource(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
    if t.Failed() {
        return // leave resources for debugging
    }
    ns := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, MyResourceNamespaceName)

    // Disable deletion protection before deleting
    patchDeletionProtectionIfEnabled(t, ns, MyResourceKind, MyResourceMinimalName)

    cleanupDeleteParallel(t, ns, MyResourceKind, MyResourceMinimalName, MyResourceCustomName)

    _, _ = terrak8s.RunKubectlAndGetOutputE(t, argocd, "delete", "application", MyResourceApplicationName, "--ignore-not-found")
    _, _ = terrak8s.RunKubectlAndGetOutputE(t, cluster, "delete", "namespace", MyResourceNamespaceName, "--ignore-not-found", "--wait=true")
}
```

---

## Helper Reference

All helpers live in `crossplane_test.go` and `argocd_test.go`.

### Wait helpers

| Helper | When to use |
| --- | --- |
| `waitSyncedAndReady(t, ns, kind, name, retries, interval)` | Wait for a composite XR to have `Synced=True` and `Ready=True` |
| `waitSyncedAndReadyByLabel(t, ns, kind, composite, retries, interval)` | Wait for a provider-managed resource found via `crossplane.io/composite=<name>` label; returns resource name |
| `waitResourceExists(t, ns, kind, name, retries, interval)` | Wait for any named resource to exist (no condition check) |
| `waitFieldEquals(t, ns, kind, name, jsonpath, value, retries, interval)` | Poll until a jsonpath field equals expected value |
| `waitCrossplanePackageReady(t, cluster, kind, name)` | Wait for a Function or Configuration to be Healthy+Installed |

### Read helpers

| Helper | When to use |
| --- | --- |
| `getField(t, ns, kind, name, jsonpath)` | Read a single field; fails test if resource not found |
| `getFirstByLabel(t, ns, kind, composite)` | Get the name of the first resource matching `crossplane.io/composite=<composite>` |

### Patch helpers

| Helper | When to use |
| --- | --- |
| `patchResource(t, ns, kind, name, jsonMergePatch)` | Apply a JSON merge patch to a resource |
| `patchAndWaitField(t, ns, kind, name, patch, jsonpath, value, retries, interval)` | Patch a resource and wait for a field on the **same** resource to change |

### Delete / protection helpers

| Helper | When to use |
| --- | --- |
| `testDeletionRejected(t, ns, kind, name)` | Assert deletion is rejected by a validating webhook (`protected` in message) |
| `testUsageBlocksDeletion(t, ns, kind, name)` | Assert Crossplane Usage prevents deletion (resource still exists after 10s) |
| `testUsage(t, ns, usageName, ofKind, ofName, byKind, byName)` | Verify a `Usage` resource exists and its `spec.of` / `spec.by` fields are correct |
| `patchDeletionProtectionIfEnabled(t, ns, kind, name)` | Disable `spec.deletionProtection` before cleanup if it is enabled |

### Cleanup helpers

| Helper | When to use |
| --- | --- |
| `cleanupDeleteAndWait(t, ns, kind, name, maxRetries)` | Delete one resource and wait for it to disappear |
| `cleanupDeleteParallel(t, ns, kind, names...)` | Delete multiple resources of the same kind concurrently |

### ArgoCD helpers

| Helper | When to use |
| --- | --- |
| `applyFile(t, cluster, file)` | `kubectl apply -f` a manifest file |
| `syncWithRetry(t, argocd, appName)` | Force ArgoCD sync and wait for `operationState.phase == Succeeded` |
| `waitApplicationHealthy(t, argocd, appName)` | Wait for ArgoCD Application `health.status == Healthy` |

---

## Choosing Retry Values

Resources in this repo take different amounts of time to become ready. Use these as starting points:

| Resource type | Typical wait | Recommended retries × interval |
| --- | --- | --- |
| Native K8s (Deployment, Service) | < 30s | `15 × 10s` |
| ECR Repository | 1–2 min | `30 × 10s` |
| S3 Bucket + IAM | 3–5 min | `60–90 × 10s` |
| RDS PostgreSQL instance | 5–10 min | `90 × 10s` |
| ElastiCache Valkey | 5–15 min | `120 × 10s` |
| Update propagation (spec → provider) | < 5 min | `30 × 10s` |

---

## Native Kubernetes Resources (no Synced/Ready)

Resources like `Deployment`, `Service`, and `Secret` do not have Crossplane status conditions. Use `waitResourceExists` for them, then read fields directly:

```go
waitResourceExists(t, ns, "deployment", "my-deployment", 30, 10*time.Second)
require.Equal(t, "docker.io/nginx:alpine",
    getField(t, ns, "deployment", "my-deployment", ".spec.template.spec.containers[0].image"))
```

For update verification on native resources, use `waitFieldEquals`:

```go
patchResource(t, ns, MyResourceKind, MyResourceName, `{"spec":{"replicas":2}}`)
waitFieldEquals(t, ns, "deployment", "my-deployment", ".spec.replicas", "2", 30, 10*time.Second)
```

---

## Istio / Label-based Sub-resources

Some resources (ServiceEntry, DestinationRule) have generated names. Look them up by label:

```go
_, err := retry.DoWithRetryE(t, "ServiceEntry for test-myresource", 15, 10*time.Second,
    func() (string, error) {
        out, err := terrak8s.RunKubectlAndGetOutputE(t, ns, "get", "serviceentries.networking.istio.io",
            "-l", fmt.Sprintf("crossplane.io/composite=%s", MyResourceName),
            "-o", "jsonpath={.items[0].metadata.name}")
        if err != nil || out == "" {
            return "", fmt.Errorf("no ServiceEntry for %s yet", MyResourceName)
        }
        return out, nil
    })
require.NoError(t, err)
```

---

## Step 5 — Register the Suite

### `suite_config_test.go`

Add the suite name to `allSuites`:

```go
var allSuites = []string{"zone", "postgresql", "cronjob", "repository", "s3bucket", "valkey", "webapp", "webaccess", "myresource"}
```

### `k8s_unit_basic_test.go`

Add two blocks — one in `parallel-tests` and one in `waitPackagesReady`.

In `testPlatformApis` / `parallel-tests`:
```go
if cfg.Has("myresource") {
    t.Run("myresource", func(t *testing.T) {
        t.Parallel()
        testMyResource(t, cluster, argocd)
    })
}
```

In `waitPackagesReady`:
```go
if cfg.Has("myresource") {
    checkPlatformApisHaveRequiredPackages(t, cluster, MyResourceConfigurationName, MyFunctionName)
}
```

Add the corresponding constants to `constants_test.go`:
```go
MyResourceConfigurationName = "platform-apis-myresource"
MyFunctionName              = "platform-apis-myresource-fn"
```

---

## Step 6 — Update CI Workflows

### `detect-changes.yml`

1. Add path filter in `files_yaml`:
```yaml
myresource:
  - 'compositions/myresource/**'
  - 'functions/mygroup/**'
  - 'helm/templates/myresource.yaml'
  - 'test/tests/helm/platform-apis-test-myresource/**'
```

2. Add env var in the `Detect changes` step:
```bash
MYRESOURCE_CHANGED: ${{ steps.changed-files.outputs.myresource_any_changed || 'false' }}
```

3. Add to `any_changes` condition:
```bash
|| [ "$MYRESOURCE_CHANGED" == "true" ]
```

4. Add to incremental `AFFECTED_LIST`:
```bash
[ "$MYRESOURCE_CHANGED" == "true" ] && AFFECTED_LIST="${AFFECTED_LIST}\"myresource\","
```

5. Add to `force-all` affected_suites (main / workflow_dispatch):
```bash
echo 'affected_suites=["zone","postgresql","cronjob","repository","s3bucket","valkey","webaccess","webapp","myresource"]' >> $GITHUB_OUTPUT
```

6. Add to incremental testhelm:
```bash
[ "$MYRESOURCE_CHANGED" == "true" ] && TESTHELM_LIST="${TESTHELM_LIST},\"platform-apis-test-myresource\""
```

7. Add to full testhelm rebuild list (inside the `$COMMON_LIB_CHANGED` branch).

### `prepare-infralib-branch.yml`

1. Add `"myresource"` to the `affected_suites` default input.
2. Add `"platform-apis-test-myresource"` to the `built_testhelm` default input.

The workflow automatically copies `<suite>_test_application.yaml` from `test/tests/templates/` for each active suite — no additional changes needed as long as the template file follows the naming convention.

---

## Cleanup Contract

The cleanup function is called via `defer` from the orchestrator. Follow these rules:

- **Return immediately if `t.Failed()`** — leave resources in place for debugging.
- Disable `deletionProtection` before deleting composites that have it. Use `patchDeletionProtectionIfEnabled`.
- For resources with dependencies (e.g. PostgreSQL databases → users → instance), delete in reverse dependency order.
- Use `cleanupDeleteParallel` for independent resources of the same kind. It waits for each resource to fully disappear before returning.
- Delete the ArgoCD Application **after composites are confirmed gone** and **before the namespace**. If the application is still active while the namespace exists, ArgoCD may try to reconcile resources back.
- Delete the namespace last with `--wait=true`.

```go
func cleanupMyResource(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
    if t.Failed() {
        return
    }
    ns := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, MyResourceNamespaceName)

    patchDeletionProtectionIfEnabled(t, ns, MyResourceKind, MyResourceMinimalName)
    cleanupDeleteParallel(t, ns, MyResourceKind, MyResourceMinimalName, MyResourceCustomName)

    _, _ = terrak8s.RunKubectlAndGetOutputE(t, argocd, "delete", "application", MyResourceApplicationName, "--ignore-not-found")
    _, _ = terrak8s.RunKubectlAndGetOutputE(t, cluster, "delete", "namespace", MyResourceNamespaceName, "--ignore-not-found", "--wait=true")
}
```

---

## Common Mistakes

**Wrong kind string** — kind strings use the CRD plural + group, not the Go type name. Verify with `kubectl get crds | grep <keyword>`.

**Using `waitSyncedAndReady` on native K8s resources** — Deployment, Service, Secret do not have Crossplane conditions. Use `waitResourceExists` + `getField` instead.

**Using name-based lookup for label-named resources** — Provider-managed resources often get generated names. Use `waitSyncedAndReadyByLabel` or `getFirstByLabel` rather than hardcoding names.

**Not disabling deletion protection before cleanup** — If `testDeletionRejected` passes, the resource has protection enabled. The cleanup must call `patchDeletionProtectionIfEnabled` before attempting deletion, otherwise cleanup will fail and leave the namespace stuck.

**Too few retries for slow AWS resources** — RDS and ElastiCache can take 10+ minutes. Set retries generously (90–120 × 10s). A test that flakes on timeout is worse than a test that waits longer.

**Registering the suite in only one place** — You must update both `allSuites`, the `parallel-tests` block, and `waitPackagesReady`. Missing `waitPackagesReady` means the test runs before Crossplane has installed the CRDs, causing immediate failures.