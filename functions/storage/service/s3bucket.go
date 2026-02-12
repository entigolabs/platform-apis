package service

import (
	"encoding/json"
	"fmt"
	"strings"

	xpcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	eksv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/eks/v1beta1"
	kmsv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/kms/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/iam/v1beta1"
	s3v1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/s3/v1beta1"
	smv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/secretsmanager/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type s3BucketParams struct {
	BucketName         string
	Namespace          string
	ProviderConfigRef  string
	Region             string
	KMSDataKeyArn      string
	KMSDataKeyAliasID  string
	KMSConfigKeyArn    string
	ClusterOIDC        string
	AWSAccount         string
	EnableVersioning   bool
	CreateSA           bool
	ServiceAccountName string
	TenancyZone        string
	Tags               map[string]*string
}

func GenerateS3BucketObjects(
	bucket v1alpha1.S3Bucket,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var cluster eksv1beta1.Cluster
	if err := base.ExtractRequiredResource(required, apis.EKSKey, &cluster); err != nil {
		return nil, err
	}

	var kmsDataAlias kmsv1beta1.Alias
	if err := base.ExtractRequiredResource(required, apis.KMSDataAliasKey, &kmsDataAlias); err != nil {
		return nil, err
	}

	var kmsConfigAlias kmsv1beta1.Alias
	if err := base.ExtractRequiredResource(required, apis.KMSConfigAliasKey, &kmsConfigAlias); err != nil {
		return nil, err
	}

	var ns corev1.Namespace
	if err := base.ExtractRequiredResource(required, apis.NamespaceKey, &ns); err != nil {
		return nil, err
	}

	bucketName := bucket.GetName()
	serviceAccountName := bucket.Spec.ServiceAccountName
	if serviceAccountName == "" {
		serviceAccountName = bucketName
	}

	tenancyZone := ""
	if ns.Labels != nil {
		tenancyZone = ns.Labels[apis.TenancyZoneLabel]
	}

	// Extract OIDC issuer from cluster identity
	clusterOIDC := ""
	if len(cluster.Status.AtProvider.Identity) > 0 && len(cluster.Status.AtProvider.Identity[0].Oidc) > 0 {
		if issuer := cluster.Status.AtProvider.Identity[0].Oidc[0].Issuer; issuer != nil {
			clusterOIDC = strings.TrimPrefix(*issuer, "https://")
		}
	}

	// Extract AWS account from cluster ARN
	awsAccount := ""
	if cluster.Status.AtProvider.Arn != nil {
		parts := strings.Split(*cluster.Status.AtProvider.Arn, ":")
		if len(parts) > 4 {
			awsAccount = parts[4]
		}
	}

	region := ""
	if cluster.Status.AtProvider.Region != nil {
		region = *cluster.Status.AtProvider.Region
	}

	kmsDataKeyArn := ""
	if kmsDataAlias.Status.AtProvider.TargetKeyArn != nil {
		kmsDataKeyArn = *kmsDataAlias.Status.AtProvider.TargetKeyArn
	}

	kmsDataKeyAliasID := ""
	if kmsDataAlias.Status.AtProvider.ID != nil {
		kmsDataKeyAliasID = *kmsDataAlias.Status.AtProvider.ID
	}

	kmsConfigKeyArn := ""
	if kmsConfigAlias.Status.AtProvider.Arn != nil {
		kmsConfigKeyArn = *kmsConfigAlias.Status.AtProvider.Arn
	}

	params := &s3BucketParams{
		BucketName:         bucketName,
		Namespace:          bucket.GetNamespace(),
		ProviderConfigRef:  env.AWSProvider,
		Region:             region,
		KMSDataKeyArn:      kmsDataKeyArn,
		KMSDataKeyAliasID:  kmsDataKeyAliasID,
		KMSConfigKeyArn:    kmsConfigKeyArn,
		ClusterOIDC:        clusterOIDC,
		AWSAccount:         awsAccount,
		EnableVersioning:   bucket.Spec.EnableVersioning,
		CreateSA:           bucket.Spec.CreateServiceAccount,
		ServiceAccountName: serviceAccountName,
		TenancyZone:        tenancyZone,
		Tags:               env.Tags,
	}

	objects := make(map[string]runtime.Object)

	addBucketResources(objects, params)
	addIAMResources(objects, params)
	addSecretsManagerResources(objects, params)

	if params.CreateSA {
		addServiceAccount(objects, params)
	}

	addCredentialsSecret(objects, params, observed)

	return objects, nil
}

func addBucketResources(objects map[string]runtime.Object, p *s3BucketParams) {
	providerConfigRef := &xpcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.BucketName)
	if p.TenancyZone != "" {
		tags[apis.TenancyZoneLabel] = base.StringPtr(p.TenancyZone)
	}

	var labels map[string]string
	if p.TenancyZone != "" {
		labels = map[string]string{apis.TenancyZoneLabel: p.TenancyZone}
	}

	// Bucket
	objects["bucket"] = &s3v1beta1.Bucket{
		TypeMeta: metav1.TypeMeta{APIVersion: apis.BucketApiVersion, Kind: apis.BucketKind},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.BucketName,
			Namespace: p.Namespace,
			Annotations: map[string]string{
				apis.AnnotationKMSDataKeyAlias: p.KMSDataKeyAliasID,
				apis.AnnotationServiceAccount:  p.ServiceAccountName,
			},
			Labels: labels,
		},
		Spec: s3v1beta1.BucketSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference:          providerConfigRef,
				WriteConnectionSecretToReference: &xpcommon.LocalSecretReference{Name: p.BucketName + "-bucket"},
			},
			ForProvider: s3v1beta1.BucketParameters{
				Region: &p.Region,
				Tags:   tags,
			},
		},
	}

	// BucketPublicAccessBlock
	objects["bucket-public-access-block"] = &s3v1beta1.BucketPublicAccessBlock{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.BucketApiVersion, Kind: apis.BucketPublicAccessBlockKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: s3v1beta1.BucketPublicAccessBlockSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: s3v1beta1.BucketPublicAccessBlockParameters{
				BucketRef:             &xpcommon.NamespacedReference{Name: p.BucketName},
				BlockPublicAcls:       base.BoolPtr(true),
				BlockPublicPolicy:     base.BoolPtr(true),
				IgnorePublicAcls:      base.BoolPtr(true),
				RestrictPublicBuckets: base.BoolPtr(true),
				Region:                &p.Region,
			},
		},
	}

	// BucketServerSideEncryptionConfiguration
	sseDefault := &s3v1beta1.RuleApplyServerSideEncryptionByDefaultParameters{
		SseAlgorithm: base.StringPtr("aws:kms"),
	}
	if p.KMSDataKeyArn != "" {
		sseDefault.KMSMasterKeyID = &p.KMSDataKeyArn
	}
	objects["bucket-server-side-encryption-configuration"] = &s3v1beta1.BucketServerSideEncryptionConfiguration{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.BucketApiVersion, Kind: apis.BucketServerSideEncryptionConfigurationKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: s3v1beta1.BucketServerSideEncryptionConfigurationSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: s3v1beta1.BucketServerSideEncryptionConfigurationParameters{
				BucketRef: &xpcommon.NamespacedReference{Name: p.BucketName},
				Region:    &p.Region,
				Rule: []s3v1beta1.BucketServerSideEncryptionConfigurationRuleParameters{
					{
						ApplyServerSideEncryptionByDefault: sseDefault,
						BucketKeyEnabled:                   base.BoolPtr(true),
					},
				},
			},
		},
	}

	// BucketVersioning
	versioningStatus := "Suspended"
	if p.EnableVersioning {
		versioningStatus = "Enabled"
	}
	objects["bucket-versioning"] = &s3v1beta1.BucketVersioning{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.BucketApiVersion, Kind: apis.BucketVersioningKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: s3v1beta1.BucketVersioningSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: s3v1beta1.BucketVersioningParameters{
				BucketRef: &xpcommon.NamespacedReference{Name: p.BucketName},
				Region:    &p.Region,
				VersioningConfiguration: &s3v1beta1.VersioningConfigurationParameters{
					Status: &versioningStatus,
				},
			},
		},
	}

	// BucketOwnershipControls
	objects["bucket-ownership-controls"] = &s3v1beta1.BucketOwnershipControls{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.BucketApiVersion, Kind: apis.BucketOwnershipControlsKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: s3v1beta1.BucketOwnershipControlsSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: s3v1beta1.BucketOwnershipControlsParameters{
				BucketRef: &xpcommon.NamespacedReference{Name: p.BucketName},
				Region:    &p.Region,
				Rule: &s3v1beta1.BucketOwnershipControlsRuleParameters{
					ObjectOwnership: base.StringPtr("BucketOwnerEnforced"),
				},
			},
		},
	}
}

