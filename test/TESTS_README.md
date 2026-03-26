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

### Tests Writing Guides

- [Writing Composition Render Tests](common/crossplane/GUIDE.md)
- [Writing Kyverno Policy Tests](common/kyverno/GUIDE.md)