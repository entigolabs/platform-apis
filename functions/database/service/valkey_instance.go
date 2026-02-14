package service

import (
	"encoding/json"
	"fmt"

	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	ec2mv1beta1 "github.com/upbound/provider-aws/apis/namespaced/ec2/v1beta1"
	elasticachemv1beta1 "github.com/upbound/provider-aws/apis/namespaced/elasticache/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/kms/v1beta1"
	smv1beta1 "github.com/upbound/provider-aws/apis/namespaced/secretsmanager/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	VPCKey                    = "VPC"
	ElasticacheSubnetGroupKey = "ElasticacheSubnetGroup"
	ComputeSubnetsKey         = "ComputeSubnets"

	elasticacheApiVersion    = "elasticache.aws.m.upbound.io/v1beta1"
	secretsmanagerApiVersion = "secretsmanager.aws.m.upbound.io/v1beta1"
)

type valkeyInstanceParams struct {
	Name              string
	Namespace         string
	ProviderConfigRef string
	Region            string
	VPCID             string
	VPCName           string
	VPCNamespace      string
	SubnetGroupName   string
	KMSDataKeyArn     string
	KMSConfigKeyArn   string
	Tags              map[string]*string
	Spec              v1alpha1.ValkeyInstanceSpec
	ComputeSubnets    []*ec2mv1beta1.Subnet
}

func GenerateValkeyInstanceObjects(
	instance v1alpha1.ValkeyInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var vpc ec2mv1beta1.VPC
	if err := base.ExtractRequiredResource(required, VPCKey, &vpc); err != nil {
		return nil, err
	}

	var elasticacheSubnetGroup elasticachemv1beta1.SubnetGroup
	if err := base.ExtractRequiredResource(required, ElasticacheSubnetGroupKey, &elasticacheSubnetGroup); err != nil {
		return nil, err
	}

	var kmsDataKey kmsmv1beta1.Key
	if err := base.ExtractRequiredResource(required, "KMSDataKey", &kmsDataKey); err != nil {
		return nil, err
	}

	var kmsConfigKey kmsmv1beta1.Key
	if err := base.ExtractRequiredResource(required, "KMSConfigKey", &kmsConfigKey); err != nil {
		return nil, err
	}

	computeSubnets, err := base.ExtractResources[*ec2mv1beta1.Subnet](required, ComputeSubnetsKey)
	if err != nil {
		return nil, err
	}

	region := ""
	if vpc.Spec.ForProvider.Region != nil {
		region = *vpc.Spec.ForProvider.Region
	}

	vpcID := ""
	if vpc.Status.AtProvider.ID != nil {
		vpcID = *vpc.Status.AtProvider.ID
	}

	subnetGroupName := ""
	if elasticacheSubnetGroup.Status.AtProvider.ID != nil {
		subnetGroupName = *elasticacheSubnetGroup.Status.AtProvider.ID
	}

	kmsDataKeyArn := ""
	if kmsDataKey.Status.AtProvider.Arn != nil {
		kmsDataKeyArn = *kmsDataKey.Status.AtProvider.Arn
	}

	kmsConfigKeyArn := ""
	if kmsConfigKey.Status.AtProvider.Arn != nil {
		kmsConfigKeyArn = *kmsConfigKey.Status.AtProvider.Arn
	}

	params := &valkeyInstanceParams{
		Name:              instance.GetName(),
		Namespace:         instance.GetNamespace(),
		ProviderConfigRef: env.AWSProvider,
		Region:            region,
		VPCID:             vpcID,
		VPCName:           vpc.Name,
		VPCNamespace:      vpc.Namespace,
		SubnetGroupName:   subnetGroupName,
		KMSDataKeyArn:     kmsDataKeyArn,
		KMSConfigKeyArn:   kmsConfigKeyArn,
		Tags:              env.Tags,
		Spec:              instance.Spec,
		ComputeSubnets:    computeSubnets,
	}

	objects := make(map[string]runtime.Object)
	addValkeyReplicationGroup(objects, params)
	addValkeySecurityGroup(objects, params)
	addValkeySecurityGroupRules(objects, params)
	addValkeySecretsManagerResources(objects, params, observed)
	addValkeyCredentialsSecret(objects, params, observed)

	return objects, nil
}

