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

### Tests Writing Guides

[Writing Composition Render Tests](common/crossplane/GUIDE.md).