func addIAMResources(objects map[string]runtime.Object, p *s3BucketParams) {
	providerConfigRef := &xpcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.BucketName)

	// IAM User
	objects["iam-user"] = &iamv1beta1.User{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.IAMApiVersion, Kind: apis.IAMUserKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: iamv1beta1.UserSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: iamv1beta1.UserParameters{
				Tags: tags,
			},
		},
	}

	// IAM Policy
	policyDoc := buildIAMPolicyDocument(p.BucketName, p.KMSDataKeyArn)
	objects["iam-policy"] = &iamv1beta1.Policy{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.IAMApiVersion, Kind: apis.IAMPolicyKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: iamv1beta1.PolicySpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: iamv1beta1.PolicyParameters{
				Policy: &policyDoc,
				Tags:   tags,
			},
		},
	}

	// UserPolicyAttachment
	objects["iam-user-policy-attachment"] = &iamv1beta1.UserPolicyAttachment{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.IAMApiVersion, Kind: apis.IAMUserPolicyAttachmentKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: iamv1beta1.UserPolicyAttachmentSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: iamv1beta1.UserPolicyAttachmentParameters{
				PolicyArnRef: &xpcommon.NamespacedReference{Name: p.BucketName},
				UserRef:      &xpcommon.NamespacedReference{Name: p.BucketName},
			},
		},
	}

	// AccessKey
	objects["iam-access-key"] = &iamv1beta1.AccessKey{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.IAMApiVersion, Kind: apis.IAMAccessKeyKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: iamv1beta1.AccessKeySpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{
				ProviderConfigReference:          providerConfigRef,
				WriteConnectionSecretToReference: &xpcommon.LocalSecretReference{Name: p.BucketName + "-access-key"},
			},
			ForProvider: iamv1beta1.AccessKeyParameters{
				UserRef: &xpcommon.NamespacedReference{Name: p.BucketName},
			},
		},
	}

	// IAM Role (IRSA)
	roleTags := make(map[string]*string)
	for k, v := range p.Tags {
		roleTags[k] = v
	}
	roleTags["Name"] = base.StringPtr(p.BucketName)

	assumeRolePolicy := buildAssumeRolePolicy(p.AWSAccount, p.ClusterOIDC, p.Namespace, p.ServiceAccountName)
	objects["iam-role"] = &iamv1beta1.Role{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.IAMApiVersion, Kind: apis.IAMRoleKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: iamv1beta1.RoleSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: iamv1beta1.RoleParameters{
				AssumeRolePolicy: &assumeRolePolicy,
				Tags:             roleTags,
			},
		},
	}

	// RolePolicyAttachment
	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", p.AWSAccount, p.BucketName)
	objects["iam-role-policy-attachment"] = &iamv1beta1.RolePolicyAttachment{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.IAMApiVersion, Kind: apis.IAMRolePolicyAttachmentKind},
		ObjectMeta: metav1.ObjectMeta{Name: p.BucketName},
		Spec: iamv1beta1.RolePolicyAttachmentSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: iamv1beta1.RolePolicyAttachmentParameters{
				PolicyArn: &policyArn,
				RoleRef:   &xpcommon.NamespacedReference{Name: p.BucketName},
			},
		},
	}
}

