// Package v1alpha2 holds the Portable Data Service API kinds this function composes.
// +kubebuilder:object:generate=true
// +groupName=database.entigo.com
// +versionName=v1alpha2
package v1alpha2

import (
	"k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PostgreSQLCluster is one PostgreSQL database cluster. Its schema is the composite resource
// definition in compositions/postgresqlcluster; these types mirror it for the function.
// +kubebuilder:object:root=true
type PostgreSQLCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              PostgreSQLClusterSpec   `json:"spec"`
	Status            PostgreSQLClusterStatus `json:"status,omitempty"`
}

// PostgreSQLClusterSpec holds the common fields every backend runs the same way, and the
// provider configuration entries of the backends the cluster may run on.
type PostgreSQLClusterSpec struct {
	ClassName          *string             `json:"className,omitempty"`
	Observe            *bool               `json:"observe,omitempty"`
	Version            *string             `json:"version,omitempty"`
	Storage            *Storage            `json:"storage,omitempty"`
	HighAvailability   *bool               `json:"highAvailability,omitempty"`
	DeletionProtection *bool               `json:"deletionProtection,omitempty"`
	Backup             *Backup             `json:"backup,omitempty"`
	ParametersRefs     []LocalReference    `json:"parametersRefs,omitempty"`
	Parameters         map[string]string   `json:"parameters,omitempty"`
	Restore            *Restore            `json:"restore,omitempty"`
	SecretName         *string             `json:"secretName,omitempty"`
	ProviderConfigs    []ProviderConfigEntry `json:"providerConfigs,omitempty"`
}

// Storage is the cluster's disk.
type Storage struct {
	SizeGB *int64 `json:"sizeGB,omitempty"`
}

// Backup is the cluster's automated backups.
type Backup struct {
	RetentionDays *int64 `json:"retentionDays,omitempty"`
}

// Restore creates the cluster from another cluster's backups.
type Restore struct {
	FromInstance string  `json:"fromInstance"`
	PointInTime  *string `json:"pointInTime,omitempty"`
}

// LocalReference names an object in the cluster's namespace.
type LocalReference struct {
	Name string `json:"name"`
}

// ProviderConfigEntry holds one backend's settings, inline in Spec or in a separate object of its
// kind named by Ref. Spec stays raw: only the backend that applies it knows its schema.
type ProviderConfigEntry struct {
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Spec       *runtime.RawExtension `json:"spec,omitempty"`
	Ref        *LocalReference       `json:"ref,omitempty"`
}

// PostgreSQLClusterStatus reports the cluster as its backend runs it.
type PostgreSQLClusterStatus struct {
	Conditions       []metav1.Condition    `json:"conditions,omitempty"`
	Version          string                `json:"version,omitempty"`
	Storage          *StorageStatus        `json:"storage,omitempty"`
	HighAvailability *bool                 `json:"highAvailability,omitempty"`
	Backup           *BackupStatus         `json:"backup,omitempty"`
	Endpoint         *Endpoint             `json:"endpoint,omitempty"`
	ConnectionSecret string                `json:"connectionSecret,omitempty"`
	ProviderConfig   *ProviderConfigStatus `json:"providerConfig,omitempty"`
	Effective        map[string]Effective  `json:"effective,omitempty"`
	Provider         *runtime.RawExtension `json:"provider,omitempty"`
}

// StorageStatus is the disk's actual size.
type StorageStatus struct {
	SizeGB int64 `json:"sizeGB,omitempty"`
}

// BackupStatus is the backups' actual retention.
type BackupStatus struct {
	RetentionDays int64 `json:"retentionDays,omitempty"`
}

// Endpoint is the address applications connect to.
type Endpoint struct {
	Host string `json:"host,omitempty"`
	Port int64  `json:"port,omitempty"`
}

// ProviderConfigStatus names the entry applied and the entries ignored, as "<apiVersion>, <kind>".
type ProviderConfigStatus struct {
	Applied string   `json:"applied,omitempty"`
	Ignored []string `json:"ignored,omitempty"`
}

// Effective is one effective value and where it came from.
type Effective struct {
	Value  runtime.RawExtension `json:"value"`
	Source string               `json:"source"`
}
