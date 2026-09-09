package main

import (
	"fmt"
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
	mqv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/mq/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	environmentName = "platform-apis-mq"
	ec2ApiVersion   = "ec2.aws.m.upbound.io/v1beta1"
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
		apis.XRKindRabbitMQBroker: {
			Instantiate: func() client.Object { return &v1alpha1.RabbitMQBroker{} },
			Generate:    g.generateRabbitMQBroker,
		},
	}
}

func (g *GroupImpl) generateRabbitMQBroker(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateRabbitMQBrokerObjects(*obj.(*v1alpha1.RabbitMQBroker), required, observed)
}

func (g *GroupImpl) GetSequence(object client.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindRabbitMQBroker:
		broker := *object.(*v1alpha1.RabbitMQBroker)
		setHash := base.GenerateFNVHash(broker.GetUID())
		sg := service.GetSGName(broker.GetName(), setHash)
		sgIngress := service.GetSGIngressName(broker.GetName(), setHash)
		sgConsoleIngress := service.GetSGConsoleIngressName(broker.GetName(), setHash)
		sgEgress := service.GetSGEgressName(broker.GetName(), setHash)
		mqBroker := service.GetBrokerName(broker.GetName(), setHash)
		firstGroup := []string{sg, sgIngress, sgConsoleIngress, sgEgress, "credentials"}
		if broker.Spec.Configuration != nil {
			engineVersion := ""
			if broker.Spec.EngineVersion != nil {
				engineVersion = *broker.Spec.EngineVersion
			}
			firstGroup = append(firstGroup, service.GetConfigurationName(broker.GetName(), engineVersion, setHash))
		}
		return base.NewSequence(true,
			firstGroup,
			[]string{mqBroker},
		)
	default:
		return base.Sequence{}
	}
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	switch observed.GetKind() {
	case "Broker":
		return service.GetRabbitMQBrokerReadyStatus(observed)
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
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

	switch compositeResource.GetKind() {
	case apis.XRKindRabbitMQBroker:
		resources["VPC"] = &fnv1.ResourceSelector{
			Kind:       "VPC",
			ApiVersion: ec2ApiVersion,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.VPC},
			Namespace:  &env.AWSProvider,
		}
		if env.ConfigKMSKey != "" {
			resources["KMSConfigKey"] = base.RequiredKMSKey(env.ConfigKMSKey, env.AWSProvider)
		}
		resources["MQSubnetGroup"] = &fnv1.ResourceSelector{
			Kind:       "SubnetGroup",
			ApiVersion: "rds.aws.m.upbound.io/v1beta1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: env.SubnetGroup},
			Namespace:  &env.AWSProvider,
		}
	}
	return resources, nil
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	switch {
	case observed.GetKind() == "Broker" && strings.HasPrefix(observed.GetAPIVersion(), "mq.aws.m.upbound.io"):
		return getMQBrokerStatus(observed)
	default:
		return nil, nil
	}
}

func getMQBrokerStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	var broker mqv1beta1.Broker
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observed.Object, &broker); err != nil {
		return nil, fmt.Errorf("cannot convert Broker object to MQ Broker: %w", err)
	}
	return runtime.DefaultUnstructuredConverter.ToUnstructured(new(service.GetRabbitMQBrokerStatusFromBroker(broker)))
}
