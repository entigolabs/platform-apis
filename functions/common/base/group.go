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
	KMSKeyKind            = "Key"
	KMSKeyApiVersion      = "kms.aws.m.upbound.io/v1beta1"
)

type ResourceHandler struct {
	Instantiate func() runtime.Object
	Generate    func(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error)
}

type Sequence struct {
	Regex bool
	Steps []Step
}

type Step struct {
	Objects []string
}

// NewSequence creates a new Sequence from the provided step objects.
// Regex patterns will be prefixed with ^ and suffixed with $
func NewSequence(regex bool, stepObjects ...[]string) Sequence {
	seq := make([]Step, 0, len(stepObjects))
	for _, objects := range stepObjects {
		if len(objects) > 0 {
			seq = append(seq, Step{Objects: objects})
		}
	}
	return Sequence{Steps: seq, Regex: regex}
}

type GroupService interface {
	SkipGeneration(compositeResource *composite.Unstructured) bool
	GetResourceHandlers() map[string]ResourceHandler
	// GetSequence Objects not listed in the sequence are created during the first step.
	GetSequence(object runtime.Object) Sequence
	GetReadyStatus(observed *composed.Unstructured) resource.Ready
	GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error)
	GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error)
}