func addValkeyReplicationGroup(objects map[string]runtime.Object, p *valkeyInstanceParams) {
	providerConfigRef := &xpvcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.Name)

	atRestEncryption := "true"
	autoMinorVersionUpgrade := fmt.Sprintf("%t", p.Spec.AutoMinorVersionUpgrade)
	engine := "valkey"
	authTokenUpdateStrategy := "SET"
	finalSnapshotIdentifier := p.Name + "-final-snapshot"

	rg := &elasticachemv1beta1.ReplicationGroup{
		TypeMeta:   metav1.TypeMeta{APIVersion: elasticacheApiVersion, Kind: "ReplicationGroup"},
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
		Spec: elasticachemv1beta1.ReplicationGroupSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference:          providerConfigRef,
				WriteConnectionSecretToReference: &xpvcommon.LocalSecretReference{Name: p.Name},
			},
			ForProvider: elasticachemv1beta1.ReplicationGroupParameters{
				Region:                   &p.Region,
				Engine:                   &engine,
				Description:              &p.Name,
				EngineVersion:            &p.Spec.EngineVersion,
				NodeType:                 &p.Spec.InstanceType,
				NumCacheClusters:         &p.Spec.NumCacheClusters,
				AutomaticFailoverEnabled: base.BoolPtr(true),
				MultiAzEnabled:           base.BoolPtr(true),
				ApplyImmediately:         base.BoolPtr(true),
				AutoMinorVersionUpgrade:  &autoMinorVersionUpgrade,
				AtRestEncryptionEnabled:  &atRestEncryption,
				TransitEncryptionEnabled: base.BoolPtr(true),
				AuthTokenSecretRef: &xpv2v1.LocalSecretKeySelector{
					LocalSecretReference: xpv2v1.LocalSecretReference{Name: p.Name + "-auth-token"},
					Key:                  "auth-token",
				},
				AutoGenerateAuthToken:   base.BoolPtr(true),
				AuthTokenUpdateStrategy: &authTokenUpdateStrategy,
				KMSKeyID:                &p.KMSDataKeyArn,
				FinalSnapshotIdentifier: &finalSnapshotIdentifier,
				MaintenanceWindow:       &p.Spec.MaintenanceWindow,
				SnapshotWindow:          &p.Spec.SnapshotWindow,
				SnapshotRetentionLimit:  &p.Spec.SnapshotRetentionLimit,
				SubnetGroupName:         &p.SubnetGroupName,
				SecurityGroupIDRefs: []xpv2v1.NamespacedReference{
					{Name: p.Name},
				},
				Tags: tags,
			},
		},
	}

	if p.Spec.ParameterGroupName != "" {
		rg.Spec.ForProvider.ParameterGroupName = &p.Spec.ParameterGroupName
	}

	objects["replication-group"] = rg
}

func addValkeySecurityGroup(objects map[string]runtime.Object, p *valkeyInstanceParams) {
	providerConfigRef := &xpvcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}
	description := fmt.Sprintf("Security group for Valkey %s", p.Name)

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.Name)

	objects["security-group"] = &ec2mv1beta1.SecurityGroup{
		TypeMeta:   metav1.TypeMeta{APIVersion: ec2ApiVersion, Kind: "SecurityGroup"},
		ObjectMeta: metav1.ObjectMeta{Name: p.Name},
		Spec: ec2mv1beta1.SecurityGroupSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: providerConfigRef,
			},
			ForProvider: ec2mv1beta1.SecurityGroupParameters_2{
				Region:      &p.Region,
				Description: &description,
				VPCIDRef:    &xpv2v1.NamespacedReference{Name: p.VPCName, Namespace: p.VPCNamespace},
				Tags:        tags,
			},
		},
	}
}

