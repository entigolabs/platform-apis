package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/crossplane-common"
)

const (
	composition     = "../apis/s3bucket-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/storage"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"
	s3Bucket        = "../examples/s3bucket.yaml"
)

func TestS3BucketCrossplaneRender(t *testing.T) {
	t.Logf("Starting storage function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "S3Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)

	t.Log("Validating storage.entigo.com S3Bucket fields")
	crossplane.AssertFieldValues(t, resources, "S3Bucket", "storage.entigo.com/v1alpha1", map[string]string{
		"metadata.name": "example-bucket",
	})

	t.Log("Validating s3.aws.m.upbound.io Bucket fields")
	crossplane.AssertFieldValues(t, resources, "Bucket", "s3.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.annotations.storage\\.entigo\\.com/service-account-name": "example-bucket",
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.region":               "eu-north-1",
		"spec.writeConnectionSecretToRef.name":  "example-bucket-bucket",
	})

	t.Log("Validating iam.aws.m.upbound.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "iam.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.assumeRolePolicy":     "*",
		"spec.forProvider.tags.Name":            "example-bucket",
	})

	t.Log("Validating iam.aws.m.upbound.io User fields")
	crossplane.AssertFieldValues(t, resources, "User", "iam.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.tags.Name":            "example-bucket",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed,
		crossplane.MockResource(t, resources, "Bucket", "s3.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "User", "iam.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "Role", "iam.aws.m.upbound.io/v1beta1", true, nil),
	)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "S3Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "AccessKey", 1)
	crossplane.AssertResourceCount(t, resources, "Policy", 1)

	t.Log("Validating iam.aws.m.upbound.io AccessKey fields")
	crossplane.AssertFieldValues(t, resources, "AccessKey", "iam.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.userRef.name":         "example-bucket",
		"spec.writeConnectionSecretToRef.name":  "example-bucket-access-key",
	})

	t.Log("Validating iam.aws.m.upbound.io Policy fields")
	crossplane.AssertFieldValues(t, resources, "Policy", "iam.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.policy":               "*",
		"spec.forProvider.tags.Name":            "example-bucket",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed,
		crossplane.MockResource(t, resources, "AccessKey", "iam.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "Policy", "iam.aws.m.upbound.io/v1beta1", true, nil),
	)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "S3Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "AccessKey", 1)
	crossplane.AssertResourceCount(t, resources, "Policy", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)

	t.Log("secretsmanager.aws.m.upbound.io Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket-credentials",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.name":                 "example-bucket-credentials",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.kmsKeyId":             "arn:aws:kms:eu-north-1:012345678901:key/mrk-1",
		"spec.providerConfigRef.name":           "crossplane-aws",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed,
		crossplane.MockResource(t, resources, "Secret", "secretsmanager.aws.m.upbound.io/v1beta1", true, nil),
	)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "S3Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "AccessKey", 1)
	crossplane.AssertResourceCount(t, resources, "Policy", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)
	crossplane.AssertResourceCount(t, resources, "SecretVersion", 1)
	crossplane.AssertResourceCount(t, resources, "UserPolicyAttachment", 1)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 1)

	t.Log("secretsmanager.aws.m.upbound.io SecretVersion fields")
	crossplane.AssertFieldValues(t, resources, "SecretVersion", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                               "example-bucket-credentials",
		"metadata.ownerReferences.0.apiVersion":       "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":             "S3Bucket",
		"metadata.ownerReferences.0.name":             "example-bucket",
		"spec.forProvider.secretIdRef.name":           "example-bucket-credentials",
		"spec.forProvider.secretStringSecretRef.key":  "credentials.json",
		"spec.forProvider.secretStringSecretRef.name": "example-bucket-credentials",
		"spec.forProvider.region":                     "eu-north-1",
	})

	t.Log("iam.aws.m.upbound.io UserPolicyAttachment fields")
	crossplane.AssertFieldValues(t, resources, "UserPolicyAttachment", "iam.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.policyArnRef.name":    "example-bucket",
		"spec.forProvider.userRef.name":         "example-bucket",
	})

	t.Log("iam.aws.m.upbound.io RolePolicyAttachment fields")
	crossplane.AssertFieldValues(t, resources, "RolePolicyAttachment", "iam.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.policyArn":            "arn:aws:iam::012345678901:policy/example-bucket",
		"spec.forProvider.roleRef.name":         "example-bucket",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed,
		crossplane.MockResource(t, resources, "SecretVersion", "secretsmanager.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "UserPolicyAttachment", "iam.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "RolePolicyAttachment", "iam.aws.m.upbound.io/v1beta1", true, nil),
	)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "S3Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "AccessKey", 1)
	crossplane.AssertResourceCount(t, resources, "Policy", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)
	crossplane.AssertResourceCount(t, resources, "SecretVersion", 1)
	crossplane.AssertResourceCount(t, resources, "UserPolicyAttachment", 1)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 1)
	crossplane.AssertResourceCount(t, resources, "BucketVersioning", 1)
	crossplane.AssertResourceCount(t, resources, "BucketOwnershipControls", 1)

	t.Log("s3.aws.m.upbound.io BucketOwnershipControls fields")
	crossplane.AssertFieldValues(t, resources, "BucketOwnershipControls", "s3.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.bucketRef.name":       "example-bucket",
		"spec.forProvider.rule.objectOwnership": "BucketOwnerEnforced",
	})

	t.Log("s3.aws.m.upbound.io BucketVersioning fields")
	crossplane.AssertFieldValues(t, resources, "BucketVersioning", "s3.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                   "example-bucket",
		"metadata.ownerReferences.0.apiVersion":           "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                 "S3Bucket",
		"metadata.ownerReferences.0.name":                 "example-bucket",
		"spec.forProvider.region":                         "eu-north-1",
		"spec.forProvider.versioningConfiguration.status": "Suspended",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed,
		crossplane.MockResource(t, resources, "BucketVersioning", "s3.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "BucketOwnershipControls", "s3.aws.m.upbound.io/v1beta1", true, nil),
	)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "S3Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "Bucket", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "AccessKey", 1)
	crossplane.AssertResourceCount(t, resources, "Policy", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)
	crossplane.AssertResourceCount(t, resources, "SecretVersion", 1)
	crossplane.AssertResourceCount(t, resources, "UserPolicyAttachment", 1)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 1)
	crossplane.AssertResourceCount(t, resources, "BucketVersioning", 1)
	crossplane.AssertResourceCount(t, resources, "BucketOwnershipControls", 1)
	crossplane.AssertResourceCount(t, resources, "BucketPublicAccessBlock", 1)
	crossplane.AssertResourceCount(t, resources, "BucketServerSideEncryptionConfiguration", 1)

	t.Log("s3.aws.m.upbound.io BucketPublicAccessBlock fields")
	crossplane.AssertFieldValues(t, resources, "BucketPublicAccessBlock", "s3.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                          "example-bucket",
		"metadata.ownerReferences.0.apiVersion":  "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":        "S3Bucket",
		"metadata.ownerReferences.0.name":        "example-bucket",
		"spec.forProvider.bucketRef.name":        "example-bucket",
		"spec.forProvider.region":                "eu-north-1",
		"spec.forProvider.blockPublicAcls":       "true",
		"spec.forProvider.blockPublicPolicy":     "true",
		"spec.forProvider.ignorePublicAcls":      "true",
		"spec.forProvider.restrictPublicBuckets": "true",
	})

	t.Log("s3.aws.m.upbound.io BucketServerSideEncryptionConfiguration fields")
	crossplane.AssertFieldValues(t, resources, "BucketServerSideEncryptionConfiguration", "s3.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-bucket",
		"metadata.ownerReferences.0.apiVersion": "storage.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "S3Bucket",
		"metadata.ownerReferences.0.name":       "example-bucket",
		"spec.forProvider.bucketRef.name":       "example-bucket",
		"spec.forProvider.rule.0.applyServerSideEncryptionByDefault.sseAlgorithm":   "aws:kms",
		"spec.forProvider.rule.0.applyServerSideEncryptionByDefault.kmsMasterKeyId": "arn:aws:kms:eu-north-1:012345678901:key/mrk-0",
		"spec.forProvider.rule.0.bucketKeyEnabled":                                  "true",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed,
		crossplane.MockResource(t, resources, "BucketPublicAccessBlock", "s3.aws.m.upbound.io/v1beta1", true, nil),
		crossplane.MockResource(t, resources, "BucketServerSideEncryptionConfiguration", "s3.aws.m.upbound.io/v1beta1", true, nil),
	)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, s3Bucket, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting storage.entigo.com S3Bucket Ready Status")
	crossplane.AssertResourceReady(t, resources, "S3Bucket", "storage.entigo.com/v1alpha1")
}
