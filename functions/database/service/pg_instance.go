package service

import (
	"fmt"
	"strings"
	"time"

	postgresv1alpha1 "github.com/crossplane-contrib/provider-sql/apis/namespaced/postgresql/v1alpha1"
	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	rdsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/rds/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	pgSqlApiVersion = "postgresql.sql.m.crossplane.io/v1alpha1"
)

func newPgInstanceCommon(pgInstance *v1alpha1.PostgreSQLInstance) instanceCommon {
	return instanceCommon{
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

func GeneratePgInstanceObjects(
	pgInstance v1alpha1.PostgreSQLInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]client.Object, error) {
	g, err := newRDSInstanceGenerator(&pgInstance, nil, newPgInstanceCommon(&pgInstance), required, observed)
	if err != nil {
		return nil, err
	}

	if rgObserved, ok := g.observed[g.names.rdsInstance]; ok {
		if v, found, _ := unstructured.NestedString(rgObserved.Resource.Object, "status", "atProvider", "engineVersionActual"); found && v != "" {
			g.engineVersionActual = &v
		}
	}

	return g.generate()
}

func (g *rdsInstanceGenerator) buildPgRDSInstance() client.Object {
	rdsInstanceName := string(g.names.rdsInstance)
	sgName := string(g.names.sg)
	region := g.vpc.Spec.ForProvider.Region
	var availabilityZone *string
	if !g.pgInstance.Spec.MultiAZ {
		availabilityZone = new(base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s%s", *region, "a")))
	}

	vpcSecurityGroupIDRef := []xpv2v1.NamespacedReference{{Name: sgName}}

	backupRetentionPeriod := g.pgInstance.Spec.BackupRetentionPeriod
	if backupRetentionPeriod == nil {
		backupRetentionPeriod = g.env.BackupRetentionPeriod
	}

	applyImmediately := false
	finalSnapshotIdentifier := string(g.names.rdsInstanceFinalSnapshot)

	if observedRDSInstance, ok := g.observed[g.names.rdsInstance]; ok {
		finalSnapshotIdentifier += getInstanceCreationTimestampSuffix(observedRDSInstance.Resource)
		applyImmediately = rdsNeedsApplyImmediately(observedRDSInstance, g.common)
	}

	rdsInstance := &rdsmv1beta1.Instance{
		TypeMeta:   metav1.TypeMeta{Kind: "Instance", APIVersion: rdsApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: rdsInstanceName, Namespace: g.pgInstance.Namespace},
		Spec: rdsmv1beta1.InstanceSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: rdsmv1beta1.InstanceParameters{
				AllocatedStorage:            &g.pgInstance.Spec.AllocatedStorage,
				ApplyImmediately:            &applyImmediately,
				AllowMajorVersionUpgrade:    &g.pgInstance.Spec.AllowMajorVersionUpgrade,
				AutoMinorVersionUpgrade:     &g.pgInstance.Spec.AutoMinorVersionUpgrade,
				AvailabilityZone:            availabilityZone,
				BackupRetentionPeriod:       backupRetentionPeriod,
				DBName:                      new("postgres"),
				DBSubnetGroupNameRef:        &xpv2v1.NamespacedReference{Name: g.subnetGroup.Name, Namespace: g.subnetGroup.Namespace},
				DeletionProtection:          &g.pgInstance.Spec.DeletionProtection,
				Engine:                      new("postgres"),
				EngineVersion:               g.pgInstance.Spec.EngineVersion,
				FinalSnapshotIdentifier:     &finalSnapshotIdentifier,
				Identifier:                  &rdsInstanceName,
				InstanceClass:               &g.pgInstance.Spec.InstanceType,
				KMSKeyIDRef:                 &xpv2v1.NamespacedReference{Name: g.kmsDataKey.Name, Namespace: g.kmsDataKey.Namespace},
				ManageMasterUserPassword:    new(true),
				MasterUserSecretKMSKeyIDRef: &xpv2v1.NamespacedReference{Name: g.kmsConfigKey.Name, Namespace: g.kmsConfigKey.Namespace},
				MultiAz:                     &g.pgInstance.Spec.MultiAZ,
				PerformanceInsightsEnabled:  new(false),
				PubliclyAccessible:          new(false),
				Region:                      region,
				SkipFinalSnapshot:           new(!*g.env.PostgresBackupBeforeDeletion),
				StorageType:                 new("gp3"),
				StorageEncrypted:            new(true),
				Tags:                        g.env.Tags,
				Username:                    new("dbadmin"),
				VPCSecurityGroupIDRefs:      vpcSecurityGroupIDRef,
			},
		},
	}

	if g.pgInstance.Spec.BackupWindow != "" {
		rdsInstance.Spec.ForProvider.BackupWindow = &g.pgInstance.Spec.BackupWindow
	}
	if g.pgInstance.Spec.MaintenanceWindow != "" {
		rdsInstance.Spec.ForProvider.MaintenanceWindow = &g.pgInstance.Spec.MaintenanceWindow
	}
	if g.pgInstance.Spec.Iops != 0 {
		rdsInstance.Spec.ForProvider.Iops = &g.pgInstance.Spec.Iops
	}
	if g.parameterGroupName != "" {
		rdsInstance.Spec.ForProvider.ParameterGroupName = &g.parameterGroupName
	} else if g.pgInstance.Spec.ParameterGroupName != "" {
		rdsInstance.Spec.ForProvider.ParameterGroupName = &g.pgInstance.Spec.ParameterGroupName
	} else if family, ok := computeFamily("postgres", g.pgInstance.Spec.EngineVersion, g.engineVersionActual); ok {
		rdsInstance.Spec.ForProvider.ParameterGroupName = new("default." + family)
	}

	if family, ok := computeFamily("postgres", g.pgInstance.Spec.EngineVersion, g.engineVersionActual); ok {
		rdsInstance.Spec.ForProvider.OptionGroupName = new("default:postgres-" + strings.TrimPrefix(family, "postgres"))
	}

	if g.pgInstance.Spec.SnapshotIdentifier != "" {
		rdsInstance.Spec.ForProvider.SnapshotIdentifier = &g.pgInstance.Spec.SnapshotIdentifier
	}

	rdsInstance.SetManagementPolicies(xpv2v1.ManagementPolicies{"*"})

	return rdsInstance
}

func (g *rdsInstanceGenerator) buildPgSqlProviderConfig() client.Object {
	pcName := string(g.names.pc)
	secretName := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", g.pgInstance.Name, "dbadmin"))
	providerConfig := &postgresv1alpha1.ProviderConfig{
		TypeMeta:   metav1.TypeMeta{Kind: "ProviderConfig", APIVersion: pgSqlApiVersion},
		ObjectMeta: metav1.ObjectMeta{Name: pcName, Namespace: g.pgInstance.Namespace},
		Spec: postgresv1alpha1.ProviderConfigSpec{
			Credentials: postgresv1alpha1.ProviderCredentials{
				Source: "PostgreSQLConnectionSecret",
				ConnectionSecretRef: xpv2v1.LocalSecretReference{
					Name: secretName,
				},
			},
			SSLMode: new("require"),
		},
	}
	return providerConfig
}

func GetPostgreSQLStatusFromDbInstance(dbInstance rdsmv1beta1.Instance) v1alpha1.PostgreSQLInstanceStatus {
	status := v1alpha1.PostgreSQLInstanceStatus{}
	base.SetBool(dbInstance.Status.AtProvider.AllowMajorVersionUpgrade, &status.AllowMajorVersionUpgrade)
	base.SetBool(dbInstance.Status.AtProvider.AutoMinorVersionUpgrade, &status.AutoMinorVersionUpgrade)
	base.SetString(dbInstance.Status.AtProvider.BackupWindow, &status.BackupWindow)
	base.SetString(new(dbInstance.Name), &status.DBInstanceIdentifier)

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
			status.LatestRestorableTime = new(metav1.NewTime(t))
		}
	}

	base.SetString(dbInstance.Status.AtProvider.MaintenanceWindow, &status.MaintenanceWindow)
	base.SetString(dbInstance.Status.AtProvider.ParameterGroupName, &status.ParameterGroupName)
	base.SetString(dbInstance.Status.AtProvider.ResourceID, &status.ResourceID)
	base.SetString(dbInstance.Status.AtProvider.Status, &status.Status)

	if dbInstance.Status.AtProvider.SnapshotIdentifier != nil {
		base.SetString(dbInstance.Status.AtProvider.SnapshotIdentifier, &status.SnapshotIdentifier)
	}

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
