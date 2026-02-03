package main

import (
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	environmentName = "platform-apis-networking"
)

type GroupImpl struct {
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindWebAccess: {
			Instantiate: func() runtime.Object { return &v1alpha1.WebAccess{} },
			Generate:    g.generateWebAccess,
		},
	}
}

func (g *GroupImpl) generateWebAccess(obj runtime.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateIstioObjects(*obj.(*v1alpha1.WebAccess), required)
}

func (g *GroupImpl) GetSequence(_ runtime.Object) base.Sequence {
	return base.Sequence{}
}

func (g *GroupImpl) GetReadyStatus(_ *composed.Unstructured) resource.Ready {
	return ""
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, _ map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindWebAccess:
		return map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
		}, nil
	default:
		return nil, nil
	}
}

func (g *GroupImpl) GetObservedStatus(_ *composed.Unstructured) (map[string]interface{}, error) {
	return nil, nil
}
