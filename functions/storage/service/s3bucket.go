package service

import (
	"encoding/json"
	"fmt"
	"strings"

	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	eksData, err := extractEKSData(required)
	if err != nil {
		return nil, err
	}

	kmsDataAlias, err := extractKMSAlias(required, apis.KMSDataAliasKey)
	if err != nil {
		return nil, err
	}

	kmsConfigAlias, err := extractKMSAlias(required, apis.KMSConfigAliasKey)
	if err != nil {
		return nil, err
	}

	ns, err := extractNamespace(required)
	if err != nil {
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

	params := &s3BucketParams{
		BucketName:         bucketName,
		Namespace:          bucket.GetNamespace(),
		ProviderConfigRef:  env.AWSProvider,
		Region:             eksData.region,
		KMSDataKeyArn:      kmsDataAlias.targetKeyArn,
		KMSDataKeyAliasID:  kmsDataAlias.id,
		KMSConfigKeyArn:    kmsConfigAlias.arn,
		ClusterOIDC:        eksData.clusterOIDC,
		AWSAccount:         eksData.awsAccount,
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
	providerConfigRef := &xpv2.ProviderConfigReference{Name: p.ProviderConfigRef, Kind: "ClusterProviderConfig"}

	// Tags
	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.BucketName)
	if p.TenancyZone != "" {
		tags[apis.TenancyZoneLabel] = base.StringPtr(p.TenancyZone)
	}

	// Labels
	var labels map[string]string
	if p.TenancyZone != "" {
		labels = map[string]string{apis.TenancyZoneLabel: p.TenancyZone}
	}

	// Bucket
	bucketObj := newUnstructured(apis.BucketApiVersion, apis.BucketKind, p.BucketName, p.Namespace)
	bucketObj.Object["metadata"].(map[string]interface{})["annotations"] = map[string]interface{}{
		apis.AnnotationKMSDataKeyAlias: p.KMSDataKeyAliasID,
		apis.AnnotationServiceAccount:  p.ServiceAccountName,
	}
	if labels != nil {
		bucketObj.Object["metadata"].(map[string]interface{})["labels"] = toInterfaceMap(labels)
	}
	bucketSpec := map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"region": p.Region,
			"tags":   toInterfaceMapPtr(tags),
		},
		"writeConnectionSecretToRef": map[string]interface{}{
			"name": p.BucketName + "-bucket",
		},
	}
	bucketObj.Object["spec"] = bucketSpec
	objects["bucket"] = bucketObj

	// BucketPublicAccessBlock
	pab := newUnstructured(apis.BucketApiVersion, apis.BucketPublicAccessBlockKind, p.BucketName, "")
	pab.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"bucketRef":             map[string]interface{}{"name": p.BucketName},
			"blockPublicAcls":       true,
			"blockPublicPolicy":     true,
			"ignorePublicAcls":      true,
			"restrictPublicBuckets": true,
			"region":                p.Region,
		},
	}
	objects["bucket-public-access-block"] = pab

	// BucketServerSideEncryptionConfiguration
	sse := newUnstructured(apis.BucketApiVersion, apis.BucketServerSideEncryptionConfigurationKind, p.BucketName, "")
	sseDefault := map[string]interface{}{
		"sseAlgorithm": "aws:kms",
	}
	if p.KMSDataKeyArn != "" {
		sseDefault["kmsMasterKeyId"] = p.KMSDataKeyArn
	}
	sse.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"bucketRef": map[string]interface{}{"name": p.BucketName},
			"region":    p.Region,
			"rule": []interface{}{
				map[string]interface{}{
					"applyServerSideEncryptionByDefault": sseDefault,
					"bucketKeyEnabled":                   true,
				},
			},
		},
	}
	objects["bucket-server-side-encryption-configuration"] = sse

	// BucketVersioning
	versioningStatus := "Suspended"
	if p.EnableVersioning {
		versioningStatus = "Enabled"
	}
	bv := newUnstructured(apis.BucketApiVersion, apis.BucketVersioningKind, p.BucketName, "")
	bv.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"bucketRef": map[string]interface{}{"name": p.BucketName},
			"region":    p.Region,
			"versioningConfiguration": map[string]interface{}{
				"status": versioningStatus,
			},
		},
	}
	objects["bucket-versioning"] = bv

	// BucketOwnershipControls
	boc := newUnstructured(apis.BucketApiVersion, apis.BucketOwnershipControlsKind, p.BucketName, "")
	boc.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"bucketRef": map[string]interface{}{"name": p.BucketName},
			"region":    p.Region,
			"rule": map[string]interface{}{
				"objectOwnership": "BucketOwnerEnforced",
			},
		},
	}
	objects["bucket-ownership-controls"] = boc
}

