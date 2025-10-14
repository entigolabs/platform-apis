// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=artifact.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// Repository generates OCI repository resources
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Status            RepositoryStatus `json:"status,omitempty"`
}

type RepositoryStatus struct {
	RepositoryUri string `json:"repositoryUri,omitempty"`
}
