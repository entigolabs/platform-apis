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

func TestRepositoryStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "artifact"), "9443")

	fs := afero.NewOsFs()

	// Load definition, composition, function, env
	xr, err := render.LoadCompositeResource(fs, "../examples/repository.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/repository-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-artifact-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	kmsKey, err := xptest.LoadUnstructured("../examples/required-resources.yaml")
	if err != nil {
		t.Fatalf("cannot load required resources: %v", err)
	}
	extraResources := []unstructured.Unstructured{envConfig, kmsKey}

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Repository")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Repository", 2)

	t.Log("TEST 1: asserting Repository fields")
	repo := xptest.FindResource(t, out1.ComposedResources, "Repository", "repository-example")
	if repo != nil {
		xptest.AssertNestedString(t, repo.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, repo.Object, "ClusterProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, repo.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
		assertEncryptionType(t, repo.Object, "KMS")
		assertKMSKey(t, repo.Object, "arn:aws:kms:eu-north-1:012345678901:key/mrk-0")
	}

	t.Log("Mocking Repository as observed and ready")
	observed := xptest.BuildObservedResources(t, out1.ComposedResources, func(kind, _ string) bool {
		return kind == "Repository"
	})

	t.Log("TEST 2: checking Repository readiness")
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

func assertEncryptionType(t *testing.T, obj map[string]interface{}, expected string) {
	t.Helper()
	configs, _, _ := unstructured.NestedSlice(obj, "spec", "forProvider", "encryptionConfiguration")
	if len(configs) == 0 {
		t.Error("spec.forProvider.encryptionConfiguration: not found or empty")
		return
	}
	config, ok := configs[0].(map[string]interface{})
	if !ok {
		t.Error("spec.forProvider.encryptionConfiguration[0]: not a map")
		return
	}
	got, _ := config["encryptionType"].(string)
	if got != expected {
		t.Errorf("encryptionConfiguration[0].encryptionType: expected %q, got %q", expected, got)
	}
}

func assertKMSKey(t *testing.T, obj map[string]interface{}, expected string) {
	t.Helper()
	configs, _, _ := unstructured.NestedSlice(obj, "spec", "forProvider", "encryptionConfiguration")
	if len(configs) == 0 {
		t.Error("spec.forProvider.encryptionConfiguration: not found or empty")
		return
	}
	config, ok := configs[0].(map[string]interface{})
	if !ok {
		t.Error("spec.forProvider.encryptionConfiguration[0]: not a map")
		return
	}
	got, _ := config["kmsKey"].(string)
	if got != expected {
		t.Errorf("encryptionConfiguration[0].kmsKey: expected %q, got %q", expected, got)
	}
}
