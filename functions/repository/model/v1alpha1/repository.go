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
// +kubebuilder:resource:scope=Cluster
type Repository struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RepositorySpec   `json:"spec"`
	Status            RepositoryStatus `json:"status,omitempty"`
}

type RepositorySpec struct {
	ImageScanningConfiguration ImageScanningConfiguration `json:"imageScanningConfiguration,omitempty"`
	// +kubebuilder:validation:Enum=IMMUTABLE;MUTABLE
	ImageTagMutability *ImageTagMutability `json:"imageTagMutability,omitempty"`
	Tags               []Tag               `json:"tags,omitempty"`
}

type ImageScanningConfiguration struct {
	ScanOnPush *bool `json:"scanOnPush"`
}

type ImageTagMutability string

const (
	ImageTagMutabilityImmutable ImageTagMutability = "IMMUTABLE"
	ImageTagMutabilityMutable   ImageTagMutability = "MUTABLE"
)

type Tag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type RepositoryStatus struct {
	RepositoryUri string `json:"repositoryUri,omitempty"`
}
