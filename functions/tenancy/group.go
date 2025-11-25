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
	environmentName = "platform-apis-database"
)

type GroupImpl struct {
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindZone: {
			Instantiate: func() runtime.Object { return &v1alpha1.Zone{} },
			Generate:    g.generateZone,
		},
	}
}

func (g *GroupImpl) generateZone(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateZoneObjects(*obj.(*v1alpha1.Zone), required, observed)
}

func (g *GroupImpl) GetSequence(object runtime.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindZone:
		return base.NewSequence(true, []string{"namespace-.*", "launchtemplate-.*"}, []string{"netpol-.*", "role-.*"},
			[]string{"rbacrole-.*", "rpa-.*"}, []string{"rb-.*", "nodepool-.*"})
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
		resources["VPC"] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: "ec2.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.VPC},
		}
		resources["KMSDataAlias"] = &fnv1.ResourceSelector{
			Kind:       "Alias",
			ApiVersion: "kms.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.DataKMSAlias},
		}
		resources["NodeSecurityGroup"] = &fnv1.ResourceSelector{
			Kind:       "SecurityGroup",
			ApiVersion: "ec2.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.SecurityGroup},
		}
		resources["Cluster"] = &fnv1.ResourceSelector{
			Kind:       "Cluster",
			ApiVersion: "eks.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.Cluster},
		}
		resources["Subnets"] = &fnv1.ResourceSelector{
			Kind:       "Subnet",
			ApiVersion: "ec2.aws.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": env.SubnetType}}},
		}
		return resources, nil
	default:
		return nil, nil
	}
}

func (g *GroupImpl) GetObservedStatus(_ *composed.Unstructured) (map[string]interface{}, error) {
	return nil, nil
}