func addIAMResources(objects map[string]runtime.Object, p *s3BucketParams) {
	providerConfigRef := &xpv2.ProviderConfigReference{Name: p.ProviderConfigRef, Kind: "ClusterProviderConfig"}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.BucketName)

	// IAM User
	user := newUnstructured(apis.IAMApiVersion, apis.IAMUserKind, p.BucketName, "")
	user.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"tags": toInterfaceMapPtr(tags),
		},
	}
	objects["iam-user"] = user

	// IAM Policy
	policy := newUnstructured(apis.IAMApiVersion, apis.IAMPolicyKind, p.BucketName, "")
	policyDoc := buildIAMPolicyDocument(p.BucketName, p.KMSDataKeyArn)
	policy.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"policy": policyDoc,
			"tags":   toInterfaceMapPtr(tags),
		},
	}
	objects["iam-policy"] = policy

	// UserPolicyAttachment
	upa := newUnstructured(apis.IAMApiVersion, apis.IAMUserPolicyAttachmentKind, p.BucketName, "")
	upa.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"policyArnRef": map[string]interface{}{"name": p.BucketName},
			"userRef":      map[string]interface{}{"name": p.BucketName},
		},
	}
	objects["iam-user-policy-attachment"] = upa

	// AccessKey
	ak := newUnstructured(apis.IAMApiVersion, apis.IAMAccessKeyKind, p.BucketName, "")
	ak.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"userRef": map[string]interface{}{"name": p.BucketName},
		},
		"writeConnectionSecretToRef": map[string]interface{}{
			"name": p.BucketName + "-access-key",
		},
	}
	objects["iam-access-key"] = ak

	// IAM Role (IRSA)
	roleTags := make(map[string]*string)
	for k, v := range p.Tags {
		roleTags[k] = v
	}
	roleTags["Name"] = base.StringPtr(p.BucketName)

	role := newUnstructured(apis.IAMApiVersion, apis.IAMRoleKind, p.BucketName, "")
	assumeRolePolicy := buildAssumeRolePolicy(p.AWSAccount, p.ClusterOIDC, p.Namespace, p.ServiceAccountName)
	role.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"tags":             toInterfaceMapPtr(roleTags),
			"assumeRolePolicy": assumeRolePolicy,
		},
	}
	objects["iam-role"] = role

	// RolePolicyAttachment
	rpa := newUnstructured(apis.IAMApiVersion, apis.IAMRolePolicyAttachmentKind, p.BucketName, "")
	rpa.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"policyArn": fmt.Sprintf("arn:aws:iam::%s:policy/%s", p.AWSAccount, p.BucketName),
			"roleRef":   map[string]interface{}{"name": p.BucketName},
		},
	}
	objects["iam-role-policy-attachment"] = rpa
}

func addSecretsManagerResources(objects map[string]runtime.Object, p *s3BucketParams) {
	providerConfigRef := &xpv2.ProviderConfigReference{Name: p.ProviderConfigRef, Kind: "ClusterProviderConfig"}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.BucketName + "-credentials")

	secretName := p.BucketName + "-credentials"

	// Secrets Manager Secret
	smSecret := newUnstructured(apis.SecretsManagerApiVersion, apis.SecretsManagerSecretKind, secretName, "")
	forProvider := map[string]interface{}{
		"name":                 secretName,
		"region":               p.Region,
		"description":          fmt.Sprintf("Credentials for bucket %s", p.BucketName),
		"recoveryWindowInDays": float64(0),
		"tags":                 toInterfaceMapPtr(tags),
	}
	if p.KMSConfigKeyArn != "" {
		forProvider["kmsKeyId"] = p.KMSConfigKeyArn
	}
	smSecret.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider":       forProvider,
	}
	objects["secrets-manager-secret"] = smSecret

	// Secrets Manager SecretVersion
	smSecretVersion := newUnstructured(apis.SecretsManagerApiVersion, apis.SecretsManagerSecretVersionKind, secretName, "")
	smSecretVersion.Object["spec"] = map[string]interface{}{
		"providerConfigRef": providerConfigRefMap(providerConfigRef),
		"forProvider": map[string]interface{}{
			"region":      p.Region,
			"secretIdRef": map[string]interface{}{"name": secretName},
			"secretStringSecretRef": map[string]interface{}{
				"name": secretName,
				"key":  "credentials.json",
			},
		},
	}
	objects["secrets-manager-secret-version"] = smSecretVersion
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

