package service

import (
	"fmt"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func GenerateTopicObjects(
	topic v1alpha1.Topic,
	required map[string][]resource.Required,
	_ map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	var msk v1alpha1.MSKInstance
	if err := base.ExtractRequiredResource(required, "MSKObserver", &msk); err != nil {
		return nil, fmt.Errorf("cannot get MSKObserver: %w", err)
	}

	if msk.Status.ProviderConfig == "" {
		return nil, fmt.Errorf("MSK for cluster %s is not ready: providerConfig is empty", topic.Spec.ClusterName)
	}

	partitions := int64(topic.Spec.Partitions)
	if partitions == 0 {
		partitions = 3
	}
	replicationFactor := int64(topic.Spec.ReplicationFactor)
	if replicationFactor == 0 {
		replicationFactor = 3
	}

	forProvider := map[string]interface{}{
		"partitions":        partitions,
		"replicationFactor": replicationFactor,
	}
	if len(topic.Spec.Config) > 0 {
		configMap := make(map[string]interface{}, len(topic.Spec.Config))
		for k, v := range topic.Spec.Config {
			configMap[k] = v
		}
		forProvider["config"] = configMap
	}

	topicResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "topic.kafka.m.crossplane.io/v1alpha1",
			"kind":       "Topic",
			"metadata": map[string]interface{}{
				"name":      topic.Name,
				"namespace": topic.Namespace,
			},
			"spec": map[string]interface{}{
				"forProvider": forProvider,
				"providerConfigRef": map[string]interface{}{
					"kind": "ClusterProviderConfig",
					"name": msk.Status.ProviderConfig,
				},
			},
		},
	}

	resourceName := "topic-" + topic.Name
	return map[string]runtime.Object{
		resourceName: topicResource,
	}, nil
}
