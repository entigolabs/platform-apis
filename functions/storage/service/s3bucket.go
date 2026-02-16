package service

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	eksmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/eks/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/iam/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/kms/v1beta1"
	s3v1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/s3/v1beta1"
	smv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/secretsmanager/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	EKSKey       = "EKS"
	KMSDataKey   = "KMSDataKey"
	KMSConfigKey = "KMSConfigKey"
	NamespaceKey = "Namespace"

	AnnotationServiceAccount = "storage.entigo.com/service-account-name"
	TenancyZoneLabel         = "tenancy.entigo.com/zone"
)

type s3BucketGenerator struct {
	// Inputs
	bucket   v1alpha1.S3Bucket
	observed map[resource.Name]resource.ObservedComposed
	env      apis.Environment
	// Dependencies
	cluster      eksmv1beta1.Cluster
	kmsDataKey   kmsmv1beta1.Key
	kmsConfigKey kmsmv1beta1.Key
	namespace    corev1.Namespace
	// Derived values
	bucketName         string
	serviceAccountName string
	tenancyZone        string
	region             string
	clusterOIDC        string
	awsAccount         string
	kmsDataKeyArn      string
	kmsConfigKeyArn    string
}

func GenerateS3BucketObjects(
	bucket v1alpha1.S3Bucket,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	g, err := newS3BucketGenerator(bucket, required, observed)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newS3BucketGenerator(
	bucket v1alpha1.S3Bucket,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*s3BucketGenerator, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var cluster eksmv1beta1.Cluster
	if err := base.ExtractRequiredResource(required, EKSKey, &cluster); err != nil {
		return nil, err
	}

	var kmsDataKey kmsmv1beta1.Key
	if err := base.ExtractRequiredResource(required, KMSDataKey, &kmsDataKey); err != nil {
		return nil, err
	}

	var kmsConfigKey kmsmv1beta1.Key
	if err := base.ExtractRequiredResource(required, KMSConfigKey, &kmsConfigKey); err != nil {
		return nil, err
	}

	var ns corev1.Namespace
	if err := base.ExtractRequiredResource(required, NamespaceKey, &ns); err != nil {
		return nil, err
	}

	g := &s3BucketGenerator{
		bucket:       bucket,
		observed:     observed,
		env:          env,
		cluster:      cluster,
		kmsDataKey:   kmsDataKey,
		kmsConfigKey: kmsConfigKey,
		namespace:    ns,
	}

	g.computeDerivedValues()
	return g, nil
}

func (g *s3BucketGenerator) computeDerivedValues() {
	g.bucketName = g.bucket.GetName()

	g.serviceAccountName = g.bucket.Spec.ServiceAccountName
	if g.serviceAccountName == "" {
		g.serviceAccountName = g.bucketName
	}

	if g.namespace.Labels != nil {
		g.tenancyZone = g.namespace.Labels[TenancyZoneLabel]
	}

	// Extract OIDC issuer from cluster identity
	if len(g.cluster.Status.AtProvider.Identity) > 0 && len(g.cluster.Status.AtProvider.Identity[0].Oidc) > 0 {
		if issuer := g.cluster.Status.AtProvider.Identity[0].Oidc[0].Issuer; issuer != nil {
			g.clusterOIDC = strings.TrimPrefix(*issuer, "https://")
		}
	}

	// Extract AWS account from cluster ARN
	if g.cluster.Status.AtProvider.Arn != nil {
		parts := strings.Split(*g.cluster.Status.AtProvider.Arn, ":")
		if len(parts) > 4 {
			g.awsAccount = parts[4]
		}
	}

	if g.cluster.Status.AtProvider.Region != nil {
		g.region = *g.cluster.Status.AtProvider.Region
	}

	if g.kmsDataKey.Status.AtProvider.Arn != nil {
		g.kmsDataKeyArn = *g.kmsDataKey.Status.AtProvider.Arn
	}

	if g.kmsConfigKey.Status.AtProvider.Arn != nil {
		g.kmsConfigKeyArn = *g.kmsConfigKey.Status.AtProvider.Arn
	}
}

func (g *s3BucketGenerator) generate() (map[string]runtime.Object, error) {
	objects := make(map[string]runtime.Object)

	g.buildBucketResources(objects)
	g.buildIAMResources(objects)
	g.buildSecretsManagerResources(objects)

	if g.bucket.Spec.CreateServiceAccount {
		g.buildServiceAccount(objects)
	}

	g.buildCredentialsSecret(objects)

	return objects, nil
}

func (g *s3BucketGenerator) providerConfigRef() *xpvcommon.ProviderConfigReference {
	return &xpvcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: g.env.AWSProvider}
}

