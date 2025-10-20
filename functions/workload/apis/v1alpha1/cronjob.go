// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=workload.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// This isn't a custom resource, in the sense that we never install its CRD.
// It is a KRM-like object, so we generate a CRD to describe its schema.

// CronJob generates deployment resources to run jobs.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type CronJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CronJobSpec `json:"spec"`
}

type CronJobSpec struct {
	WorkloadSpec `json:",inline"`
	Schedule     string `json:"schedule"`
	// +kubebuilder:validation:Enum=Allow;Forbid;Replace
	ConcurrencyPolicy string `json:"concurrencyPolicy"`
}

func (c *CronJob) GetName() string {
	return c.Name
}

func (c *CronJob) GetNamespace() string {
	return c.Namespace
}

func (c *CronJob) GetWorkloadSpec() *WorkloadSpec {
	return &c.Spec.WorkloadSpec
}
