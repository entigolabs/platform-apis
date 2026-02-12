package main

import (
	"context"
	"fmt"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
	"github.com/entigolabs/platform-apis/service"
	"google.golang.org/protobuf/types/known/durationpb"
)

const (
	requiredEKSJson = `{
		"apiVersion": "eks.aws.m.upbound.io/v1beta1", "kind": "Cluster",
		"metadata": {"name": "eks"},
		"status": {"atProvider": {
			"region": "eu-north-1",
			"arn": "arn:aws:eks:eu-north-1:111111111111:cluster/eks",
			"identity": [{"oidc": [{"issuer": "https://oidc.eks.eu-north-1.amazonaws.com/id/ABCDEF1234567890"}]}]
		}}
	}`
	requiredKMSDataKeyJson = `{
		"apiVersion": "kms.aws.m.upbound.io/v1beta1", "kind": "Key",
		"metadata": {"name": "data"},
		"status": {"atProvider": {"arn": "arn:aws:kms:eu-north-1:111111111111:key/mrk-data123"}}
	}`
	requiredKMSConfigKeyJson = `{
		"apiVersion": "kms.aws.m.upbound.io/v1beta1", "kind": "Key",
		"metadata": {"name": "config"},
		"status": {"atProvider": {"arn": "arn:aws:kms:eu-north-1:111111111111:key/mrk-config456"}}
	}`
	requiredNamespaceJson = `{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": {"name": "default", "labels": {"tenancy.entigo.com/zone": "zone-a"}}
	}`
)

const s3bucketJson = `{
	"apiVersion": "storage.entigo.com/v1alpha1",
	"kind": "S3Bucket",
	"metadata": {"name":"test-bucket","namespace":"default"},
	"spec": {
		"enableVersioning": false,
		"createServiceAccount": true,
		"crossplane": {
			"compositionRef": {
				"name": "s3buckets.storage.entigo.com"
			}
		}
	}
}`

const s3bucketCustomSAJson = `{
	"apiVersion": "storage.entigo.com/v1alpha1",
	"kind": "S3Bucket",
	"metadata": {"name":"test-bucket","namespace":"default"},
	"spec": {
		"createServiceAccount": true,
		"serviceAccountName": "my-custom-sa",
		"crossplane": {
			"compositionRef": {
				"name": "s3buckets.storage.entigo.com"
			}
		}
	}
}`

const s3bucketNoSAJson = `{
	"apiVersion": "storage.entigo.com/v1alpha1",
	"kind": "S3Bucket",
	"metadata": {"name":"test-bucket","namespace":"default"},
	"spec": {
		"enableVersioning": true,
		"createServiceAccount": false,
		"crossplane": {
			"compositionRef": {
				"name": "s3buckets.storage.entigo.com"
			}
		}
	}
}`

func allRequiredResources() map[string]*fnv1.Resources {
	return map[string]*fnv1.Resources{
		base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, map[string]interface{}{
			"awsProvider":  "aws-provider",
			"dataKMSKey":   "data",
			"configKMSKey": "config",
		}),
		service.EKSKey:       {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredEKSJson)}}},
		service.KMSDataKey:   {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSDataKeyJson)}}},
		service.KMSConfigKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSConfigKeyJson)}}},
		service.NamespaceKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredNamespaceJson)}}},
	}
}

func observedReadyResource(apiVersion, kind, name string) *fnv1.Resource {
	return &fnv1.Resource{Resource: resource.MustStructJSON(fmt.Sprintf(`{
		"apiVersion": %q, "kind": %q,
		"metadata": {"name": %q},
		"status": {"conditions": [{"type": "Ready", "status": "True"}]}
	}`, apiVersion, kind, name))}
}

func observedReadyResourceWithConnectionDetails(apiVersion, kind, name string, details map[string][]byte) *fnv1.Resource {
	res := observedReadyResource(apiVersion, kind, name)
	res.ConnectionDetails = details
	return res
}

