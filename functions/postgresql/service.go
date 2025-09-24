package main

import (
	"fmt"
	"maps"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/model/v1alpha1"
	esv1 "github.com/external-secrets/external-secrets/apis/externalsecrets/v1"
	"github.com/upbound/provider-aws/apis/ec2/v1beta1"
	kmsv1beta1 "github.com/upbound/provider-aws/apis/kms/v1beta1"
	rdsv1beta1 "github.com/upbound/provider-aws/apis/rds/v1beta1"
	rdsv1beta3 "github.com/upbound/provider-aws/apis/rds/v1beta3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ec2ApiVersion = "ec2.aws.upbound.io/v1beta1"
)

func GeneratePostgreSQLObjects(
	postgreSQL v1alpha1.PostgreSQL,
	requiredResources map[string][]resource.Required,
	provider string,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {

	objs := make(map[string]runtime.Object)
	readinessMap := make(map[resource.Name]bool)
	for name, obs := range observed {
		readinessMap[name] = base.IsResourceReady(obs.Resource)
	}

	var VPC v1beta1.VPC
	var KMSKey kmsv1beta1.Key
	var SubnetGroup rdsv1beta1.SubnetGroup
	if err := base.ExtractRequiredResource(requiredResources, "VPC", &VPC); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(requiredResources, "KMSKey", &KMSKey); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(requiredResources, "DBSubnetGroup", &SubnetGroup); err != nil {
		return nil, err
	}

	sgNameString := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", postgreSQL.Name, "sg"))
	sgName := resource.Name(sgNameString)
	sgRuleIngressName := resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", sgNameString, "ingress")))
	sgRuleEgressName := resource.Name(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", sgNameString, "egress")))
	dbInstanceName := resource.Name(postgreSQL.Name)

	maps.Copy(objs, getSecurityGroup(postgreSQL, VPC, provider))
	if !readinessMap[sgName] || !readinessMap[sgRuleIngressName] || !readinessMap[sgRuleEgressName] {
		return objs, nil
	}

	maps.Copy(objs, getDBInstance(postgreSQL, VPC, KMSKey, SubnetGroup, provider))
	observedDBInstance, ok := observed[dbInstanceName]
	if !ok {
		return objs, nil
	}
	dbInstanceReady := GetInstanceReadyStatus(observedDBInstance.Resource)
	if dbInstanceReady == resource.ReadyFalse {
		return objs, nil
	}
	secretARN, secretStatus, found := getSecretARNFromInstanceStatus(observedDBInstance.Resource)
	if !found || secretStatus != "active" {
		return objs, nil
	}

	maps.Copy(objs, getExternalSecret(postgreSQL, secretARN))

	return objs, nil
}

func getSecretARNFromInstanceStatus(instance *composed.Unstructured) (string, string, bool) {
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

func getSecurityGroup(postgreSQL v1alpha1.PostgreSQL, VPC v1beta1.VPC, provider string) map[string]runtime.Object {
	groups := make(map[string]runtime.Object)
	name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", postgreSQL.Name, "sg"))
	region := VPC.Spec.ForProvider.Region

	ingressFromPort := float64(5432)
	ingressToPort := float64(5432)
	egressFromPort := float64(0)
	egressToPort := float64(0)
	description := "allow traffic from vpc"

	securityGroup := &v1beta1.SecurityGroup{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecurityGroup",
			APIVersion: ec2ApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1beta1.SecurityGroupSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: provider},
			},
			ForProvider: v1beta1.SecurityGroupParameters_2{
				Region: region,
				VPCIDRef: &xpv1.Reference{
					Name: VPC.Name,
				},
				Description: &description,
				Tags:        map[string]*string{"Name": &name},
			},
		},
	}
	groups[name] = securityGroup
	ingressName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", name, "ingress"))
	cidrBlock := "0.0.0.0/0"
	ingressType := "ingress"
	ingressProtocol := "tcp"
	ingressRule := &v1beta1.SecurityGroupRule{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecurityGroupRule",
			APIVersion: ec2ApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: ingressName,
		},
		Spec: v1beta1.SecurityGroupRuleSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: provider},
			},
			ForProvider: v1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv1.Reference{Name: name},
				Type:               &ingressType,
				FromPort:           &ingressFromPort,
				ToPort:             &ingressToPort,
				Protocol:           &ingressProtocol,
				CidrBlocks:         []*string{&cidrBlock},
				Description:        &description,
			},
		},
	}
	groups[ingressName] = ingressRule
	egressName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", name, "egress"))
	egressType := "egress"
	egressProtocol := "-1"
	egressRule := &v1beta1.SecurityGroupRule{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SecurityGroupRule",
			APIVersion: ec2ApiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: egressName,
		},
		Spec: v1beta1.SecurityGroupRuleSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: provider},
			},
			ForProvider: v1beta1.SecurityGroupRuleParameters_2{
				Region:             region,
				SecurityGroupIDRef: &xpv1.Reference{Name: name},
				Type:               &egressType,
				FromPort:           &egressFromPort,
				ToPort:             &egressToPort,
				Protocol:           &egressProtocol,
				CidrBlocks:         []*string{&cidrBlock},
				Description:        &description,
			},
		},
	}
	groups[egressName] = egressRule
	return groups
}

