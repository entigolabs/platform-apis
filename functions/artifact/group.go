package main

import (
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
	environmentName = "platform-apis-artifact"
)

type GroupImpl struct {
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindRepository: {
			Instantiate: func() runtime.Object { return &v1alpha1.Repository{} },
			Generate:    g.generateRepository,
		},
	}
}

func (g *GroupImpl) generateRepository(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateRepositoryObject(*obj.(*v1alpha1.Repository), required)
}

func (g *GroupImpl) GetReadyStatus(_ *composed.Unstructured) resource.Ready {
	return ""
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindRepository:
		resources := map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
		}
		if _, envPresent := required[base.EnvironmentKey]; !envPresent {
			return resources, nil
		}
		kms, err := getRequiredKMS(required)
		if err != nil {
			return nil, err
		}
		resources[apis.KMSDataKey] = kms
		return resources, nil
	default:
		return nil, nil
	}
}

func getRequiredKMS(required map[string][]resource.Required) (*fnv1.ResourceSelector, error) {
	env, err := service.GetEnvironment(required)
	if err != nil {
		return nil, err
	}
	return base.RequiredKMSKey(env.DataKMSKey, env.AWSProvider), nil
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	switch {
	case observed.GetKind() == apis.RepositoryKind && strings.HasPrefix(observed.GetAPIVersion(), "ecr.aws.m.upbound.io"):
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
