package render_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane/v2/cmd/crank/render"
	xptest "github.com/entigolabs/platform-apis/test/common/crossplane"
	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestWebAppStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "workload"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/webapp.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/webapp-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-workload-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	extraResources := []unstructured.Unstructured{envConfig}

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Deployment, Service and Secret resources")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Deployment", 1, "Service", 1, "Secret", 1)

	t.Log("TEST 1: asserting Deployment fields")
	dep := xptest.FindResource(t, out1.ComposedResources, "Deployment", "new-web-app")
	if dep != nil {
		xptest.AssertNestedString(t, dep.Object, "new-web-app", "metadata", "labels", "app")
		xptest.AssertNestedString(t, dep.Object, "new-web-app", "spec", "selector", "matchLabels", "app")
	}

	t.Log("TEST 1: asserting Service fields")
	svc := xptest.FindResource(t, out1.ComposedResources, "Service", "new-web-app-service")
	if svc != nil {
		xptest.AssertNestedString(t, svc.Object, "new-web-app", "spec", "selector", "app")
	}

	t.Log("TEST 1: asserting Secret fields")
	sec := xptest.FindResource(t, out1.ComposedResources, "Secret", "new-web-app-nginx-secret")
	if sec != nil {
		xptest.AssertNestedString(t, sec.Object, "Opaque", "type")
	}

	t.Log("Mocking step 1 as observed and ready")
	observed := buildWebAppObserved(t, out1.ComposedResources)

	t.Log("TEST 2: checking WebApp readiness")
	out2, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertReady(t, out2.CompositeResource)
}

func buildWebAppObserved(t *testing.T, resources []xptest.ComposedUnstructured) []xptest.ComposedUnstructured {
	t.Helper()
	obs := xptest.BuildObservedResources(t, resources, func(kind, _ string) bool { return true })
	for i := range obs {
		if obs[i].GetKind() == "Deployment" {
			_ = unstructured.SetNestedField(obs[i].Object, float64(1), "status", "readyReplicas")
			_ = unstructured.SetNestedField(obs[i].Object, float64(1), "status", "replicas")
			_ = unstructured.SetNestedField(obs[i].Object, float64(1), "status", "updatedReplicas")
			_ = unstructured.SetNestedSlice(obs[i].Object, []interface{}{
				map[string]interface{}{"type": "Synced", "status": "True"},
				map[string]interface{}{"type": "Available", "status": "True"},
			}, "status", "conditions")
		}
	}
	return obs
}
