package base

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	AppLabel               = "app"
	ResourceLabel          = "entigo.com/resource"
	ResourceKindLabel      = "entigo.com/resource-kind"
	EnvironmentKey         = "Environment"
	EnvironmentKind        = "EnvironmentConfig"
	EnvironmentApiVersion  = "apiextensions.crossplane.io/v1beta1"
	KMSKeyKind             = "Key"
	KMSKeyApiVersion       = "kms.aws.m.upbound.io/v1beta1"
	NamespaceKey           = "Namespace"
	TenancyApiVersion      = "tenancy.entigo.com/v1alpha1"
	ZoneKey                = "Zone"
	ZoneEnvKey             = "ZoneEnvironment"
	ZoneEnvName            = "platform-apis-zone"
	TenancyZoneLabel       = "tenancy.entigo.com/zone"
	TenancyZoneAWSTag      = "entigo:zone"
	TenancyWorkspaceLabel  = "tenancy.entigo.com/workspace"
	TenancyWorkspaceAWSTag = "entigo:workspace"
)

type ResourceHandler struct {
	Instantiate func() client.Object
	Generate    func(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error)
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
	SetLogger(log logging.Logger)
	SkipGeneration(compositeResource *composite.Unstructured) bool
	GetResourceHandlers() map[string]ResourceHandler
	// GetSequence Objects not listed in the sequence are created during the first step.
	GetSequence(object client.Object) Sequence
	GetReadyStatus(observed *composed.Unstructured) resource.Ready
	GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error)
	GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error)
}

// PostStatusProcessor is an optional interface that GroupService implementations can implement
// to perform custom status aggregation (e.g., collecting arrays from multiple resources)
type PostStatusProcessor interface {
	PostProcessStatus(status map[string]interface{}, observed map[resource.Name]resource.ObservedComposed) (map[string]interface{}, error)
}