// EKS data extracted from required resource
type eksData struct {
	region      string
	clusterOIDC string
	awsAccount  string
}

func extractEKSData(required map[string][]resource.Required) (*eksData, error) {
	if len(required[apis.EKSKey]) == 0 {
		return nil, fmt.Errorf("EKS cluster not found in required resources")
	}
	eksResource := required[apis.EKSKey][0].Resource

	region, _, _ := unstructured.NestedString(eksResource.Object, "status", "atProvider", "region")
	if region == "" {
		return nil, fmt.Errorf("EKS cluster region not available")
	}

	// Extract OIDC issuer
	clusterOIDC := ""
	identity, found, _ := unstructured.NestedSlice(eksResource.Object, "status", "atProvider", "identity")
	if found && len(identity) > 0 {
		if identityMap, ok := identity[0].(map[string]interface{}); ok {
			if oidcList, ok := identityMap["oidc"].([]interface{}); ok && len(oidcList) > 0 {
				if oidcMap, ok := oidcList[0].(map[string]interface{}); ok {
					if issuer, ok := oidcMap["issuer"].(string); ok {
						clusterOIDC = strings.TrimPrefix(issuer, "https://")
					}
				}
			}
		}
	}

	// Extract AWS Account from ARN
	awsAccount := ""
	arn, _, _ := unstructured.NestedString(eksResource.Object, "status", "atProvider", "arn")
	if arn != "" {
		parts := strings.Split(arn, ":")
		if len(parts) > 4 {
			awsAccount = parts[4]
		}
	}

	return &eksData{
		region:      region,
		clusterOIDC: clusterOIDC,
		awsAccount:  awsAccount,
	}, nil
}

// KMS Alias data extracted from required resource
type kmsAliasData struct {
	targetKeyArn string
	arn          string
	id           string
}

func extractKMSAlias(required map[string][]resource.Required, key string) (*kmsAliasData, error) {
	if len(required[key]) == 0 {
		return nil, fmt.Errorf("KMS alias %s not found in required resources", key)
	}
	aliasResource := required[key][0].Resource

	targetKeyArn, _, _ := unstructured.NestedString(aliasResource.Object, "status", "atProvider", "targetKeyArn")
	arn, _, _ := unstructured.NestedString(aliasResource.Object, "status", "atProvider", "arn")
	id, _, _ := unstructured.NestedString(aliasResource.Object, "status", "atProvider", "id")

	return &kmsAliasData{
		targetKeyArn: targetKeyArn,
		arn:          arn,
		id:           id,
	}, nil
}

func extractNamespace(required map[string][]resource.Required) (*corev1.Namespace, error) {
	if len(required[apis.NamespaceKey]) == 0 {
		return nil, fmt.Errorf("namespace not found in required resources")
	}
	var ns corev1.Namespace
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(required[apis.NamespaceKey][0].Resource.Object, &ns); err != nil {
		return nil, fmt.Errorf("cannot convert namespace: %w", err)
	}
	return &ns, nil
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

// Helper functions

func newUnstructured(apiVersion, kind, name, namespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]interface{}{
				"name": name,
			},
		},
	}
	if namespace != "" {
		obj.Object["metadata"].(map[string]interface{})["namespace"] = namespace
	}
	return obj
}

func providerConfigRefMap(ref *xpv2.ProviderConfigReference) map[string]interface{} {
	return map[string]interface{}{
		"name": ref.Name,
		"kind": ref.Kind,
	}
}

func toInterfaceMap(m map[string]string) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func toInterfaceMapPtr(m map[string]*string) map[string]interface{} {
	result := make(map[string]interface{}, len(m))
	for k, v := range m {
		if v != nil {
			result[k] = *v
		}
	}
	return result
}
