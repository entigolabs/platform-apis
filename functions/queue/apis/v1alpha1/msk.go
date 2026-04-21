// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=queue.entigo.com
// +versionName=v1alpha1

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MSKInstance generates Managed Streaming for Apache Kafka resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type MSKInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MSKSpec   `json:"spec"`
	Status            MSKStatus `json:"status,omitempty"`
}

// MSKSpec specifies the desired state of MSK.
type MSKSpec struct {
	ClusterARN string `json:"clusterARN"`
}

// MSKStatus specifies the observed state of MSK.
type MSKStatus struct {
	Brokers        string `json:"brokers,omitempty"`
	BrokersScram   string `json:"brokersscram,omitempty"`
	ARN            string `json:"arn,omitempty"`
	Region         string `json:"region,omitempty"`
	ProviderConfig string `json:"providerConfig,omitempty"`
}
