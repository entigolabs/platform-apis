## Writing Composition Render Tests (`*_render_test.go`)

### File and Package Conventions

- Test files **must** end with `_test.go` (e.g. `webapp_render_test.go`).
- The package must be `package test`.
- Exported test entry-points **must** start with `Test` (e.g. `TestWebAppCrossplaneRender`).
- Sub-tests called via `t.Run(...)` are unexported (start with a lowercase letter).
- Use `t.Parallel()` wherever possible — both in the top-level test and in each sub-test — so independent scenarios run concurrently.

```go
package test

import (
    "testing"
    "github.com/entigolabs/crossplane-common"
)

func TestMyCrossplaneRender(t *testing.T) {
    crossplane.StartCustomFunction(t, function, "9443")
    t.Run("ScenarioA", testScenarioA)
    t.Run("ScenarioB", testScenarioB)
}

func testScenarioA(t *testing.T) {
    t.Parallel()
    // ...
}
```

### Typical File Layout

```
compositions/<name>/test/
  <name>_render_test.go   ← test code
```

Constants block at the top of the file — always use relative paths from the test directory:

```go
const (
    composition     = "../apis/<name>-composition.yaml"
    env             = "../examples/environment-config.yaml"
    function        = "../../../functions/<function-name>"   // local Go function path
    functionsConfig = "../../../test/common/functions-dev.yaml"
    required        = "../examples/required-resources.yaml"  // if needed
)
```

### Starting a Local Go Function

For compositions backed by a local Go function, start it once per test with `StartCustomFunction`.
The function is automatically killed when the test finishes.

```go
crossplane.StartCustomFunction(t, function, "9443")
```

### Rendering Resources

`CrossplaneRender` invokes `crossplane render` in-process and returns all rendered resources.

```go
tmpDir := t.TempDir()
extra    := filepath.Join(tmpDir, "extra.yaml")
observed := filepath.Join(tmpDir, "observed.yaml")

// Build extra resources file (EnvironmentConfig + required resources)
crossplane.AppendYamlToResources(t, env, extra)
crossplane.AppendYamlToResources(t, required, extra)

// First render — no observed state yet
resources := crossplane.CrossplaneRender(
    t,
    myResource,       // path to example XR YAML
    composition,      // path to composition YAML
    functionsConfig,  // path to functions config YAML
    crossplane.Ptr(extra),  // optional extra resources (-e flag); pass nil to omit
    nil,                    // optional observed resources (-o flag); pass nil to omit
)
```

### Asserting Rendered Resources

**Count check** — verify the expected number of resources of a given kind:
```go
crossplane.AssertResourceCount(t, resources, "Deployment", 1)
crossplane.AssertResourceCount(t, resources, "Service", 1)
```

**Field values** — assert that at least one resource of `kind`+`apiVersion` matches all provided fields.
Use `"*"` as the expected value to assert only that the field exists and is non-null.
Field paths follow [gjson](https://github.com/tidwall/gjson) dot-notation:

```go
crossplane.AssertFieldValues(t, resources, "Deployment", "apps/v1", map[string]string{
    "metadata.name":                      "my-app",
    "metadata.namespace":                 "default",
    "metadata.ownerReferences.0.kind":    "WebApp",
    "spec.replicas":                      "1",
    "spec.template.spec.containers.0.image": "*",  // just assert it's set
})
```

**Ready status** — assert that the XR composite resource has `condition.type=Ready` and `status=True`:
```go
crossplane.AssertResourceReady(t, resources, "WebApp", "workload.entigo.com/v1alpha1")
```

### Multi-Step Rendering (Simulating Observed State)

Many compositions behave differently once managed resources become ready. Simulate this by mocking
an observed state and re-rendering:

```go
// Mock a resource as ready, optionally overriding status or spec fields
mockedDeploy := crossplane.MockByKind(
    t, resources,
    "Deployment", "apps/v1",
    true,  // makeReady: adds Synced+Ready conditions when true
    map[string]interface{}{
        "status.readyReplicas":   float64(1),
        "status.replicas":        float64(1),
        "status.updatedReplicas": float64(1),
    },
)

// A resource with no interesting status or spec overrides — just mark it ready
mockedService := crossplane.MockByKind(t, resources, "Service", "v1", true, nil)

// Append mocked resources to the observed file
crossplane.AppendToResources(t, observed, mockedDeploy, mockedService)

// Re-render with observed state
resources = crossplane.CrossplaneRender(t, myResource, composition, functionsConfig,
    crossplane.Ptr(extra), crossplane.Ptr(observed))

crossplane.AssertResourceReady(t, resources, "WebApp", "workload.entigo.com/v1alpha1")
```

### Parsing and Mocking YAML Resource Files

Use `ParseYamlFileToUnstructured` when you need to select specific resources from a multi-document
YAML file before appending them to the extra or observed resource files:

```go
allInstances := crossplane.ParseYamlFileToUnstructured(t, instances)
for _, u := range allInstances {
    if u.GetName() == "my-specific-instance" {
        crossplane.AppendToResources(t, tempInstance, u)
    }
}
```

Use `MockByKind` to find and mock the first matching resource from a parsed slice:

```go
mskUnstructured := crossplane.ParseYamlFileToUnstructured(t, mskObserverResource)
mockedMsk := crossplane.MockByKind(t, mskUnstructured, "MSK", "kafka.entigo.com/v1alpha1", true, nil)
crossplane.AppendToResources(t, extra, mockedMsk)
```

Use `Mock` when you already hold a specific resource (e.g. inside a `range` loop) and don't need to search:

```go
for _, res := range resources {
    if res.GetKind() == "SecurityGroupRule" {
        crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
    }
}
```

### Complete Minimal Example

```go
package test

import (
    "path/filepath"
    "testing"

    "github.com/entigolabs/crossplane-common"
)

const (
    composition     = "../apis/myresource-composition.yaml"
    env             = "../examples/environment-config.yaml"
    function        = "../../../functions/myfunc"
    functionsConfig = "../../../test/common/functions-dev.yaml"
    myResourceYAML  = "../examples/myresource.yaml"
)

func TestMyResourceCrossplaneRender(t *testing.T) {
    crossplane.StartCustomFunction(t, function, "9443")

    tmpDir  := t.TempDir()
    extra   := filepath.Join(tmpDir, "extra.yaml")
    observed := filepath.Join(tmpDir, "observed.yaml")

    crossplane.AppendYamlToResources(t, env, extra)

    t.Log("Rendering...")
    resources := crossplane.CrossplaneRender(t, myResourceYAML, composition, functionsConfig,
        crossplane.Ptr(extra), nil)

    t.Log("Asserting counts")
    crossplane.AssertResourceCount(t, resources, "MyResource", 1)
    crossplane.AssertResourceCount(t, resources, "SomeManagedResource", 1)

    t.Log("Asserting fields")
    crossplane.AssertFieldValues(t, resources, "SomeManagedResource", "example.com/v1beta1", map[string]string{
        "metadata.name":          "my-resource",
        "spec.forProvider.region": "eu-north-1",
    })

    t.Log("Mocking observed state")
    crossplane.AppendToResources(t, observed,
        crossplane.MockByKind(t, resources, "SomeManagedResource", "example.com/v1beta1", true, nil))

    t.Log("Re-rendering with observed state")
    resources = crossplane.CrossplaneRender(t, myResourceYAML, composition, functionsConfig,
        crossplane.Ptr(extra), crossplane.Ptr(observed))

    t.Log("Asserting ready status")
    crossplane.AssertResourceReady(t, resources, "MyResource", "mygroup.entigo.com/v1alpha1")
}
```