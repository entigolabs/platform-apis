// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ValkeyInstance generates Valkey (ElastiCache) resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type ValkeyInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ValkeyInstanceSpec   `json:"spec"`
	Status            ValkeyInstanceStatus `json:"status,omitempty"`
}

type ValkeyInstanceSpec struct {
	// +kubebuilder:default=true
	DeletionProtection bool `json:"deletionProtection,omitempty"`
	// +kubebuilder:default="8.2"
	EngineVersion string `json:"engineVersion"`
	// +kubebuilder:default="cache.t4g.small"
	InstanceType string `json:"instanceType"`
	// +kubebuilder:default=2
	// +kubebuilder:validation:Minimum=2
	// +kubebuilder:validation:Maximum=6
	NumCacheClusters float64 `json:"numCacheClusters,omitempty"`
	// +kubebuilder:default=true
	AutoMinorVersionUpgrade bool `json:"autoMinorVersionUpgrade,omitempty"`
	// +kubebuilder:default="sun:05:00-sun:06:00"
	MaintenanceWindow string `json:"maintenanceWindow,omitempty"`
	// +kubebuilder:default="03:00-05:00"
	SnapshotWindow string `json:"snapshotWindow,omitempty"`
	// +kubebuilder:default=7
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=35
	SnapshotRetentionLimit float64 `json:"snapshotRetentionLimit,omitempty"`
	ParameterGroupName     string  `json:"parameterGroupName,omitempty"`
}

type ValkeyInstanceStatus struct {
	Endpoint                *ValkeyInstanceEndpoint      `json:"endpoint,omitempty"`
	AutoMinorVersionUpgrade bool                         `json:"autoMinorVersionUpgrade,omitempty"`
	KMSKeyID                string                       `json:"kmsKeyId,omitempty"`
	KMSKeyAlias             string                       `json:"kmsKeyAlias,omitempty"`
	MultiAZEnabled          bool                         `json:"multiAZenabled,omitempty"`
	ParameterGroupName      string                       `json:"parameterGroupName,omitempty"`
	SecurityGroup           *ValkeyInstanceSecurityGroup `json:"securityGroup,omitempty"`
}

type ValkeyInstanceEndpoint struct {
	Address string  `json:"address,omitempty"`
	Port    float64 `json:"port,omitempty"`
}

type ValkeyInstanceSecurityGroup struct {
	Name        string                            `json:"name,omitempty"`
	Description string                            `json:"description,omitempty"`
	ID          string                            `json:"id,omitempty"`
	Arn         string                            `json:"arn,omitempty"`
	Rules       []ValkeyInstanceSecurityGroupRule `json:"rules,omitempty"`
}

type ValkeyInstanceSecurityGroupRule struct {
	CidrBlocks  []string `json:"cidrBlocks,omitempty"`
	Description string   `json:"description,omitempty"`
	FromPort    int      `json:"fromPort,omitempty"`
	ToPort      int      `json:"toPort,omitempty"`
	Protocol    string   `json:"protocol,omitempty"`
	Self        bool     `json:"self,omitempty"`
	Type        string   `json:"type,omitempty"`
}
