package main

import (
	"strings"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	XRKindRepository = "Repository"

	RequiredRepositoryKey = "Repository"
	RepositoryKind        = "Repository"
	RepositoryApiVersion  = "ecr.aws.m.upbound.io/v1beta1"

	KMSDataKey = "KMSDataKey"
)

type GroupImpl struct {
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		XRKindRepository: {
			Instantiate: func() runtime.Object { return &v1alpha1.Repository{} },
			Generate:    g.generateRepository,
		},
	}
}

func (g *GroupImpl) generateRepository(obj runtime.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return GenerateRepositoryObject(*obj.(*v1alpha1.Repository), required)
}

func (g *GroupImpl) GetReadyStatus(_ *composed.Unstructured) resource.Ready {
	return ""
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case XRKindRepository:
		resources := map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig("platform-apis-repository"),
		}
		env, envPresent := required[base.EnvironmentKey]
		if !envPresent {
			return resources, nil
		}
		kms, err := getRequiredKMS(env)
		if err != nil {
			return nil, err
		}
		resources[KMSDataKey] = kms
		resources[RequiredRepositoryKey] = &fnv1.ResourceSelector{
			Kind:       RepositoryKind,
			ApiVersion: RepositoryApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: compositeResource.GetName()},
		}
		return resources, nil
	default:
		return nil, nil
	}
}

func getRequiredKMS(resources []resource.Required) (*fnv1.ResourceSelector, error) {
	env, err := getEnvironment(resources)
	if err != nil {
		return nil, err
	}
	return base.RequiredKMSKey(env.DataKMSKey, env.AWSProvider), nil
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	switch {
	case observed.GetKind() == RepositoryKind && strings.HasPrefix(observed.GetAPIVersion(), "ecr.aws.m.upbound.io"):
		return getRepositoryStatus(observed)
	default:
		return nil, nil
	}
}

func getRepositoryStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	uri, found, err := unstructured.NestedString(observed.Object, "status", "atProvider", "repositoryUrl")
	if err != nil || !found {
		return nil, nil
	}
	return map[string]interface{}{"repositoryUri": uri}, nil
}

func (g *GroupImpl) AddStatusConditions(_ *composite.Unstructured, _ map[resource.Name]resource.ObservedComposed) {
	// No-op
}
