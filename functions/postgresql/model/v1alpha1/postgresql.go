// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// PostgreSQL generates PostgreSQL database resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type PostgreSQL struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PostgreSQLSpec   `json:"spec"`
	Status            PostgreSQLStatus `json:"status,omitempty"`
}

type PostgreSQLSpec struct {
	// +kubebuilder:default=20
	AllocatedStorage float64 `json:"allocatedStorage"`
	// +kubebuilder:default=false
	AllowMajorVersionUpgrade bool `json:"allowMajorVersionUpgrade,omitempty"`
	// +kubebuilder:default=true
	AutoMinorVersionUpgrade bool   `json:"autoMinorVersionUpgrade,omitempty"`
	BackupWindow            string `json:"backupWindow,omitempty"`
	// +kubebuilder:default=true
	CreateExternalSecret bool `json:"createExternalSecret,omitempty"`
	// +kubebuilder:default=true
	DeletionProtection bool   `json:"deletionProtection"`
	EngineVersion      string `json:"engineVersion"`
	InstanceClass      string `json:"instanceClass"`
	Iops               string `json:"iops,omitempty"`
	MaintenanceWindow  string `json:"maintenanceWindow,omitempty"`
	// +kubebuilder:default=false
	MultiAZ            bool   `json:"multiAZ"`
	ParameterGroupName string `json:"parameterGroupName,omitempty"`
	// +kubebuilder:default=false
	PerformanceInsightsEnabled bool `json:"performanceInsightsEnabled,omitempty"`
	// +kubebuilder:default=gp3
	StorageType           string   `json:"storageType"`
	VpcSecurityGroupIDRef []string `json:"vpcSecurityGroupIdRef,omitempty"`
}

type PostgreSQLStatus struct {
	Conditions                 []metav1.Condition `json:"conditions,omitempty"`
	AllowMajorVersionUpgrade   bool               `json:"allowMajorVersionUpgrade"`
	AutoMinorVersionUpgrade    bool               `json:"autoMinorVersionUpgrade"`
	BackupWindow               string             `json:"backupWindow,omitempty"`
	Endpoint                   PostgreSQLEndpoint `json:"endpoint,omitempty"`
	Iops                       float64            `json:"iops,omitempty"`
	KMSKeyID                   string             `json:"kmsKeyId,omitempty"`
	LatestRestorableTime       *metav1.Time       `json:"latestRestorableTime,omitempty"`
	MaintenanceWindow          string             `json:"maintenanceWindow,omitempty"`
	ParameterGroupName         string             `json:"parameterGroupName,omitempty"`
	PerformanceInsightsEnabled bool               `json:"performanceInsightsEnabled"`
	ResourceID                 string             `json:"resourceId,omitempty"`
	Status                     string             `json:"status,omitempty"`
	StorageEncrypted           bool               `json:"storageEncrypted"`
	StorageThroughput          float64            `json:"storageThroughput,omitempty"`
	VpcSecurityGroupIds        []string           `json:"vpcSecurityGroupIds,omitempty"`
}

type PostgreSQLEndpoint struct {
	Address      string  `json:"address"`
	HostedZoneID string  `json:"hostedZoneId"`
	Port         float64 `json:"port"`
}
