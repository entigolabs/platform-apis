// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MariaDBDatabase generates MariaDB database, grant and extension resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type MariaDBDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MariaDBDatabaseSpec   `json:"spec"`
	Status            MariaDBDatabaseStatus `json:"status,omitempty"`
}

type MariaDBDatabaseSpec struct {
	Name        string                     `json:"name,omitempty"`
	InstanceRef MariaDBDatabaseInstanceRef `json:"instanceRef"`
	//DefaultCharacterSet *string                    `json:"defaultCharacterSet,omitempty"`
	//DefaultCollation    *string                    `json:"defaultCollation,omitempty"`
	// +kubebuilder:default=true
	DeletionProtection bool `json:"deletionProtection"`
}

type MariaDBDatabaseInstanceRef struct {
	Name string `json:"name"`
}

type MariaDBDatabaseStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