func (g *s3BucketGenerator) buildTags() map[string]*string {
	tags := make(map[string]*string)
	maps.Copy(tags, g.env.Tags)
	tags["Name"] = base.StringPtr(g.bucketName)
	return tags
}

func (g *s3BucketGenerator) buildBucketTags() map[string]*string {
	tags := g.buildTags()
	if g.tenancyZone != "" {
		tags[TenancyZoneLabel] = base.StringPtr(g.tenancyZone)
	}
	return tags
}

func (g *s3BucketGenerator) buildBucketResources(objects map[string]runtime.Object) {
	tags := g.buildBucketTags()

	var labels map[string]string
	if g.tenancyZone != "" {
		labels = map[string]string{TenancyZoneLabel: g.tenancyZone}
	}

	// Bucket
	objects["bucket"] = &s3v1beta1.Bucket{
		TypeMeta: metav1.TypeMeta{APIVersion: "s3.aws.m.upbound.io/v1beta1", Kind: "Bucket"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.bucketName,
			Namespace: g.bucket.GetNamespace(),
			Annotations: map[string]string{
				AnnotationServiceAccount: g.serviceAccountName,
			},
			Labels: labels,
		},
		Spec: s3v1beta1.BucketSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference:          g.providerConfigRef(),
				WriteConnectionSecretToReference: &xpvcommon.LocalSecretReference{Name: g.bucketName + "-bucket"},
			},
			ForProvider: s3v1beta1.BucketParameters{
				Region: &g.region,
				Tags:   tags,
			},
		},
	}

	// BucketPublicAccessBlock
	objects["bucket-public-access-block"] = &s3v1beta1.BucketPublicAccessBlock{
		TypeMeta:   metav1.TypeMeta{APIVersion: "s3.aws.m.upbound.io/v1beta1", Kind: "BucketPublicAccessBlock"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: s3v1beta1.BucketPublicAccessBlockSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: s3v1beta1.BucketPublicAccessBlockParameters{
				BucketRef:             &xpvcommon.NamespacedReference{Name: g.bucketName},
				BlockPublicAcls:       base.BoolPtr(true),
				BlockPublicPolicy:     base.BoolPtr(true),
				IgnorePublicAcls:      base.BoolPtr(true),
				RestrictPublicBuckets: base.BoolPtr(true),
				Region:                &g.region,
			},
		},
	}

	// BucketServerSideEncryptionConfiguration
	sseDefault := &s3v1beta1.RuleApplyServerSideEncryptionByDefaultParameters{
		SseAlgorithm: base.StringPtr("aws:kms"),
	}
	if g.kmsDataKeyArn != "" {
		sseDefault.KMSMasterKeyID = &g.kmsDataKeyArn
	}
	objects["bucket-server-side-encryption-configuration"] = &s3v1beta1.BucketServerSideEncryptionConfiguration{
		TypeMeta:   metav1.TypeMeta{APIVersion: "s3.aws.m.upbound.io/v1beta1", Kind: "BucketServerSideEncryptionConfiguration"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: s3v1beta1.BucketServerSideEncryptionConfigurationSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: s3v1beta1.BucketServerSideEncryptionConfigurationParameters{
				BucketRef: &xpvcommon.NamespacedReference{Name: g.bucketName},
				Region:    &g.region,
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
	if g.bucket.Spec.EnableVersioning {
		versioningStatus = "Enabled"
	}
	objects["bucket-versioning"] = &s3v1beta1.BucketVersioning{
		TypeMeta:   metav1.TypeMeta{APIVersion: "s3.aws.m.upbound.io/v1beta1", Kind: "BucketVersioning"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: s3v1beta1.BucketVersioningSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: s3v1beta1.BucketVersioningParameters{
				BucketRef: &xpvcommon.NamespacedReference{Name: g.bucketName},
				Region:    &g.region,
				VersioningConfiguration: &s3v1beta1.VersioningConfigurationParameters{
					Status: &versioningStatus,
				},
			},
		},
	}

	// BucketOwnershipControls
	objects["bucket-ownership-controls"] = &s3v1beta1.BucketOwnershipControls{
		TypeMeta:   metav1.TypeMeta{APIVersion: "s3.aws.m.upbound.io/v1beta1", Kind: "BucketOwnershipControls"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: s3v1beta1.BucketOwnershipControlsSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: s3v1beta1.BucketOwnershipControlsParameters{
				BucketRef: &xpvcommon.NamespacedReference{Name: g.bucketName},
				Region:    &g.region,
				Rule: &s3v1beta1.BucketOwnershipControlsRuleParameters{
					ObjectOwnership: base.StringPtr("BucketOwnerEnforced"),
				},
			},
		},
	}
}

func (g *s3BucketGenerator) buildIAMResources(objects map[string]runtime.Object) {
	tags := g.buildTags()

	// IAM User
	objects["iam-user"] = &iamv1beta1.User{
		TypeMeta:   metav1.TypeMeta{APIVersion: "iam.aws.m.upbound.io/v1beta1", Kind: "User"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: iamv1beta1.UserSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: iamv1beta1.UserParameters{
				Tags: tags,
			},
		},
	}

	// IAM Policy
	policyDoc := g.buildIAMPolicyDocument()
	objects["iam-policy"] = &iamv1beta1.Policy{
		TypeMeta:   metav1.TypeMeta{APIVersion: "iam.aws.m.upbound.io/v1beta1", Kind: "Policy"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: iamv1beta1.PolicySpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: iamv1beta1.PolicyParameters{
				Policy: &policyDoc,
				Tags:   tags,
			},
		},
	}

	// UserPolicyAttachment
	objects["iam-user-policy-attachment"] = &iamv1beta1.UserPolicyAttachment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "iam.aws.m.upbound.io/v1beta1", Kind: "UserPolicyAttachment"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: iamv1beta1.UserPolicyAttachmentSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: iamv1beta1.UserPolicyAttachmentParameters{
				PolicyArnRef: &xpvcommon.NamespacedReference{Name: g.bucketName},
				UserRef:      &xpvcommon.NamespacedReference{Name: g.bucketName},
			},
		},
	}

	// AccessKey
	objects["iam-access-key"] = &iamv1beta1.AccessKey{
		TypeMeta:   metav1.TypeMeta{APIVersion: "iam.aws.m.upbound.io/v1beta1", Kind: "AccessKey"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: iamv1beta1.AccessKeySpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference:          g.providerConfigRef(),
				WriteConnectionSecretToReference: &xpvcommon.LocalSecretReference{Name: g.bucketName + "-access-key"},
			},
			ForProvider: iamv1beta1.AccessKeyParameters{
				UserRef: &xpvcommon.NamespacedReference{Name: g.bucketName},
			},
		},
	}

	// IAM Role (IRSA)
	assumeRolePolicy := g.buildAssumeRolePolicy()
	objects["iam-role"] = &iamv1beta1.Role{
		TypeMeta:   metav1.TypeMeta{APIVersion: "iam.aws.m.upbound.io/v1beta1", Kind: "Role"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: iamv1beta1.RoleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: iamv1beta1.RoleParameters{
				AssumeRolePolicy: &assumeRolePolicy,
				Tags:             tags,
			},
		},
	}

	// RolePolicyAttachment
	policyArn := fmt.Sprintf("arn:aws:iam::%s:policy/%s", g.awsAccount, g.bucketName)
	objects["iam-role-policy-attachment"] = &iamv1beta1.RolePolicyAttachment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "iam.aws.m.upbound.io/v1beta1", Kind: "RolePolicyAttachment"},
		ObjectMeta: metav1.ObjectMeta{Name: g.bucketName},
		Spec: iamv1beta1.RolePolicyAttachmentSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: iamv1beta1.RolePolicyAttachmentParameters{
				PolicyArn: &policyArn,
				RoleRef:   &xpvcommon.NamespacedReference{Name: g.bucketName},
			},
		},
	}
}

