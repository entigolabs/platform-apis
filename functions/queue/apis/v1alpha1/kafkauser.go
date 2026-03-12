// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=kafka.entigo.com
// +versionName=v1alpha1

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// KafkaUser generates Kafka user resources including SCRAM credentials and ACLs.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type KafkaUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              KafkaUserSpec   `json:"spec"`
	Status            KafkaUserStatus `json:"status,omitempty"`
}

// KafkaUserSpec specifies the desired state of a KafkaUser.
type KafkaUserSpec struct {
	ClusterName    string     `json:"clusterName"`
	ConsumerGroups []string   `json:"consumerGroups,omitempty"`
	ACLs           []KafkaACL `json:"acls,omitempty"`
}

// KafkaACL defines an access control list entry for a Kafka topic.
type KafkaACL struct {
	Topic     string `json:"topic"`
	Operation string `json:"operation"`
}

// KafkaUserStatus specifies the observed state of a KafkaUser.
type KafkaUserStatus struct{}
