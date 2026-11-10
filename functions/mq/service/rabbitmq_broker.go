package service

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"maps"
	"math/big"
	"strings"

	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	ec2mv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/ec2/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/kms/v1beta1"
	mqv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/mq/v1beta1"
	rdsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/rds/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ec2ApiVersion = "ec2.aws.m.upbound.io/v1beta1"
	mqApiVersion  = "mq.aws.m.upbound.io/v1beta1"

	adminUsername = "mqadmin"

	credentialsKey = "credentials"
)

type rabbitMQBrokerGenerator struct {
	rabbitMQBroker v1alpha1.RabbitMQBroker
	observed       map[resource.Name]resource.ObservedComposed
	env            apis.Environment
	hash           string
	vpc            ec2mv1beta1.VPC
	kmsDataKey     kmsmv1beta1.Key
	subnetGroup    rdsmv1beta1.SubnetGroup
	names          resourceNames
	readinessMap   map[resource.Name]bool
	username       string
	password       string
}

type resourceNames struct {
	sg, sgIngress, sgConsoleIngress, sgMgmtIngress, sgEgress, configuration, broker resource.Name
}

func GenerateRabbitMQBrokerObjects(
	rabbitMQBroker v1alpha1.RabbitMQBroker,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]client.Object, error) {
	g, err := newRabbitMQBrokerGenerator(rabbitMQBroker, required, observed)
	if err != nil {
		return nil, err
	}

	return g.generate()
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func newRabbitMQBrokerGenerator(
	rabbitMQBroker v1alpha1.RabbitMQBroker,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*rabbitMQBrokerGenerator, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var vpc ec2mv1beta1.VPC
	var kmsDataKey kmsmv1beta1.Key
	var subnetGroup rdsmv1beta1.SubnetGroup

	if err := base.ExtractRequiredResource(required, "VPC", &vpc); err != nil {
		return nil, err
	}
	if _, err := base.ExtractOptionalResource(required, "KMSDataKey", &kmsDataKey); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "MQSubnetGroup", &subnetGroup); err != nil {
		return nil, err
	}

	password, err := resolvePassword(observed)
	if err != nil {
		return nil, err
	}

	g := &rabbitMQBrokerGenerator{
		rabbitMQBroker: rabbitMQBroker,
		observed:       observed,
		env:            env,
		hash:           base.GenerateFNVHash(rabbitMQBroker.UID),
		vpc:            vpc,
		kmsDataKey:     kmsDataKey,
		subnetGroup:    subnetGroup,
		username:       adminUsername,
		password:       password,
	}

	g.generateNames()

	g.readinessMap = make(map[resource.Name]bool)
	for name, obs := range observed {
		g.readinessMap[name] = isResourceReady(obs.Resource)
	}

	return g, nil
}

func (g *rabbitMQBrokerGenerator) generate() (map[string]client.Object, error) {
	desired := make(map[string]client.Object)

	if !g.rabbitMQBroker.Spec.PubliclyAccessible {
		maps.Copy(desired, g.buildSecurityGroup())
	}

	desired[credentialsKey] = g.buildCredentialsSecret()

	// Amazon MQ cannot disassociate/revert a broker's configuration once set, only switch to another.
	// The XRD enforces that spec.configuration cannot be removed once set (only edited), so a nil
	// configuration here means it was never set.
	if g.rabbitMQBroker.Spec.Configuration != nil {
		desired[string(g.names.configuration)] = g.buildConfiguration()
		g.keepStaleConfigurations(desired)
	}

	desired[string(g.names.broker)] = g.buildBroker()

	return desired, nil
}

func (g *rabbitMQBrokerGenerator) engineVersion() string {
	if g.rabbitMQBroker.Spec.EngineVersion != nil {
		return *g.rabbitMQBroker.Spec.EngineVersion
	}
	return ""
}