func getDBInstance(
	postgreSQL v1alpha1.PostgreSQL,
	VPC v1beta1.VPC,
	KMSKey kmsv1beta1.Key,
	DBSubnetGroup rdsv1beta1.SubnetGroup,
	provider string,
) map[string]runtime.Object {
	dbInstances := make(map[string]runtime.Object)
	region := VPC.Spec.ForProvider.Region
	availabilityZone := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s%s", *region, "a"))

	defaultSecurityGroupIDRefName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", postgreSQL.Name, "sg"))
	customVpcSecurityGroupIDRef := postgreSQL.Spec.VpcSecurityGroupIDRef
	vpcSecurityGroupIDRef := []xpv1.Reference{{Name: defaultSecurityGroupIDRefName}}
	if len(customVpcSecurityGroupIDRef) > 0 {
		for _, groupRef := range customVpcSecurityGroupIDRef {
			ref := xpv1.Reference{Name: groupRef}
			vpcSecurityGroupIDRef = append(vpcSecurityGroupIDRef, ref)
		}
	}

	dbName := "postgres"
	engine := "postgres"
	identifierPrefix := "postgresql-"
	identifier := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s%s", identifierPrefix, postgreSQL.Name))
	masterUsername := "dbadmin"
	manageMasterUserPassword := true
	performanceInsightsEnabled := false
	publiclyAccessible := false
	skipFinalSnapshot := true
	storageEncrypted := true

	dbInstance := &rdsv1beta3.Instance{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Instance",
			APIVersion: "rds.aws.upbound.io/v1beta3",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: postgreSQL.Name,
		},
		Spec: rdsv1beta3.InstanceSpec{
			ResourceSpec: xpv1.ResourceSpec{
				ProviderConfigReference: &xpv1.Reference{Name: provider},
			},
			ForProvider: rdsv1beta3.InstanceParameters{
				AllocatedStorage:         &postgreSQL.Spec.AllocatedStorage,
				AllowMajorVersionUpgrade: &postgreSQL.Spec.AllowMajorVersionUpgrade,
				AutoMinorVersionUpgrade:  &postgreSQL.Spec.AutoMinorVersionUpgrade,
				AvailabilityZone:         &availabilityZone,
				DBName:                   &dbName,
				DBSubnetGroupNameRef: &xpv1.Reference{
					Name: DBSubnetGroup.Name,
				},
				DeletionProtection: &postgreSQL.Spec.DeletionProtection,
				Engine:             &engine,
				EngineVersion:      &postgreSQL.Spec.EngineVersion,
				Identifier:         &identifier,
				IdentifierPrefix:   &identifierPrefix,
				InstanceClass:      &postgreSQL.Spec.InstanceClass,
				KMSKeyIDRef: &xpv1.Reference{
					Name: KMSKey.Name,
				},
				ManageMasterUserPassword: &manageMasterUserPassword,
				MasterUserSecretKMSKeyIDRef: &xpv1.Reference{
					Name: KMSKey.Name,
				},
				MultiAz:                    &postgreSQL.Spec.MultiAZ,
				PerformanceInsightsEnabled: &performanceInsightsEnabled,
				PubliclyAccessible:         &publiclyAccessible,
				Region:                     region,
				SkipFinalSnapshot:          &skipFinalSnapshot,
				StorageType:                &postgreSQL.Spec.StorageType,
				StorageEncrypted:           &storageEncrypted,
				Username:                   &masterUsername,
				VPCSecurityGroupIDRefs:     vpcSecurityGroupIDRef,
			},
		},
	}

	if postgreSQL.Spec.BackupWindow != "" {
		dbInstance.Spec.ForProvider.BackupWindow = &postgreSQL.Spec.BackupWindow
	}

	if postgreSQL.Spec.ParameterGroupName != "" {
		dbInstance.Spec.ForProvider.ParameterGroupName = &postgreSQL.Spec.ParameterGroupName
	}

	if postgreSQL.Spec.PerformanceInsightsEnabled {
		dbInstance.Spec.ForProvider.PerformanceInsightsEnabled = &postgreSQL.Spec.PerformanceInsightsEnabled
	}

	dbInstance.SetDeletionPolicy("Delete")
	dbInstances[dbInstance.Name] = dbInstance
	return dbInstances
}

