// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLDatabase generates PostgreSQL database, grant and extension resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type PostgreSQLDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PostgreSQLDatabaseSpec   `json:"spec"`
	Status            PostgreSQLDatabaseStatus `json:"status,omitempty"`
}

type PostgreSQLDatabaseSpec struct {
	Owner           string                               `json:"owner"`
	InstanceRef     PostgreSQLDatabaseInstanceRef        `json:"instanceRef"`
	Encoding        string                               `json:"encoding,omitempty"`
	LCCType         string                               `json:"lcCType,omitempty"`
	LCCollate       string                               `json:"lcCollate,omitempty"`
	DBTemplate      string                               `json:"dbTemplate,omitempty"`
	Extensions      []string                             `json:"extensions,omitempty"`
	ExtensionConfig map[string]PostgreSQLExtensionConfig `json:"extensionConfig,omitempty"`
}

type PostgreSQLDatabaseInstanceRef struct {
	Name string `json:"name"`
}

type PostgreSQLExtensionConfig struct {
	Schema string `json:"schema,omitempty"`
}

type PostgreSQLDatabaseStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}
