// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=storage.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// S3Bucket generates S3 bucket resources
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced
type S3Bucket struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              S3BucketSpec   `json:"spec,omitempty"`
	Status            S3BucketStatus `json:"status,omitempty"`
}

type S3BucketSpec struct {
	// +kubebuilder:default=false
	EnableVersioning bool `json:"enableVersioning,omitempty"`
	// +kubebuilder:default=true
	CreateServiceAccount bool `json:"createServiceAccount,omitempty"`
	// +kubebuilder:default=""
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type S3BucketStatus struct {
	Region                       string `json:"region,omitempty"`
	S3Uri                        string `json:"s3Uri,omitempty"`
	S3Url                        string `json:"s3Url,omitempty"`
	KmsKeyAlias                  string `json:"kmsKeyAlias,omitempty"`
	KmsKeyId                     string `json:"kmsKeyId,omitempty"`
	BucketKeyEnabled             bool   `json:"bucketKeyEnabled,omitempty"`
	EncryptionType               string `json:"encryptionType,omitempty"`
	VersioningEnabled            bool   `json:"versioningEnabled,omitempty"`
	BlockPublicAclsEnabled       bool   `json:"blockPublicAclsEnabled,omitempty"`
	BlockPublicPolicyEnabled     bool   `json:"blockPublicPolicyEnabled,omitempty"`
	IgnorePublicAclsEnabled      bool   `json:"ignorePublicAclsEnabled,omitempty"`
	RestrictPublicBucketsEnabled bool   `json:"restrictPublicBucketsEnabled,omitempty"`
	AclStatus                    string `json:"aclStatus,omitempty"`
	ObjectOwnership              string `json:"objectOwnership,omitempty"`
	ServiceAccountName           string `json:"serviceAccountName,omitempty"`
}
