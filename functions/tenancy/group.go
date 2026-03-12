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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	environmentName = "platform-apis-zone"
	ec2ApiVersion   = "ec2.aws.upbound.io/v1beta1"
	infralibZone    = "infralib"
)

type GroupImpl struct {
	log logging.Logger
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SetLogger(log logging.Logger) {
	g.log = log
}

func (g *GroupImpl) SkipGeneration(compositeResource *composite.Unstructured) bool {
	return compositeResource.GetKind() == apis.XRKindZone && compositeResource.GetName() == infralibZone
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindZone: {
			Instantiate: func() client.Object { return &v1alpha1.Zone{} },
			Generate:    g.generateZone,
		},
	}
}

func (g *GroupImpl) generateZone(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateZoneObjects(*obj.(*v1alpha1.Zone), required, observed)
}

func (g *GroupImpl) GetSequence(object client.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindZone:
		return base.NewSequence(true, []string{"namespace-.*", "launchtemplate-.*", "kyverno-mutate-.*"}, []string{"netpol-.*", "role-.*", "sidecar-.*", "kyverno-validate-.*"},
			[]string{"rbacrole-.*", "rpa-.*", "ae-.*"}, []string{"rb-.*", "nodepool-.*"})
	default:
		return base.Sequence{}
	}
}

func (g *GroupImpl) GetReadyStatus(_ *composed.Unstructured) resource.Ready {
	return ""
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindZone:
		// Converting required to access namespaces
		var zone v1alpha1.Zone
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(compositeResource.Object, &zone); err != nil {
			return nil, err
		}
		resources := map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
			service.NamespaceKey: {
				Kind:       "Namespace",
				ApiVersion: "v1",
				Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{
					base.TenancyZoneLabel: zone.Name,
				}}},
			},
		}
		if _, envPresent := required[base.EnvironmentKey]; !envPresent {
			return resources, nil
		}
		env, err := service.GetEnvironment(required)
		if err != nil {
			return nil, err
		}
		namespaces, err := base.ExtractResources[*corev1.Namespace](required, service.NamespaceKey)
		if err != nil {
			return nil, err
		}
		resources[service.VPCKey] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: ec2ApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.VPC},
		}
		resources[service.KMSDataAliasKey] = &fnv1.ResourceSelector{
			Kind:       "Alias",
			ApiVersion: "kms.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.DataKMSAlias},
		}
		resources[service.SecurityGroupKey] = &fnv1.ResourceSelector{
			Kind:       "SecurityGroup",
			ApiVersion: ec2ApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.SecurityGroup},
		}
		resources[service.ClusterKey] = &fnv1.ResourceSelector{
			Kind:       "Cluster",
			ApiVersion: "eks.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.Cluster},
		}
		resources[service.ComputeSubnetsKey] = subnetSelector(env.ComputeSubnetType)
		resources[service.ServiceSubnetsKey] = subnetSelector(env.ServiceSubnetType)
		resources[service.PublicSubnetsKey] = subnetSelector(env.PublicSubnetType)
		resources[service.ControlSubnetsKey] = subnetSelector(env.ControlSubnetType)
		for _, ns := range service.GetUniqueNamespaces(zone, namespaces) {
			if ns == "" {
				continue
			}
			resources[ns+service.IngressKey] = &fnv1.ResourceSelector{
				Kind:       "Ingress",
				ApiVersion: "networking.k8s.io/v1",
				Namespace:  &ns,
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
			}
			resources[ns+service.ServiceKey] = &fnv1.ResourceSelector{
				Kind:       "Service",
				ApiVersion: "v1",
				Namespace:  &ns,
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
			}
		}
		return resources, nil
	default:
		return nil, nil
	}
}

func subnetSelector(subnetType string) *fnv1.ResourceSelector {
	return &fnv1.ResourceSelector{
		Kind:       "Subnet",
		ApiVersion: ec2ApiVersion,
		Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": subnetType}}},
	}
}

func (g *GroupImpl) GetObservedStatus(_ *composed.Unstructured) (map[string]interface{}, error) {
	return nil, nil
}