func (g *s3BucketGenerator) buildSecretsManagerResources(objects map[string]runtime.Object) {
	tags := make(map[string]*string)
	maps.Copy(tags, g.env.Tags)
	tags["Name"] = base.StringPtr(g.bucketName + "-credentials")

	secretName := g.bucketName + "-credentials"
	description := fmt.Sprintf("Credentials for bucket %s", g.bucketName)
	recoveryWindow := float64(0)

	// Secrets Manager Secret
	smSecretParams := smv1beta1.SecretParameters{
		Name:                 &secretName,
		Region:               &g.region,
		Description:          &description,
		RecoveryWindowInDays: &recoveryWindow,
		Tags:                 tags,
	}
	if g.kmsConfigKeyArn != "" {
		smSecretParams.KMSKeyID = &g.kmsConfigKeyArn
	}
	objects["secrets-manager-secret"] = &smv1beta1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: "secretsmanager.aws.m.upbound.io/v1beta1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider:         smSecretParams,
		},
	}

	// Secrets Manager SecretVersion
	objects["secrets-manager-secret-version"] = &smv1beta1.SecretVersion{
		TypeMeta:   metav1.TypeMeta{APIVersion: "secretsmanager.aws.m.upbound.io/v1beta1", Kind: "SecretVersion"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretVersionSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: smv1beta1.SecretVersionParameters{
				Region:      &g.region,
				SecretIDRef: &xpvcommon.NamespacedReference{Name: secretName},
				SecretStringSecretRef: &xpvcommon.LocalSecretKeySelector{
					LocalSecretReference: xpvcommon.LocalSecretReference{Name: secretName},
					Key:                  "credentials.json",
				},
			},
		},
	}
}

