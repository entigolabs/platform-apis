package main

import (
	"strings"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	environmentName = "platform-apis-kafka"
)

type GroupImpl struct{}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) SkipGeneration(_ *composite.Unstructured) bool {
	return false
}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindMSK: {
			Instantiate: func() runtime.Object { return &v1alpha1.MSKInstance{} },
			Generate:    g.generateMSKInstance,
		},
		apis.XRKindTopic: {
			Instantiate: func() runtime.Object { return &v1alpha1.Topic{} },
			Generate:    g.generateTopic,
		},
		apis.XRKindKafkaUser: {
			Instantiate: func() runtime.Object { return &v1alpha1.KafkaUser{} },
			Generate:    g.generateKafkaUser,
		},
	}
}

func (g *GroupImpl) generateMSKInstance(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateMskInstanceObjects(*obj.(*v1alpha1.MSKInstance), required, observed)
}

func (g *GroupImpl) generateTopic(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateTopicObjects(*obj.(*v1alpha1.Topic), required, observed)
}

func (g *GroupImpl) generateKafkaUser(obj runtime.Object, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	return service.GenerateKafkaUserObjects(*obj.(*v1alpha1.KafkaUser), required, observed)
}

func (g *GroupImpl) GetSequence(object runtime.Object) base.Sequence {
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
	switch observed.GetKind() {
	case "Secret":
		return resource.ReadyTrue
	case "ClusterProviderConfig":
		return resource.ReadyTrue
	default:
		return ""
	}
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindMSK:
		return nil, nil
	case apis.XRKindTopic:
		return g.getTopicRequiredResources(compositeResource)
	case apis.XRKindKafkaUser:
		return g.getKafkaUserRequiredResources(compositeResource, required)
	default:
		return nil, nil
	}
}

func (g *GroupImpl) getTopicRequiredResources(compositeResource *composite.Unstructured) (map[string]*fnv1.ResourceSelector, error) {
	clusterName, _, _ := unstructured.NestedString(compositeResource.Object, "spec", "clusterName")
	return map[string]*fnv1.ResourceSelector{
		"MSKObserver": {
			Kind:       "MSK",
			ApiVersion: "kafka.entigo.com/v1alpha1",
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
		ApiVersion: "kafka.entigo.com/v1alpha1",
		Match:      &fnv1.ResourceSelector_MatchName{MatchName: clusterName + "-observed"},
	}
	resources["KMSConfigKey"] = base.RequiredKMSKey("config", env.AWSProvider)
	return resources, nil
}

func (g *GroupImpl) GetObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	if observed.GetKind() != "Cluster" || !strings.HasPrefix(observed.GetAPIVersion(), "kafka.aws.upbound.io") {
		return nil, nil
	}

	brokers, _, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "bootstrapBrokersSaslIam")
	brokersScram, _, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "bootstrapBrokersSaslScram")
	arn, _, _ := unstructured.NestedString(observed.Object, "status", "atProvider", "arn")

	if brokers == "" && arn == "" {
		return nil, nil
	}

	region := ""
	providerConfig := ""
	externalName := ""
	if annotations := observed.GetAnnotations(); annotations != nil {
		externalName = annotations["crossplane.io/external-name"]
	}
	if externalName != "" {
		if colonParts := strings.Split(externalName, ":"); len(colonParts) >= 4 {
			region = colonParts[3]
		}
		if slashParts := strings.Split(externalName, "/"); len(slashParts) >= 2 {
			providerConfig = slashParts[1] + "-observed"
		}
	}

	return map[string]interface{}{
		"brokers":        brokers,
		"brokersscram":   brokersScram,
		"arn":            arn,
		"region":         region,
		"providerConfig": providerConfig,
	}, nil
}
