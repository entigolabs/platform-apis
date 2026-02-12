package main

import (
	"fmt"
	"strings"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	environmentName = "platform-apis-storage"
)

type GroupImpl struct{}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindS3Bucket: {
			Instantiate: func() runtime.Object { return &v1alpha1.S3Bucket{} },
			Generate:    g.generateS3Bucket,
		},
	}
}

func (g *GroupImpl) generateS3Bucket(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateS3BucketObjects(*obj.(*v1alpha1.S3Bucket), required, observed)
}

func (g *GroupImpl) GetSequence(_ runtime.Object) base.Sequence {
	return base.NewSequence(false,
		[]string{"bucket", "iam-user", "iam-role"},
		[]string{"bucket-versioning", "iam-access-key", "iam-policy"},
		[]string{"bucket-ownership-controls", "iam-user-policy-attachment", "iam-role-policy-attachment"},
		[]string{"bucket-public-access-block", "credentials", "secrets-manager-secret"},
		[]string{"bucket-server-side-encryption-configuration", "secrets-manager-secret-version"},
	)
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	// Credentials secret should always be considered ready
	if observed.GetKind() == "Secret" && observed.GetAPIVersion() == "v1" {
		return resource.ReadyTrue
	}
	// ServiceAccount should always be considered ready
	if observed.GetKind() == "ServiceAccount" && observed.GetAPIVersion() == "v1" {
		return resource.ReadyTrue
	}
	return ""
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindS3Bucket:
		resources := map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
		}
		if _, envPresent := required[base.EnvironmentKey]; !envPresent {
			return resources, nil
		}
		env, err := service.GetEnvironment(required)
		if err != nil {
			return nil, err
		}
		ns := compositeResource.GetNamespace()
		resources[service.EKSKey] = &fnv1.ResourceSelector{
			Kind:       "Cluster",
			ApiVersion: "eks.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: "eks"},
		}
		resources[service.KMSDataAliasKey] = &fnv1.ResourceSelector{
			Kind:       "Alias",
			ApiVersion: "kms.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.DataKMSKey},
		}
		resources[service.KMSConfigAliasKey] = &fnv1.ResourceSelector{
			Kind:       "Alias",
			ApiVersion: "kms.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.ConfigKMSKey},
		}
		resources[service.NamespaceKey] = &fnv1.ResourceSelector{
			Kind:       "Namespace",
			ApiVersion: "v1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: ns},
		}
		return resources, nil
	default:
		return nil, nil
	}
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	apiVersion := observed.GetAPIVersion()
	kind := observed.GetKind()

	switch {
	case kind == "Bucket" && strings.HasPrefix(apiVersion, "s3.aws.m.upbound.io"):
		return getBucketStatus(observed)
	case kind == "BucketServerSideEncryptionConfiguration" && strings.HasPrefix(apiVersion, "s3.aws.m.upbound.io"):
		return getSSEStatus(observed)
	case kind == "BucketPublicAccessBlock" && strings.HasPrefix(apiVersion, "s3.aws.m.upbound.io"):
		return getPublicAccessBlockStatus(observed)
	case kind == "BucketOwnershipControls" && strings.HasPrefix(apiVersion, "s3.aws.m.upbound.io"):
		return getOwnershipControlsStatus(observed)
	default:
		return nil, nil
	}
}

func getBucketStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	if region, found, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "region"); found {
		status["region"] = region
	}

	if enabled, found, _ := unstructured.NestedBool(observed.Object, "status", "atProvider", "versioning", "enabled"); found {
		status["versioningEnabled"] = enabled
	}

	name := observed.GetName()
	if name != "" {
		status["s3Uri"] = fmt.Sprintf("s3://%s", name)
	}

	if domainName, found, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "bucketRegionalDomainName"); found {
		status["s3Url"] = fmt.Sprintf("https://%s", domainName)
	}

	annotations := observed.GetAnnotations()
	if annotations != nil {
		if aliasID, ok := annotations[service.AnnotationKMSDataKeyAlias]; ok {
			status["kmsKeyAlias"] = strings.TrimPrefix(aliasID, "alias/")
		}
		if saName, ok := annotations[service.AnnotationServiceAccount]; ok {
			status["serviceAccountName"] = saName
		}
	}

	return status, nil
}

func getSSEStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	if algo, found, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "rule", "0", "applyServerSideEncryptionByDefault", "sseAlgorithm"); found {
		status["encryptionType"] = algo
	}
	if keyId, found, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "rule", "0", "applyServerSideEncryptionByDefault", "kmsMasterKeyId"); found {
		status["kmsKeyId"] = keyId
	}
	if enabled, found, _ := unstructured.NestedBool(observed.Object, "status", "atProvider", "rule", "0", "bucketKeyEnabled"); found {
		status["bucketKeyEnabled"] = enabled
	}

	return status, nil
}

func getPublicAccessBlockStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	if v, found, _ := unstructured.NestedBool(observed.Object, "status", "atProvider", "blockPublicAcls"); found {
		status["blockPublicAclsEnabled"] = v
	}
	if v, found, _ := unstructured.NestedBool(observed.Object, "status", "atProvider", "blockPublicPolicy"); found {
		status["blockPublicPolicyEnabled"] = v
	}
	if v, found, _ := unstructured.NestedBool(observed.Object, "status", "atProvider", "ignorePublicAcls"); found {
		status["ignorePublicAclsEnabled"] = v
	}
	if v, found, _ := unstructured.NestedBool(observed.Object, "status", "atProvider", "restrictPublicBuckets"); found {
		status["restrictPublicBucketsEnabled"] = v
	}

	return status, nil
}

func getOwnershipControlsStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	if v, found, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "rule", "objectOwnership"); found {
		status["objectOwnership"] = v
	}

	return status, nil
}
