package base

import (
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	AppLabel              = "app"
	ResourceLabel         = "entigo.com/resource"
	ResourceKindLabel     = "entigo.com/resource-kind"
	EnvironmentKey        = "Environment"
	EnvironmentKind       = "EnvironmentConfig"
	EnvironmentApiVersion = "apiextensions.crossplane.io/v1beta1"
)

type ResourceHandler struct {
	Instantiate func() runtime.Object
	Generate    func(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error)
}

type GroupService interface {
	GetResourceHandlers() map[string]ResourceHandler
	GetReadyStatus(observed *composed.Unstructured) resource.Ready
	GetRequiredResources(compositeResource *composite.Unstructured) map[string]*fnv1.ResourceSelector
	GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error)
	AddStatusConditions(compositeResource *composite.Unstructured, observed map[resource.Name]resource.ObservedComposed)
}
