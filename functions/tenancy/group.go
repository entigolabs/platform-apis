package main

import (
	"maps"

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
	environmentName              = "platform-apis-zone"
	ec2ApiVersion                = "ec2.aws.upbound.io/v1beta1"
	ingressClassParamsApiVersion = "elbv2.k8s.aws/v1beta1"
	gatewayApiVersion            = "gateway.networking.k8s.io/v1"
	lbConfigApiVersion           = "gateway.k8s.aws/v1beta1"
	ociContainerEngineApiVersion = "containerengine.oci.upbound.io/v1alpha1"
	ociNetworkingApiVersion      = "networking.oci.upbound.io/v1alpha1"
	infralibZone                 = "infralib"
)

type GroupImpl struct {
	log           logging.Logger
	cloudProvider apis.CloudProvider
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
	return service.GenerateZoneObjects(*obj.(*v1alpha1.Zone), required, observed, g.log, g.cloudProvider)
}

func (g *GroupImpl) GetSequence(object client.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindZone:
		return base.NewSequence(true, []string{"namespace-.*", "launchtemplate-.*", "kyverno-mutate-.*"}, []string{"netpol-.*", "role-.*", "sidecar-.*", "kyverno-validate-.*"},
			[]string{"rbacrole-.*", "rpa-.*", "ae-.*", "crb-.*"}, []string{"rb-.*", "nodepool-.*"})
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
		if err := env.ValidateFor(g.cloudProvider); err != nil {
			return nil, err
		}
		if g.cloudProvider == apis.CloudProviderOCI {
			maps.Copy(resources, ociSelectors(env))
		} else {
			maps.Copy(resources, awsSelectors(env))
		}
		resources[service.IngressClassKey] = &fnv1.ResourceSelector{
			Kind:       "IngressClass",
			ApiVersion: "networking.k8s.io/v1",
			Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
		}
		resources[service.GatewayKey] = &fnv1.ResourceSelector{
			Kind:       "Gateway",
			ApiVersion: gatewayApiVersion,
			Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
		}
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
			resources[ns+service.HTTPRouteKey] = &fnv1.ResourceSelector{
				Kind:       "HTTPRoute",
				ApiVersion: gatewayApiVersion,
				Namespace:  &ns,
				Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
			}
		}
		return resources, nil
	default:
		return nil, nil
	}
}

func awsSelectors(env apis.Environment) map[string]*fnv1.ResourceSelector {
	resources := make(map[string]*fnv1.ResourceSelector)
	resources[service.VPCKey] = base.RequiredClusterVPC(env.VPC)
	if env.DataKMSAlias != "" {
		resources[service.KMSDataAliasKey] = &fnv1.ResourceSelector{
			Kind:       "Alias",
			ApiVersion: "kms.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.DataKMSAlias},
		}
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
	resources[service.SubnetsKey] = &fnv1.ResourceSelector{
		Kind:       "Subnet",
		ApiVersion: ec2ApiVersion,
		Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
	}
	resources[service.IngressClassParamsKey] = &fnv1.ResourceSelector{
		Kind:       "IngressClassParams",
		ApiVersion: ingressClassParamsApiVersion,
		Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
	}
	resources[service.LoadBalancerConfigurationKey] = &fnv1.ResourceSelector{
		Kind:       "LoadBalancerConfiguration",
		ApiVersion: lbConfigApiVersion,
		Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
	}
	return resources
}

func ociSelectors(env apis.Environment) map[string]*fnv1.ResourceSelector {
	resources := map[string]*fnv1.ResourceSelector{
		service.ClusterKey: {
			Kind:       "Cluster",
			ApiVersion: ociContainerEngineApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.Cluster},
		},
		service.SecurityGroupKey: {
			Kind:       "NetworkSecurityGroup",
			ApiVersion: ociNetworkingApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.SecurityGroup},
		},
		service.SubnetsKey: {
			Kind:       "Subnet",
			ApiVersion: ociNetworkingApiVersion,
			Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}},
		},
	}
	if env.DataKMSKey != "" {
		resources[service.KMSDataKeyKey] = &fnv1.ResourceSelector{
			Kind:       "Key",
			ApiVersion: "kms.oci.upbound.io/v1alpha1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.DataKMSKey},
		}
	}
	for _, name := range env.GetReferenceNodePools() {
		resources[service.GetReferenceNodePoolKey(name)] = &fnv1.ResourceSelector{
			Kind:       "NodePool",
			ApiVersion: ociContainerEngineApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: name},
		}
	}
	return resources
}

func (g *GroupImpl) GetObservedStatus(_ *composed.Unstructured) (map[string]interface{}, error) {
	return nil, nil
}
