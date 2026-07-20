// Package v1alpha1 contains the input type for this Function
// +kubebuilder:object:generate=true
// +groupName=mq.entigo.com
// +versionName=v1alpha1
package v1alpha1

import (
	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RabbitMQBroker generates RabbitMQ broker resources.
// +kubebuilder:object:root=true
// +kubebuilder:storageversion
type RabbitMQBroker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              RabbitMQBrokerSpec   `json:"spec"`
	Status            RabbitMQBrokerStatus `json:"status,omitempty"`
}

type RabbitMQBrokerSpec struct {
	// Whether to automatically upgrade to new minor versions of brokers as Amazon MQ makes releases available.
	// +kubebuilder:default=true
	AutoMinorVersionUpgrade bool `json:"autoMinorVersionUpgrade,omitempty"`
	// Name of the broker.
	BrokerName *string `json:"brokerName,omitempty"`
	// Configuration block for broker configuration.
	Configuration *RabbitMQBrokerConfiguration `json:"configuration,omitempty"`
	// Deployment mode of the broker. Valid values are SINGLE_INSTANCE, ACTIVE_STANDBY_MULTI_AZ, and CLUSTER_MULTI_AZ. Default is SINGLE_INSTANCE.
	// +kubebuilder:default="SINGLE_INSTANCE"
	DeploymentMode *string `json:"deploymentMode,omitempty"`
	// +kubebuilder:default="RabbitMQ"
	EngineType *string `json:"engineType,omitempty"`
	// Version of the broker engine.
	// +kubebuilder:default="4.2"
	EngineVersion *string `json:"engineVersion,omitempty"`
	InstanceType  string  `json:"instanceType"`
	// Configuration block for the maintenance window start time.
	MaintenanceWindowStartTime *RabbitMQBrokerMaintenanceWindowStartTime `json:"maintenanceWindowStartTime,omitempty"`
}

type RabbitMQBrokerStatus struct {
	Conditions                 []metav1.Condition                                   `json:"conditions,omitempty"`
	BrokerName                 *string                                              `json:"brokerName,omitempty"`
	AutoMinorVersionUpgrade    bool                                                 `json:"autoMinorVersionUpgrade,omitempty"`
	Configuration              *RabbitMQBrokerConfigurationObservation              `json:"configuration,omitempty"`
	DeploymentMode             *string                                              `json:"deploymentMode,omitempty"`
	EncryptionOptions          *RabbitMQBrokerEncryptionOptionsObservation          `json:"encryptionOptions,omitempty"`
	EngineType                 *string                                              `json:"engineType,omitempty"`
	EngineVersion              *string                                              `json:"engineVersion,omitempty"`
	InstanceType               string                                               `json:"instanceType"`
	AmazonMQBrokerID           *string                                              `json:"amazonMQBrokerID,omitempty"`
	Instances                  []RabbitMQBrokerInstancesObservation                 `json:"instances,omitempty"`
	MaintenanceWindowStartTime *RabbitMQBrokerMaintenanceWindowStartTimeObservation `json:"maintenanceWindowStartTime,omitempty"`
	PendingDataReplicationMode *string                                              `json:"pendingDataReplicationMode,omitempty"`
	PubliclyAccessible         *bool                                                `json:"publiclyAccessible,omitempty"`
	Region                     *string                                              `json:"region,omitempty"`
	SecurityGroups             []*string                                            `json:"securityGroups,omitempty"`
	StorageType                *string                                              `json:"storageType,omitempty"`
	SubnetIds                  []*string                                            `json:"subnetIds,omitempty"`
}

type RabbitMQBrokerConfiguration struct {
	// Configuration ID.
	// +crossplane:generate:reference:type=github.com/upbound/provider-aws/v2/apis/namespaced/mq/v1beta1.Configuration
	// +crossplane:generate:reference:extractor=github.com/crossplane/upjet/v2/pkg/resource.ExtractResourceID()
	// +kubebuilder:validation:Optional
	ID *string `json:"id,omitempty"`
	// Reference to a Configuration in mq to populate id.
	// +kubebuilder:validation:Optional
	IDRef *v1.NamespacedReference `json:"idRef,omitempty"`
	// Selector for a Configuration in mq to populate id.
	// +kubebuilder:validation:Optional
	IDSelector *v1.NamespacedSelector `json:"idSelector,omitempty"`
	// Revision of the Configuration.
	// +kubebuilder:validation:Optional
	Revision *float64 `json:"revision,omitempty"`
}

type RabbitMQBrokerMaintenanceWindowStartTime struct {
	// Day of the week, e.g., MONDAY, TUESDAY, or WEDNESDAY.
	// +kubebuilder:validation:Optional
	DayOfWeek *string `json:"dayOfWeek"`
	// Time, in 24-hour format, e.g., 02:00.
	// +kubebuilder:validation:Optional
	TimeOfDay *string `json:"timeOfDay"`
	// Time zone in either the Country/City format or the UTC offset format, e.g., CET.
	// +kubebuilder:validation:Optional
	TimeZone *string `json:"timeZone"`
}

type RabbitMQBrokerConfigurationObservation struct {
	// Configuration ID.
	ID *string `json:"id,omitempty"`
	// Revision of the Configuration.
	Revision *float64 `json:"revision,omitempty"`
}

type RabbitMQBrokerEncryptionOptionsObservation struct {
	// ARN of KMS CMK to use for encryption at rest. Requires setting use_aws_owned_key to false. To perform drift detection when AWS-managed CMKs or customer-managed CMKs are in use, this value must be configured.
	KMSKeyID *string `json:"kmsKeyId,omitempty"`
	// Whether to enable an AWS-owned KMS CMK not in your account. Defaults to true. Setting to false without configuring kms_key_id creates an AWS-managed CMK aliased to aws/mq in your account.
	UseAwsOwnedKey *bool `json:"useAwsOwnedKey,omitempty"`
}

type RabbitMQBrokerInstancesObservation struct {
	// URL of the ActiveMQ Web Console or the RabbitMQ Management UI depending on engine_type.
	ConsoleURL *string `json:"consoleUrl,omitempty"`
	// Broker's wire-level protocol endpoints referenceable.
	Endpoints []*string `json:"endpoints,omitempty"`
	// IP Address of the broker.
	IPAddress *string `json:"ipAddress,omitempty"`
}

type RabbitMQBrokerMaintenanceWindowStartTimeObservation struct {
	// Day of the week, e.g., MONDAY, TUESDAY, or WEDNESDAY.
	DayOfWeek *string `json:"dayOfWeek,omitempty"`
	// Time, in 24-hour format, e.g., 02:00.
	TimeOfDay *string `json:"timeOfDay,omitempty"`
	// Time zone in either the Country/City format or the UTC offset format, e.g., CET.
	TimeZone *string `json:"timeZone,omitempty"`
}
