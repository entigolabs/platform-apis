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

func TestZoneStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "tenancy"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/zone.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/zone-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-tenancy-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	requiredResources, err := xptest.LoadUnstructuredMulti("../examples/required-resources.yaml")
	if err != nil {
		t.Fatalf("cannot load required resources: %v", err)
	}
	extraResources := append([]unstructured.Unstructured{envConfig}, requiredResources...)

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering AppProject, MutatingPolicy, LaunchTemplate, Namespace resources")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "AppProject", 1, "MutatingPolicy", 4, "LaunchTemplate", 2, "Namespace", 2)

	t.Log("TEST 1: asserting Namespace fields")
	ns := xptest.FindResource(t, out1.ComposedResources, "Namespace", "abfe")
	if ns != nil {
		xptest.AssertNestedString(t, ns.Object, "testzone", "metadata", "labels", "tenancy.entigo.com/zone")
		xptest.AssertNestedString(t, ns.Object, "baseline", "metadata", "labels", "pod-security.kubernetes.io/enforce")
	}

	t.Log("TEST 1: asserting LaunchTemplate fields")
	lt := xptest.FindResource(t, out1.ComposedResources, "LaunchTemplate", "testzone-default")
	if lt != nil {
		xptest.AssertNestedString(t, lt.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, lt.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
	}

	t.Log("Mocking step 1 as observed and ready")
	observed := xptest.BuildObservedReady(t, out1.ComposedResources)

	t.Log("TEST 2: rendering NetworkPolicy, Role, ValidatingPolicy resources")
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
	xptest.AssertCounts(t, out2, "NetworkPolicy", 2, "Role", 1, "ValidatingPolicy", 2)

	t.Log("TEST 2: asserting NetworkPolicy fields")
	np := xptest.FindResource(t, out2.ComposedResources, "NetworkPolicy", "abfe-zone")
	if np != nil {
		xptest.AssertNestedString(t, np.Object, "abfe", "metadata", "namespace")
	}

	t.Log("TEST 2: asserting IAM Role fields")
	iamRole := xptest.FindResource(t, out2.ComposedResources, "Role", "testzone")
	if iamRole != nil {
		xptest.AssertNestedString(t, iamRole.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
	}

	t.Log("Mocking step 2 as observed and ready")
	observed = xptest.BuildObservedReady(t, out2.ComposedResources)

	t.Log("TEST 3: rendering AccessEntry, Role, RolePolicyAttachment resources")
	out3, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out3, "AccessEntry", 1, "Role", 5, "RolePolicyAttachment", 4)

	t.Log("TEST 3: asserting AccessEntry fields")
	ae := xptest.FindResource(t, out3.ComposedResources, "AccessEntry", "testzone")
	if ae != nil {
		xptest.AssertNestedString(t, ae.Object, "eks", "spec", "forProvider", "clusterNameRef", "name")
		xptest.AssertNestedString(t, ae.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, ae.Object, "EC2_LINUX", "spec", "forProvider", "type")
	}

	t.Log("TEST 3: asserting RolePolicyAttachment fields")
	rpa := xptest.FindResource(t, out3.ComposedResources, "RolePolicyAttachment", "testzone-wn")
	if rpa != nil {
		xptest.AssertNestedString(t, rpa.Object, "testzone", "spec", "forProvider", "roleRef", "name")
		xptest.AssertNestedString(t, rpa.Object, "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy", "spec", "forProvider", "policyArn")
	}

	t.Log("TEST 3: asserting RBAC Role fields")
	rbacRole := xptest.FindResource(t, out3.ComposedResources, "Role", "abfe-all")
	if rbacRole != nil {
		xptest.AssertNestedString(t, rbacRole.Object, "abfe", "metadata", "namespace")
	}

	t.Log("Mocking step 3 as observed and ready")
	observed = xptest.BuildObservedReady(t, out3.ComposedResources)

	t.Log("TEST 4: rendering RoleBinding resources")
	out4, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out4, "RoleBinding", 6)

	t.Log("TEST 4: asserting RoleBinding fields")
	rb := xptest.FindResource(t, out4.ComposedResources, "RoleBinding", "abfe-contributor")
	if rb != nil {
		xptest.AssertNestedString(t, rb.Object, "abfe", "metadata", "namespace")
		xptest.AssertNestedString(t, rb.Object, "abfe-all", "roleRef", "name")
	}

	t.Log("Mocking step 4 as observed and ready")
	observed = xptest.BuildObservedReady(t, out4.ComposedResources)

	t.Log("TEST 5: checking Zone readiness")
	out5, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertReady(t, out5.CompositeResource)
}
