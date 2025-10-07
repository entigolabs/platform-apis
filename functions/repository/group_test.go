package main

import (
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"google.golang.org/protobuf/types/known/durationpb"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

func TestRunFunction(t *testing.T) {
	cases := map[string]test.Case{
		"CreateRepositoryObjects": {
			Reason: "The Function should create repository objects",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{
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
							}`),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						RequiredRepositoryKey: {Items: []*fnv1.Resource{}},
						base.EnvironmentKey:   test.GetEnvironmentConfigWithData("platform-apis-repository", map[string]interface{}{"region": "eu-north-1", "provider": "aws-provider"}),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"repository": {Resource: resource.MustStructJSON(`
{"apiVersion":"ecr.aws.m.upbound.io/v1beta2","kind":"Repository","metadata":{"creationTimestamp":null,"labels":{"entigo.com/resource":"repository","entigo.com/resource-kind":"Repository","region":"eu-north-1"},"name":"repository","namespace":"default"},"spec":{"forProvider":{"imageScanningConfiguration":{"scanOnPush":null},"region":"eu-north-1"},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							RequiredRepositoryKey: {Kind: RepositoryKind, ApiVersion: RepositoryApiVersion, Match: &fnv1.ResourceSelector_MatchName{MatchName: "repository"}},
							base.EnvironmentKey:   base.RequiredEnvironmentConfig("platform-apis-repository"),
						},
					},
				},
			},
		},
	}

	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases, "force-sync", "lastTransitionTime")
}
