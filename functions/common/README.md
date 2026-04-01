# function-base

Shared base library for writing Entigo Platform Crossplane composition functions in Go.

It handles the boilerplate of the Crossplane function SDK — gRPC serving, required resource resolution, desired resource generation, status propagation, and tag/label injection. Individual function implementations only need to provide business logic.

## Packages

| Package | Import path                                | Purpose                                               |
|---------|--------------------------------------------|-------------------------------------------------------|
| `base`  | `github.com/entigolabs/function-base/base` | Core framework: `Function`, `GroupService`, utilities |
| `test`  | `github.com/entigolabs/function-base/test` | Test helpers for writing function integration tests   |

---

## How it works

The `Function` type (in `base/fn.go`) implements the Crossplane `FunctionRunnerService` gRPC interface. Each reconciliation cycle it:

1. Fetches the observed composite resource.
2. Calls `GroupService.SkipGeneration` — returns early if true.
3. Asks the implementation for required resources (`GetRequiredResources`), adds namespace/zone/env selectors automatically for namespaced composites, and sets `rsp.Requirements`.
4. Returns early if any declared required resources are not yet present in the request.
5. Calls the resource handler registered for the composite kind (`GetResourceHandlers`) to generate desired composed objects.
6. Applies tag and label injection (zone, workspace, `tags.entigo.com/` prefixed labels/annotations) onto every generated resource via `injectZone`.
7. Respects a deployment sequence (`GetSequence`) — resources in later steps are only created once earlier steps are ready.
8. Collects status from observed resources (`GetObservedStatus`) and writes it back to the composite.

---

## Writing a function

### 1. `main.go` — embed `base.CLI` and pass your implementation

```go
package main

import (
    "github.com/alecthomas/kong"
    "github.com/entigolabs/function-base/base"
)

type CLI struct{ base.CLI }

func (c *CLI) Run() error {
    return c.CLI.Run(&GroupImpl{})
}

func main() {
    cli := &CLI{}
    ctx := kong.Parse(cli, kong.Description("My Composition Function."))
    ctx.FatalIfErrorf(ctx.Run())
}
```

`base.CLI` wires up the gRPC server, logger, mTLS certificates, and `WORKSPACE` environment variable automatically.

### 2. Implement `GroupService`

Create a struct that satisfies the `base.GroupService` interface:

```go
type GroupImpl struct {
    log logging.Logger
}

var _ base.GroupService = &GroupImpl{}
```

#### `SetLogger`
Store the logger for use in other methods.

```go
func (g *GroupImpl) SetLogger(log logging.Logger) { g.log = log }
```

#### `SkipGeneration`
Return `true` to skip the entire reconciliation for a composite (e.g. during deletion).

```go
func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool { return false }
```

#### `GetResourceHandlers`
Return a map from composite kind to a `ResourceHandler`. Each handler provides:
- `Instantiate` — returns an empty typed object for the composite kind.
- `Generate` — converts the composite into a map of desired composed objects, keyed by resource name.

```go
func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
    return map[string]base.ResourceHandler{
        "Repository": {
            Instantiate: func() client.Object { return &v1alpha1.Repository{} },
            Generate:    g.generateRepository,
        },
    }
}

func (g *GroupImpl) generateRepository(obj client.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
    return service.GenerateRepository(*obj.(*v1alpha1.Repository), required)
}
```

#### `GetRequiredResources`
Tell the framework which additional resources to fetch before generation runs. Called twice per reconciliation — first with an empty `required` map, then again once those resources are available. Use that to chain dependencies:

```go
func (g *GroupImpl) GetRequiredResources(cr *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
    // Always fetch the environment config first.
    resources := map[string]*fnv1.ResourceSelector{
        base.EnvironmentKey: base.RequiredEnvironmentConfig("platform-apis-base"),
    }
    // Only request the KMS key once the environment config is available.
    if _, present := required[base.EnvironmentKey]; !present {
        return resources, nil
    }
    env, err := service.GetEnvironment(required)
    if err != nil {
        return nil, err
    }
    resources["KMSKey"] = base.RequiredKMSKey(env.KMSKeyName, env.AWSProvider)
    return resources, nil
}
```

For namespaced composites the framework automatically adds the `Namespace` required resource, and once the namespace is fetched it also adds the tenancy `Zone` and its `EnvironmentTags` config. You do not need to request these yourself.

#### `GetSequence`
Control the order in which composed resources are created. Resources not listed in any step are created immediately. Resources in step N are only created once all resources in step N-1 are ready.

Patterns can be literal names or regexes (set `Regex: true`):

```go
func (g *GroupImpl) GetSequence(_ client.Object) base.Sequence {
    // Create the database first, then the application.
    return base.NewSequence(false,
        []string{"database"},
        []string{"app-deployment", "app-service"},
    )
}
```

Return an empty `Sequence{}` if ordering does not matter.

#### `GetReadyStatus`
Override the ready status for a specific observed resource. Return `""` to fall back to the standard Crossplane condition check.

