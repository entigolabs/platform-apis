package service

import (
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/crossplane-runtime/v2/pkg/errors"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	ec2mv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/ec2/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/kms/v1beta1"
	rdsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/rds/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	ec2ApiVersion           = "ec2.aws.m.upbound.io/v1beta1"
	rdsApiVersion           = "rds.aws.m.upbound.io/v1beta1"
	parameterGroupKeyPrefix = "parameter-group-"

	// parameterGroupApplyMethodKey is a reserved key in ParameterGroupParameters: it sets how every
	// parameter in the group is applied ("immediate" or "pending-reboot") and is not itself a DB
	// parameter. AWS rejects "immediate" for static parameters (e.g. max_connections), so callers
	// must opt into "pending-reboot" for those.
	parameterGroupApplyMethodKey = "applyMethod"
	defaultParameterApplyMethod  = "immediate"
)

type resourceNames struct {
	sg, sgIngress, sgEgress, rdsInstance, rdsInstanceFinalSnapshot, es, pc resource.Name
}

// instanceCommon holds the engine-agnostic fields shared by PostgreSQLInstance and MariaDBInstance,
type instanceCommon struct {
	name                     string
	namespace                string
	uid                      types.UID
	engine                   string // "postgres" | "mariadb"
	engineTitle              string // "PostgreSQL" | "MariaDB", used in error messages
	engineVersion            *string
	parameterGroupName       string
	parameterGroupParameters map[string]string
	snapshotIdentifier       string
	allocatedStorage         float64
	instanceType             string
	iops                     float64
	multiAZ                  bool
	maintenanceWindow        string
}

type rdsInstanceGenerator struct {
	// Inputs
	pgInstance      *v1alpha1.PostgreSQLInstance
	mariaDBInstance *v1alpha1.MariaDBInstance
	common          instanceCommon
	observed        map[resource.Name]resource.ObservedComposed
	env             apis.Environment
	hash            string
	// Dependencies
	vpc          ec2mv1beta1.VPC
	kmsDataKey   kmsmv1beta1.Key
	kmsConfigKey kmsmv1beta1.Key
	subnetGroup  rdsmv1beta1.SubnetGroup
	// Internal State
	names               resourceNames
	readinessMap        map[resource.Name]bool
	parameterGroupName  string
	engineVersionActual *string
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func newRDSInstanceGenerator(
	pgInstance *v1alpha1.PostgreSQLInstance,
	mariaDBInstance *v1alpha1.MariaDBInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*rdsInstanceGenerator, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var vpc ec2mv1beta1.VPC
	var kmsDataKey kmsmv1beta1.Key
	var kmsConfigKey kmsmv1beta1.Key
	var subnetGroup rdsmv1beta1.SubnetGroup

	if err := base.ExtractRequiredResource(required, "VPC", &vpc); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "KMSDataKey", &kmsDataKey); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "KMSConfigKey", &kmsConfigKey); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "DBSubnetGroup", &subnetGroup); err != nil {
		return nil, err
	}

	var common instanceCommon
	if pgInstance != nil {
		common = instanceCommon{
			name:                     pgInstance.Name,
			namespace:                pgInstance.Namespace,
			uid:                      pgInstance.UID,
			engine:                   "postgres",
			engineTitle:              "PostgreSQL",
			engineVersion:            pgInstance.Spec.EngineVersion,
			parameterGroupName:       pgInstance.Spec.ParameterGroupName,
			parameterGroupParameters: pgInstance.Spec.ParameterGroupParameters,
			snapshotIdentifier:       pgInstance.Spec.SnapshotIdentifier,
			allocatedStorage:         pgInstance.Spec.AllocatedStorage,
			instanceType:             pgInstance.Spec.InstanceType,
			iops:                     pgInstance.Spec.Iops,
			multiAZ:                  pgInstance.Spec.MultiAZ,
			maintenanceWindow:        pgInstance.Spec.MaintenanceWindow,
		}
	}
	if mariaDBInstance != nil {
		common = instanceCommon{
			name:                     mariaDBInstance.Name,
			namespace:                mariaDBInstance.Namespace,
			uid:                      mariaDBInstance.UID,
			engine:                   "mariadb",
			engineTitle:              "MariaDB",
			engineVersion:            mariaDBInstance.Spec.EngineVersion,
			parameterGroupName:       mariaDBInstance.Spec.ParameterGroupName,
			parameterGroupParameters: mariaDBInstance.Spec.ParameterGroupParameters,
			snapshotIdentifier:       mariaDBInstance.Spec.SnapshotIdentifier,
			allocatedStorage:         mariaDBInstance.Spec.AllocatedStorage,
			instanceType:             mariaDBInstance.Spec.InstanceType,
			iops:                     mariaDBInstance.Spec.Iops,
			multiAZ:                  mariaDBInstance.Spec.MultiAZ,
			maintenanceWindow:        mariaDBInstance.Spec.MaintenanceWindow,
		}
	}

	g := &rdsInstanceGenerator{
		pgInstance:      pgInstance,
		mariaDBInstance: mariaDBInstance,
		common:          common,
		observed:        observed,
		env:             env,
		hash:            base.GenerateFNVHash(common.uid),
		vpc:             vpc,
		kmsDataKey:      kmsDataKey,
		kmsConfigKey:    kmsConfigKey,
		subnetGroup:     subnetGroup,
	}

	g.generateNames()

	if err := g.checkSecretConflict(required); err != nil {
		return nil, err
	}

	g.readinessMap = make(map[resource.Name]bool)
	for name, obs := range observed {
		g.readinessMap[name] = isResourceReady(obs.Resource)
	}

	return g, nil
}