func addSecretsManagerResources(objects map[string]runtime.Object, p *s3BucketParams) {
	providerConfigRef := &xpcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.BucketName + "-credentials")

	secretName := p.BucketName + "-credentials"
	description := fmt.Sprintf("Credentials for bucket %s", p.BucketName)
	recoveryWindow := float64(0)

	// Secrets Manager Secret
	smSecretParams := smv1beta1.SecretParameters{
		Name:                 &secretName,
		Region:               &p.Region,
		Description:          &description,
		RecoveryWindowInDays: &recoveryWindow,
		Tags:                 tags,
	}
	if p.KMSConfigKeyArn != "" {
		smSecretParams.KMSKeyID = &p.KMSConfigKeyArn
	}
	objects["secrets-manager-secret"] = &smv1beta1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.SecretsManagerApiVersion, Kind: apis.SecretsManagerSecretKind},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider:         smSecretParams,
		},
	}

	// Secrets Manager SecretVersion
	objects["secrets-manager-secret-version"] = &smv1beta1.SecretVersion{
		TypeMeta:   metav1.TypeMeta{APIVersion: apis.SecretsManagerApiVersion, Kind: apis.SecretsManagerSecretVersionKind},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretVersionSpec{
			ManagedResourceSpec: xpv2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: smv1beta1.SecretVersionParameters{
				Region:      &p.Region,
				SecretIDRef: &xpcommon.NamespacedReference{Name: secretName},
				SecretStringSecretRef: &xpcommon.LocalSecretKeySelector{
					LocalSecretReference: xpcommon.LocalSecretReference{Name: secretName},
					Key:                  "credentials.json",
				},
			},
		},
	}
}