```go
func (g *GroupImpl) GetReadyStatus(_ *composed.Unstructured) resource.Ready { return "" }
```

#### `GetObservedStatus`
Extract fields from observed composed resources to write back onto the composite status.

```go
func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
    if observed.GetKind() != "Repository" {
        return nil, nil
    }
    uri, found, err := unstructured.NestedString(observed.Object, "status", "atProvider", "repositoryUrl")
    if err != nil || !found {
        return nil, nil
    }
    return map[string]interface{}{"repositoryUri": uri}, nil
}
```

---

## Tag and label injection

The framework automatically propagates tags and labels onto every generated resource that has a `spec.forProvider.tags` field. The priority order (highest wins):

1. **CR labels/annotations** — `tags.entigo.com/<Key>` labels and annotations on the composite resource. Annotations override labels for the same key in `spec.forProvider.tags`; labels are also propagated as Kubernetes labels.
2. **Zone labels/annotations** — same prefix on the tenancy Zone object.
3. **Zone environment config** — `EnvironmentTags` config named `platform-apis-zone`, fetched automatically when a zone is present.
4. **CR api-group environment config** — base tags set directly in `spec.forProvider.tags` by the `Generate` implementation (lowest priority, overridden by all of the above).

Additionally:
- The tenancy zone name is set as the `tenancy.entigo.com/zone` Kubernetes label and the `entigo:zone` AWS tag.
- The `WORKSPACE` environment variable is set as the `tenancy.entigo.com/workspace` label and the `entigo:workspace` AWS tag.
- AWS tag count is validated against the 44-tag limit after all injections are applied.

---

## Useful base utilities

| Function                                            | Description                                                                  |
|-----------------------------------------------------|------------------------------------------------------------------------------|
| `base.GetEnvironment(key, required, &obj)`          | Deserialise a required EnvironmentConfig into a typed struct and validate it |
| `base.ExtractRequiredResource(required, key, &obj)` | Deserialise the first required resource for a key into a typed object        |
| `base.ExtractResources[T](required, key)`           | Deserialise all required resources for a key into a typed slice              |
| `base.RequiredEnvironmentConfig(name)`              | Build a `ResourceSelector` for a named EnvironmentConfig                     |
| `base.RequiredKMSKey(name, namespace)`              | Build a `ResourceSelector` for a namespaced KMS Key                          |
| `base.RequiredNamespace(name)`                      | Build a `ResourceSelector` for a Namespace                                   |
| `base.GetTenancyZone(required, log)`                | Read the zone label from the fetched Namespace                               |
| `base.GenerateEligibleKubernetesName(s, limit)`     | Sanitise an arbitrary string into a valid Kubernetes name                    |
| `base.StringPtr`, `BoolPtr`, `Int32Ptr`, …          | Pointer helpers for scalar types                                             |

---

## Writing tests

Use `test.RunFunctionCases` to run table-driven integration tests against the full `RunFunction` pipeline:

```go
func TestMyFunction(t *testing.T) {
    cases := map[string]test.Case{
        "CreateObjects": {
            Reason: "Should generate the expected composed resources",
            Args: test.Args{
                Req: &fnv1.RunFunctionRequest{
                    Observed: &fnv1.State{
                        Composite: &fnv1.Resource{Resource: resource.MustStructJSON(compositeJSON)},
                    },
                    RequiredResources: map[string]*fnv1.Resources{
                        base.EnvironmentKey: test.EnvironmentConfigResourceWithData("env", envData),
                        base.NamespaceKey:   test.Namespace("ns", "zone-a"),
                    },
                },
            },
            Want: test.Want{
                Rsp: &fnv1.RunFunctionResponse{
                    Meta:    &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
                    Desired: &fnv1.State{Resources: map[string]*fnv1.Resource{ /* ... */ }},
                },
            },
        },
    }
    test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, cases)
}
```

### Test helpers (`test` package)

| Helper | Description |
|---|---|
| `test.EnvironmentConfigResourceWithData(name, data)` | Build a required EnvironmentConfig resource |
| `test.KMSKeyResource(name, namespace, arnSuffix)` | Build a required KMS Key resource |
| `test.Namespace(name, zone)` | Build a required Namespace resource with a zone label |
| `test.RunFunctionCases(t, serviceFn, cases, ignoredFields...)` | Run all cases through the full function pipeline |
| `test.RequiredResource(data)` | Build a `resource.Required` with an arbitrary `data` field |
| `test.RequiredNamespace(labels)` | Build a `resource.Required` for a Namespace with given labels |
| `test.RequiredZoneObject(labels, annotations)` | Build a `resource.Required` for a Zone object |
| `test.RequiredEnvTags(tags)` | Build a `resource.Required` from a `map[string]string` of tags |
| `test.CompositeResource(name, namespace, labels, annotations)` | Build a `*resource.Composite` |

---

## Running tests

```shell
# From the functions/common directory
go test ./...
```