func (g *rdsInstanceGenerator) generateNames() {
	name := g.common.name
	g.names.sg = resource.Name(GetSGName(name, g.hash))
	g.names.sgIngress = resource.Name(GetSGIngressName(name, g.hash))
	g.names.sgEgress = resource.Name(GetSGEgressName(name, g.hash))
	g.names.rdsInstance = resource.Name(GetRDSInstanceName(name, g.hash))
	g.names.rdsInstanceFinalSnapshot = resource.Name(GetRDSInstanceFinalSnapshotName(name, g.hash))
	g.names.es = resource.Name(GetESName(name, g.hash))
	g.names.pc = resource.Name(GetPCName(name))
}

func GetSGName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-%s", instanceName, hash))
}

func GetSGIngressName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-ingress-%s", instanceName, hash))
}

func GetSGEgressName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-egress-%s", instanceName, hash))
}

func GetRDSInstanceName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-instance-%s", instanceName, hash))
}

func GetRDSInstanceFinalSnapshotName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-instance-snapshot-%s", instanceName, hash))
}

func GetESName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-es-%s", instanceName, hash))
}

func GetPCName(instanceName string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-providerconfig", instanceName))
}

func computeFamily(engine string, engineVersion, engineVersionActual *string) (string, bool) {
	v := engineVersion
	if v == nil {
		v = engineVersionActual
	}
	if v == nil {
		return "", false
	}
	parts := strings.Split(*v, ".")
	if engine == "mariadb" {
		if len(parts) < 2 {
			return "", false
		}
		return fmt.Sprintf("%s%s.%s", engine, parts[0], parts[1]), true
	}
	return fmt.Sprintf("%s%s", engine, parts[0]), true
}

func (g *rdsInstanceGenerator) generate() (map[string]client.Object, error) {
	desired := make(map[string]client.Object)

	maps.Copy(desired, g.buildSecurityGroup())
	maps.Copy(desired, g.buildProviderConfig())

	if err := g.applyParameterGroup(desired); err != nil {
		return desired, err
	}

	observedRDSInstance, rdsExists := g.observed[g.names.rdsInstance]
	recreateRDS := rdsExists && g.common.snapshotIdentifier != getSnapshotIdentifierFromObserved(observedRDSInstance.Resource)

	if !recreateRDS {
		maps.Copy(desired, g.buildRDSInstance())
	}
	if !rdsExists || recreateRDS {
		return desired, nil
	}

	maps.Copy(desired, g.buildExternalSecretIfReady(observedRDSInstance.Resource))
	return desired, nil
}

func (g *rdsInstanceGenerator) buildProviderConfig() map[string]client.Object {
	if g.mariaDBInstance != nil {
		return g.buildMariaDBSqlProviderConfig()
	}
	return g.buildPgSqlProviderConfig()
}

func (g *rdsInstanceGenerator) buildRDSInstance() map[string]client.Object {
	if g.mariaDBInstance != nil {
		return g.buildMariaDBRDSInstance()
	}
	return g.buildPgRDSInstance()
}

func (g *rdsInstanceGenerator) applyParameterGroup(desired map[string]client.Object) error {
	if g.common.parameterGroupParameters == nil {
		return nil
	}
	if g.common.parameterGroupName != "" {
		return errors.Errorf("%s instance may have parameterGroupName or parameterGroupParameters, not both", g.common.engineTitle)
	}
	if family, ok := computeFamily(g.common.engine, g.common.engineVersion, g.engineVersionActual); ok {
		maps.Copy(desired, g.buildParameterGroup(family))
		g.keepStaleParameterGroups(desired, family)
	}
	return nil
}

