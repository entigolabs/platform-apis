package crossplane

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/unstructured/composed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/unstructured/composite"
	pkgv1 "github.com/crossplane/crossplane/v2/apis/pkg/v1"
	"github.com/crossplane/crossplane/v2/cmd/crank/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func WorkspaceRoot() string {
	if ws := os.Getenv("WORKSPACE"); ws != "" {
		return ws
	}
	return "/workspace"
}

func LoadUnstructured(path string) (unstructured.Unstructured, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return unstructured.Unstructured{}, err
	}
	var obj map[string]interface{}
	if err := yaml.Unmarshal(data, &obj); err != nil {
		return unstructured.Unstructured{}, err
	}
	return unstructured.Unstructured{Object: obj}, nil
}

func DevFunctions(names ...string) []pkgv1.Function {
	fns := make([]pkgv1.Function, len(names))
	for i, name := range names {
		fn := pkgv1.Function{}
		fn.SetName(name)
		fn.SetAnnotations(map[string]string{
			"render.crossplane.io/runtime": "Development",
		})
		fns[i] = fn
	}
	return fns
}

func StartFunction(ctx context.Context, t *testing.T, funcDir, port string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", "run", ".", "--insecure", "--debug")
	cmd.Dir = funcDir
	if err := cmd.Start(); err != nil {
		t.Fatalf("cannot start function in %s: %v", funcDir, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	WaitForPort(t, port, 60*time.Second)
}

func WaitForPort(t *testing.T, port string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "localhost:"+port, time.Second)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("timeout waiting for port %s", port)
}

func AssertCounts(t *testing.T, out render.Outputs, kindCounts ...interface{}) {
	t.Helper()
	counts := make(map[string]int)
	if out.CompositeResource != nil {
		counts[out.CompositeResource.GetKind()]++
	}
	for _, r := range out.ComposedResources {
		counts[r.GetKind()]++
	}
	for i := 0; i+1 < len(kindCounts); i += 2 {
		kind := kindCounts[i].(string)
		expected := kindCounts[i+1].(int)
		actual := counts[kind]
		if actual != expected {
			t.Errorf("Kind %s: expected count %d, got %d", kind, expected, actual)
		} else {
			t.Logf("SUCCESS: %s count: %d/%d", kind, actual, expected)
		}
	}
}

func AssertReady(t *testing.T, xr *composite.Unstructured) {
	t.Helper()
	if xr == nil {
		t.Error("composite resource is nil")
		return
	}
	conditions, _, _ := unstructured.NestedSlice(xr.Object, "status", "conditions")
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" && cond["status"] == "True" {
			t.Logf("SUCCESS: %s is Ready", xr.GetKind())
			return
		}
	}
	t.Errorf("FAIL: %s is NOT Ready. Conditions: %v", xr.GetKind(), conditions)
}

func BuildObservedResources(t *testing.T, resources []composed.Unstructured, isReady func(kind, apiVersion string) bool) []composed.Unstructured {
	t.Helper()
	readyConditions := []interface{}{
		map[string]interface{}{"type": "Synced", "status": "True"},
		map[string]interface{}{"type": "Ready", "status": "True"},
	}
	var observed []composed.Unstructured
	for _, r := range resources {
		clone := CloneComposed(t, r)
		if isReady(r.GetKind(), r.GetAPIVersion()) {
			status, _ := clone.Object["status"].(map[string]interface{})
			if status == nil {
				status = make(map[string]interface{})
			}
			status["conditions"] = readyConditions
			clone.Object["status"] = status
		}
		observed = append(observed, clone)
	}
	return observed
}

func FindResource(t *testing.T, resources []composed.Unstructured, kind, name string) *composed.Unstructured {
	t.Helper()
	for i := range resources {
		if resources[i].GetKind() == kind && resources[i].GetName() == name {
			return &resources[i]
		}
	}
	t.Errorf("resource Kind=%s Name=%s not found", kind, name)
	return nil
}

func CloneComposed(t *testing.T, r composed.Unstructured) composed.Unstructured {
	t.Helper()
	data, err := json.Marshal(r.Object)
	if err != nil {
		t.Fatalf("cannot marshal composed resource: %v", err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("cannot unmarshal composed resource: %v", err)
	}
	clone := composed.New()
	clone.Object = obj
	return *clone
}
