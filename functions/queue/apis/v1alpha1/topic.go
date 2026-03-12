// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=kafka.entigo.com
// +versionName=v1alpha1

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Topic generates Kafka Topic resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type Topic struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TopicSpec   `json:"spec"`
	Status            TopicStatus `json:"status,omitempty"`
}

// TopicSpec specifies the desired state of a Topic.
type TopicSpec struct {
	// +kubebuilder:default=3
	Partitions int32 `json:"partitions,omitempty"`
	// +kubebuilder:default=3
	ReplicationFactor int32             `json:"replicationFactor,omitempty"`
	Config            map[string]string `json:"config,omitempty"`
	ClusterName       string            `json:"clusterName"`
}

// TopicStatus specifies the observed state of a Topic.
type TopicStatus struct{}