func (g *rdsInstanceGenerator) buildExternalSecretIfReady(observed *composed.Unstructured) map[string]client.Object {
	secretARN, secretStatus, found := getSecretARNFromRDSInstanceStatus(observed)
	if !found || secretStatus != "active" {
		return nil
	}
	endpoint, found := getEndpointFromRDSInstanceStatus(observed)
	if !found {
		return nil
	}
	port, found := getPortFromRDSInstanceStatus(observed)
	if !found {
		return nil
	}
	return g.buildExternalSecret(secretARN, endpoint, port)
}

func (g *rdsInstanceGenerator) checkSecretConflict(required map[string][]resource.Required) error {
	secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", g.common.name, "dbadmin"))

	conflictingSecrets, found := required["Secret"]
	if !found || len(conflictingSecrets) == 0 {
		return nil
	}
	conflictingSecret := conflictingSecrets[0].Resource
	expectedExternalSecretName := string(g.names.es)

	ownerReferences, _, err := unstructured.NestedSlice(conflictingSecret.Object, "metadata", "ownerReferences")
	if err != nil {
		return fmt.Errorf("cannot read owner references from existing Secret '%s': %w", secretName, err)
	}

	isManagedByExpectedEs := false
	for _, owner := range ownerReferences {
		ownerMap, ok := owner.(map[string]interface{})
		if !ok {
			continue
		}
		ownerKind, _, _ := unstructured.NestedString(ownerMap, "kind")
		if ownerKind != "ExternalSecret" {
			continue
		}
		ownerName, _, _ := unstructured.NestedString(ownerMap, "name")
		if ownerName == expectedExternalSecretName {
			isManagedByExpectedEs = true
			break
		}
	}

	if !isManagedByExpectedEs {
		return fmt.Errorf(
			"naming conflict: a Secret named '%s' already exists in namespace '%s' but is not managed by '%s' Instance ExternalSecret ('%s')",
			secretName,
			g.common.namespace,
			g.common.name,
			expectedExternalSecretName,
		)
	}
	return nil
}

func (g *rdsInstanceGenerator) keepStaleParameterGroups(objects map[string]client.Object, currentFamily string) {
	currentKey := parameterGroupKeyPrefix + currentFamily
	if g.rdsInstanceSwitchedTo(g.parameterGroupName) {
		return
	}
	for key, observedResource := range g.observed {
		name := string(key)
		if name == currentKey || !strings.HasPrefix(name, parameterGroupKeyPrefix) {
			continue
		}
		spec, found, _ := unstructured.NestedMap(observedResource.Resource.Object, "spec")
		if !found {
			continue
		}
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(observedResource.Resource.GetAPIVersion())
		obj.SetKind(observedResource.Resource.GetKind())
		obj.SetName(observedResource.Resource.GetName())
		_ = unstructured.SetNestedMap(obj.Object, spec, "spec")
		_ = unstructured.SetNestedMap(obj.Object, map[string]interface{}{"atProvider": map[string]interface{}{}}, "status")
		objects[name] = obj
	}
}

func (g *rdsInstanceGenerator) rdsInstanceSwitchedTo(parameterGroupName string) bool {
	rdsInstanceObserved, ok := g.observed[g.names.rdsInstance]
	if !ok {
		return false
	}
	appliedName, found, _ := unstructured.NestedString(rdsInstanceObserved.Resource.Object, "status", "atProvider", "parameterGroupName")
	return found && appliedName != "" && appliedName == parameterGroupName
}

func (g *rdsInstanceGenerator) buildSecurityGroup() map[string]client.Object {
	groups := make(map[string]client.Object)
	sgName := string(g.names.sg)
	region := g.vpc.Spec.ForProvider.Region
	description := "allow traffic from vpc"

	securityGroup := &ec2mv1beta1.SecurityGroup{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroup", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: sgName, Namespace: g.common.namespace},
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

	ingressName := string(g.names.sgIngress)
	cidrBlock := "0.0.0.0/0"
	ingressType := "ingress"
	ingressProtocol := "tcp"
	ingressPort := float64(5432)
	ingressRule := &ec2mv1beta1.SecurityGroupRule{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroupRule", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: ingressName, Namespace: g.common.namespace},
		Spec: ec2mv1beta1.SecurityGroupRuleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: sgName},
				Type:               &ingressType,
				FromPort:           &ingressPort,
				ToPort:             &ingressPort,
				Protocol:           &ingressProtocol,
				CidrBlocks:         []*string{&cidrBlock},
				Description:        &description,
			},
		},
	}
	groups[ingressName] = ingressRule

	egressName := string(g.names.sgEgress)
	egressType := "egress"
	egressProtocol := "-1"
	egressPort := float64(0)
	egressRule := &ec2mv1beta1.SecurityGroupRule{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroupRule", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: egressName, Namespace: g.common.namespace},
		Spec: ec2mv1beta1.SecurityGroupRuleSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: ec2mv1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv2v1.NamespacedReference{Name: sgName},
				Type:               &egressType,
				FromPort:           &egressPort,
				ToPort:             &egressPort,
				Protocol:           &egressProtocol,
				CidrBlocks:         []*string{&cidrBlock},
				Description:        &description,
			},
		},
	}
	groups[egressName] = egressRule
	return groups
}