func addValkeySecurityGroupRules(objects map[string]runtime.Object, p *valkeyInstanceParams) {
	providerConfigRef := &xpvcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}

	for _, subnet := range p.ComputeSubnets {
		cidrBlock := ""
		if subnet.Status.AtProvider.CidrBlock != nil {
			cidrBlock = *subnet.Status.AtProvider.CidrBlock
		}
		if cidrBlock == "" {
			continue
		}

		subnetName := subnet.GetName()
		ruleName := fmt.Sprintf("%s-ingress-%s", p.Name, subnetName)
		ingressType := "ingress"
		protocol := "tcp"
		port := float64(6379)
		description := fmt.Sprintf("Allow Valkey access from %s", subnetName)

		objects["sg-ingress-"+subnetName] = &ec2mv1beta1.SecurityGroupRule{
			TypeMeta:   metav1.TypeMeta{APIVersion: ec2ApiVersion, Kind: "SecurityGroupRule"},
			ObjectMeta: metav1.ObjectMeta{Name: ruleName},
			Spec: ec2mv1beta1.SecurityGroupRuleSpec{
				ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
					ProviderConfigReference: providerConfigRef,
				},
				ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
					Region:             &p.Region,
					Type:               &ingressType,
					SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: p.Name},
					Protocol:           &protocol,
					FromPort:           &port,
					ToPort:             &port,
					CidrBlocks:         []*string{&cidrBlock},
					Description:        &description,
				},
			},
		}
	}
}

func addValkeySecretsManagerResources(objects map[string]runtime.Object, p *valkeyInstanceParams, observed map[resource.Name]resource.ObservedComposed) {
	providerConfigRef := &xpvcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: p.ProviderConfigRef}

	tags := make(map[string]*string)
	for k, v := range p.Tags {
		tags[k] = v
	}
	tags["Name"] = base.StringPtr(p.Name)

	secretName := p.Name + "-credentials"
	description := fmt.Sprintf("Valkey connection credentials for %s", p.Name)
	recoveryWindow := float64(0)

	smSecretParams := smv1beta1.SecretParameters{
		Name:                 &secretName,
		Region:               &p.Region,
		RecoveryWindowInDays: &recoveryWindow,
		Description:          &description,
		Tags:                 tags,
	}
	if p.KMSConfigKeyArn != "" {
		smSecretParams.KMSKeyID = &p.KMSConfigKeyArn
	}

	objects["secrets-manager-secret"] = &smv1beta1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: secretsmanagerApiVersion, Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider:         smSecretParams,
		},
	}

	if _, ok := observed["credentials"]; !ok {
		return
	}

	objects["secrets-manager-secret-version"] = &smv1beta1.SecretVersion{
		TypeMeta:   metav1.TypeMeta{APIVersion: secretsmanagerApiVersion, Kind: "SecretVersion"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretVersionSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: providerConfigRef},
			ForProvider: smv1beta1.SecretVersionParameters{
				Region:      &p.Region,
				SecretIDRef: &xpvcommon.NamespacedReference{Name: secretName},
				SecretStringSecretRef: &xpvcommon.LocalSecretKeySelector{
					LocalSecretReference: xpvcommon.LocalSecretReference{Name: secretName},
					Key:                  "credentials.json",
				},
			},
		},
	}
}

func addValkeyCredentialsSecret(objects map[string]runtime.Object, p *valkeyInstanceParams, observed map[resource.Name]resource.ObservedComposed) {
	rgObserved, ok := observed["replication-group"]
	if !ok {
		return
	}
	connDetails := rgObserved.ConnectionDetails
	if len(connDetails) == 0 {
		return
	}

	authToken := string(connDetails["attribute.auth_token"])
	port := string(connDetails["port"])
	primaryEndpoint := string(connDetails["primary_endpoint_address"])
	readerEndpoint := string(connDetails["reader_endpoint_address"])

	if authToken == "" || primaryEndpoint == "" {
		return
	}

	credJSON, _ := json.Marshal(map[string]string{
		"AUTH_TOKEN":       authToken,
		"PORT":             port,
		"PRIMARY_ENDPOINT": primaryEndpoint,
		"READER_ENDPOINT":  readerEndpoint,
	})

	secretName := p.Name + "-credentials"

	objects["credentials"] = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: p.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"AUTH_TOKEN":       authToken,
			"PORT":             port,
			"PRIMARY_ENDPOINT": primaryEndpoint,
			"READER_ENDPOINT":  readerEndpoint,
			"credentials.json": string(credJSON),
		},
	}
}