// keepStaleConfigurations re-emits previously created Configuration managed resources for other engine
// versions so they are not garbage-collected while the broker is still switching to the current one.
// Once the broker reports it has switched to the current Configuration, the stale ones are dropped from
// desired and get deleted.
func (g *rabbitMQBrokerGenerator) keepStaleConfigurations(desired map[string]client.Object) {
	if g.brokerSwitchedToConfiguration(g.names.configuration) {
		return
	}
	for name, observed := range g.observed {
		if name == g.names.configuration || observed.Resource.GetKind() != "Configuration" {
			continue
		}
		desired[string(name)] = staleConfiguration(observed)
	}
}

// brokerSwitchedToConfiguration reports whether the observed broker has the given Configuration applied
// (its AWS configuration id matches the Configuration's external name).
func (g *rabbitMQBrokerGenerator) brokerSwitchedToConfiguration(configName resource.Name) bool {
	brokerObserved, ok := g.observed[g.names.broker]
	if !ok {
		return false
	}
	appliedID, found, _ := unstructured.NestedString(brokerObserved.Resource.Object, "status", "atProvider", "configuration", "id")
	if !found || appliedID == "" {
		return false
	}
	configObserved, ok := g.observed[configName]
	if !ok {
		return false
	}
	return observedAWSID(configObserved) == appliedID
}

func staleConfiguration(observed resource.ObservedComposed) client.Object {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion(observed.Resource.GetAPIVersion())
	obj.SetKind(observed.Resource.GetKind())
	obj.SetName(observed.Resource.GetName())
	obj.SetNamespace(observed.Resource.GetNamespace())
	if spec, found, _ := unstructured.NestedMap(observed.Resource.Object, "spec"); found {
		_ = unstructured.SetNestedMap(obj.Object, spec, "spec")
	}
	return obj
}

func (g *rabbitMQBrokerGenerator) buildConfiguration() client.Object {
	configName := string(g.names.configuration)
	region := g.vpc.Spec.ForProvider.Region
	return &mqv1beta1.Configuration{
		TypeMeta:   metav1.TypeMeta{Kind: "Configuration", APIVersion: mqApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: configName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: mqv1beta1.ConfigurationSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: mqv1beta1.ConfigurationParameters_2{
				Name:          &configName,
				EngineType:    g.rabbitMQBroker.Spec.EngineType,
				EngineVersion: g.rabbitMQBroker.Spec.EngineVersion,
				Data:          &g.rabbitMQBroker.Spec.Configuration.Data,
				Description:   g.rabbitMQBroker.Spec.Configuration.Description,
				Region:        region,
			},
		},
	}
}

// brokerConfigurationTarget is the Configuration the broker gets wired to and the engine version that goes
// with it.
type brokerConfigurationTarget struct {
	name          string
	id            *string
	revision      *float64
	engineVersion *string
}

// brokerConfigurationTarget picks the Configuration for the broker. The Configuration for the desired
// engine version is wired, with its AWS id set explicitly, as soon as it is observed with that id. Until
// then the broker stays on the Configuration it currently has applied, at that Configuration's engine
// version, so an engine-version bump reaches Amazon MQ as one UpdateBroker carrying both the new engine
// version and the new Configuration id. Relying on the provider's reference resolution alone sent the
// engine upgrade with the previous Configuration id and cost a second reboot after the upgrade to switch.
// On first creation nothing is observed yet and the broker is left to the provider's reference
// resolution, as before.
func (g *rabbitMQBrokerGenerator) brokerConfigurationTarget() brokerConfigurationTarget {
	desired := brokerConfigurationTarget{
		name:          string(g.names.configuration),
		engineVersion: g.rabbitMQBroker.Spec.EngineVersion,
	}
	if observed, ok := g.observed[g.names.configuration]; ok {
		desired.revision = latestRevision(observed)
		if id := observedAWSID(observed); id != "" {
			desired.id = &id
			return desired
		}
	}

	appliedName, applied, ok := g.appliedConfiguration()
	if !ok {
		return desired
	}
	appliedID := observedAWSID(applied)
	target := brokerConfigurationTarget{
		name:          appliedName,
		id:            &appliedID,
		revision:      latestRevision(applied),
		engineVersion: desired.engineVersion,
	}
	if appliedVersion, found, _ := unstructured.NestedString(applied.Resource.Object, "spec", "forProvider", "engineVersion"); found && appliedVersion != "" {
		target.engineVersion = &appliedVersion
	}
	return target
}

