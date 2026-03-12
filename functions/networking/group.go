package main

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	environmentName = "platform-apis-networking"
)

type GroupImpl struct {
	log logging.Logger
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SetLogger(log logging.Logger) {
	g.log = log
}

func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindWebAccess: {
			Instantiate: func() client.Object { return &v1alpha1.WebAccess{} },
			Generate:    g.generateWebAccess,
		},
	}
}

func (g *GroupImpl) generateWebAccess(obj client.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateIstioObjects(*obj.(*v1alpha1.WebAccess), required)
}

func (g *GroupImpl) GetSequence(_ client.Object) base.Sequence {
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
