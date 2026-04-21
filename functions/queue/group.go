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
	environmentName = "platform-apis-queue"
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
		apis.XRKindMSK: {
			Instantiate: func() client.Object { return &v1alpha1.MSKInstance{} },
			Generate:    g.generateMSKInstance,
		},
		apis.XRKindTopic: {
			Instantiate: func() client.Object { return &v1alpha1.Topic{} },
			Generate:    g.generateTopic,
		},
		apis.XRKindKafkaUser: {
			Instantiate: func() client.Object { return &v1alpha1.KafkaUser{} },
			Generate:    g.generateKafkaUser,
		},
	}
}

func (g *GroupImpl) generateMSKInstance(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateMskInstanceObjects(*obj.(*v1alpha1.MSKInstance), required, observed)
}

func (g *GroupImpl) generateTopic(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateTopicObjects(*obj.(*v1alpha1.Topic), required, observed)
}

func (g *GroupImpl) generateKafkaUser(obj client.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	return service.GenerateKafkaUserObjects(*obj.(*v1alpha1.KafkaUser), required, observed)
}

func (g *GroupImpl) GetSequence(object client.Object) base.Sequence {
	switch object.GetObjectKind().GroupVersionKind().Kind {
	case apis.XRKindKafkaUser:
		return base.NewSequence(true,
			[]string{"k8s-secret-.*"},
			[]string{"aws-secret-.*"},
			[]string{"secret-version-.*"},
			[]string{"secret-policy-.*"},
			[]string{"scram-association-.*"},
			[]string{"acl-.*"},
		)
	default:
		return base.Sequence{}
	}
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	return ""
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindMSK:
		return g.getMSKRequiredResources(required)
	case apis.XRKindTopic:
		return g.getTopicRequiredResources(compositeResource)
	case apis.XRKindKafkaUser:
		return g.getKafkaUserRequiredResources(compositeResource, required)
	default:
		return nil, nil
	}
}

func (g *GroupImpl) getMSKRequiredResources(_ map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	return map[string]*fnv1.ResourceSelector{
		base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
	}, nil
}

func (g *GroupImpl) getTopicRequiredResources(compositeResource *composite.Unstructured) (map[string]*fnv1.ResourceSelector, error) {
	clusterName, _, _ := unstructured.NestedString(compositeResource.Object, "spec", "clusterName")
	return map[string]*fnv1.ResourceSelector{
		"MSKObserver": {
			Kind:       "MSK",
			ApiVersion: "queue.entigo.com/v1alpha1",
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: clusterName + "-observed"},
		},
	}, nil
}

func (g *GroupImpl) getKafkaUserRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
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
	clusterName, _, _ := unstructured.NestedString(compositeResource.Object, "spec", "clusterName")
	resources["MSKObserver"] = &fnv1.ResourceSelector{
		Kind:       "MSK",
		ApiVersion: "queue.entigo.com/v1alpha1",
		Match:      &fnv1.ResourceSelector_MatchName{MatchName: clusterName + "-observed"},
	}
	resources["KMSConfigKey"] = base.RequiredKMSKey(env.ConfigKMSKey, env.AWSProvider)
	return resources, nil
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	kind := observed.GetKind()

	if kind == "ClusterProviderConfig" && strings.HasPrefix(observed.GetAPIVersion(), "kafka.m.crossplane.io") {
		providerConfigName := observed.GetName()
		if providerConfigName == "" {
			return nil, nil
		}
		return map[string]interface{}{
			"providerConfig": providerConfigName,
		}, nil
	}

	if (kind != "Cluster" && kind != "ServerlessCluster") || !strings.HasPrefix(observed.GetAPIVersion(), "kafka.aws.upbound.io") {
		return nil, nil
	}

	brokers, _, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "bootstrapBrokersSaslIam")
	brokersScram, _, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "bootstrapBrokersSaslScram")
	arn, _, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "arn")

	if brokers == "" && arn == "" {
		return nil, nil
	}

	region := ""
	if colonParts := strings.Split(arn, ":"); len(colonParts) >= 4 {
		region = colonParts[3]
	}

	return map[string]interface{}{
		"brokers":      brokers,
		"brokersscram": brokersScram,
		"arn":          arn,
		"region":       region,
	}, nil
}