// appliedConfiguration returns the observed Configuration whose AWS id the broker currently reports as
// applied, if both are observed.
func (g *rabbitMQBrokerGenerator) appliedConfiguration() (string, resource.ObservedComposed, bool) {
	brokerObserved, ok := g.observed[g.names.broker]
	if !ok {
		return "", resource.ObservedComposed{}, false
	}
	appliedID, found, _ := unstructured.NestedString(brokerObserved.Resource.Object, "status", "atProvider", "configuration", "id")
	if !found || appliedID == "" {
		return "", resource.ObservedComposed{}, false
	}
	for name, observed := range g.observed {
		if observed.Resource.GetKind() != "Configuration" {
			continue
		}
		if observedAWSID(observed) == appliedID {
			return string(name), observed, true
		}
	}
	return "", resource.ObservedComposed{}, false
}

// observedAWSID is the AWS id of an observed managed resource: status.atProvider.id, falling back to the
// crossplane external-name annotation the provider sets right after creation.
func observedAWSID(observed resource.ObservedComposed) string {
	if id, found, _ := unstructured.NestedString(observed.Resource.Object, "status", "atProvider", "id"); found && id != "" {
		return id
	}
	return observed.Resource.GetAnnotations()["crossplane.io/external-name"]
}

func latestRevision(observed resource.ObservedComposed) *float64 {
	revision, found, err := unstructured.NestedFloat64(observed.Resource.Object, "status", "atProvider", "latestRevision")
	if err != nil || !found {
		return nil
	}
	return &revision
}

func resolvePassword(observed map[resource.Name]resource.ObservedComposed) (string, error) {
	observedSecret, ok := observed[credentialsKey]
	if !ok {
		return generatePassword()
	}

	passwordB64, found, _ := unstructured.NestedString(observedSecret.Resource.Object, "data", "password")
	if !found || passwordB64 == "" {
		return generatePassword()
	}

	passwordBytes, err := base64.StdEncoding.DecodeString(passwordB64)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode existing credentials secret: %w", err)
	}
	return string(passwordBytes), nil
}

func generatePassword() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 32)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		b[i] = chars[n.Int64()]
	}
	return string(b), nil
}

func GetCredentialsSecretName(rabbitMQBrokerName string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-credentials", rabbitMQBrokerName))
}

func GetConnectionSecretName(rabbitMQBrokerName string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-connection", rabbitMQBrokerName))
}

func (g *rabbitMQBrokerGenerator) buildCredentialsSecret() client.Object {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetCredentialsSecretName(g.rabbitMQBroker.Name),
			Namespace: g.rabbitMQBroker.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"username": g.username,
			"password": g.password,
		},
	}
}

func GetSGName(RabbitMQBrokerName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-%s", RabbitMQBrokerName, hash))
}

func GetSGIngressName(RabbitMQBrokerName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-ingress-%s", RabbitMQBrokerName, hash))
}

func GetSGConsoleIngressName(RabbitMQBrokerName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-console-ingress-%s", RabbitMQBrokerName, hash))
}

func GetSGMgmtIngressName(RabbitMQBrokerName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-mgmt-ingress-%s", RabbitMQBrokerName, hash))
}

func GetSGEgressName(RabbitMQBrokerName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-egress-%s", RabbitMQBrokerName, hash))
}

func GetBrokerName(RabbitMQBrokerName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-broker-%s", RabbitMQBrokerName, hash))
}

// GetConfigurationName is version-scoped: Amazon MQ Configuration engineVersion is immutable, so an
// engine-version bump must create a new Configuration (new name) rather than mutate the existing one.
func GetConfigurationName(RabbitMQBrokerName string, engineVersion string, hash string) string {
	version := strings.ReplaceAll(engineVersion, ".", "-")
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-config-%s-%s", RabbitMQBrokerName, version, hash))
}

