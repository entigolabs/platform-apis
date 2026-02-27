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

func TestWebAccessStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "networking"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/webaccess.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/webaccess-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-networking-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	extraResources := []unstructured.Unstructured{envConfig}

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering VirtualService, ServiceEntry, DestinationRule resources")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "VirtualService", 1, "ServiceEntry", 2, "DestinationRule", 2)

	t.Log("TEST 1: asserting VirtualService fields")
	vs := xptest.FindResource(t, out1.ComposedResources, "VirtualService", "new-web-access")
	if vs != nil {
		assertStringSliceContains(t, vs.Object, "istio-gateway/istio-gateway", "spec", "gateways")
	}

	t.Log("TEST 1: asserting DestinationRule fields")
	dr1 := xptest.FindResource(t, out1.ComposedResources, "DestinationRule", "new-web-access-service1-test-svc-cluster-local-dr")
	if dr1 != nil {
		xptest.AssertNestedString(t, dr1.Object, "service1.test.svc.cluster.local", "spec", "host")
	}
	dr2 := xptest.FindResource(t, out1.ComposedResources, "DestinationRule", "new-web-access-service2-default-svc-cluster-local-dr")
	if dr2 != nil {
		xptest.AssertNestedString(t, dr2.Object, "service2.default.svc.cluster.local", "spec", "host")
	}

	t.Log("Mocking step 1 as observed and ready")
	observed := xptest.BuildObservedReady(t, out1.ComposedResources)

	t.Log("TEST 2: checking WebAccess readiness")
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

func assertStringSliceContains(t *testing.T, obj map[string]interface{}, expected string, fields ...string) {
	t.Helper()
	items, _, err := unstructured.NestedStringSlice(obj, fields...)
	if err != nil {
		t.Errorf("field %v: error: %v", fields, err)
		return
	}
	for _, item := range items {
		if item == expected {
			return
		}
	}
	t.Errorf("field %v: expected to contain %q, got %v", fields, expected, items)
}
