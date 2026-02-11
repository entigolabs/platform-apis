package apis

const (
	XRKindS3Bucket = "S3Bucket"

	BucketKind       = "Bucket"
	BucketApiVersion = "s3.aws.m.upbound.io/v1beta1"

	BucketPublicAccessBlockKind                 = "BucketPublicAccessBlock"
	BucketServerSideEncryptionConfigurationKind = "BucketServerSideEncryptionConfiguration"
	BucketVersioningKind                        = "BucketVersioning"
	BucketOwnershipControlsKind                 = "BucketOwnershipControls"

	IAMUserKind                 = "User"
	IAMPolicyKind               = "Policy"
	IAMUserPolicyAttachmentKind = "UserPolicyAttachment"
	IAMAccessKeyKind            = "AccessKey"
	IAMRoleKind                 = "Role"
	IAMRolePolicyAttachmentKind = "RolePolicyAttachment"
	IAMApiVersion               = "iam.aws.m.upbound.io/v1beta1"

	SecretsManagerSecretKind        = "Secret"
	SecretsManagerSecretVersionKind = "SecretVersion"
	SecretsManagerApiVersion        = "secretsmanager.aws.m.upbound.io/v1beta1"

	EKSKey            = "EKS"
	KMSDataAliasKey   = "KMSDataAlias"
	KMSConfigAliasKey = "KMSConfigAlias"
	NamespaceKey      = "Namespace"

	AnnotationKMSDataKeyAlias = "storage.entigo.com/kms-data-key-alias"
	AnnotationServiceAccount  = "storage.entigo.com/service-account-name"
	TenancyZoneLabel          = "tenancy.entigo.com/zone"
)