func GetPostgreSQLStatusFromDbInstance(dbInstance rdsv1beta3.Instance) v1alpha1.PostgreSQLStatus {
	status := v1alpha1.PostgreSQLStatus{}

	base.SetBool(dbInstance.Spec.ForProvider.AllowMajorVersionUpgrade, &status.AllowMajorVersionUpgrade)
	base.SetBool(dbInstance.Spec.ForProvider.AutoMinorVersionUpgrade, &status.AutoMinorVersionUpgrade)
	base.SetString(dbInstance.Spec.ForProvider.BackupWindow, &status.BackupWindow)

	endpoint := v1alpha1.PostgreSQLEndpoint{}

	base.SetString(dbInstance.Status.AtProvider.Address, &endpoint.Address)
	base.SetString(dbInstance.Status.AtProvider.HostedZoneID, &endpoint.HostedZoneID)
	base.SetFloat64(dbInstance.Status.AtProvider.Port, &endpoint.Port)

	status.Endpoint = endpoint

	base.SetFloat64(dbInstance.Spec.ForProvider.Iops, &status.Iops)
	base.SetString(dbInstance.Status.AtProvider.KMSKeyID, &status.KMSKeyID)

	if dbInstance.Status.AtProvider.LatestRestorableTime != nil {
		t, err := time.Parse(time.RFC3339, *dbInstance.Status.AtProvider.LatestRestorableTime)
		if err == nil {
			restorableTime := metav1.NewTime(t)
			status.LatestRestorableTime = &restorableTime
		}
	}

	base.SetString(dbInstance.Spec.ForProvider.MaintenanceWindow, &status.MaintenanceWindow)
	base.SetString(dbInstance.Status.AtProvider.ParameterGroupName, &status.ParameterGroupName)
	base.SetBool(dbInstance.Status.AtProvider.PerformanceInsightsEnabled, &status.PerformanceInsightsEnabled)
	base.SetString(dbInstance.Status.AtProvider.ResourceID, &status.ResourceID)
	base.SetString(dbInstance.Status.AtProvider.Status, &status.Status)
	base.SetBool(dbInstance.Spec.ForProvider.StorageEncrypted, &status.StorageEncrypted)
	base.SetFloat64(dbInstance.Spec.ForProvider.StorageThroughput, &status.StorageThroughput)

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

func GetInstanceReadyStatus(observed *composed.Unstructured) resource.Ready {
	address, addressFound, addressErr := unstructured.NestedString(observed.Object, "status", "atProvider", "address")
	hostedZoneId, hostedZoneIdFound, hostedZoneIdErr := unstructured.NestedString(observed.Object, "status", "atProvider", "hostedZoneId")
	port, portFound, portErr := unstructured.NestedFloat64(observed.Object, "status", "atProvider", "port")
	notReady := !addressFound || addressErr != nil || address == "" ||
		!hostedZoneIdFound || hostedZoneIdErr != nil || hostedZoneId == "" ||
		!portFound || portErr != nil || port == 0
	if notReady {
		return resource.ReadyFalse
	}
	conditions, foundCond, errCond := unstructured.NestedSlice(observed.Object, "status", "conditions")
	if errCond == nil && foundCond {
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "Ready" && cond["status"] == "True" {
				return resource.ReadyTrue
			}
		}
	}
	return resource.ReadyFalse
}

func getExternalSecret(postgreSQL v1alpha1.PostgreSQL, secretARN string) map[string]runtime.Object {
	externalSecrets := make(map[string]runtime.Object)
	name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", postgreSQL.Name, "es"))
	targetName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", postgreSQL.Name, "secret"))
	remoteKey := secretARN
	forceSyncTime := fmt.Sprintf("%d", time.Now().Add(10*time.Second).Unix())

	externalSecret := &esv1.ExternalSecret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExternalSecret",
			APIVersion: "external-secrets.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{"force-sync": forceSyncTime},
			Name:        name,
			Namespace:   "default",
		},
		Spec: esv1.ExternalSecretSpec{
			RefreshInterval: &metav1.Duration{Duration: time.Minute * 15},
			RefreshPolicy:   esv1.ExternalSecretRefreshPolicy("Periodic"),
			SecretStoreRef: esv1.SecretStoreRef{
				Name: "external-secrets",
				Kind: "ClusterSecretStore",
			},
			Target: esv1.ExternalSecretTarget{
				Name:           targetName,
				CreationPolicy: esv1.ExternalSecretCreationPolicy("Owner"),
				DeletionPolicy: esv1.ExternalSecretDeletionPolicy("Delete"),
			},
			Data: []esv1.ExternalSecretData{
				{
					SecretKey: "password",
					RemoteRef: esv1.ExternalSecretDataRemoteRef{
						Property: "password",
						Key:      remoteKey,
						Version:  "AWSCURRENT",
					},
				},
			},
		},
	}
	externalSecrets[name] = externalSecret
	return externalSecrets
}
