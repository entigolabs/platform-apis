package service

import (
	"encoding/json"
	"fmt"
	"log"
	"maps"

	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
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

type valkeyInstanceGenerator struct {
	// Inputs
	instance v1alpha1.ValkeyInstance
	observed map[resource.Name]resource.ObservedComposed
	env      apis.Environment
	// Dependencies
	vpc                    ec2mv1beta1.VPC
	elasticacheSubnetGroup elasticachemv1beta1.SubnetGroup
	kmsDataKey             kmsmv1beta1.Key
	kmsConfigKey           kmsmv1beta1.Key
	computeSubnets         []*ec2mv1beta1.Subnet
	// Derived values
	region          string
	vpcID           string
	subnetGroupName string
	kmsDataKeyArn   string
	kmsConfigKeyArn string
}

func GenerateValkeyInstanceObjects(
	instance v1alpha1.ValkeyInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	g, err := newValkeyInstanceGenerator(instance, required, observed)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newValkeyInstanceGenerator(
	instance v1alpha1.ValkeyInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*valkeyInstanceGenerator, error) {
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

	g := &valkeyInstanceGenerator{
		instance:               instance,
		observed:               observed,
		env:                    env,
		vpc:                    vpc,
		elasticacheSubnetGroup: elasticacheSubnetGroup,
		kmsDataKey:             kmsDataKey,
		kmsConfigKey:           kmsConfigKey,
		computeSubnets:         computeSubnets,
	}

	g.computeDerivedValues()
	return g, nil
}

func (g *valkeyInstanceGenerator) computeDerivedValues() {
	if g.vpc.Spec.ForProvider.Region != nil {
		g.region = *g.vpc.Spec.ForProvider.Region
	}

	if g.vpc.Status.AtProvider.ID != nil {
		g.vpcID = *g.vpc.Status.AtProvider.ID
	}

	if g.elasticacheSubnetGroup.Status.AtProvider.ID != nil {
		g.subnetGroupName = *g.elasticacheSubnetGroup.Status.AtProvider.ID
	}

	if g.kmsDataKey.Status.AtProvider.Arn != nil {
		g.kmsDataKeyArn = *g.kmsDataKey.Status.AtProvider.Arn
	}

	if g.kmsConfigKey.Status.AtProvider.Arn != nil {
		g.kmsConfigKeyArn = *g.kmsConfigKey.Status.AtProvider.Arn
	}
}

func (g *valkeyInstanceGenerator) generate() (map[string]runtime.Object, error) {
	objects := make(map[string]runtime.Object)

	g.buildReplicationGroup(objects)
	g.buildSecurityGroup(objects)
	g.buildSecurityGroupRules(objects)
	g.buildSecretsManagerResources(objects)
	g.buildCredentialsSecret(objects)

	return objects, nil
}

func (g *valkeyInstanceGenerator) providerConfigRef() *xpvcommon.ProviderConfigReference {
	return &xpvcommon.ProviderConfigReference{Kind: "ClusterProviderConfig", Name: g.env.AWSProvider}
}

func (g *valkeyInstanceGenerator) buildTags() map[string]*string {
	tags := make(map[string]*string)
	maps.Copy(tags, g.env.Tags)
	tags["Name"] = base.StringPtr(g.instance.GetName())
	return tags
}

func (g *valkeyInstanceGenerator) buildReplicationGroup(objects map[string]runtime.Object) {
	name := g.instance.GetName()
	tags := g.buildTags()

	atRestEncryption := "true"
	autoMinorVersionUpgrade := fmt.Sprintf("%t", g.instance.Spec.AutoMinorVersionUpgrade)
	engine := "valkey"
	authTokenUpdateStrategy := "SET"
	finalSnapshotIdentifier := name + "-final-snapshot"

	rg := &elasticachemv1beta1.ReplicationGroup{
		TypeMeta:   metav1.TypeMeta{APIVersion: elasticacheApiVersion, Kind: "ReplicationGroup"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: elasticachemv1beta1.ReplicationGroupSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference:          g.providerConfigRef(),
				WriteConnectionSecretToReference: &xpv2v1.LocalSecretReference{Name: name},
			},
			ForProvider: elasticachemv1beta1.ReplicationGroupParameters{
				Region:                   &g.region,
				Engine:                   &engine,
				Description:              &name,
				EngineVersion:            &g.instance.Spec.EngineVersion,
				NodeType:                 &g.instance.Spec.InstanceType,
				NumCacheClusters:         &g.instance.Spec.NumCacheClusters,
				AutomaticFailoverEnabled: base.BoolPtr(true),
				MultiAzEnabled:           base.BoolPtr(true),
				ApplyImmediately:         base.BoolPtr(true),
				AutoMinorVersionUpgrade:  &autoMinorVersionUpgrade,
				AtRestEncryptionEnabled:  &atRestEncryption,
				TransitEncryptionEnabled: base.BoolPtr(true),
				AuthTokenSecretRef: &xpv2v1.LocalSecretKeySelector{
					LocalSecretReference: xpv2v1.LocalSecretReference{Name: name + "-auth-token"},
					Key:                  "auth-token",
				},
				AutoGenerateAuthToken:   base.BoolPtr(true),
				AuthTokenUpdateStrategy: &authTokenUpdateStrategy,
				KMSKeyID:                &g.kmsDataKeyArn,
				FinalSnapshotIdentifier: &finalSnapshotIdentifier,
				MaintenanceWindow:       &g.instance.Spec.MaintenanceWindow,
				SnapshotWindow:          &g.instance.Spec.SnapshotWindow,
				SnapshotRetentionLimit:  &g.instance.Spec.SnapshotRetentionLimit,
				SubnetGroupName:         &g.subnetGroupName,
				SecurityGroupIDRefs: []xpv2v1.NamespacedReference{
					{Name: name},
				},
				Tags: tags,
			},
		},
	}

	if g.instance.Spec.ParameterGroupName != "" {
		rg.Spec.ForProvider.ParameterGroupName = &g.instance.Spec.ParameterGroupName
	}

	objects["replication-group"] = rg
}

func (g *valkeyInstanceGenerator) buildSecurityGroup(objects map[string]runtime.Object) {
	name := g.instance.GetName()
	description := fmt.Sprintf("Security group for Valkey %s", name)
	tags := g.buildTags()

	objects["security-group"] = &ec2mv1beta1.SecurityGroup{
		TypeMeta:   metav1.TypeMeta{APIVersion: ec2ApiVersion, Kind: "SecurityGroup"},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: ec2mv1beta1.SecurityGroupSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: g.providerConfigRef(),
			},
			ForProvider: ec2mv1beta1.SecurityGroupParameters_2{
				Region:      &g.region,
				Description: &description,
				VPCIDRef:    &xpv2v1.NamespacedReference{Name: g.vpc.Name, Namespace: g.vpc.Namespace},
				Tags:        tags,
			},
		},
	}
}

