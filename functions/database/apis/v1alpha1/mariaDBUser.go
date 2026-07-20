// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MariaDBUser generates MariaDB role and grant resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type MariaDBUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              MariaDBUserSpec   `json:"spec"`
	Status            MariaDBUserStatus `json:"status,omitempty"`
}

type MariaDBUserStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type MariaDBUserSpec struct {
	Name        string                 `json:"name,omitempty"`
	InstanceRef MariaDBUserInstanceRef `json:"instanceRef"`
	DatabaseRef *MariaDBDatabaseRef    `json:"databaseRef,omitempty"`
	Table       *string                `json:"table,omitempty" default:"*"`
	Privileges  MariaDBUserPrivileges  `json:"privileges"`
	// +kubebuilder:default=5
	MaxConnectionsPerHour *int `json:"maxConnectionsPerHour,omitempty"`
	// +kubebuilder:default=20
	MaxQueriesPerHour *int `json:"maxQueriesPerHour,omitempty"`
	// +kubebuilder:default=10
	MaxUpdatesPerHour *int `json:"maxUpdatesPerHour,omitempty"`
	// +kubebuilder:default=2
	MaxUserConnections *int              `json:"maxUserConnections,omitempty"`
	Grant              *MariaDBUserGrant `json:"grant,omitempty"`
}

type MariaDBUserInstanceRef struct {
	Name string `json:"name"`
}

type MariaDBUserGrant struct {
	Users []string `json:"users,omitempty"`
}

type MariaDBDatabaseRef struct {
	Name      string  `json:"name"`
	Namespace *string `json:"namespace,omitempty"`
}

// MariaDBUserPrivileges is a list of the privileges to be granted
// +kubebuilder:validation:MinItems:=1
type MariaDBUserPrivileges []MariaDBUserPrivilege

// MariaDBUserPrivilege represents a privilege to be granted
// +kubebuilder:validation:Pattern:="^([A-Z0-9_ ]+|[A-Z_ ]+ [(]`[^`]+`(, `[^`]+`)*[)])$"
type MariaDBUserPrivilege string

// ToStringSlice converts the slice of privileges to strings
func (gp *MariaDBUserPrivileges) ToStringSlice() []string {
	if gp == nil {
		return []string{}
	}
	out := make([]string, len(*gp))
	for i, v := range *gp {
		out[i] = string(v)
	}
	return out
}
