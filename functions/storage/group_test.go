package main

import (
	"fmt"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
	"github.com/entigolabs/platform-apis/service"
	"google.golang.org/protobuf/types/known/durationpb"
)

var defaultEnvironmentData = map[string]interface{}{
	"awsProvider":  "aws-provider",
	"dataKMSKey":   "data",
	"configKMSKey": "config",
}

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

const (
	s3bucketJson = `{
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

	s3bucketCustomSAJson = `{
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

	s3bucketNoSAJson = `{
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
)

const (
	bucketResJson                  = `{"apiVersion":"s3.aws.m.upbound.io/v1beta1","kind":"Bucket","metadata":{"annotations":{"storage.entigo.com/service-account-name":"%s"},"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"default"},"spec":{"forProvider":{"region":"eu-north-1","tags":{"Name":"%s","tenancy.entigo.com/zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"%s-bucket"}},"status":{"atProvider":{}}}`
	bucketPublicAccessBlockResJson = `{"apiVersion":"s3.aws.m.upbound.io/v1beta1","kind":"BucketPublicAccessBlock","metadata":{"name":"%s"},"spec":{"forProvider":{"blockPublicAcls":true,"blockPublicPolicy":true,"bucketRef":{"name":"%s"},"ignorePublicAcls":true,"region":"eu-north-1","restrictPublicBuckets":true},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	bucketSSEResJson               = `{"apiVersion":"s3.aws.m.upbound.io/v1beta1","kind":"BucketServerSideEncryptionConfiguration","metadata":{"name":"%s"},"spec":{"forProvider":{"bucketRef":{"name":"%s"},"region":"eu-north-1","rule":[{"applyServerSideEncryptionByDefault":{"kmsMasterKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","sseAlgorithm":"aws:kms"},"bucketKeyEnabled":true}]},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	bucketVersioningResJson        = `{"apiVersion":"s3.aws.m.upbound.io/v1beta1","kind":"BucketVersioning","metadata":{"name":"%s"},"spec":{"forProvider":{"bucketRef":{"name":"%s"},"region":"eu-north-1","versioningConfiguration":{"status":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	bucketOwnershipResJson         = `{"apiVersion":"s3.aws.m.upbound.io/v1beta1","kind":"BucketOwnershipControls","metadata":{"name":"%s"},"spec":{"forProvider":{"bucketRef":{"name":"%s"},"region":"eu-north-1","rule":{"objectOwnership":"BucketOwnerEnforced"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	iamUserResJson                 = `{"apiVersion":"iam.aws.m.upbound.io/v1beta1","kind":"User","metadata":{"name":"%s"},"spec":{"forProvider":{"tags":{"Name":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	iamPolicyResJson               = `{"apiVersion":"iam.aws.m.upbound.io/v1beta1","kind":"Policy","metadata":{"name":"%s"},"spec":{"forProvider":{"policy":"{\"Statement\":[{\"Action\":[\"kms:Encrypt\",\"kms:Decrypt\",\"kms:ReEncrypt*\",\"kms:GenerateDataKey*\",\"kms:DescribeKey\"],\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123\"]},{\"Action\":\"s3:*\",\"Effect\":\"Allow\",\"Resource\":[\"arn:aws:s3:::%s\",\"arn:aws:s3:::%s/*\"]}],\"Version\":\"2012-10-17\"}","tags":{"Name":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	iamUserPolicyAttachmentResJson = `{"apiVersion":"iam.aws.m.upbound.io/v1beta1","kind":"UserPolicyAttachment","metadata":{"name":"%s"},"spec":{"forProvider":{"policyArnRef":{"name":"%s"},"userRef":{"name":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	iamAccessKeyResJson            = `{"apiVersion":"iam.aws.m.upbound.io/v1beta1","kind":"AccessKey","metadata":{"name":"%s"},"spec":{"forProvider":{"userRef":{"name":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"%s-access-key"}},"status":{"atProvider":{}}}`
	iamRoleResJson                 = `{"apiVersion":"iam.aws.m.upbound.io/v1beta1","kind":"Role","metadata":{"name":"%s"},"spec":{"forProvider":{"assumeRolePolicy":"{\"Statement\":[{\"Action\":\"sts:AssumeRoleWithWebIdentity\",\"Condition\":{\"StringEquals\":{\"oidc.eks.eu-north-1.amazonaws.com/id/ABCDEF1234567890:aud\":\"sts.amazonaws.com\",\"oidc.eks.eu-north-1.amazonaws.com/id/ABCDEF1234567890:sub\":\"system:serviceaccount:default:%s\"}},\"Effect\":\"Allow\",\"Principal\":{\"Federated\":\"arn:aws:iam::111111111111:oidc-provider/oidc.eks.eu-north-1.amazonaws.com/id/ABCDEF1234567890\"}}],\"Version\":\"2012-10-17\"}","tags":{"Name":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	iamRolePolicyAttachmentResJson = `{"apiVersion":"iam.aws.m.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"name":"%s"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::111111111111:policy/%s","roleRef":{"name":"%s"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	serviceAccountResJson          = `{"apiVersion":"v1","kind":"ServiceAccount","metadata":{"annotations":{"eks.amazonaws.com/role-arn":"arn:aws:iam::111111111111:role/%s"},"name":"%s","namespace":"default"}}`
	smSecretResJson                = `{"apiVersion":"secretsmanager.aws.m.upbound.io/v1beta1","kind":"Secret","metadata":{"name":"%s-credentials"},"spec":{"forProvider":{"description":"Credentials for bucket %s","kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-config456","name":"%s-credentials","recoveryWindowInDays":0,"region":"eu-north-1","tags":{"Name":"%s-credentials"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	smSecretVersionResJson         = `{"apiVersion":"secretsmanager.aws.m.upbound.io/v1beta1","kind":"SecretVersion","metadata":{"name":"%s-credentials"},"spec":{"forProvider":{"region":"eu-north-1","secretIdRef":{"name":"%s-credentials"},"secretStringSecretRef":{"key":"credentials.json","name":"%s-credentials"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	credentialsResJson             = `{"apiVersion":"v1","kind":"Secret","metadata":{"name":"%s-credentials","namespace":"default"},"stringData":{"AWS_ACCESS_KEY_ID":"AKIAIOSFODNN7EXAMPLE","AWS_SECRET_ACCESS_KEY":"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY","BUCKET_ARN":"arn:aws:s3:::%s","BUCKET_NAME":"%s","BUCKET_REGION":"eu-north-1","credentials.json":"{\"AWS_ACCESS_KEY_ID\": \"AKIAIOSFODNN7EXAMPLE\", \"AWS_SECRET_ACCESS_KEY\": \"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\", \"BUCKET_REGION\": \"eu-north-1\", \"BUCKET_ARN\": \"arn:aws:s3:::%s\", \"BUCKET_NAME\": \"%s\"}"},"type":"Opaque"}`
)

func allRequiredResources(environmentData map[string]interface{}) map[string]*fnv1.Resources {
	return map[string]*fnv1.Resources{
		base.EnvironmentKey:  test.EnvironmentConfigResourceWithData(environmentName, environmentData),
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

func expectedRequirements(awsProvider string, environmentData map[string]interface{}) *fnv1.Requirements {
	return &fnv1.Requirements{
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
	}
}

func desiredResources(bucketName, saName, versioningStatus string) map[string]*fnv1.Resource {
	return map[string]*fnv1.Resource{
		"bucket":                     {Resource: resource.MustStructJSON(fmt.Sprintf(bucketResJson, saName, bucketName, bucketName, bucketName)), Ready: 1},
		"bucket-public-access-block": {Resource: resource.MustStructJSON(fmt.Sprintf(bucketPublicAccessBlockResJson, bucketName, bucketName)), Ready: 1},
		"bucket-server-side-encryption-configuration": {Resource: resource.MustStructJSON(fmt.Sprintf(bucketSSEResJson, bucketName, bucketName)), Ready: 1},
		"bucket-versioning":                           {Resource: resource.MustStructJSON(fmt.Sprintf(bucketVersioningResJson, bucketName, bucketName, versioningStatus)), Ready: 1},
		"bucket-ownership-controls":                   {Resource: resource.MustStructJSON(fmt.Sprintf(bucketOwnershipResJson, bucketName, bucketName)), Ready: 1},
		"iam-user":                                    {Resource: resource.MustStructJSON(fmt.Sprintf(iamUserResJson, bucketName, bucketName)), Ready: 1},
		"iam-policy":                                  {Resource: resource.MustStructJSON(fmt.Sprintf(iamPolicyResJson, bucketName, bucketName, bucketName, bucketName)), Ready: 1},
		"iam-user-policy-attachment":                  {Resource: resource.MustStructJSON(fmt.Sprintf(iamUserPolicyAttachmentResJson, bucketName, bucketName, bucketName)), Ready: 1},
		"iam-access-key":                              {Resource: resource.MustStructJSON(fmt.Sprintf(iamAccessKeyResJson, bucketName, bucketName, bucketName)), Ready: 1},
		"iam-role":                                    {Resource: resource.MustStructJSON(fmt.Sprintf(iamRoleResJson, bucketName, saName, bucketName)), Ready: 1},
		"iam-role-policy-attachment":                  {Resource: resource.MustStructJSON(fmt.Sprintf(iamRolePolicyAttachmentResJson, bucketName, bucketName, bucketName)), Ready: 1},
		"service-account":                             {Resource: resource.MustStructJSON(fmt.Sprintf(serviceAccountResJson, bucketName, saName)), Ready: 1},
		"secrets-manager-secret":                      {Resource: resource.MustStructJSON(fmt.Sprintf(smSecretResJson, bucketName, bucketName, bucketName, bucketName)), Ready: 1},
		"secrets-manager-secret-version":              {Resource: resource.MustStructJSON(fmt.Sprintf(smSecretVersionResJson, bucketName, bucketName, bucketName)), Ready: 1},
		"credentials":                                 {Resource: resource.MustStructJSON(fmt.Sprintf(credentialsResJson, bucketName, bucketName, bucketName, bucketName, bucketName))},
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
	environmentData := defaultEnvironmentData
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
					Meta:         &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Requirements: expectedRequirements(awsProvider, environmentData),
				},
			},
		},
	}
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, cases)
}

func TestS3BucketGeneration(t *testing.T) {
	environmentData := defaultEnvironmentData
	awsProvider := environmentData["awsProvider"].(string)
	bucketName := "test-bucket"

	allDesired := desiredResources(bucketName, bucketName, "Suspended")

	noSADesired := desiredResources(bucketName, bucketName, "Enabled")
	delete(noSADesired, "service-account")

	step1Desired := make(map[string]*fnv1.Resource)
	for _, key := range []string{"bucket", "iam-user", "iam-role", "service-account"} {
		step1Desired[key] = &fnv1.Resource{Resource: allDesired[key].Resource}
	}

	customSADesired := desiredResources(bucketName, "my-custom-sa", "Suspended")

	cases := map[string]test.Case{
		"FullGeneration": {
			Reason: "All observed resources present, should generate all 15 desired resources",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(s3bucketJson)},
						Resources: allObservedComposedResources(),
					},
					RequiredResources: allRequiredResources(environmentData),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: allDesired,
					},
					Requirements: expectedRequirements(awsProvider, environmentData),
				},
			},
		},
		"NoServiceAccount": {
			Reason: "Should not create service-account when createServiceAccount=false, versioning=Enabled",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(s3bucketNoSAJson)},
						Resources: allObservedComposedResources(),
					},
					RequiredResources: allRequiredResources(environmentData),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: noSADesired,
					},
					Requirements: expectedRequirements(awsProvider, environmentData),
				},
			},
		},
		"NoCredentialsWithoutObserved": {
			Reason: "Without observed resources, only step 1 resources should be generated",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(s3bucketJson)},
					},
					RequiredResources: allRequiredResources(environmentData),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: step1Desired,
					},
					Requirements: expectedRequirements(awsProvider, environmentData),
				},
			},
		},
		"CustomServiceAccountName": {
			Reason: "Should use custom service account name in SA, bucket annotation, and IAM role policy",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(s3bucketCustomSAJson)},
						Resources: allObservedComposedResources(),
					},
					RequiredResources: allRequiredResources(environmentData),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: customSADesired,
					},
					Requirements: expectedRequirements(awsProvider, environmentData),
				},
			},
		},
	}
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, cases)
}
