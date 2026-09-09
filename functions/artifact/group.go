package main

import (
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	environmentName = "platform-apis-artifact"
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
		apis.XRKindRepository: {
			Instantiate: func() client.Object { return &v1alpha1.Repository{} },
			Generate:    g.generateRepository,
		},
	}
}

func (g *GroupImpl) generateRepository(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateRepositoryObject(*obj.(*v1alpha1.Repository), required, observed)
}

func (g *GroupImpl) GetSequence(object client.Object) base.Sequence {
	// The lifecycle policy is rejected by AWS until the repository it points at exists.
	return base.NewSequence(true, []string{object.GetName()}, []string{service.LifecyclePolicyResourcePrefix + ".*"})
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
		env, err := service.GetEnvironment(required)
		if err != nil {
			return nil, err
		}
		resources[apis.VPCKey] = base.RequiredVPC(env.VPC, env.AWSProvider)
		if env.DataKMSKey != "" {
			resources[apis.KMSDataKey] = base.RequiredKMSKey(env.DataKMSKey, env.AWSProvider)
		}
		return resources, nil
	default:
		return nil, nil
	}
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