func (g *rabbitMQBrokerGenerator) generateNames() {
	g.names.sg = resource.Name(GetSGName(g.rabbitMQBroker.Name, g.hash))
	g.names.sgIngress = resource.Name(GetSGIngressName(g.rabbitMQBroker.Name, g.hash))
	g.names.sgConsoleIngress = resource.Name(GetSGConsoleIngressName(g.rabbitMQBroker.Name, g.hash))
	g.names.sgMgmtIngress = resource.Name(GetSGMgmtIngressName(g.rabbitMQBroker.Name, g.hash))
	g.names.sgEgress = resource.Name(GetSGEgressName(g.rabbitMQBroker.Name, g.hash))
	g.names.configuration = resource.Name(GetConfigurationName(g.rabbitMQBroker.Name, g.engineVersion(), g.hash))
	g.names.broker = resource.Name(GetBrokerName(g.rabbitMQBroker.Name, g.hash))
}

func (g *rabbitMQBrokerGenerator) buildSecurityGroup() map[string]client.Object {
	groups := make(map[string]client.Object)
	sgName := string(g.names.sg)
	region := g.vpc.Spec.ForProvider.Region
	description := "allow traffic from vpc"

	securityGroup := &ec2mv1beta1.SecurityGroup{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroup", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: sgName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: ec2mv1beta1.SecurityGroupSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupParameters_2{
				Region:      region,
				VPCIDRef:    &xpv2v1.NamespacedReference{Name: g.vpc.Name, Namespace: g.vpc.Namespace},
				Description: &description,
				Tags:        map[string]*string{"Name": &sgName},
			},
		},
	}
	groups[sgName] = securityGroup

	cidrBlock := "0.0.0.0/0"
	amqpsPort := float64(5671)
	consolePort := float64(443)
	mgmtPort := float64(15671)

	ingressName := string(g.names.sgIngress)
	groups[ingressName] = &ec2mv1beta1.SecurityGroupRule{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroupRule", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: ingressName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: ec2mv1beta1.SecurityGroupRuleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: sgName},
				Type:               new("ingress"),
				FromPort:           &amqpsPort,
				ToPort:             &amqpsPort,
				Protocol:           new("tcp"),
				CidrBlocks:         []*string{&cidrBlock},
				Description:        new("allow amqps from vpc"),
			},
		},
	}

	consoleIngressName := string(g.names.sgConsoleIngress)
	groups[consoleIngressName] = &ec2mv1beta1.SecurityGroupRule{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroupRule", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: consoleIngressName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: ec2mv1beta1.SecurityGroupRuleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: sgName},
				Type:               new("ingress"),
				FromPort:           &consolePort,
				ToPort:             &consolePort,
				Protocol:           new("tcp"),
				CidrBlocks:         []*string{&cidrBlock},
				Description:        new("allow management console from vpc"),
			},
		},
	}

	mgmtIngressName := string(g.names.sgMgmtIngress)
	groups[mgmtIngressName] = &ec2mv1beta1.SecurityGroupRule{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroupRule", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: mgmtIngressName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: ec2mv1beta1.SecurityGroupRuleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: sgName},
				Type:               new("ingress"),
				FromPort:           &mgmtPort,
				ToPort:             &mgmtPort,
				Protocol:           new("tcp"),
				CidrBlocks:         []*string{&cidrBlock},
				Description:        new("allow management api from vpc"),
			},
		},
	}

	egressName := string(g.names.sgEgress)
	egressPort := float64(0)
	egressRule := &ec2mv1beta1.SecurityGroupRule{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroupRule", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: egressName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: ec2mv1beta1.SecurityGroupRuleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: sgName},
				Type:               new("egress"),
				FromPort:           &egressPort,
				ToPort:             &egressPort,
				Protocol:           new("-1"),
				CidrBlocks:         []*string{&cidrBlock},
				Description:        &description,
			},
		},
	}
	groups[egressName] = egressRule
	return groups
}