func GetValkeyReplicationGroupReadyStatus(observed *composed.Unstructured) resource.Ready {
	address, addressFound, addressErr := unstructured.NestedString(observed.Object, "status", "atProvider", "primaryEndpointAddress")
	port, portFound, portErr := unstructured.NestedFloat64(observed.Object, "status", "atProvider", "port")
	notReady := !addressFound || addressErr != nil || address == "" ||
		!portFound || portErr != nil || port == 0
	if notReady {
		return resource.ReadyFalse
	}
	return base.GetCrossplaneReadyStatus(observed)
}

func GetValkeyStatusFromReplicationGroup(rg elasticachemv1beta1.ReplicationGroup) v1alpha1.ValkeyInstanceStatus {
	status := v1alpha1.ValkeyInstanceStatus{}

	if rg.Status.AtProvider.AutoMinorVersionUpgrade != nil {
		status.AutoMinorVersionUpgrade = *rg.Status.AtProvider.AutoMinorVersionUpgrade == "true"
	}
	base.SetString(rg.Status.AtProvider.KMSKeyID, &status.KMSKeyID)
	base.SetBool(rg.Status.AtProvider.MultiAzEnabled, &status.MultiAZEnabled)
	base.SetString(rg.Status.AtProvider.ParameterGroupName, &status.ParameterGroupName)

	if rg.Status.AtProvider.PrimaryEndpointAddress != nil && rg.Status.AtProvider.Port != nil {
		status.Endpoint = &v1alpha1.ValkeyInstanceEndpoint{
			Address: *rg.Status.AtProvider.PrimaryEndpointAddress,
			Port:    *rg.Status.AtProvider.Port,
		}
	}

	return status
}

func GetValkeySecurityGroupStatus(sg ec2mv1beta1.SecurityGroup) *v1alpha1.ValkeyInstanceSecurityGroup {
	sgStatus := &v1alpha1.ValkeyInstanceSecurityGroup{}

	if sg.Status.AtProvider.Tags != nil {
		if name, ok := sg.Status.AtProvider.Tags["Name"]; ok && name != nil {
			sgStatus.Name = *name
		}
	}
	base.SetString(sg.Status.AtProvider.Description, &sgStatus.Description)
	base.SetString(sg.Status.AtProvider.ID, &sgStatus.ID)
	base.SetString(sg.Status.AtProvider.Arn, &sgStatus.Arn)

	return sgStatus
}

func GetValkeySecurityGroupRuleStatus(sgr ec2mv1beta1.SecurityGroupRule) v1alpha1.ValkeyInstanceSecurityGroupRule {
	rule := v1alpha1.ValkeyInstanceSecurityGroupRule{}

	if sgr.Status.AtProvider.CidrBlocks != nil {
		for _, cidr := range sgr.Status.AtProvider.CidrBlocks {
			if cidr != nil {
				rule.CidrBlocks = append(rule.CidrBlocks, *cidr)
			}
		}
	}
	base.SetString(sgr.Status.AtProvider.Description, &rule.Description)
	base.SetString(sgr.Status.AtProvider.Protocol, &rule.Protocol)
	base.SetString(sgr.Status.AtProvider.Type, &rule.Type)
	if sgr.Status.AtProvider.FromPort != nil {
		rule.FromPort = int(*sgr.Status.AtProvider.FromPort)
	}
	if sgr.Status.AtProvider.ToPort != nil {
		rule.ToPort = int(*sgr.Status.AtProvider.ToPort)
	}
	if sgr.Status.AtProvider.Self != nil {
		rule.Self = *sgr.Status.AtProvider.Self
	}

	return rule
}
