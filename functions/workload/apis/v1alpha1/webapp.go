// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=workload.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// WebApp generates deployment resources to run web applications.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type WebApp struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WebAppSpec `json:"spec"`
}

type WebAppSpec struct {
	WorkloadSpec `json:",inline"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:default=1
	Replicas int32 `json:"replicas,omitempty"`
}

func (w *WebApp) GetName() string {
	return w.Name
}

func (w *WebApp) GetNamespace() string {
	return w.Namespace
}

func (w *WebApp) GetWorkloadSpec() *WorkloadSpec {
	return &w.Spec.WorkloadSpec
}