func (g *rabbitMQBrokerGenerator) buildBroker() client.Object {
	brokerName := string(g.names.broker)
	region := g.vpc.Spec.ForProvider.Region

	availableSubnets := g.subnetGroup.Status.AtProvider.SubnetIds
	deploymentMode := ""
	if g.rabbitMQBroker.Spec.DeploymentMode != nil {
		deploymentMode = *g.rabbitMQBroker.Spec.DeploymentMode
	}
	var subnetIds []*string
	if deploymentMode == "SINGLE_INSTANCE" || deploymentMode == "" {
		if len(availableSubnets) > 0 {
			subnetIds = availableSubnets[:1]
		}
	} else {
		subnetIds = append(subnetIds, availableSubnets...)
	}

	broker := &mqv1beta1.Broker{
		TypeMeta:   metav1.TypeMeta{Kind: "Broker", APIVersion: mqApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: brokerName, Namespace: g.rabbitMQBroker.Namespace},
		Spec: mqv1beta1.BrokerSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference:          &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
				WriteConnectionSecretToReference: &xpv2v1.LocalSecretReference{Name: GetConnectionSecretName(g.rabbitMQBroker.Name)},
			},
			ForProvider: mqv1beta1.BrokerParameters{
				BrokerName:              &brokerName,
				ApplyImmediately:        new(true),
				AutoMinorVersionUpgrade: &g.rabbitMQBroker.Spec.AutoMinorVersionUpgrade,
				DeploymentMode:          g.rabbitMQBroker.Spec.DeploymentMode,
				EngineType:              g.rabbitMQBroker.Spec.EngineType,
				EngineVersion:           g.rabbitMQBroker.Spec.EngineVersion,
				HostInstanceType:        g.rabbitMQBroker.Spec.InstanceType,
				PubliclyAccessible:      &g.rabbitMQBroker.Spec.PubliclyAccessible,
				Region:                  region,
				EncryptionOptions: &mqv1beta1.EncryptionOptionsParameters{
					KMSKeyID:       g.kmsDataKeyIDRef(),
					UseAwsOwnedKey: new(false),
				},
				SubnetIds: subnetIds,
			},
			InitProvider: mqv1beta1.BrokerInitParameters{
				User: []mqv1beta1.UserInitParameters{{
					Username:      new(g.username),
					ConsoleAccess: new(true),
					PasswordSecretRef: xpv2v1.LocalSecretKeySelector{
						LocalSecretReference: xpv2v1.LocalSecretReference{Name: GetCredentialsSecretName(g.rabbitMQBroker.Name)},
						Key:                  "password",
					},
				}},
			},
		},
	}

	if !g.rabbitMQBroker.Spec.PubliclyAccessible {
		broker.Spec.ForProvider.SecurityGroupRefs = []xpv2v1.NamespacedReference{{Name: string(g.names.sg)}}
		// Also set the resolved id once the SecurityGroup is observed. The provider's reference resolver
		// persists resolved values with a server-side-apply patch that carries only the fields it changed,
		// so a field only it owns is dropped whenever it re-resolves another reference on the broker (it
		// cleared securityGroups when configuration.id changed on an engine bump, and the next update went
		// to Amazon MQ with an empty security-group list). Owning the value here as well keeps it in place.
		if sgObserved, ok := g.observed[g.names.sg]; ok {
			if id := observedAWSID(sgObserved); id != "" {
				broker.Spec.ForProvider.SecurityGroups = []*string{&id}
			}
		}
	}

	if g.rabbitMQBroker.Spec.Configuration != nil {
		target := g.brokerConfigurationTarget()
		resolveAlways := xpv2v1.ResolvePolicyAlways
		broker.Spec.ForProvider.Configuration = &mqv1beta1.ConfigurationParameters{
			ID: target.id,
			IDRef: &xpv2v1.NamespacedReference{
				Name:      target.name,
				Namespace: g.rabbitMQBroker.Namespace,
				Policy:    &xpv2v1.Policy{Resolve: &resolveAlways},
			},
			Revision: target.revision,
		}
		broker.Spec.ForProvider.EngineVersion = target.engineVersion
	}
	if g.rabbitMQBroker.Spec.MaintenanceWindowStartTime != nil {
		broker.Spec.ForProvider.MaintenanceWindowStartTime = &mqv1beta1.MaintenanceWindowStartTimeParameters{
			DayOfWeek: g.rabbitMQBroker.Spec.MaintenanceWindowStartTime.DayOfWeek,
			TimeOfDay: g.rabbitMQBroker.Spec.MaintenanceWindowStartTime.TimeOfDay,
			TimeZone:  g.rabbitMQBroker.Spec.MaintenanceWindowStartTime.TimeZone,
		}
	}

	broker.SetManagementPolicies(xpv2v1.ManagementPolicies{"*"})
	return broker
}

