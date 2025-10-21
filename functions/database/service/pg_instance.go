package service

import (
	"fmt"
	"maps"
	"time"

	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	ec2mv1beta1 "github.com/upbound/provider-aws/apis/namespaced/ec2/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/kms/v1beta1"
	rdsmv1beta1 "github.com/upbound/provider-aws/apis/namespaced/rds/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ec2ApiVersion = "ec2.aws.m.upbound.io/v1beta1"
	rdsApiVersion = "rds.aws.m.upbound.io/v1beta1"
)

type pgInstanceGenerator struct {
	// Inputs
	pgInstance v1alpha1.PostgreSQLInstance
	observed   map[resource.Name]resource.ObservedComposed
	env        apis.Environment
	hash       string
	// Dependencies
	vpc          ec2mv1beta1.VPC
	kmsDataKey   kmsmv1beta1.Key
	kmsConfigKey kmsmv1beta1.Key
	subnetGroup  rdsmv1beta1.SubnetGroup
	// Internal State
	names        resourceNames
	readinessMap map[resource.Name]bool
}

type resourceNames struct {
	sg, sgIngress, sgEgress, rdsInstance, rdsInstanceSnapshot, es resource.Name
}

func GeneratePgInstanceObjects(
	pgInstance v1alpha1.PostgreSQLInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	g, err := newPgInstanceGenerator(pgInstance, required, observed)
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

func newPgInstanceGenerator(
	pgInstance v1alpha1.PostgreSQLInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*pgInstanceGenerator, error) {

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

	g := &pgInstanceGenerator{
		pgInstance:   pgInstance,
		observed:     observed,
		env:          env,
		hash:         base.GenerateFNVHash(pgInstance.UID),
		vpc:          vpc,
		kmsDataKey:   kmsDataKey,
		kmsConfigKey: kmsConfigKey,
		subnetGroup:  subnetGroup,
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

func (g *pgInstanceGenerator) generate() (map[string]runtime.Object, error) {
	desired := make(map[string]runtime.Object)

	maps.Copy(desired, g.buildSecurityGroup())
	if !g.isReady(g.names.sg) || !g.isReady(g.names.sgIngress) || !g.isReady(g.names.sgEgress) {
		return desired, nil
	}

	maps.Copy(desired, g.buildRDSInstance())
	observedRDSInstance, ok := g.observed[g.names.rdsInstance]
	if !ok {
		return desired, nil
	}
	if GetRDSInstanceReadyStatus(observedRDSInstance.Resource) == resource.ReadyFalse {
		return desired, nil
	}
	secretARN, secretStatus, found := getSecretARNFromRDSInstanceStatus(observedRDSInstance.Resource)
	if !found || secretStatus != "active" {
		return desired, nil
	}

	maps.Copy(desired, g.buildExternalSecret(secretARN))

	return desired, nil
}

func (g *pgInstanceGenerator) checkSecretConflict(required map[string][]resource.Required) error {
	secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", g.pgInstance.Name, "dbadmin"))

	conflictingSecrets, found := required["Secret"]
	if !found || len(conflictingSecrets) == 0 {
		return nil
	}
	conflictingSecret := conflictingSecrets[0].Resource
	annotations := conflictingSecret.GetAnnotations()
	actualAnnotationValue, annotationFound := annotations["crossplane.io/composition-resource-name"]

	if !annotationFound || actualAnnotationValue != string(g.names.es) {
		return fmt.Errorf(
			"naming conflict: a Secret named '%s' already exists in namespace '%s' but is not managed by this PostgreSQLInstance's ExternalSecret ('%s')",
			secretName,
			g.pgInstance.Namespace,
			string(g.names.es),
		)
	}
	return nil
}

func (g *pgInstanceGenerator) generateNames() {
	g.names.sg = resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-%s", g.pgInstance.Name, g.hash)))
	g.names.sgIngress = resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-ingress-%s", g.pgInstance.Name, g.hash)))
	g.names.sgEgress = resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-sg-egress-%s", g.pgInstance.Name, g.hash)))
	g.names.rdsInstance = resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-instance-%s", g.pgInstance.Name, g.hash)))
	g.names.rdsInstanceSnapshot = resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-instance-snapshot-%s", g.pgInstance.Name, g.hash)))
	g.names.es = resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-es-%s", g.pgInstance.Name, g.hash)))
}

func (g *pgInstanceGenerator) isReady(name resource.Name) bool {
	return g.readinessMap[name]
}

func (g *pgInstanceGenerator) buildSecurityGroup() map[string]runtime.Object {
	groups := make(map[string]runtime.Object)
	sgName := string(g.names.sg)
	region := g.vpc.Spec.ForProvider.Region
	description := "allow traffic from vpc"

	securityGroup := &ec2mv1beta1.SecurityGroup{
		TypeMeta:   metav1.TypeMeta{Kind: "SecurityGroup", APIVersion: ec2ApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: sgName, Namespace: g.pgInstance.Namespace},
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
		ObjectMeta: metav1.ObjectMeta{Name: ingressName, Namespace: g.pgInstance.Namespace},
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
		ObjectMeta: metav1.ObjectMeta{Name: egressName, Namespace: g.pgInstance.Namespace},
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

func (g *pgInstanceGenerator) buildRDSInstance() map[string]runtime.Object {
	rdsInstances := make(map[string]runtime.Object)
	rdsInstanceName := string(g.names.rdsInstance)
	sgName := string(g.names.sg)
	snapshotIdentifier := string(g.names.rdsInstanceSnapshot)
	region := g.vpc.Spec.ForProvider.Region
	availabilityZone := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s%s", *region, "a"))

	vpcSecurityGroupIDRef := []xpv2v1.NamespacedReference{{Name: sgName}}

	dbName, engine, storageType, masterUsername := "postgres", "postgres", "gp3", "dbadmin"
	manageMasterUserPassword, performanceInsightsEnabled, publiclyAccessible, skipFinalSnapshot, storageEncrypted := true, false, false, false, true

	rdsInstance := &rdsmv1beta1.Instance{
		TypeMeta:   metav1.TypeMeta{Kind: "Instance", APIVersion: rdsApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: rdsInstanceName, Namespace: g.pgInstance.Namespace},
		Spec: rdsmv1beta1.InstanceSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: rdsmv1beta1.InstanceParameters{
				AllocatedStorage:            &g.pgInstance.Spec.AllocatedStorage,
				AllowMajorVersionUpgrade:    &g.pgInstance.Spec.AllowMajorVersionUpgrade,
				AutoMinorVersionUpgrade:     &g.pgInstance.Spec.AutoMinorVersionUpgrade,
				AvailabilityZone:            &availabilityZone,
				DBName:                      &dbName,
				DBSubnetGroupNameRef:        &xpv2v1.NamespacedReference{Name: g.subnetGroup.Name, Namespace: g.subnetGroup.Namespace},
				DeletionProtection:          &g.pgInstance.Spec.DeletionProtection,
				Engine:                      &engine,
				EngineVersion:               &g.pgInstance.Spec.EngineVersion,
				FinalSnapshotIdentifier:     &snapshotIdentifier,
				Identifier:                  &rdsInstanceName,
				InstanceClass:               &g.pgInstance.Spec.InstanceType,
				KMSKeyIDRef:                 &xpv2v1.NamespacedReference{Name: g.kmsDataKey.Name, Namespace: g.kmsDataKey.Namespace},
				ManageMasterUserPassword:    &manageMasterUserPassword,
				MasterUserSecretKMSKeyIDRef: &xpv2v1.NamespacedReference{Name: g.kmsConfigKey.Name, Namespace: g.kmsConfigKey.Namespace},
				MultiAz:                     &g.pgInstance.Spec.MultiAZ,
				PerformanceInsightsEnabled:  &performanceInsightsEnabled,
				PubliclyAccessible:          &publiclyAccessible,
				Region:                      region,
				SkipFinalSnapshot:           &skipFinalSnapshot,
				StorageType:                 &storageType,
				StorageEncrypted:            &storageEncrypted,
				Username:                    &masterUsername,
				VPCSecurityGroupIDRefs:      vpcSecurityGroupIDRef,
				Tags:                        g.env.Tags,
			},
		},
	}

	if g.pgInstance.Spec.BackupWindow != "" {
		rdsInstance.Spec.ForProvider.BackupWindow = &g.pgInstance.Spec.BackupWindow
	}
	if g.pgInstance.Spec.MaintenanceWindow != "" {
		rdsInstance.Spec.ForProvider.MaintenanceWindow = &g.pgInstance.Spec.MaintenanceWindow
	}
	if g.pgInstance.Spec.ParameterGroupName != "" {
		rdsInstance.Spec.ForProvider.ParameterGroupName = &g.pgInstance.Spec.ParameterGroupName
	}
	rdsInstance.SetManagementPolicies(xpv2v1.ManagementPolicies{"*"})

	rdsInstances[rdsInstance.Name] = rdsInstance
	return rdsInstances
}

func (g *pgInstanceGenerator) buildExternalSecret(secretARN string) map[string]runtime.Object {
	externalSecrets := make(map[string]runtime.Object)
	esName := string(g.names.es)
	targetName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", g.pgInstance.Name, "dbadmin"))
	forceSyncTime := fmt.Sprintf("%d", time.Now().Add(10*time.Second).Unix())

	externalSecret := &esv1.ExternalSecret{
		TypeMeta: metav1.TypeMeta{Kind: "ExternalSecret", APIVersion: "external-secrets.io/v1"},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"force-sync": forceSyncTime},
			Name:        esName,
			Namespace:   g.pgInstance.Namespace,
		},
		Spec: esv1.ExternalSecretSpec{
			RefreshInterval: &metav1.Duration{Duration: time.Minute * 15},
			RefreshPolicy:   esv1.ExternalSecretRefreshPolicy("Periodic"),
			SecretStoreRef:  esv1.SecretStoreRef{Name: "external-secrets", Kind: "ClusterSecretStore"},
			Target: esv1.ExternalSecretTarget{
				Name:           targetName,
				CreationPolicy: esv1.ExternalSecretCreationPolicy("Owner"),
				DeletionPolicy: esv1.ExternalSecretDeletionPolicy("Delete"),
			},
			Data: []esv1.ExternalSecretData{
				{
					SecretKey: "password",
					RemoteRef: esv1.ExternalSecretDataRemoteRef{Property: "password", Key: secretARN, Version: "AWSCURRENT"},
				},
			},
		},
	}
	externalSecrets[esName] = externalSecret
	return externalSecrets
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

func GetPostgreSQLStatusFromDbInstance(dbInstance rdsmv1beta1.Instance) v1alpha1.PostgreSQLInstanceStatus {
	status := v1alpha1.PostgreSQLInstanceStatus{}
	dbInstanceName := dbInstance.Name

	base.SetBool(dbInstance.Status.AtProvider.AllowMajorVersionUpgrade, &status.AllowMajorVersionUpgrade)
	base.SetBool(dbInstance.Status.AtProvider.AutoMinorVersionUpgrade, &status.AutoMinorVersionUpgrade)
	base.SetString(dbInstance.Status.AtProvider.BackupWindow, &status.BackupWindow)
	base.SetString(&dbInstanceName, &status.DBInstanceIdentifier)

	endpoint := v1alpha1.PostgreSQLInstanceEndpoint{}

	base.SetString(dbInstance.Status.AtProvider.Address, &endpoint.Address)
	base.SetString(dbInstance.Status.AtProvider.HostedZoneID, &endpoint.HostedZoneID)
	base.SetFloat64(dbInstance.Status.AtProvider.Port, &endpoint.Port)

	status.Endpoint = endpoint

	base.SetString(dbInstance.Status.AtProvider.FinalSnapshotIdentifier, &status.FinalSnapshotIdentifier)
	base.SetFloat64(dbInstance.Status.AtProvider.Iops, &status.Iops)
	base.SetString(dbInstance.Status.AtProvider.KMSKeyID, &status.KMSKeyID)

	if dbInstance.Status.AtProvider.LatestRestorableTime != nil {
		t, err := time.Parse(time.RFC3339, *dbInstance.Status.AtProvider.LatestRestorableTime)
		if err == nil {
			restorableTime := metav1.NewTime(t)
			status.LatestRestorableTime = &restorableTime
		}
	}

	base.SetString(dbInstance.Status.AtProvider.MaintenanceWindow, &status.MaintenanceWindow)
	base.SetString(dbInstance.Status.AtProvider.ParameterGroupName, &status.ParameterGroupName)
	base.SetString(dbInstance.Status.AtProvider.ResourceID, &status.ResourceID)
	base.SetString(dbInstance.Status.AtProvider.Status, &status.Status)
	base.SetBool(dbInstance.Status.AtProvider.StorageEncrypted, &status.StorageEncrypted)
	base.SetFloat64(dbInstance.Status.AtProvider.StorageThroughput, &status.StorageThroughput)
	base.SetString(dbInstance.Status.AtProvider.StorageType, &status.StorageType)

	var vpcSecurityGroupsIds []string
	if len(dbInstance.Status.AtProvider.VPCSecurityGroupIds) > 0 {
		for _, id := range dbInstance.Status.AtProvider.VPCSecurityGroupIds {
			if id != nil {
				vpcSecurityGroupsIds = append(vpcSecurityGroupsIds, *id)
			}
		}
	}

	status.VpcSecurityGroupIds = vpcSecurityGroupsIds
	return status
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
