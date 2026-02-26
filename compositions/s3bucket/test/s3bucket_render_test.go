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

func TestS3BucketStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "storage"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/s3bucket.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/s3bucket-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-storage-fn")

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

	t.Log("TEST 1: rendering step 1 resources (Bucket, User, Role)")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Bucket", 1, "User", 1, "Role", 1)

	t.Log("TEST 1: asserting Bucket fields")
	bucket := xptest.FindResource(t, out1.ComposedResources, "Bucket", "example-bucket")
	if bucket != nil {
		xptest.AssertNestedString(t, bucket.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, bucket.Object, "ClusterProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, bucket.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
		xptest.AssertNestedString(t, bucket.Object, "example-bucket", "spec", "forProvider", "tags", "Name")
		xptest.AssertNestedString(t, bucket.Object, "bar", "spec", "forProvider", "tags", "foo")
	}

	t.Log("TEST 1: asserting User fields")
	user := xptest.FindResource(t, out1.ComposedResources, "User", "example-bucket")
	if user != nil {
		xptest.AssertNestedString(t, user.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
	}

	t.Log("Mocking step 1 as observed and ready")
	observed := xptest.BuildObservedResources(t, out1.ComposedResources, func(kind, _ string) bool { return true })

	t.Log("TEST 2: rendering step 2 resources (AccessKey, Policy)")
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
	xptest.AssertCounts(t, out2, "AccessKey", 1, "Policy", 1)

	t.Log("Mocking step 2 as observed and ready")
	observed = append(observed, xptest.BuildObservedResources(t, out2.ComposedResources, func(kind, _ string) bool { return true })...)

	t.Log("TEST 3: rendering step 3 resources (Secret)")
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
	xptest.AssertCounts(t, out3, "Secret", 1)

	t.Log("TEST 3: asserting secrets-manager Secret fields")
	smSecret := xptest.FindResource(t, out3.ComposedResources, "Secret", "example-bucket-credentials")
	if smSecret != nil {
		xptest.AssertNestedString(t, smSecret.Object, "example-bucket-credentials", "spec", "forProvider", "name")
		xptest.AssertNestedString(t, smSecret.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedString(t, smSecret.Object, "arn:aws:kms:eu-north-1:012345678901:key/mrk-1", "spec", "forProvider", "kmsKeyId")
	}

	t.Log("Mocking step 3 as observed and ready")
	observed = append(observed, xptest.BuildObservedResources(t, out3.ComposedResources, func(kind, _ string) bool { return true })...)

	t.Log("TEST 4: rendering step 4 resources (SecretVersion, UserPolicyAttachment, RolePolicyAttachment)")
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
	xptest.AssertCounts(t, out4, "SecretVersion", 1, "UserPolicyAttachment", 1, "RolePolicyAttachment", 1)

	t.Log("Mocking step 4 as observed and ready")
	observed = append(observed, xptest.BuildObservedResources(t, out4.ComposedResources, func(kind, _ string) bool { return true })...)

	t.Log("TEST 5: rendering step 5 resources (BucketVersioning, BucketOwnershipControls)")
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
	xptest.AssertCounts(t, out5, "BucketVersioning", 1, "BucketOwnershipControls", 1)

	t.Log("TEST 5: asserting BucketVersioning fields")
	bv := xptest.FindResource(t, out5.ComposedResources, "BucketVersioning", "example-bucket")
	if bv != nil {
		xptest.AssertNestedString(t, bv.Object, "Suspended", "spec", "forProvider", "versioningConfiguration", "status")
	}

	t.Log("Mocking step 5 as observed and ready")
	observed = append(observed, xptest.BuildObservedResources(t, out5.ComposedResources, func(kind, _ string) bool { return true })...)

	t.Log("TEST 6: rendering step 6 resources (BucketPublicAccessBlock, BucketServerSideEncryptionConfiguration)")
	out6, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out6, "BucketPublicAccessBlock", 1, "BucketServerSideEncryptionConfiguration", 1)

	t.Log("TEST 6: asserting BucketPublicAccessBlock fields")
	bpab := xptest.FindResource(t, out6.ComposedResources, "BucketPublicAccessBlock", "example-bucket")
	if bpab != nil {
		xptest.AssertNestedBool(t, bpab.Object, true, "spec", "forProvider", "blockPublicAcls")
		xptest.AssertNestedBool(t, bpab.Object, true, "spec", "forProvider", "blockPublicPolicy")
		xptest.AssertNestedBool(t, bpab.Object, true, "spec", "forProvider", "ignorePublicAcls")
		xptest.AssertNestedBool(t, bpab.Object, true, "spec", "forProvider", "restrictPublicBuckets")
	}

	t.Log("TEST 6: asserting BucketServerSideEncryptionConfiguration fields")
	bsse := xptest.FindResource(t, out6.ComposedResources, "BucketServerSideEncryptionConfiguration", "example-bucket")
	if bsse != nil {
		assertSSERule(t, bsse.Object, "aws:kms", "arn:aws:kms:eu-north-1:012345678901:key/mrk-0")
	}

	t.Log("Mocking step 6 as observed and ready")
	observed = append(observed, xptest.BuildObservedResources(t, out6.ComposedResources, func(kind, _ string) bool { return true })...)

	t.Log("TEST 7: checking S3Bucket readiness")
	out7, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertReady(t, out7.CompositeResource)
}

func assertSSERule(t *testing.T, obj map[string]interface{}, expectedAlgo, expectedKMSKey string) {
	t.Helper()
	rules, _, _ := unstructured.NestedSlice(obj, "spec", "forProvider", "rule")
	if len(rules) == 0 {
		t.Error("spec.forProvider.rule: not found or empty")
		return
	}
	rule, ok := rules[0].(map[string]interface{})
	if !ok {
		t.Error("spec.forProvider.rule[0]: not a map")
		return
	}
	sseDef, ok := rule["applyServerSideEncryptionByDefault"].(map[string]interface{})
	if !ok {
		t.Error("spec.forProvider.rule[0].applyServerSideEncryptionByDefault: not found")
		return
	}
	if got, _ := sseDef["sseAlgorithm"].(string); got != expectedAlgo {
		t.Errorf("sseAlgorithm: expected %q, got %q", expectedAlgo, got)
	}
	if got, _ := sseDef["kmsMasterKeyId"].(string); got != expectedKMSKey {
		t.Errorf("kmsMasterKeyId: expected %q, got %q", expectedKMSKey, got)
	}
}