func (g *rdsInstanceGenerator) buildParameterGroup(family string) map[string]client.Object {
	groups := make(map[string]client.Object)
	pgName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-parameterGroup-%s-%s", g.common.name, family, g.hash))
	region := g.vpc.Spec.ForProvider.Region
	g.parameterGroupName = pgName
	tags := g.env.Tags
	description := fmt.Sprintf("Parameter group for %s %s", g.common.engineTitle, g.common.name)

	applyMethod := defaultParameterApplyMethod
	if v := g.common.parameterGroupParameters[parameterGroupApplyMethodKey]; v != "" {
		applyMethod = v
	}

	parameters := make([]rdsmv1beta1.ParameterParameters, 0)
	for key, value := range g.common.parameterGroupParameters {
		if key == parameterGroupApplyMethodKey {
			continue
		}
		parameter := rdsmv1beta1.ParameterParameters{
			ApplyMethod: &applyMethod,
			Name:        &key,
			Value:       &value,
		}
		parameters = append(parameters, parameter)
	}

	pg := &rdsmv1beta1.ParameterGroup{
		TypeMeta:   metav1.TypeMeta{APIVersion: rdsApiVersion, Kind: "ParameterGroup"},
		ObjectMeta: metav1.ObjectMeta{Name: pgName},
		Spec: rdsmv1beta1.ParameterGroupSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: rdsmv1beta1.ParameterGroupParameters{
				Region:      region,
				Family:      &family,
				Description: &description,
				Parameter:   parameters,
				Tags:        tags,
			},
		},
	}

	groups[parameterGroupKeyPrefix+family] = pg
	return groups
}

func (g *rdsInstanceGenerator) buildExternalSecret(secretARN string, endpoint string, port float64) map[string]client.Object {
	externalSecrets := make(map[string]client.Object)
	esName := string(g.names.es)
	targetName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", g.common.name, "dbadmin"))

	externalSecret := &esv1.ExternalSecret{
		TypeMeta: metav1.TypeMeta{Kind: "ExternalSecret", APIVersion: "external-secrets.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{},
			Name:        esName,
			Namespace:   g.common.namespace,
		},
		Spec: esv1.ExternalSecretSpec{
			RefreshInterval: &metav1.Duration{Duration: time.Minute * 15},
			RefreshPolicy:   esv1.ExternalSecretRefreshPolicy("Periodic"),
			SecretStoreRef:  esv1.SecretStoreRef{Name: g.env.EsClusterSecretStore, Kind: "ClusterSecretStore"},
			Target: esv1.ExternalSecretTarget{
				Name:           targetName,
				CreationPolicy: esv1.ExternalSecretCreationPolicy("Owner"),
				DeletionPolicy: esv1.ExternalSecretDeletionPolicy("Delete"),
				Template: &esv1.ExternalSecretTemplate{
					Data: map[string]string{
						"username": "dbadmin",
						"password": "{{ .password | toString }}",
						"endpoint": endpoint,
						"port":     fmt.Sprintf("%d", int(port)),
					},
				},
			},
			Data: []esv1.ExternalSecretData{
				{
					SecretKey: "password",
					RemoteRef: esv1.ExternalSecretDataRemoteRef{Property: "password", Key: secretARN, Version: "AWSCURRENT"},
				},
			},
		},
	}

	if annotation := g.resolveForceSyncAnnotation(); annotation != "" {
		externalSecret.Annotations["force-sync"] = annotation
	}

	externalSecrets[esName] = externalSecret
	return externalSecrets
}