func allObservedComposedResources() map[string]*fnv1.Resource {
	return map[string]*fnv1.Resource{
		"bucket": observedReadyResourceWithConnectionDetails(
			"s3.aws.m.upbound.io/v1beta1", "Bucket", "test-bucket",
			map[string][]byte{
				"region": []byte("eu-north-1"),
				"arn":    []byte("arn:aws:s3:::test-bucket"),
				"id":     []byte("test-bucket"),
			},
		),
		"bucket-public-access-block":                  observedReadyResource("s3.aws.m.upbound.io/v1beta1", "BucketPublicAccessBlock", "test-bucket"),
		"bucket-server-side-encryption-configuration": observedReadyResource("s3.aws.m.upbound.io/v1beta1", "BucketServerSideEncryptionConfiguration", "test-bucket"),
		"bucket-versioning":                           observedReadyResource("s3.aws.m.upbound.io/v1beta1", "BucketVersioning", "test-bucket"),
		"bucket-ownership-controls":                   observedReadyResource("s3.aws.m.upbound.io/v1beta1", "BucketOwnershipControls", "test-bucket"),
		"iam-user":                                    observedReadyResource("iam.aws.m.upbound.io/v1beta1", "User", "test-bucket"),
		"iam-policy":                                  observedReadyResource("iam.aws.m.upbound.io/v1beta1", "Policy", "test-bucket"),
		"iam-user-policy-attachment":                  observedReadyResource("iam.aws.m.upbound.io/v1beta1", "UserPolicyAttachment", "test-bucket"),
		"iam-access-key": observedReadyResourceWithConnectionDetails(
			"iam.aws.m.upbound.io/v1beta1", "AccessKey", "test-bucket",
			map[string][]byte{
				"username": []byte("AKIAIOSFODNN7EXAMPLE"),
				"password": []byte("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"),
			},
		),
		"iam-role":                       observedReadyResource("iam.aws.m.upbound.io/v1beta1", "Role", "test-bucket"),
		"iam-role-policy-attachment":     observedReadyResource("iam.aws.m.upbound.io/v1beta1", "RolePolicyAttachment", "test-bucket"),
		"service-account":                observedReadyResource("v1", "ServiceAccount", "test-bucket"),
		"secrets-manager-secret":         observedReadyResource("secretsmanager.aws.m.upbound.io/v1beta1", "Secret", "test-bucket-credentials"),
		"secrets-manager-secret-version": observedReadyResource("secretsmanager.aws.m.upbound.io/v1beta1", "SecretVersion", "test-bucket-credentials"),
	}
}

func TestS3BucketPhaseOne(t *testing.T) {
	xr := resource.MustStructJSON(s3bucketJson)

	cases := map[string]test.Case{
		"RequestEnvironment": {
			Reason: "Should only request EnvironmentConfig when none present",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: xr},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
						},
					},
				},
			},
		},
	}
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, cases)
}

func TestS3BucketPhaseTwo(t *testing.T) {
	xr := resource.MustStructJSON(s3bucketJson)
	environmentData := map[string]interface{}{
		"awsProvider":  "aws-provider",
		"dataKMSKey":   "data",
		"configKMSKey": "config",
	}
	awsProvider := environmentData["awsProvider"].(string)

	cases := map[string]test.Case{
		"RequestAllResources": {
			Reason: "Should request all required resources when environment is present",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: xr},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
							service.EKSKey: {
								Kind:       "Cluster",
								ApiVersion: "eks.aws.m.upbound.io/v1beta1",
								Match:      &fnv1.ResourceSelector_MatchName{MatchName: "eks"},
								Namespace:  &awsProvider,
							},
							service.KMSDataKey:   base.RequiredKMSKey(environmentData["dataKMSKey"].(string), awsProvider),
							service.KMSConfigKey: base.RequiredKMSKey(environmentData["configKMSKey"].(string), awsProvider),
							service.NamespaceKey: {
								Kind:       "Namespace",
								ApiVersion: "v1",
								Match:      &fnv1.ResourceSelector_MatchName{MatchName: "default"},
							},
						},
					},
				},
			},
		},
	}
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, cases)
}

func TestS3BucketFullGeneration(t *testing.T) {
	xr := resource.MustStructJSON(s3bucketJson)

	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
			Resources: allObservedComposedResources(),
		},
		RequiredResources: allRequiredResources(),
	}

	f := base.NewFunction(logging.NewNopLogger(), &GroupImpl{})
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
		t.Fatalf("Response failure: %v", rsp.GetResults()[0].GetMessage())
	}

	desired := rsp.GetDesired().GetResources()

	expectedResources := []string{
		"bucket", "bucket-public-access-block", "bucket-server-side-encryption-configuration",
		"bucket-versioning", "bucket-ownership-controls",
		"iam-user", "iam-policy", "iam-user-policy-attachment", "iam-access-key",
		"iam-role", "iam-role-policy-attachment",
		"service-account",
		"secrets-manager-secret", "secrets-manager-secret-version",
		"credentials",
	}

	if len(desired) != len(expectedResources) {
		t.Errorf("Expected %d desired resources, got %d", len(expectedResources), len(desired))
		for name := range desired {
			t.Logf("  Got resource: %s", name)
		}
	}

	for _, name := range expectedResources {
		if _, ok := desired[name]; !ok {
			t.Errorf("Missing expected resource: %s", name)
		}
	}
}

