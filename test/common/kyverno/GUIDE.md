## Writing Kyverno Policy Tests

Kyverno policy tests verify that the policies shipped with a Helm chart correctly allow or deny Kubernetes resource operations — without needing a real cluster. The test library lives in `test/common/kyverno/` and is imported as `github.com/entigolabs/static-common/kyverno`.

### What these tests do

Each test:
1. Renders the Helm chart with `helm template` to get the policy YAML.
2. Extracts only the policies relevant to the resource kind being tested.
3. Replaces any `resource.List()` calls in CEL expressions with static fixture data so the test runs offline.
4. Runs `kyverno apply` against the resource and asserts that the outcome is `pass` or `fail`.

### File and package conventions

- Test files must end with `_test.go` (e.g. `zone_kyverno_policies_test.go`).
- The package must be `package test`.
- Exported entry-points must start with `Test`.
- Group subtests by policy — one `t.Run` per policy, one table of cases per subtest.
- Call `t.Parallel()` in every subtest and every table case so they run concurrently.

```go
package test

import (
    "testing"
    "github.com/entigolabs/static-common/kyverno"
)

const chartDir = "../../../helm"

func TestKyvernoPolicies(t *testing.T) {
    t.Run("MyPolicy", testMyPolicy)
}
```

### TestScenario

`kyverno.TestScenario` describes a single test case:

```go
type TestScenario struct {
    HelmValues       map[string]string // extra --set flags for helm template; nil uses chart defaults
    ResourceYAML     string            // the Kubernetes resource YAML being tested (required)
    VariablesYAML    string            // overrides request.operation and other globals (see GenerateOperationValues)
    UserInfoYAML     string            // sets who is making the request (see GenerateUserInfo); defaults to an unprivileged user
    ExpectedAction   string            // "pass" — policy allows the resource; "fail" — policy denies it
    ExpectedInOutput string            // optional: substring that must appear in kyverno output (e.g. a generated resource name)
}
```

Run a scenario by calling:

```go
kyverno.RunPolicyCheck(t, chartDir, scenario)
```

### Resource generators

Use these helpers to build resource YAML without writing raw strings:

| Function | What it generates |
|---|---|
| `GenerateNamespace(name, zone, enforce, warn)` | `v1/Namespace` with zone and pod-security labels |
| `GenerateZone(name)` | `tenancy.entigo.com/v1alpha1/Zone` |
| `GenerateZoneWithNamespaces(name, namespaces)` | Zone with `spec.namespaces` populated |
| `GenerateArgoApp(name, project, destNamespace)` | `argoproj.io/v1alpha1/Application` |
| `GenerateUserInfo(groups...)` | kyverno `UserInfo` placing the caller in the given groups |
| `GenerateOperationValues(operation)` | kyverno `Values` setting `request.operation` (`"CREATE"`, `"UPDATE"`, `"DELETE"`) |

### Structuring a test

Group cases by policy using a nested `t.Run` + table pattern. The `runCases` helper below is a common idiom already used in the zone tests:

```go
func testMyPolicy(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name     string
        scenario kyverno.TestScenario
    }{
        {
            name: "pass: valid resource is accepted",
            scenario: kyverno.TestScenario{
                ExpectedAction: "pass",
                ResourceYAML:   kyverno.GenerateNamespace("my-ns", "my-zone", "restricted", "restricted"),
            },
        },
        {
            name: "fail: invalid resource is denied",
            scenario: kyverno.TestScenario{
                ExpectedAction: "fail",
                ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "my-zone", "privileged", "restricted"),
            },
        },
    }
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            kyverno.RunPolicyCheck(t, chartDir, tc.scenario)
        })
    }
}
```

### Testing user permissions

Policies that check who is making the request (e.g. denying contributors from creating namespaces) use `UserInfoYAML` to set the caller's group membership:

```go
{
    name: "fail: contributor cannot create a namespace",
    scenario: kyverno.TestScenario{
        ExpectedAction: "fail",
        ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
        UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
    },
},
{
    name: "pass: maintainer can create a namespace",
    scenario: kyverno.TestScenario{
        ExpectedAction: "pass",
        ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
        UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
    },
},
```

If `UserInfoYAML` is omitted the framework uses a default unprivileged user (`system:authenticated`).

### Testing UPDATE and DELETE operations

The kyverno CLI always simulates a `CREATE` operation. To test `UPDATE` or `DELETE`, pass a `VariablesYAML` that overrides `request.operation`:

```go
{
    name: "fail: contributor cannot update a namespace",
    scenario: kyverno.TestScenario{
        ExpectedAction: "fail",
        ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
        UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
        VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
    },
},
{
    name: "fail: zone deletion blocked when namespaces still attached",
    scenario: kyverno.TestScenario{
        ExpectedAction: "fail",
        ResourceYAML:   kyverno.GenerateZone("my-zone"),
        VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
    },
},
```

