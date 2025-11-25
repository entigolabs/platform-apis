// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=tenancy.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// Zone generates zone resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type Zone struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ZoneSpec `json:"spec"`
}

// ZoneSpec defines the desired state of Zone
type ZoneSpec struct {
	// Enable cluster-level permissions for the zone
	// +kubebuilder:default=false
	ClusterPermissions bool `json:"clusterPermissions,omitempty"`

	AppProject *AppProject `json:"appProject,omitempty"`

	// List of namespaces to manage as part of the Zone. At least one namespace must be specified.
	// +kubebuilder:validation:MinItems=1
	Namespaces []Namespace `json:"namespaces"`

	// List of node pool configurations for cluster-autoscaler
	// +kubebuilder:validation:MinItems=1
	Pools []Pool `json:"pools"`
}

type AppProject struct {
	// OIDC groups with full access to applications
	ContributorGroups []string `json:"contributorGroups,omitempty"`
}

type Namespace struct {
	// Name of the namespace.
	Name string `json:"name"`

	// Name of the Node Pool where to schedule workloads. If not specified, the first pool is used.
	Pool string `json:"pool,omitempty"`
}

type Pool struct {
	// Name of the node pool
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`

	// Node pool requirements including instance type, capacity type, zone, and scaling limits
	// +kubebuilder:validation:MinItems=1
	Requirements []Requirement `json:"requirements"`
}

type Requirement struct {
	// Requirement key
	// +kubebuilder:validation:Enum=instance-type;capacity-type;zone;min-size;max-size;desired-size
	Key string `json:"key"`

	// Single value for capacity-type, min-size, max-size, or desired-size. Can be string or integer.
	Value string `json:"value,omitempty"`

	// Array of values for instance-type or availability-zone
	Values []string `json:"values,omitempty"`
}