func TestS3BucketNoServiceAccount(t *testing.T) {
	xr := resource.MustStructJSON(s3bucketNoSAJson)

	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
			Resources: allObservedComposedResources(),
		},
		RequiredResources: allRequiredResources(),
	}

	f := base.NewFunction(logging.NewNopLogger(), &GroupImpl{})
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
		t.Fatalf("Response failure: %v", rsp.GetResults()[0].GetMessage())
	}

	desired := rsp.GetDesired().GetResources()

	// Should NOT have service-account when createServiceAccount=false
	if _, ok := desired["service-account"]; ok {
		t.Error("service-account should not be created when createServiceAccount=false")
	}

	// Should still have credentials
	if _, ok := desired["credentials"]; !ok {
		t.Error("credentials secret should still be created")
	}

	// Should have 14 resources (15 minus service-account)
	if len(desired) != 14 {
		t.Errorf("Expected 14 desired resources, got %d", len(desired))
		for name := range desired {
			t.Logf("  Got resource: %s", name)
		}
	}
}

func TestS3BucketNoCredentialsWithoutObserved(t *testing.T) {
	xr := resource.MustStructJSON(s3bucketJson)

	// No observed composed resources - only composite
	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
		},
		RequiredResources: allRequiredResources(),
	}

	f := base.NewFunction(logging.NewNopLogger(), &GroupImpl{})
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
		t.Fatalf("Response failure: %v", rsp.GetResults()[0].GetMessage())
	}

	desired := rsp.GetDesired().GetResources()

	// credentials should NOT exist since no observed connection details
	if _, ok := desired["credentials"]; ok {
		t.Error("credentials should not be created without observed connection details")
	}

	// Step 1 resources + service-account should be present (first reconciliation)
	for _, name := range []string{"bucket", "iam-user", "iam-role", "service-account"} {
		if _, ok := desired[name]; !ok {
			t.Errorf("Missing expected resource: %s", name)
		}
	}
}

func TestS3BucketCustomServiceAccountName(t *testing.T) {
	xr := resource.MustStructJSON(s3bucketCustomSAJson)

	req := &fnv1.RunFunctionRequest{
		Observed: &fnv1.State{
			Composite: &fnv1.Resource{Resource: xr},
			Resources: allObservedComposedResources(),
		},
		RequiredResources: allRequiredResources(),
	}

	f := base.NewFunction(logging.NewNopLogger(), &GroupImpl{})
	rsp, err := f.RunFunction(context.Background(), req)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(rsp.GetResults()) > 0 && rsp.GetResults()[0].GetSeverity() == fnv1.Severity_SEVERITY_FATAL {
		t.Fatalf("Response failure: %v", rsp.GetResults()[0].GetMessage())
	}

	desired := rsp.GetDesired().GetResources()

	// Service account should use custom name
	sa, ok := desired["service-account"]
	if !ok {
		t.Fatal("Missing service-account resource")
	}
	saName := sa.GetResource().GetFields()["metadata"].GetStructValue().GetFields()["name"].GetStringValue()
	if saName != "my-custom-sa" {
		t.Errorf("Expected service account name 'my-custom-sa', got '%s'", saName)
	}

	// Bucket annotation should reference the custom SA name
	bucket, ok := desired["bucket"]
	if !ok {
		t.Fatal("Missing bucket resource")
	}
	annotations := bucket.GetResource().GetFields()["metadata"].GetStructValue().GetFields()["annotations"].GetStructValue().GetFields()
	if annotations[service.AnnotationServiceAccount].GetStringValue() != "my-custom-sa" {
		t.Errorf("Expected bucket annotation %s='my-custom-sa', got '%s'",
			service.AnnotationServiceAccount, annotations[service.AnnotationServiceAccount].GetStringValue())
	}
}
