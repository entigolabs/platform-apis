package crossplane

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/unstructured/composed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource/unstructured/composite"
	apiv1 "github.com/crossplane/crossplane/v2/apis/apiextensions/v1"
	pkgv1 "github.com/crossplane/crossplane/v2/apis/pkg/v1"
	"github.com/crossplane/crossplane/v2/cmd/crank/render"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

type ComposedUnstructured = composed.Unstructured

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

func AssertNestedBool(t *testing.T, obj map[string]interface{}, expected bool, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedBool(obj, fields...)
	if err != nil {
		t.Errorf("field %v: error: %v", fields, err)
		return
	}
	if !found {
		t.Errorf("field %v: not found", fields)
		return
	}
	if got != expected {
		t.Errorf("field %v: expected %v, got %v", fields, expected, got)
	}
}

func AssertNestedString(t *testing.T, obj map[string]interface{}, expected string, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedString(obj, fields...)
	if err != nil {
		t.Errorf("field %v: error: %v", fields, err)
		return
	}
	if !found {
		t.Errorf("field %v: not found", fields)
		return
	}
	if got != expected {
		t.Errorf("field %v: expected %q, got %q", fields, expected, got)
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

func BuildObservedReady(t *testing.T, resources []composed.Unstructured) []composed.Unstructured {
	t.Helper()
	return BuildObservedResources(t, resources, func(_, _ string) bool { return true })
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

func DockerFunctionsFromHelm(t *testing.T, valuesPath string, names ...string) []pkgv1.Function {
	t.Helper()
	data, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("cannot read %s: %v", valuesPath, err)
	}
	var values map[string]interface{}
	if err := yaml.Unmarshal(data, &values); err != nil {
		t.Fatalf("cannot parse %s: %v", valuesPath, err)
	}
	functions, _ := values["functions"].(map[string]interface{})
	fns := make([]pkgv1.Function, 0, len(names))
	for _, name := range names {
		entry, ok := functions[name].(map[string]interface{})
		if !ok {
			t.Fatalf("function %q not found in %s .functions", name, valuesPath)
		}
		image, _ := entry["image"].(string)
		tag, _ := entry["tag"].(string)
		if image == "" || tag == "" {
			t.Fatalf("function %q in %s missing image or tag", name, valuesPath)
		}
		fn := pkgv1.Function{}
		fn.SetName(name)
		fn.SetAnnotations(map[string]string{"render.crossplane.io/runtime": "Docker"})
		fn.Spec.Package = image + ":" + tag
		fns = append(fns, fn)
	}
	return fns
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

func FindResourceByKind(t *testing.T, resources []composed.Unstructured, kind string) *composed.Unstructured {
	t.Helper()
	for i := range resources {
		if resources[i].GetKind() == kind {
			return &resources[i]
		}
	}
	t.Errorf("resource Kind=%s not found", kind)
	return nil
}

func HelmValues() string {
	return filepath.Join(WorkspaceRoot(), "helm", "values.yaml")
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

func LoadUnstructuredMulti(path string) ([]unstructured.Unstructured, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var result []unstructured.Unstructured
	for {
		var obj map[string]interface{}
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if obj != nil {
			result = append(result, unstructured.Unstructured{Object: obj})
		}
	}
	return result, nil
}

func RemoveCompositionStep(comp *apiv1.Composition, stepName string) {
	filtered := make([]apiv1.PipelineStep, 0, len(comp.Spec.Pipeline))
	for _, step := range comp.Spec.Pipeline {
		if step.Step != stepName {
			filtered = append(filtered, step)
		}
	}
	comp.Spec.Pipeline = filtered
}

func StartFunction(ctx context.Context, t *testing.T, funcDir, port string) {
	t.Helper()
	cmd := exec.CommandContext(ctx, "go", "run", ".", "--insecure", "--debug")
	cmd.Dir = funcDir

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("cannot start function in %s: %v", funcDir, err)
	}
	t.Cleanup(func() {
		// Kill the entire process group (go run + the function binary child).
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})
	waitForPort(t, port, 5*time.Minute)
}

func WorkspaceRoot() string {
	if ws := os.Getenv("WORKSPACE"); ws != "" {
		return ws
	}
	return "/workspace"
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

func waitForPort(t *testing.T, port string, timeout time.Duration) {
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