> **Note on DELETE policies:** policies that use a `matchCondition` of `request.operation == "DELETE"` would never evaluate under the kyverno CLI because it always simulates CREATE. The framework automatically strips that matchCondition and rewrites `oldObject.` references to `object.` when testing custom resources (like `Zone`), so deletion policies evaluate correctly against the submitted resource YAML.

### Testing with non-default Helm values

Some policies behave differently depending on chart values (e.g. `podSecurity` level). Pass `HelmValues` to override defaults:

```go
{
    name: "pass: baseline enforce+warn allowed when podSecurity is baseline",
    scenario: kyverno.TestScenario{
        ExpectedAction: "pass",
        HelmValues: map[string]string{
            "zone.install":                       "true",
            "zone.environmentConfig.podSecurity": "baseline",
        },
        ResourceYAML: kyverno.GenerateNamespace("my-ns", "my-zone", "baseline", "baseline"),
    },
},
```

### Offline fixtures

Tests run without a real cluster. The framework seeds two sets of static fixture data:

**Zones** — `resource.List("tenancy.entigo.com/v1alpha1", "zones", "")` resolves to:
- `my-zone`
- `default-zone-name`

Any zone name not in this list is treated as non-existent by the policy.

**Namespaces** (used by zone ownership and deletion check policies):
- `attached-ns` — labeled `tenancy.entigo.com/zone: my-zone`
- `stolen-ns` — no zone label

These fixtures are hardcoded so tests are fully deterministic. When writing test cases, use these names to match the expected policy behaviour:

```go
// "my-zone" exists → deletion blocked because "attached-ns" references it
ResourceYAML: kyverno.GenerateZone("my-zone"),

// "other-zone" doesn't own any namespace → deletion allowed
ResourceYAML: kyverno.GenerateZone("other-zone"),

// "new-zone" tries to claim "attached-ns" which belongs to "my-zone" → denied
ResourceYAML: kyverno.GenerateZoneWithNamespaces("new-zone", []string{"attached-ns"}),
```

### How the framework picks file mode vs cluster mode

- **Built-in resources** (`v1`, `apps/`, `batch/`, `networking.k8s.io/`, …): the resource YAML is written to a temp file and passed via `--resource`. `KUBECONFIG=/dev/null` prevents any real cluster access.
- **Custom CRDs** (`tenancy.entigo.com/…`, `argoproj.io/…`, …): the framework starts an in-process fake Kubernetes API server and passes `--cluster`. The fake server handles API discovery, serves the tested resource, and provides the static namespace fixture list. You don't need to do anything differently — just provide the correct resource YAML and the framework routes automatically.

### Verifying generated resources

For generating policies (e.g. generating a Namespace from an ArgoCD Application), assert that the output contains the expected resource name:

```go
{
    name: "pass: ArgoApp generates namespace",
    scenario: kyverno.TestScenario{
        ExpectedAction:   "pass",
        ResourceYAML:     kyverno.GenerateArgoApp("my-app", "my-project", "my-namespace"),
        ExpectedInOutput: "my-namespace",
    },
},
```

### Complete example

```go
package test

import (
    "testing"
    "github.com/entigolabs/static-common/kyverno"
)

const chartDir = "../../../helm"

func TestKyvernoPolicies(t *testing.T) {
    t.Run("ContributorDeny", testContributorDeny)
    t.Run("ZoneDeletionCheck", testZoneDeletionCheck)
}

func testContributorDeny(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name     string
        scenario kyverno.TestScenario
    }{
        {
            name: "fail: contributor cannot create a namespace",
            scenario: kyverno.TestScenario{
                ExpectedAction: "fail",
                ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
                UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
            },
        },
        {
            name: "fail: contributor cannot update a namespace",
            scenario: kyverno.TestScenario{
                ExpectedAction: "fail",
                ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
                UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
                VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
            },
        },
    }
    runCases(t, cases)
}

func testZoneDeletionCheck(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name     string
        scenario kyverno.TestScenario
    }{
        {
            name: "fail: zone deletion blocked when namespaces still attached",
            scenario: kyverno.TestScenario{
                ExpectedAction: "fail",
                ResourceYAML:   kyverno.GenerateZone("my-zone"),
                VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
            },
        },
        {
            name: "pass: zone deletion allowed when no namespaces attached",
            scenario: kyverno.TestScenario{
                ExpectedAction: "pass",
                ResourceYAML:   kyverno.GenerateZone("other-zone"),
                VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
            },
        },
    }
    runCases(t, cases)
}

func runCases(t *testing.T, cases []struct {
    name     string
    scenario kyverno.TestScenario
}) {
    t.Helper()
    for _, tc := range cases {
        tc := tc
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            kyverno.RunPolicyCheck(t, chartDir, tc.scenario)
        })
    }
}
```