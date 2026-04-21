package service

import (
	"fmt"

	common2 "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	// The official API package for the Kafka Topic
	kafkav1alpha1 "github.com/crossplane-contrib/provider-kafka/apis/namespaced/topic/v1alpha1"
	common "github.com/crossplane-contrib/provider-kafka/apis/v1alpha1"
)

func GenerateTopicObjects(
	topic v1alpha1.Topic,
	required map[string][]resource.Required,
	_ map[resource.Name]resource.ObservedComposed,
) (map[string]client.Object, error) {
	var msk v1alpha1.MSKInstance
	if err := base.ExtractRequiredResource(required, "MSKObserver", &msk); err != nil {
		return nil, fmt.Errorf("cannot get MSKObserver: %w", err)
	}

	if msk.Status.ProviderConfig == "" {
		return nil, fmt.Errorf("MSK for cluster %s is not ready: providerConfig is empty", topic.Spec.ClusterName)
	}

	partitions := int(topic.Spec.Partitions)
	if partitions == 0 {
		partitions = 3
	}
	replicationFactor := int(topic.Spec.ReplicationFactor)
	if replicationFactor == 0 {
		replicationFactor = 3
	}

	topicResource := &kafkav1alpha1.Topic{
		TypeMeta: metav1.TypeMeta{Kind: "Topic", APIVersion: "topic.kafka.m.crossplane.io/v1alpha1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      topic.Name,
			Namespace: topic.Namespace,
		},
		Spec: kafkav1alpha1.TopicSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &common2.ProviderConfigReference{
					Name: msk.Status.ProviderConfig,
					Kind: "ClusterProviderConfig",
				},
			},
			ForProvider: common.TopicParameters{
				Partitions:        partitions,
				ReplicationFactor: replicationFactor,
				Config:            topic.Spec.Config,
			},
		},
	}

	resourceName := "topic-" + topic.Name
	return map[string]client.Object{
		resourceName: topicResource,
	}, nil
}
