## Platform-Apis Tests

### Prerequisites
Go requires import of one library which is actually a folder in entigo-infralib repo.
Clone infralib repo and use symlink to mock folder existence in platform-apis.

```
ln -s <full path to cloned entigo-infralib repo>/common <full path to cloned platform-apis repo>/common
```

### Test Suite

Test Suite consists of 2 test steps:
1. Static tests (code tests). Include:
    * Resources rendering using Crossplane CLI - allows you to test if all resources rendered in right order
    and if all resources metadata and specs fields populated properly.
    * Kyverno policies check Kyverno CLI - allows you to test if all policies implemented properly.
    *  Go tests for functions testing.
2. End-2-end (live platform-apis package) tests:
    * Running in AWS cluster using entigo-infralib test image.

All test written in Go (v. 1.25.7)

---

## Running Tests Locally

### Kyverno Policy Tests

Kyverno policy tests (e.g. `zone_kyverno_policies_test.go`) require two CLI tools to be installed:

- [`helm`](https://helm.sh/docs/intro/install/) — renders the Helm chart before applying policies
- [`kyverno`](https://kyverno.io/docs/kyverno-cli/) — evaluates Kyverno policies against rendered resources

```bash
brew install helm kyverno
```

Tests live alongside other composition tests in `compositions/<name>/test/` and are run the same way:

```bash
cd compositions/zone/test
go test -v ./...

# Run a single policy group
go test -v -run TestKyvernoPolicies/ZoneDeletionCheck ./...
```

### Composition Render Tests

Composition render tests live in `compositions/<name>/test/` and are standard Go tests that can be run directly without Docker.

**Run a single composition's render tests:**
```bash
cd compositions/<name>/test
go test -v ./...
```

**Examples:**
```bash
cd compositions/repository/test && go test -v ./...
cd compositions/webapp/test && go test -v ./...
cd compositions/valkey/test && go test -v ./...
```

> **Kafka tests require Docker.**
> Kafka compositions are not yet migrated to a local Go function and rely on a remote function image
> that `crossplane render` pulls and runs via the Docker daemon. Make sure Docker is running before
> executing Kafka render tests:
> ```bash
> cd compositions/kafka/test && go test -v ./...
> ```

### Function Unit Tests

Function unit tests live inside each function module and are plain Go unit tests.

**Run tests for a specific function:**
```bash
cd functions/<name>
go test -v ./...
```

**Examples:**
```bash
cd functions/artifact && go test -v ./...
cd functions/workload && go test -v ./...
cd functions/database && go test -v ./...
```

---

## Running Tests via Docker (CI)

The `run-tests.sh` script runs tests inside the `entigolabs/platform-apis-test-runner` Docker image,
which mirrors the CI environment and has Go module caches pre-warmed.

```bash
# All tests (auto-discovers compositions, functions, helm)
./run-tests.sh

# Single composition
./run-tests.sh compositions/repository

# Single function
./run-tests.sh functions/artifact

# Helm tests
./run-tests.sh helm
```

The script mounts the repository root into the container as `/workspace` and passes the target
workdir to `test/runner.sh` via `--type=<composition|function|helm>`.

To use a locally built test image instead of pulling the published one:
```bash
IMAGE_NAME=my-local-test-runner:latest ./run-tests.sh compositions/webapp
```

---

## End-to-End Integration Tests (`test/tests/`)

These are live integration tests that run against a real AWS Kubernetes cluster. Unlike the static and Docker-based tests above, they cannot be run locally — they require a running cluster with Crossplane, ArgoCD, and all platform-apis packages installed.

### Technologies

| Library | Purpose |
| --- | --- |
| [`terratest/modules/k8s`](https://terratest.gruntwork.io/) | kubectl wrappers, polling helpers (`retry.DoWithRetryE`) |
| [`terratest/modules/retry`](https://terratest.gruntwork.io/) | Retry loop primitives |
| [`testify/require`](https://github.com/stretchr/testify) | Assertions |
| [`entigo-infralib-common/k8s`](https://github.com/entigolabs/entigo-infralib) | Cluster connection setup (`k8s.CheckKubectlConnection`) |
| ArgoCD | Deploys test resources via Helm charts into the cluster |
| Crossplane | Manages the platform-apis XRDs and compositions under test |

### Test Structure

```
test/tests/
├── config/                         # Cluster-specific environment overrides
│   ├── aws_biz.yaml
│   └── aws_pri.yaml
├── templates/                      # ArgoCD Application + Namespace manifests
│   ├── zone_test_application.yaml
│   ├── cronjob_test_application.yaml
│   ├── postgresql_test_application.yaml
│   ├── repository_test_application.yaml
│   ├── s3bucket_test_application.yaml
│   ├── valkey_test_application.yaml
│   ├── webapp_test_application.yaml
│   ├── webaccess_test_application.yaml
│   └── ...
├── helm/
│   └── platform-apis-test-<suite>/ # Helm charts with test resource manifests
│       ├── Chart.yaml
│       └── templates/
│           └── <suite>_test.yaml   # The actual CRs deployed in the cluster
├── testconfig/
│   └── suites.yaml                 # Written by CI; controls which suites run
├── constants_test.go               # All resource names and kind strings
├── suite_config_test.go            # Suite selection logic
├── crossplane_test.go              # Shared wait/patch/read helpers
├── argocd_test.go                  # ArgoCD sync helpers
├── k8s_unit_basic_test.go          # Entry point; dispatches to suite functions
├── k8s_zone_test.go
├── k8s_postgresql_test.go
├── k8s_cronjob_test.go
├── k8s_repository_test.go
├── k8s_s3bucket_test.go
├── k8s_valkey_test.go
├── k8s_webapp_test.go
└── k8s_webaccess_test.go
```

### How Tests Run

1. **CI builds** the platform-apis Helm chart and all `platform-apis-test-<suite>` Helm charts and pushes them to GHCR.
2. **`prepare-infralib-branch`** workflow clones `entigolabs/entigo-infralib`, copies the `test/tests/` Go files and the relevant ArgoCD Application templates into the infralib module, and commits to a feature branch.
3. **`run-infralib-tests`** triggers the infralib test pipeline, which runs `go test ./...` from the infralib module against the live cluster.
4. Each `TestK8sPlatformApis*` function connects to the cluster, reads `testconfig/suites.yaml` to determine which suites are active, waits for all required Crossplane packages in parallel, then dispatches to the suite-specific test functions in parallel.

### Suite Selection

The file `testconfig/suites.yaml` is written by CI before tests execute and lists which suites are active for the current run:

```yaml
suites:
  - zone
  - postgresql
  - s3bucket
```

If this file is missing (e.g. during development), the fallback `allSuites` slice in `suite_config_test.go` is used, which includes all production-ready suites. **kafka is intentionally excluded** from both `allSuites` and all CI-generated suite lists until its function implementation is complete.

CI generates this file based on what changed (`detect-changes.yml`):

| Trigger | Functions/compositions built | Test Helm charts built | Suites run |
|---|---|---|---|
| `workflow_dispatch` | all | all | all |
| Push to `main` (something changed) | changed only | all | all |
| Push to `main` (nothing changed) | none | none | none |
| Pull request | changed only | changed only | changed only |

### Active Suites

| Suite | XRD kind | Function | Description |
|---|---|---|---|
| `zone` | `Zone` | `platform-apis-tenancy-fn` | Base infrastructure; always deployed first |
| `cronjob` | `CronJob` | `platform-apis-workload-fn` | Kubernetes CronJob wrapper |
| `postgresql` | `PostgreSQLInstance`, `PostgreSQLUser`, `PostgreSQLDatabase` | `platform-apis-database-fn` | RDS PostgreSQL with users and databases |
| `repository` | `Repository` | `platform-apis-artifact-fn` | ECR container image repository |
| `s3bucket` | `S3Bucket` | `platform-apis-storage-fn` | S3 bucket with IAM and encryption |
| `valkey` | `ValkeyInstance` | `platform-apis-database-fn` | ElastiCache Valkey (Redis-compatible) cluster |
| `webapp` | `WebApp` | `platform-apis-workload-fn` | Kubernetes Deployment + Service + Secret |
| `webaccess` | `WebAccess` | `platform-apis-networking-fn` | Istio VirtualService + ServiceEntry + DestinationRule |

### Test Pattern (CRUD)

Each suite file follows the same structure:

```
testOrchestrator        — applies ArgoCD app, syncs, defers cleanup, runs sub-tests
  testCreate            — waitSyncedAndReady on the composite XR
  testSubResources      — verify provider-managed or native resources exist/are ready
  testRead              — assert status fields and spec propagation
  testUpdate            — patch the composite, verify change propagates to sub-resources
  testDeleteProtection  — (where applicable) verify webhook rejects deletion when protected
cleanupSuite            — disable deletion protection if needed, delete composites, delete ArgoCD app
                          (namespace is kept so zone-managed resources persist for faster re-runs)
```

### Adding a New Suite

1. Create `test/tests/helm/platform-apis-test-<suite>/Chart.yaml` and `templates/<suite>_test.yaml` with the test CRs.
2. Create `test/tests/templates/<suite>_test_application.yaml` with the ArgoCD `Application` + `Namespace`.
3. Create `test/tests/k8s_<suite>_test.go` with the CRUD test function `test<Suite>(t, cluster, argocd)`.
4. Add constants to `test/tests/constants_test.go`.
5. Add `"<suite>"` to `allSuites` in `suite_config_test.go`.
6. Add `if cfg.Has("<suite>")` block in `k8s_unit_basic_test.go` (both `parallel-tests` and `waitPackagesReady`).
7. Update `.github/workflows/detect-changes.yml`: add path filter in `files_yaml`, `CHANGED` env var, include in `any_changes`, `affected_suites`, and `testhelm` lists.
8. Update `.github/workflows/prepare-infralib-branch.yml`: add to `affected_suites` and `built_testhelm` defaults. The template is copied automatically if it follows the `<suite>_test_application.yaml` naming convention.

---

### Tests Writing Guides

- [Writing Composition Render Tests](common/crossplane/GUIDE.md)
- [Writing Kyverno Policy Tests](common/kyverno/GUIDE.md)
- [Writing End-to-End Integration Tests](tests/GUIDE.md)