func (g *valkeyInstanceGenerator) buildSecurityGroupRules(objects map[string]runtime.Object) {
	name := g.instance.GetName()

	for _, subnet := range g.computeSubnets {
		cidrBlock := ""
		if subnet.Status.AtProvider.CidrBlock != nil {
			cidrBlock = *subnet.Status.AtProvider.CidrBlock
		}
		if cidrBlock == "" {
			continue
		}

		subnetName := subnet.GetName()
		ruleName := fmt.Sprintf("%s-ingress-%s", name, subnetName)
		ingressType := "ingress"
		protocol := "tcp"
		port := float64(6379)
		description := fmt.Sprintf("Allow Valkey access from %s", subnetName)

		objects["sg-ingress-"+subnetName] = &ec2mv1beta1.SecurityGroupRule{
			TypeMeta:   metav1.TypeMeta{APIVersion: ec2ApiVersion, Kind: "SecurityGroupRule"},
			ObjectMeta: metav1.ObjectMeta{Name: ruleName},
			Spec: ec2mv1beta1.SecurityGroupRuleSpec{
				ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
					ProviderConfigReference: g.providerConfigRef(),
				},
				ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
					Region:             &g.region,
					Type:               &ingressType,
					SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: name},
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

func (g *valkeyInstanceGenerator) buildSecretsManagerResources(objects map[string]runtime.Object) {
	name := g.instance.GetName()
	tags := g.buildTags()

	secretName := name + "-credentials"
	description := fmt.Sprintf("Valkey connection credentials for %s", name)
	recoveryWindow := float64(0)

	smSecretParams := smv1beta1.SecretParameters{
		Name:                 &secretName,
		Region:               &g.region,
		RecoveryWindowInDays: &recoveryWindow,
		Description:          &description,
		Tags:                 tags,
	}
	if g.kmsConfigKeyArn != "" {
		smSecretParams.KMSKeyID = &g.kmsConfigKeyArn
	}

	objects["secrets-manager-secret"] = &smv1beta1.Secret{
		TypeMeta:   metav1.TypeMeta{APIVersion: secretsmanagerApiVersion, Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider:         smSecretParams,
		},
	}

	if _, ok := g.observed["credentials"]; !ok {
		return
	}

	objects["secrets-manager-secret-version"] = &smv1beta1.SecretVersion{
		TypeMeta:   metav1.TypeMeta{APIVersion: secretsmanagerApiVersion, Kind: "SecretVersion"},
		ObjectMeta: metav1.ObjectMeta{Name: secretName},
		Spec: smv1beta1.SecretVersionSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{ProviderConfigReference: g.providerConfigRef()},
			ForProvider: smv1beta1.SecretVersionParameters{
				Region:      &g.region,
				SecretIDRef: &xpv2v1.NamespacedReference{Name: secretName},
				SecretStringSecretRef: &xpv2v1.LocalSecretKeySelector{
					LocalSecretReference: xpv2v1.LocalSecretReference{Name: secretName},
					Key:                  "credentials.json",
				},
			},
		},
	}
}

func (g *valkeyInstanceGenerator) buildCredentialsSecret(objects map[string]runtime.Object) {
	rgObserved, ok := g.observed["replication-group"]
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

	credJSON := fmt.Sprintf(
		`{"AUTH_TOKEN": %s, "PORT": %s, "PRIMARY_ENDPOINT": %s, "READER_ENDPOINT": %s}`,
		mustJSONString(authToken),
		mustJSONString(port),
		mustJSONString(primaryEndpoint),
		mustJSONString(readerEndpoint),
	)

	name := g.instance.GetName()
	secretName := name + "-credentials"

	objects["credentials"] = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: g.instance.GetNamespace(),
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"AUTH_TOKEN":       authToken,
			"PORT":             port,
			"PRIMARY_ENDPOINT": primaryEndpoint,
			"READER_ENDPOINT":  readerEndpoint,
			"credentials.json": credJSON,
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

func mustJSONString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		log.Printf("mustJSONString: failed to marshal string: %v", err)
		return ""
	}
	return string(b)
}
