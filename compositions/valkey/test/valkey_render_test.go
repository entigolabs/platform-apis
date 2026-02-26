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

func TestValkeyStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "database"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "test-input.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/valkey-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-database-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	required, err := xptest.LoadUnstructuredMulti("../examples/required-resources.yaml")
	if err != nil {
		t.Fatalf("cannot load required resources: %v", err)
	}
	extraResources := append([]unstructured.Unstructured{envConfig}, required...)

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering step 1 resources (SecurityGroup)")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "SecurityGroup", 1)

	t.Log("TEST 1: asserting SecurityGroup fields")
	sg := xptest.FindResource(t, out1.ComposedResources, "SecurityGroup", "example-valkey-with-custom-settings")
	if sg != nil {
		xptest.AssertNestedString(t, sg.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, sg.Object, "ClusterProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, sg.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
	}

	t.Log("Mocking step 1 as observed and ready")
	observed := xptest.BuildObservedResources(t, out1.ComposedResources, func(kind, _ string) bool { return true })

	t.Log("TEST 2: rendering step 2 resources (ReplicationGroup)")
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
	xptest.AssertCounts(t, out2, "ReplicationGroup", 1)

	t.Log("TEST 2: asserting ReplicationGroup fields")
	rg := xptest.FindResource(t, out2.ComposedResources, "ReplicationGroup", "example-valkey-with-custom-settings")
	if rg != nil {
		xptest.AssertNestedString(t, rg.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, rg.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
		xptest.AssertNestedString(t, rg.Object, "arn:aws:kms:eu-north-1:123456789012:key/data-key-uuid", "spec", "forProvider", "kmsKeyId")
		xptest.AssertNestedString(t, rg.Object, "valkey", "spec", "forProvider", "engine")
		xptest.AssertNestedString(t, rg.Object, "8.2", "spec", "forProvider", "engineVersion")
	}

	t.Log("Mocking step 2 as observed and ready (with endpoint/port for readiness check)")
	observed = append(observed, buildValkeyObserved(t, out2.ComposedResources)...)

	t.Log("TEST 3: rendering step 3 resources (SecurityGroupRule)")
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
	xptest.AssertCounts(t, out3, "SecurityGroupRule", 1)

	t.Log("TEST 3: asserting SecurityGroupRule fields")
	sgr := xptest.FindResource(t, out3.ComposedResources, "SecurityGroupRule", "example-valkey-with-custom-settings-ingress-compute-subnet")
	if sgr != nil {
		xptest.AssertNestedString(t, sgr.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, sgr.Object, "ingress", "spec", "forProvider", "type")
		xptest.AssertNestedString(t, sgr.Object, "tcp", "spec", "forProvider", "protocol")
	}

	t.Log("Mocking step 3 as observed and ready")
	observed = append(observed, buildValkeyObserved(t, out3.ComposedResources)...)

	t.Log("TEST 4: rendering step 4 resources (Secret: secrets-manager-secret)")
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
	xptest.AssertCounts(t, out4, "Secret", 1)

	t.Log("TEST 4: asserting SM Secret fields")
	smSecret := xptest.FindResource(t, out4.ComposedResources, "Secret", "example-valkey-with-custom-settings-credentials")
	if smSecret != nil {
		xptest.AssertNestedString(t, smSecret.Object, "example-valkey-with-custom-settings-credentials", "spec", "forProvider", "name")
		xptest.AssertNestedString(t, smSecret.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, smSecret.Object, "arn:aws:kms:eu-north-1:123456789012:key/config-key-uuid", "spec", "forProvider", "kmsKeyId")
	}

	t.Log("Mocking step 4 as observed and ready")
	observed = append(observed, buildValkeyObserved(t, out4.ComposedResources)...)

	t.Log("TEST 5: checking ValkeyInstance readiness")
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

// buildValkeyObserved builds observed resources with Ready/Synced conditions, and additionally
// sets primaryEndpointAddress and port on any ReplicationGroup (required by GetValkeyReplicationGroupReadyStatus).
func buildValkeyObserved(t *testing.T, resources []xptest.ComposedUnstructured) []xptest.ComposedUnstructured {
	t.Helper()
	obs := xptest.BuildObservedResources(t, resources, func(kind, _ string) bool { return true })
	for i := range obs {
		if obs[i].GetKind() == "ReplicationGroup" {
			_ = unstructured.SetNestedField(obs[i].Object, "primary.endpoint.example.com", "status", "atProvider", "primaryEndpointAddress")
			_ = unstructured.SetNestedField(obs[i].Object, float64(6379), "status", "atProvider", "port")
		}
	}
	return obs
}
