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
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
						apis.KMSDataKey:     test.KMSKeyResource(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"creationTimestamp":null,"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKeyRef":{"name":"data","namespace":"aws-provider"}}],"region":"eu-north-1"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
							apis.KMSDataKey:     base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
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
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
						apis.KMSDataKey:     test.KMSKeyResource(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"annotations":{"crossplane.io/external-name":"example/path/repository"},"creationTimestamp":null,"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKeyRef":{"name":"data","namespace":"aws-provider"}}],"region":"eu-north-1"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
							apis.KMSDataKey:     base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
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
						apis.KMSDataKey:     test.KMSKeyResource(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta1","kind":"Repository","metadata":{"creationTimestamp":null,"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"encryptionConfiguration":[{"encryptionType":"KMS","kmsKeyRef":{"name":"data","namespace":"aws-provider"}}],"imageScanningConfiguration":{"scanOnPush":true},"imageTagMutability":"MUTABLE","region":"eu-north-1","tags":{"env":"test-environment"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
							apis.KMSDataKey:     base.RequiredKMSKey(environmentData["dataKMSKey"].(string), environmentData["awsProvider"].(string)),
						},
					},
				},
			},
		},
	}

	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases)
}