func addServiceAccount(objects map[string]runtime.Object, p *s3BucketParams) {
	sa := &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.ServiceAccountName,
			Namespace: p.Namespace,
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn": fmt.Sprintf("arn:aws:iam::%s:role/%s", p.AWSAccount, p.BucketName),
			},
		},
	}
	objects["service-account"] = sa
}

func addCredentialsSecret(objects map[string]runtime.Object, p *s3BucketParams, observed map[resource.Name]resource.ObservedComposed) {
	akObserved, akOk := observed["iam-access-key"]
	bucketObserved, bOk := observed["bucket"]
	if !akOk || !bOk {
		return
	}
	akDetails := akObserved.ConnectionDetails
	bucketDetails := bucketObserved.ConnectionDetails
	if len(akDetails) == 0 || len(bucketDetails) == 0 {
		return
	}

	accessKeyID := string(akDetails["username"])
	secretAccessKey := string(akDetails["password"])
	bucketRegion := string(bucketDetails["region"])
	bucketArn := string(bucketDetails["arn"])
	bucketNameVal := string(bucketDetails["id"])

	if accessKeyID == "" || secretAccessKey == "" {
		return
	}

	credJSON, _ := json.Marshal(map[string]string{
		"AWS_ACCESS_KEY_ID":     accessKeyID,
		"AWS_SECRET_ACCESS_KEY": secretAccessKey,
		"BUCKET_REGION":         bucketRegion,
		"BUCKET_ARN":            bucketArn,
		"BUCKET_NAME":           bucketNameVal,
	})

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      p.BucketName + "-credentials",
			Namespace: p.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"AWS_ACCESS_KEY_ID":     accessKeyID,
			"AWS_SECRET_ACCESS_KEY": secretAccessKey,
			"BUCKET_REGION":         bucketRegion,
			"BUCKET_ARN":            bucketArn,
			"BUCKET_NAME":           bucketNameVal,
			"credentials.json":      string(credJSON),
		},
	}
	objects["credentials"] = secret
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func buildIAMPolicyDocument(bucketName, kmsDataKeyArn string) string {
	var statements []interface{}

	if kmsDataKeyArn != "" {
		statements = append(statements, map[string]interface{}{
			"Effect": "Allow",
			"Action": []string{
				"kms:Encrypt",
				"kms:Decrypt",
				"kms:ReEncrypt*",
				"kms:GenerateDataKey*",
				"kms:DescribeKey",
			},
			"Resource": []string{kmsDataKeyArn},
		})
	}

	statements = append(statements, map[string]interface{}{
		"Effect": "Allow",
		"Action": "s3:*",
		"Resource": []string{
			fmt.Sprintf("arn:aws:s3:::%s", bucketName),
			fmt.Sprintf("arn:aws:s3:::%s/*", bucketName),
		},
	})

	doc := map[string]interface{}{
		"Version":   "2012-10-17",
		"Statement": statements,
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

func buildAssumeRolePolicy(awsAccount, clusterOIDC, namespace, serviceAccountName string) string {
	doc := map[string]interface{}{
		"Version": "2012-10-17",
		"Statement": []interface{}{
			map[string]interface{}{
				"Effect": "Allow",
				"Principal": map[string]interface{}{
					"Federated": fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", awsAccount, clusterOIDC),
				},
				"Action": "sts:AssumeRoleWithWebIdentity",
				"Condition": map[string]interface{}{
					"StringEquals": map[string]interface{}{
						clusterOIDC + ":aud": "sts.amazonaws.com",
						clusterOIDC + ":sub": fmt.Sprintf("system:serviceaccount:%s:%s", namespace, serviceAccountName),
					},
				},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}