func (g *rdsInstanceGenerator) resolveForceSyncAnnotation() string {
	observed, found := g.observed[g.names.es]

	if found && isResourceReady(observed.Resource) {
		return ""
	}

	newTimestamp := fmt.Sprintf("%d", time.Now().Add(10*time.Second).Unix())

	if !found {
		return newTimestamp
	}

	existingSync, hasAnnotation := observed.Resource.GetAnnotations()["force-sync"]
	if !hasAnnotation {
		return newTimestamp
	}

	ts, err := strconv.ParseInt(existingSync, 10, 64)
	if err == nil && time.Unix(ts, 0).After(time.Now()) {
		return existingSync
	}

	return newTimestamp
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

func getSecretARNFromRDSInstanceStatus(instance *composed.Unstructured) (string, string, bool) {
	masterUserSecret, found, err := unstructured.NestedSlice(instance.Object, "status", "atProvider", "masterUserSecret")
	if err != nil || !found || len(masterUserSecret) == 0 {
		return "", "", false
	}

	secretMap, ok := masterUserSecret[0].(map[string]interface{})
	if !ok {
		return "", "", false
	}

	secretARN, arnFound, arnErr := unstructured.NestedString(secretMap, "secretArn")
	if arnErr != nil {
		return "", "", false
	}
	secretStatus, statusFound, statusErr := unstructured.NestedString(secretMap, "secretStatus")
	if statusErr != nil {
		return "", "", false
	}

	if !arnFound || !statusFound {
		return "", "", false
	}

	return secretARN, secretStatus, true
}

func getEndpointFromRDSInstanceStatus(instance *composed.Unstructured) (string, bool) {
	endpoint, found, err := unstructured.NestedString(instance.Object, "status", "atProvider", "address")
	if err != nil || !found {
		return "", false
	}

	return endpoint, true
}

func getPortFromRDSInstanceStatus(instance *composed.Unstructured) (float64, bool) {
	port, found, err := unstructured.NestedFloat64(instance.Object, "status", "atProvider", "port")
	if err != nil || !found {
		return 0, false
	}

	return port, true
}

func getSnapshotIdentifierFromObserved(instance *composed.Unstructured) string {
	snapshot, found, err := unstructured.NestedString(instance.Object, "spec", "forProvider", "snapshotIdentifier")
	if err != nil || !found {
		return ""
	}
	return snapshot
}

func GetRDSInstanceReadyStatus(observed *composed.Unstructured) resource.Ready {
	address, addressFound, addressErr := unstructured.NestedString(observed.Object, "status", "atProvider", "address")
	hostedZoneId, hostedZoneIdFound, hostedZoneIdErr := unstructured.NestedString(observed.Object, "status", "atProvider", "hostedZoneId")
	port, portFound, portErr := unstructured.NestedFloat64(observed.Object, "status", "atProvider", "port")
	notReady := !addressFound || addressErr != nil || address == "" ||
		!hostedZoneIdFound || hostedZoneIdErr != nil || hostedZoneId == "" ||
		!portFound || portErr != nil || port == 0
	if notReady {
		return resource.ReadyFalse
	}
	return base.GetCrossplaneReadyStatus(observed)
}

func getInstanceCreationTimestampSuffix(instance *composed.Unstructured) string {
	if instance == nil {
		return ""
	}

	creationTimestamp, found, err := unstructured.NestedString(instance.Object, "metadata", "creationTimestamp")
	if !found || err != nil || creationTimestamp == "" {
		return ""
	}

	t, err := time.Parse(time.RFC3339, creationTimestamp)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("-%s", t.Format("20060102-150405"))
}

func paramChanged[T comparable](observed *T, desired T) bool {
	return observed != nil && *observed != desired
}

func rdsNeedsApplyImmediately(observed resource.ObservedComposed, common instanceCommon) bool {
	var observedInstance rdsmv1beta1.Instance
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(observed.Resource.Object, &observedInstance); err != nil {
		return false
	}

	atProvider := observedInstance.Status.AtProvider
	return paramChanged(atProvider.AllocatedStorage, common.allocatedStorage) ||
		(common.engineVersion != nil && paramChanged(atProvider.EngineVersion, *common.engineVersion)) ||
		paramChanged(atProvider.InstanceClass, common.instanceType) ||
		(common.iops != 0 && paramChanged(atProvider.Iops, common.iops)) ||
		(atProvider.MultiAz != nil && *atProvider.MultiAz) != common.multiAZ ||
		(common.maintenanceWindow != "" && paramChanged(atProvider.MaintenanceWindow, common.maintenanceWindow)) ||
		(common.parameterGroupName != "" && paramChanged(atProvider.ParameterGroupName, common.parameterGroupName))
}
