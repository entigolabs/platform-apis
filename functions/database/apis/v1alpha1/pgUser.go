// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLUser generates PostgreSQL role and grant resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type PostgreSQLUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PostgreSQLUserSpec   `json:"spec"`
	Status            PostgreSQLUserStatus `json:"status,omitempty"`
}

type PostgreSQLUserStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type PostgreSQLUserSpec struct {
	Name        string                    `json:"name,omitempty"`
	InstanceRef PostgreSQLUserInstanceRef `json:"instanceRef"`
	// +kubebuilder:default=true
	Login bool `json:"login"`
	// +kubebuilder:default=false
	CreateDb bool `json:"createDb"`
	// +kubebuilder:default=false
	CreateRole bool `json:"createRole"`
	// +kubebuilder:default=true
	Inherit bool                 `json:"inherit"`
	Grant   *PostgreSQLUserGrant `json:"grant,omitempty"`
}

type PostgreSQLUserInstanceRef struct {
	Name string `json:"name"`
}

type PostgreSQLUserGrant struct {
	Roles []string `json:"roles,omitempty"`
}
