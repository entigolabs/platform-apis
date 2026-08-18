package main

import (
	"maps"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/platform-apis/apis"
	"google.golang.org/protobuf/types/known/durationpb"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

const (
	repoJson = `{
		"apiVersion": "artifact.entigo.com/v1alpha1",
		"kind": "Repository",
		"metadata": {"name":"repository","namespace":"default"},
		"spec": {
			"crossplane": {
				"compositionRef": {
					"name": "repositories.artifact.entigo.com"
				}
			}
		}
	}`
	repoLifecycleJson = `{
		"apiVersion": "artifact.entigo.com/v1alpha1",
		"kind": "Repository",
		"metadata": {"name":"repository","namespace":"default"},
		"spec": {
			"lifecycleRules": [
				{"tagPatterns": ["*-cloud"], "keepCount": 10},
				{"untagged": true, "expireAfterDays": 14}
			],
			"crossplane": {
				"compositionRef": {
					"name": "repositories.artifact.entigo.com"
				}
			}
		}
	}`
	repoPathJson = `{
		"apiVersion": "artifact.entigo.com/v1alpha1",
		"kind": "Repository",
		"metadata": {"name":"repository","namespace":"default"},
		"spec": {
            "path": "example/path/",
			"crossplane": {
				"compositionRef": {
					"name": "repositories.artifact.entigo.com"
				}
			}
		}
	}`
)

func TestArtifactFunction(t *testing.T) {
	repoResource := resource.MustStructJSON(repoJson)
	repoPathResource := resource.MustStructJSON(repoPathJson)
	repoLifecycleResource := resource.MustStructJSON(repoLifecycleJson)
	environmentData := map[string]interface{}{
		"awsProvider": "aws-provider",
		"dataKMSKey":  "data",
	}
	optEnvironmentData := map[string]interface{}{
		"scanOnPush":         true,
		"imageTagMutability": "MUTABLE",
		"tags": map[string]interface{}{
			"env": "test-environment",
		},
	}
	maps.Copy(optEnvironmentData, environmentData)
	// Mirrors artifact.environmentConfig.lifecycleRules in test/tests/config/aws_biz.yaml.
	lifecycleEnvironmentData := map[string]interface{}{
		"lifecycleRules": []interface{}{
			map[string]interface{}{"tagPrefixes": []interface{}{"release", "DEPLOYED"}, "keepCount": int64(10)},
			map[string]interface{}{"tagPatterns": []interface{}{"*.*.*"}, "keepCount": int64(30)},
			map[string]interface{}{"tagPatterns": []interface{}{"feature-*", "*-cloud"}, "keepCount": int64(5)},
			map[string]interface{}{"untagged": true, "expireAfterDays": int64(7)},
			map[string]interface{}{"expireAfterDays": int64(90)},
		},
	}
	maps.Copy(lifecycleEnvironmentData, environmentData)
	kmsKeyResource := test.KMSKeyResource(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string), "mrk-6c709a49a34940a48025f3bbc412827e")
	zone := "zone-a"

	cases := map[string]test.Case{
		"CreateRepositoryObjects": {
			Reason: "The Function should create repository objects",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: repoResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						apis.KMSDataKey: kmsKeyResource,
						base.ZoneKey:    test.ZoneWithMetadata(zone, map[string]interface{}{base.TagsPrefix + "foo": "bar"}, map[string]interface{}{base.TagsPrefix + "bar": "foo"}),
						base.ZoneEnvKey: test.EnvironmentConfigResourceWithData(base.ZoneEnvName, map[string]interface{}{"tags": map[string]interface{}{"custom-tag": "custom-value"}}),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tags.entigo.com/foo":"bar","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKey":"arn:aws:kms:eu-north-1:111111111111:key/mrk-6c709a49a34940a48025f3bbc412827e"}],"region":"eu-north-1","tags":{"bar":"foo","custom-tag":"custom-value","entigo:zone":"zone-a","foo":"bar"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							apis.KMSDataKey: base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
		"CreateRepositoryObjectsWithPath": {
			Reason: "The Function should create repository objects with repository path",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: repoPathResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						apis.KMSDataKey: kmsKeyResource,
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"annotations":{"crossplane.io/external-name":"example/path/repository"},"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKey":"arn:aws:kms:eu-north-1:111111111111:key/mrk-6c709a49a34940a48025f3bbc412827e"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							apis.KMSDataKey: base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
		"CreateRepositoryObjectsAllEnv": {
			Reason: "The Function should create repository objects with all environment variables",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: repoResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData),
						apis.KMSDataKey:     kmsKeyResource,
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKey":"arn:aws:kms:eu-north-1:111111111111:key/mrk-6c709a49a34940a48025f3bbc412827e"}],"imageScanningConfiguration":{"scanOnPush":true},"imageTagMutability":"MUTABLE","region":"eu-north-1","tags":{"entigo:zone":"zone-a","env":"test-environment"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							apis.KMSDataKey: base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
		"CreateLifecyclePolicyFromEnvironment": {
			Reason: "The Function should create a lifecycle policy from the environment default once the repository is ready",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: repoResource,
						},
						Resources: map[string]*fnv1.Resource{
							"repository": observedReadyRepository(),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, lifecycleEnvironmentData),
						apis.KMSDataKey:     kmsKeyResource,
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository":       {Resource: resource.MustStructJSON(`{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKey":"arn:aws:kms:eu-north-1:111111111111:key/mrk-6c709a49a34940a48025f3bbc412827e"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`), Ready: 1},
							"lifecycle-policy": {Resource: resource.MustStructJSON(`{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"LifecyclePolicy","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"policy":"{\"rules\":[{\"rulePriority\":1,\"description\":\"Keep 10 latest images tagged with prefix release, DEPLOYED\",\"selection\":{\"tagStatus\":\"tagged\",\"tagPrefixList\":[\"release\",\"DEPLOYED\"],\"countType\":\"imageCountMoreThan\",\"countNumber\":10},\"action\":{\"type\":\"expire\"}},{\"rulePriority\":2,\"description\":\"Keep 30 latest images matching tag pattern *.*.*\",\"selection\":{\"tagStatus\":\"tagged\",\"tagPatternList\":[\"*.*.*\"],\"countType\":\"imageCountMoreThan\",\"countNumber\":30},\"action\":{\"type\":\"expire\"}},{\"rulePriority\":3,\"description\":\"Keep 5 latest images matching tag pattern feature-*, *-cloud\",\"selection\":{\"tagStatus\":\"tagged\",\"tagPatternList\":[\"feature-*\",\"*-cloud\"],\"countType\":\"imageCountMoreThan\",\"countNumber\":5},\"action\":{\"type\":\"expire\"}},{\"rulePriority\":4,\"description\":\"Expire untagged images older than 7 days\",\"selection\":{\"tagStatus\":\"untagged\",\"countType\":\"sinceImagePushed\",\"countUnit\":\"days\",\"countNumber\":7},\"action\":{\"type\":\"expire\"}},{\"rulePriority\":5,\"description\":\"Expire images older than 90 days\",\"selection\":{\"tagStatus\":\"any\",\"countType\":\"sinceImagePushed\",\"countUnit\":\"days\",\"countNumber\":90},\"action\":{\"type\":\"expire\"}}]}","region":"eu-north-1","repository":"repository"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							apis.KMSDataKey: base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
		"CreateLifecyclePolicyFromSpec": {
			Reason: "The Function should let the repository spec replace the environment default rules",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: repoLifecycleResource,
						},
						Resources: map[string]*fnv1.Resource{
							"repository": observedReadyRepository(),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, lifecycleEnvironmentData),
						apis.KMSDataKey:     kmsKeyResource,
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository":       {Resource: resource.MustStructJSON(`{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKey":"arn:aws:kms:eu-north-1:111111111111:key/mrk-6c709a49a34940a48025f3bbc412827e"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`), Ready: 1},
							"lifecycle-policy": {Resource: resource.MustStructJSON(`{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"LifecyclePolicy","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"policy":"{\"rules\":[{\"rulePriority\":1,\"description\":\"Keep 10 latest images matching tag pattern *-cloud\",\"selection\":{\"tagStatus\":\"tagged\",\"tagPatternList\":[\"*-cloud\"],\"countType\":\"imageCountMoreThan\",\"countNumber\":10},\"action\":{\"type\":\"expire\"}},{\"rulePriority\":2,\"description\":\"Expire untagged images older than 14 days\",\"selection\":{\"tagStatus\":\"untagged\",\"countType\":\"sinceImagePushed\",\"countUnit\":\"days\",\"countNumber\":14},\"action\":{\"type\":\"expire\"}}]}","region":"eu-north-1","repository":"repository"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							apis.KMSDataKey: base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
		"SkipLifecyclePolicyUntilRepositoryIsReady": {
			Reason: "The Function should not create a lifecycle policy before the repository it points at exists",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: repoResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, lifecycleEnvironmentData),
						apis.KMSDataKey:     kmsKeyResource,
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","tenancy.entigo.com/zone":"zone-a"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKey":"arn:aws:kms:eu-north-1:111111111111:key/mrk-6c709a49a34940a48025f3bbc412827e"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							apis.KMSDataKey: base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
	}

	test.AddEnvironmentConfig(cases, environmentName, environmentData)
	test.AddZoneResources(cases, "default", zone)
	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases)
}

func observedReadyRepository() *fnv1.Resource {
	return &fnv1.Resource{Resource: resource.MustStructJSON(`{
		"apiVersion": "ecr.aws.m.upbound.io/v1beta1", "kind": "Repository",
		"metadata": {"name": "repository", "namespace": "default"},
		"status": {"conditions": [{"type": "Ready", "status": "True"}]}
	}`)}
}
