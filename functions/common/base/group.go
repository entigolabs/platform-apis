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
	VPCKind                = "VPC"
	VPCApiVersion          = "ec2.aws.m.upbound.io/v1beta1"
	VPCClusterApiVersion   = "ec2.aws.upbound.io/v1beta1"
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
	// GetObservedStatus returns status fields read from one composed resource. A nil value removes the
	// field from the composite's status.
	GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error)
}

// CompositeStatusProvider is an optional interface that GroupService implementations can implement
// to add status derived from the composite resource itself and its required resources, such as
// which of its settings were applied, rather than from a composed resource.
type CompositeStatusProvider interface {
	GetCompositeStatus(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]interface{}, error)
}

// CloudAcceptedCondition shows on the user's object whether the cloud, or the backend before it,
// accepted the object: the cloud's rejection otherwise appears only on the provider's own object.
const CloudAcceptedCondition = "CloudAccepted"

// NotAcceptedError marks a refusal by the cloud or the backend, such as an operation the backend
// cannot perform. The function reports it as CloudAccepted: False as well as failing, so nothing
// composed changes.
type NotAcceptedError struct{ Err error }

func (e *NotAcceptedError) Error() string { return e.Err.Error() }
func (e *NotAcceptedError) Unwrap() error { return e.Err }

// NotAccepted marks err as a refusal by the cloud or the backend.
func NotAccepted(err error) error { return &NotAcceptedError{Err: err} }

// Condition is a condition a GroupService reports on the composite resource.
type Condition struct {
	Type    string
	Status  bool
	Reason  string
	Message string
}

// ConditionProvider is an optional interface that GroupService implementations can implement to
// report conditions on the composite resource after it composed. A false condition is also emitted
// as a warning event.
type ConditionProvider interface {
	GetConditions(compositeResource *composite.Unstructured, observed map[resource.Name]resource.ObservedComposed) []Condition
}

// PostStatusProcessor is an optional interface that GroupService implementations can implement
// to perform custom status aggregation (e.g., collecting arrays from multiple resources)
type PostStatusProcessor interface {
	PostProcessStatus(status map[string]interface{}, observed map[resource.Name]resource.ObservedComposed) (map[string]interface{}, error)
}
