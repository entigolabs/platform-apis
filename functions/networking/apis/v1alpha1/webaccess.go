// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=networking.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// WebAccess generates Istio resources to provide web access to workload.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type WebAccess struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WebAccessSpec `json:"spec"`
}

type WebAccessSpec struct {
	Gateway string   `json:"gateway,omitempty"`
	Domain  string   `json:"domain"`
	Aliases []string `json:"aliases,omitempty"`
	Paths   []Path   `json:"paths"`
}

type Path struct {
	Path      string `json:"path"`
	Host      string `json:"host"`
	Namespace string `json:"namespace,omitempty"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port uint32 `json:"port"`
	// +kubebuilder:validation:Enum=Prefix;Exact
	PathType   string `json:"pathType"`
	TargetPath string `json:"targetPath,omitempty"`
}