func (g *s3BucketGenerator) buildServiceAccount(objects map[string]runtime.Object) {
	objects["service-account"] = &corev1.ServiceAccount{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ServiceAccount",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.serviceAccountName,
			Namespace: g.bucket.GetNamespace(),
			Annotations: map[string]string{
				"eks.amazonaws.com/role-arn": fmt.Sprintf("arn:aws:iam::%s:role/%s", g.awsAccount, g.bucketName),
			},
		},
	}
}

func (g *s3BucketGenerator) buildCredentialsSecret(objects map[string]runtime.Object) {
	akObserved, akOk := g.observed["iam-access-key"]
	bucketObserved, bOk := g.observed["bucket"]
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

	objects["credentials"] = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.bucketName + "-credentials",
			Namespace: g.bucket.GetNamespace(),
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
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func (g *s3BucketGenerator) buildIAMPolicyDocument() string {
	var statements []any

	if g.kmsDataKeyArn != "" {
		statements = append(statements, map[string]any{
			"Effect": "Allow",
			"Action": []string{
				"kms:Encrypt",
				"kms:Decrypt",
				"kms:ReEncrypt*",
				"kms:GenerateDataKey*",
				"kms:DescribeKey",
			},
			"Resource": []string{g.kmsDataKeyArn},
		})
	}

	statements = append(statements, map[string]any{
		"Effect": "Allow",
		"Action": "s3:*",
		"Resource": []string{
			fmt.Sprintf("arn:aws:s3:::%s", g.bucketName),
			fmt.Sprintf("arn:aws:s3:::%s/*", g.bucketName),
		},
	})

	doc := map[string]any{
		"Version":   "2012-10-17",
		"Statement": statements,
	}
	b, _ := json.Marshal(doc)
	return string(b)
}

func (g *s3BucketGenerator) buildAssumeRolePolicy() string {
	doc := map[string]any{
		"Version": "2012-10-17",
		"Statement": []any{
			map[string]any{
				"Effect": "Allow",
				"Principal": map[string]any{
					"Federated": fmt.Sprintf("arn:aws:iam::%s:oidc-provider/%s", g.awsAccount, g.clusterOIDC),
				},
				"Action": "sts:AssumeRoleWithWebIdentity",
				"Condition": map[string]any{
					"StringEquals": map[string]any{
						g.clusterOIDC + ":aud": "sts.amazonaws.com",
						g.clusterOIDC + ":sub": fmt.Sprintf("system:serviceaccount:%s:%s", g.bucket.GetNamespace(), g.serviceAccountName),
					},
				},
			},
		},
	}
	b, _ := json.Marshal(doc)
	return string(b)
}