func isResourceReady(observed *composed.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(observed.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}
		if conditionMap["type"] == "Ready" && conditionMap["status"] == "True" {
			return true
		}
	}
	return false
}

func GetRabbitMQBrokerStatusFromBroker(broker mqv1beta1.Broker) v1alpha1.RabbitMQBrokerStatus {
	atProvider := broker.Status.AtProvider

	status := v1alpha1.RabbitMQBrokerStatus{
		AutoMinorVersionUpgrade:    atProvider.AutoMinorVersionUpgrade,
		AmazonMQBrokerID:           atProvider.ID,
		BrokerName:                 atProvider.BrokerName,
		DeploymentMode:             atProvider.DeploymentMode,
		EngineType:                 atProvider.EngineType,
		EngineVersion:              atProvider.EngineVersion,
		InstanceType:               atProvider.HostInstanceType,
		PendingDataReplicationMode: atProvider.PendingDataReplicationMode,
		PubliclyAccessible:         atProvider.PubliclyAccessible,
		Region:                     atProvider.Region,
		StorageType:                atProvider.StorageType,
		SecurityGroups:             atProvider.SecurityGroups,
		SubnetIds:                  atProvider.SubnetIds,
	}

	if atProvider.Configuration != nil {
		status.Configuration = &v1alpha1.RabbitMQBrokerConfigurationObservation{
			ID:       atProvider.Configuration.ID,
			Revision: atProvider.Configuration.Revision,
		}
	}

	if atProvider.EncryptionOptions != nil {
		status.EncryptionOptions = &v1alpha1.RabbitMQBrokerEncryptionOptionsObservation{
			KMSKeyID:       atProvider.EncryptionOptions.KMSKeyID,
			UseAwsOwnedKey: atProvider.EncryptionOptions.UseAwsOwnedKey,
		}
	}

	if atProvider.MaintenanceWindowStartTime != nil {
		status.MaintenanceWindowStartTime = &v1alpha1.RabbitMQBrokerMaintenanceWindowStartTimeObservation{
			DayOfWeek: atProvider.MaintenanceWindowStartTime.DayOfWeek,
			TimeOfDay: atProvider.MaintenanceWindowStartTime.TimeOfDay,
			TimeZone:  atProvider.MaintenanceWindowStartTime.TimeZone,
		}
	}

	for _, instanceOb := range atProvider.Instances {
		status.Instances = append(status.Instances, v1alpha1.RabbitMQBrokerInstancesObservation{
			ConsoleURL: instanceOb.ConsoleURL,
			Endpoints:  instanceOb.Endpoints,
			IPAddress:  instanceOb.IPAddress,
		})
	}

	return status
}

func GetRabbitMQBrokerReadyStatus(observed *composed.Unstructured) resource.Ready {
	if isResourceReady(observed) {
		return resource.ReadyTrue
	}
	return resource.ReadyFalse
}

// kmsDataKeyIDRef returns the data KMS key ARN, or nil where the environment has no KMS
// module. EncryptionOptionsParameters has no KMSKeyIDRef/Selector, so the ARN has to be
// resolved here rather than left to the provider. UseAwsOwnedKey stays false either way,
// so Amazon MQ creates an AWS-managed CMK aliased to aws/mq in the account rather than
// using an AWS-owned key outside it.
func (g *rabbitMQBrokerGenerator) kmsDataKeyIDRef() *string {
	return g.kmsDataKey.Status.AtProvider.Arn
}
