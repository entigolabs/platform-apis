package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

const (
	requiredResVPCjson = `{ 
		"apiVersion": "ec2.aws.m.upbound.io/v1beta1", "kind": "VPC",
		"metadata": {"annotations": {"crossplane.io/external-name": "vpc-01cda48a237c4850f"}, "name": "test-net-vpc", "namespace":"aws-provider"},
		"spec": {"forProvider": {"region": "eu-north-1"}}
	}`
	requiredDBSubnetGroupJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"SubnetGroup",
		"metadata":{"annotations":{"crossplane.io/external-name":"test-net-vpc"},"name":"test-net-vpc", "namespace":"aws-provider"}
	}`
	postgresInputJson           = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "17.2","instanceType": "db.t3.micro"}}`
	postgresSnapshotInputJson   = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "17.2","instanceType": "db.t3.micro","snapshotIdentifier":"rds:test-snapshot-id"}}`
	sgResJson                   = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"description":"allow traffic from vpc","region":"eu-north-1","tags":{"Name":"%s","entigo:zone":"zone-a"}, "vpcIdRef":{"name":"test-net-vpc","namespace":"aws-provider"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	ingressResJson              = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow traffic from vpc","fromPort":5432,"protocol":"tcp","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":5432,"type":"ingress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	egressResJson               = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow traffic from vpc","fromPort":0,"protocol":"-1","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":0,"type":"egress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	instanceResJson             = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"default.postgres17","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	instanceWithSnapshotResJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"default.postgres17","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"snapshotIdentifier":"rds:test-snapshot-id","storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	esResJson                   = `{"apiVersion":"external-secrets.io/v1","kind":"ExternalSecret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"data":[{"remoteRef":{"key":"arn:aws:secretsmanager:eu-north-1:123456789012:secret:test-db-secret-xyz","property":"password","version":"AWSCURRENT"},"secretKey":"password"}],"refreshInterval":"15m0s","refreshPolicy":"Periodic","secretStoreRef":{"kind":"ClusterSecretStore","name":"external-secrets"},"target":{"creationPolicy":"Owner","deletionPolicy":"Delete","name":"%s", "template":{"metadata":{},"data":{"endpoint":"test.rds.amazonaws.com","password":"{{ .password | toString }}","port":"5432","username":"dbadmin"}}}},"status":{"binding":{},"refreshTime":null}}`
	providerConfigJson          = `{"apiVersion":"postgresql.sql.m.crossplane.io/v1alpha1","kind":"ProviderConfig","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"credentials":{"connectionSecretRef":{"name":"test-db-dbadmin"},"source":"PostgreSQLConnectionSecret"},"sslMode":"require"},"status":{}}`

	postgresParameterGroupInputJson                = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "17.2","instanceType": "db.t3.micro","parameterGroupParameters":{"max_connections":"200"}}}`
	postgresParameterGroupUpdatedInputJson         = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "17.2","instanceType": "db.t3.micro","parameterGroupParameters":{"max_connections":"300"}}}`
	postgresNoEngineVersionInputJson               = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"instanceType": "db.t3.micro"}}`
	postgresNoEngineVersionParameterGroupInputJson = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"instanceType": "db.t3.micro","parameterGroupParameters":{"max_connections":"200"}}}`

	pgParameterGroupResJson          = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s"},"spec":{"forProvider":{"description":"Parameter group for PostgreSQL test-db","family":"postgres17","parameter":[{"applyMethod":"immediate","name":"max_connections","value":"200"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	pgParameterGroupUpdatedResJson   = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s"},"spec":{"forProvider":{"description":"Parameter group for PostgreSQL test-db","family":"postgres17","parameter":[{"applyMethod":"immediate","name":"max_connections","value":"300"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	pgOldFamilyParameterGroupResJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s"},"spec":{"forProvider":{"description":"Parameter group for PostgreSQL test-db","family":"postgres16","parameter":[{"applyMethod":"immediate","name":"max_connections","value":"200"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`

	instanceWithParamGroupResJson        = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"%s","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	instanceNoEngineVersionResJson       = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	instanceNoEngineVersionWithPGResJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"%s","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`

	instanceObservedEngineVersionActualJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"name":"%s","namespace":"testspace"},"status":{"atProvider":{"engineVersionActual":"17.2"}}}`
	instanceObservedStillOnOldPGJson        = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"name":"%s","namespace":"testspace"},"status":{"atProvider":{"parameterGroupName":"%s"}}}`
	instanceObservedSwitchedToNewPGJson     = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"name":"%s","namespace":"testspace"},"status":{"atProvider":{"parameterGroupName":"%s"}}}`
)

func withReadyStatus(jsonStr string) *fnv1.Resource {
	u := &unstructured.Unstructured{}
	if err := u.UnmarshalJSON([]byte(jsonStr)); err != nil {
		panic(err)
	}
	conditions := []interface{}{
		map[string]interface{}{
			"type":   "Ready",
			"status": "True",
			"reason": "Available",
		},
		map[string]interface{}{
			"type":   "Synced",
			"status": "True",
			"reason": "ReconcileSuccess",
		},
	}
	err := unstructured.SetNestedSlice(u.Object, conditions, "status", "conditions")
	if err != nil {
		panic(fmt.Sprintf("failed set nested slice to unstructured: %v", err))
	}

	if u.GetKind() == "Instance" {
		atProvider := map[string]interface{}{
			"address":      "test.rds.amazonaws.com",
			"hostedZoneId": "Z12345",
			"port":         float64(5432),
		}
		err := unstructured.SetNestedMap(u.Object, atProvider, "status", "atProvider")
		if err != nil {
			panic(fmt.Sprintf("failed set nested map to unstructured: %v", err))
		}

		masterUserSecret := []interface{}{
			map[string]interface{}{
				"secretArn":    "arn:aws:secretsmanager:eu-north-1:123456789012:secret:test-db-secret-xyz",
				"secretStatus": "active",
			},
		}
		err = unstructured.SetNestedSlice(u.Object, masterUserSecret, "status", "atProvider", "masterUserSecret")
		if err != nil {
			panic(fmt.Sprintf("failed set nested slice to unstructured: %v", err))
		}
	}

	modifiedJSON, err := u.MarshalJSON()
	if err != nil {
		panic(fmt.Sprintf("failed to marshal modified unstructured object back to JSON: %v", err))
	}
	return &fnv1.Resource{Resource: resource.MustStructJSON(string(modifiedJSON))}
}

func TestDatabaseFunction(t *testing.T) {
	var cr v1alpha1.PostgreSQLInstance
	if err := json.Unmarshal([]byte(postgresInputJson), &cr); err != nil {
		t.Fatalf("Failed to unmarshal test composite resource: %v", err)
	}
	setHash := base.GenerateFNVHash(cr.UID)

	environmentData := map[string]interface{}{
		"awsProvider":            "aws-provider",
		"dataKMSKey":             "data",
		"configKMSKey":           "config",
		"vpc":                    "test-net-vpc",
		"subnetGroup":            "test-net-vpc",
		"elasticacheSubnetGroup": "test-elasticache-sg",
		"esClusterSecretStore":   "external-secrets",
		"backupRetentionPeriod":  float64(14),
	}
	optEnvironmentData := map[string]interface{}{
		"tags": map[string]interface{}{
			"env": "test-environment",
		},
	}
	maps.Copy(optEnvironmentData, environmentData)

	pgInstanceName := "test-db"
	sgName := service.GetSGName(pgInstanceName, setHash)
	sgIngressName := service.GetSGIngressName(pgInstanceName, setHash)
	sgEgressName := service.GetSGEgressName(pgInstanceName, setHash)
	instanceName := service.GetRDSInstanceName(pgInstanceName, setHash)
	snapshotName := service.GetRDSInstanceFinalSnapshotName(pgInstanceName, setHash)
	esName := service.GetESName(pgInstanceName, setHash)
	pcName := service.GetPCName(pgInstanceName)
	secretName := "test-db-dbadmin"
	ns := "testspace"
	reqResNs := "aws-provider"
	pgGroupName17 := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-parameterGroup-%s-%s", pgInstanceName, "postgres17", setHash))
	pgGroupName16 := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-parameterGroup-%s-%s", pgInstanceName, "postgres16", setHash))
	pgGroupKey17 := "parameter-group-postgres17"
	pgGroupKey16 := "parameter-group-postgres16"
	postgresRequiredResources := func() map[string]*fnv1.Resources {
		return map[string]*fnv1.Resources{
			"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
			"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
			"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
			"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
			"Secret":        {Items: []*fnv1.Resource{}},
		}
	}
	postgresExpectedRequirements := func() *fnv1.Requirements {
		return &fnv1.Requirements{
			Resources: map[string]*fnv1.ResourceSelector{
				"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
				"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
				"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
				"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
				"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
			},
		}
	}

	cases := map[string]test.Case{
		"PostgreSQLInstance/Stage 1: Create Network when Secret is not found": {
			Reason: "When all requirements are met and no secret exists, desire the network stack.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresInputJson)},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
						"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
						"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":        {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName))},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
							"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
		"PostgreSQLInstance/Stage 2: Create Instance when Network is Ready": {
			Reason: "When network is ready, should desire the network stack AND the RDS Instance.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}}`),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
						"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
						"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":        {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceResJson, instanceName, snapshotName, instanceName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
							"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
		"PostgreSQLInstance/Stage 3: Create ExternalSecret when Instance is Ready": {
			Reason: "When instance is ready, should desire all resources including the ExternalSecret.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db", "namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"storageType":"gp3"}}`),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							instanceName:  withReadyStatus(fmt.Sprintf(`{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"creationTimestamp":null,"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"default.postgres17","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`, instanceName, snapshotName, instanceName, sgName)),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
						"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
						"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":        {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false,"storageType":"gp3","dbInstanceIdentifier":"%s"}}`, instanceName))},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"default.postgres17","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`, instanceName, snapshotName, instanceName, sgName)), Ready: 1},
							esName:        {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName))},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
							"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
		"PostgreSQLInstance/Stage 4: Set Composite Ready when All Resources are Ready": {
			Reason: "When all composed resources are ready, the composite itself should become Ready.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"storageEncrypted":false}}`),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							instanceName:  withReadyStatus(fmt.Sprintf(instanceResJson, instanceName, snapshotName, instanceName, sgName)),
							esName:        withReadyStatus(fmt.Sprintf(esResJson, esName, secretName)),
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName))},
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
						"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
						"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":        {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false,"dbInstanceIdentifier":"%s"}}`, instanceName)),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceResJson, instanceName, snapshotName, instanceName, sgName)), Ready: 1},
							esName:        {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
							"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
		"PostgreSQLInstance/AllEnvData: Added optional environment data": {
			Reason: "When optional environment data is provided, the generated resources should include the optional data.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"storageEncrypted":false}}`),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							instanceName:  withReadyStatus(fmt.Sprintf(instanceResJson, instanceName, snapshotName, instanceName, sgName)),
							esName:        withReadyStatus(fmt.Sprintf(esResJson, esName, secretName)),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData),
						"VPC":               {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSDataKey":        test.KMSKeyResource("data", reqResNs, "mrk-data123"),
						"KMSConfigKey":      test.KMSKeyResource("config", reqResNs, "mrk-config456"),
						"DBSubnetGroup":     {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":            {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false,"dbInstanceIdentifier":"%s"}}`, instanceName)),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName: {Resource: resource.MustStructJSON(`
{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-db-instance-811c9dc5","namespace":"testspace"},"spec":{"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"postgres","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","finalSnapshotIdentifier":"test-db-instance-snapshot-811c9dc5","identifier":"test-db-instance-811c9dc5","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:postgres-17","parameterGroupName":"default.postgres17","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"env":"test-environment","entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"test-db-sg-811c9dc5"}]},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							esName: {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName)), Ready: 1},
							pcName: {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName))},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
							"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
		"PostgreSQLInstance/WithSnapshot: snapshotIdentifier propagated to RDS Instance": {
			Reason: "When snapshotIdentifier is set in spec and network is ready, the RDS Instance should include snapshotIdentifier in forProvider.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: resource.MustStructJSON(postgresSnapshotInputJson),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
						"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
						"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":        {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceWithSnapshotResJson, instanceName, snapshotName, instanceName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
							"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 1: ParameterGroupParameters set creates ParameterGroup alongside network stack": {
			Reason: "With no observed resources and parameterGroupParameters set, ParameterGroup should be desired together with the SecurityGroup/ProviderConfig stack.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresParameterGroupInputJson)},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName))},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 2: Instance withheld while ParameterGroup is not yet Ready": {
			Reason: "Network resources being ready is not enough; the RDS Instance must also wait for the generated ParameterGroup to be Ready.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 3: Instance references generated ParameterGroup once network and ParameterGroup are Ready": {
			Reason: "Once the network stack and ParameterGroup are both ready, the RDS Instance should be desired with parameterGroupName set to the generated ParameterGroup.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
							pgGroupKey17:  withReadyStatus(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17)),
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceWithParamGroupResJson, instanceName, snapshotName, instanceName, pgGroupName17, sgName))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 4: EngineVersion omitted leaves Instance without an explicit version or parameterGroupName": {
			Reason: "engineVersion is optional; when unset, the RDS Instance must not pin a version (AWS picks its own default/latest) and, since the family can't be computed yet, must not guess a parameterGroupName either.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresNoEngineVersionInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceNoEngineVersionResJson, instanceName, snapshotName, instanceName, sgName))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 5: EngineVersion omitted with ParameterGroupParameters set withholds ParameterGroup until family is known": {
			Reason: "The ParameterGroup can't be built without a family, and the family can't be computed without an engine version (spec or observed engineVersionActual), so it must be withheld rather than created without one.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresNoEngineVersionParameterGroupInputJson)},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 6: ParameterGroup is created once Instance reports its actual engine version": {
			Reason: "Once the RDS Instance exists and AWS has reported status.atProvider.engineVersionActual, the family can be derived from it, so the custom ParameterGroup gets built and the Instance is wired to it.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresNoEngineVersionParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceObservedEngineVersionActualJson, instanceName))},
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17))},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceNoEngineVersionWithPGResJson, instanceName, snapshotName, instanceName, pgGroupName17, sgName)), Ready: 2},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 7: EngineVersion change builds new ParameterGroup while keeping the old one alive": {
			Reason: "family is immutable on the AWS ParameterGroup and can't be replaced in place. AWS also refuses to delete a ParameterGroup while an Instance still uses it, so the old one must keep being desired (kept alive, unchanged) alongside the newly-built one until the Instance's observed status confirms it switched away.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
							pgGroupKey16:  withReadyStatus(fmt.Sprintf(pgOldFamilyParameterGroupResJson, pgGroupName16)),
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceObservedStillOnOldPGJson, instanceName, pgGroupName16))},
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							pgGroupKey16:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgOldFamilyParameterGroupResJson, pgGroupName16)), Ready: 1},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17))},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceWithParamGroupResJson, instanceName, snapshotName, instanceName, pgGroupName17, sgName)), Ready: 2},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 7b: old ParameterGroup is dropped once Instance confirms the switch": {
			Reason: "Only the Instance's observed status (not the desired spec we've already been setting) proves AWS actually stopped using the old ParameterGroup - only then is it safe to stop declaring it, so Crossplane deletes it.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
							pgGroupKey16:  withReadyStatus(fmt.Sprintf(pgOldFamilyParameterGroupResJson, pgGroupName16)),
							pgGroupKey17:  withReadyStatus(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17)),
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceObservedSwitchedToNewPGJson, instanceName, pgGroupName17))},
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceWithParamGroupResJson, instanceName, snapshotName, instanceName, pgGroupName17, sgName)), Ready: 2},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 8: Removing ParameterGroupParameters deletes ParameterGroup and reverts Instance to the default": {
			Reason: "Clearing parameterGroupParameters must withhold the ParameterGroup so Crossplane deletes it, and the Instance must be pinned explicitly to default.<family> - parameterGroupName is Optional+Computed on the provider, so merely omitting it would leave the previously-applied custom group name untouched.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
							pgGroupKey17:  withReadyStatus(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17)),
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceResJson, instanceName, snapshotName, instanceName, sgName))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
		"PostgreSQLInstance/ParameterGroup Stage 9: Changing a ParameterGroupParameters value updates the existing ParameterGroup in place": {
			Reason: "family is unchanged, only the parameter value changed. parameter/value is Optional (not ForceNew) on the provider, so the same ParameterGroup must be updated in place, not withheld/recreated.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(postgresParameterGroupUpdatedInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(providerConfigJson, pcName)),
							pgGroupKey17:  withReadyStatus(fmt.Sprintf(pgParameterGroupResJson, pgGroupName17)),
						},
					},
					RequiredResources: postgresRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(providerConfigJson, pcName)), Ready: 1},
							pgGroupKey17:  {Resource: resource.MustStructJSON(fmt.Sprintf(pgParameterGroupUpdatedResJson, pgGroupName17)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceWithParamGroupResJson, instanceName, snapshotName, instanceName, pgGroupName17, sgName))},
						},
					},
					Requirements: postgresExpectedRequirements(),
				},
			},
		},
	}

	test.AddEnvironmentConfig(cases, environmentName, environmentData)
	test.AddZoneResources(cases, ns, "zone-a")
	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases, "annotations", "force-sync", "lastTransitionTime")
}

func TestAddDBInstanceStatus(t *testing.T) {
	cases := map[string]struct {
		observed map[resource.Name]resource.ObservedComposed
		want     map[string]interface{}
	}{
		"AllStatusFieldsProperlyPopulated": {
			observed: map[resource.Name]resource.ObservedComposed{
				"db-instance-test": {
					Resource: &composed.Unstructured{
						Unstructured: unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "rds.aws.m.upbound.io/v1beta1",
								"kind":       "Instance",
								"metadata": map[string]interface{}{
									"name": "db-instance-test",
								},
								"spec": map[string]interface{}{},
								"status": map[string]interface{}{
									"atProvider": map[string]interface{}{
										"address":                  "testdb.c1k4qme2k72a.eu-north-1.rds.amazonaws.com",
										"allowMajorVersionUpgrade": false,
										"autoMinorVersionUpgrade":  true,
										"backupWindow":             "02:00-02:30",
										"finalSnapshotIdentifier":  "testSnapshotIdentifier",
										"hostedZoneId":             "TESTHOSTEDZONE",
										"iops":                     3000,
										"kmsKeyId":                 "arn:aws:kms:eu-north-1:111111111111:key/test",
										"latestRestorableTime":     "2025-01-01T00:00:00Z",
										"maintenanceWindow":        "wed:06:00-wed:06:30",
										"parameterGroupName":       "default.postgres17",
										"port":                     5432,
										"resourceId":               "db-TESTRESID",
										"status":                   "available",
										"storageEncrypted":         true,
										"storageThroughput":        125,
										"vpcSecurityGroupIds": []interface{}{
											"sg-00000000000000000",
										},
									},
								},
							},
						},
					},
				},
			},
			want: map[string]interface{}{
				"allowMajorVersionUpgrade": false,
				"autoMinorVersionUpgrade":  true,
				"backupWindow":             "02:00-02:30",
				"dbInstanceIdentifier":     "db-instance-test",
				"endpoint": map[string]interface{}{
					"address":      "testdb.c1k4qme2k72a.eu-north-1.rds.amazonaws.com",
					"hostedZoneId": "TESTHOSTEDZONE",
					"port":         float64(5432),
				},
				"finalSnapshotIdentifier": "testSnapshotIdentifier",
				"iops":                    float64(3000),
				"kmsKeyId":                "arn:aws:kms:eu-north-1:111111111111:key/test",
				"latestRestorableTime":    "2025-01-01T00:00:00Z",
				"maintenanceWindow":       "wed:06:00-wed:06:30",
				"parameterGroupName":      "default.postgres17",
				"resourceId":              "db-TESTRESID",
				"status":                  "available",
				"storageEncrypted":        true,
				"storageThroughput":       float64(125),
				"vpcSecurityGroupIds": []interface{}{
					"sg-00000000000000000",
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			groupService := &GroupImpl{}
			got, err := groupService.GetObservedStatus(tc.observed["db-instance-test"].Resource)
			if err != nil {
				t.Errorf("AllStatusFieldsProperlyPopulated() = function getCompositeResourceStatus returned err")
			}
			diff := cmp.Diff(tc.want, got, cmpopts.EquateEmpty())
			if diff != "" {
				t.Errorf("AllStatusFieldsProperlyPopulated() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInstanceGetReadyStatus(t *testing.T) {
	cases := map[string]struct {
		observed *composed.Unstructured
		want     resource.Ready
	}{
		"InstanceReady": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Instance",
						"status": map[string]interface{}{
							"atProvider": map[string]interface{}{
								"address":      "testdb.c1k4qme2k72a.eu-north-1.rds.amazonaws.com",
								"hostedZoneId": "TESTHOSTEDZONE",
								"port":         float64(5432),
							},
							"conditions": []interface{}{
								map[string]interface{}{
									"type":   "Ready",
									"status": "True",
								},
							},
						},
					},
				},
			},
			want: resource.ReadyTrue,
		},
		"InstanceNotReady": {
			observed: &composed.Unstructured{
				Unstructured: unstructured.Unstructured{
					Object: map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "Instance",
						"status": map[string]interface{}{
							"atProvider": map[string]interface{}{
								"address":      "testdb.c1k4qme2k72a.eu-north-1.rds.amazonaws.com",
								"hostedZoneId": "TESTHOSTEDZONE",
								"port":         float64(0),
							},
						},
					},
				},
			},
			want: resource.ReadyFalse,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			groupService := &GroupImpl{}
			if got := groupService.GetReadyStatus(tc.observed); !cmp.Equal(got, tc.want, cmpopts.EquateEmpty()) {
				t.Errorf("getReadyStatus() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Valkey test constants
const (
	valkeyInputJson = `{"apiVersion":"database.entigo.com/v1alpha1","kind":"ValkeyInstance","metadata":{"name":"test-valkey","namespace":"testspace"},"spec":{"engineVersion":"8.2","instanceType":"cache.t4g.small","numCacheClusters":2,"autoMinorVersionUpgrade":true,"maintenanceWindow":"sun:05:00-sun:06:00","snapshotWindow":"03:00-05:00","snapshotRetentionLimit":7}}`

	valkeyElasticacheSubnetGroupJson = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"SubnetGroup","metadata":{"name":"test-elasticache-sg","namespace":"aws-provider"},"status":{"atProvider":{"id":"test-elasticache-sg"}}}`
	valkeyComputeSubnetJson          = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"Subnet","metadata":{"name":"compute-1a","namespace":"aws-provider","labels":{"subnet-type":"compute"}},"status":{"atProvider":{"cidrBlock":"10.0.1.0/24"}}}`

	valkeySGResJson              = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey"},"spec":{"forProvider":{"description":"Security group for Valkey test-valkey","region":"eu-north-1","tags":{"Name":"test-valkey","entigo:zone":"zone-a"},"vpcIdRef":{"name":"test-net-vpc","namespace":"aws-provider"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	valkeySMSecretResJson        = `{"apiVersion":"secretsmanager.aws.m.upbound.io/v1beta1","kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-credentials"},"spec":{"forProvider":{"description":"Valkey connection credentials for test-valkey","kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-config456","name":"test-valkey-credentials","recoveryWindowInDays":0,"region":"eu-north-1","tags":{"Name":"test-valkey","entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	valkeySMSecretVersionResJson = `{"apiVersion":"secretsmanager.aws.m.upbound.io/v1beta1","kind":"SecretVersion","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-credentials"},"spec":{"forProvider":{"region":"eu-north-1","secretIdRef":{"name":"test-valkey-credentials"},"secretStringSecretRef":{"key":"credentials.json","name":"test-valkey-credentials"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	valkeyRGResJson              = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey"},"spec":{"forProvider":{"applyImmediately":true,"atRestEncryptionEnabled":"true","authTokenSecretRef":{"key":"auth-token","name":"test-valkey-auth-token"},"authTokenUpdateStrategy":"SET","autoGenerateAuthToken":true,"autoMinorVersionUpgrade":"true","automaticFailoverEnabled":true,"description":"test-valkey","engine":"valkey","engineVersion":"8.2","finalSnapshotIdentifier":"test-valkey-final-snapshot","kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","maintenanceWindow":"sun:05:00-sun:06:00","multiAzEnabled":true,"nodeType":"cache.t4g.small","numCacheClusters":2,"parameterGroupName":"default.valkey8","region":"eu-north-1","securityGroupIdRefs":[{"name":"test-valkey"}],"snapshotRetentionLimit":7,"snapshotWindow":"03:00-05:00","subnetGroupName":"test-elasticache-sg","tags":{"Name":"test-valkey","entigo:zone":"zone-a"},"transitEncryptionEnabled":true},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"test-valkey"}},"status":{"atProvider":{}}}`
	valkeySGRuleResJson          = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-ingress-compute-1a"},"spec":{"forProvider":{"cidrBlocks":["10.0.1.0/24"],"description":"Allow Valkey access from compute-1a","fromPort":6379,"protocol":"tcp","region":"eu-north-1","securityGroupIdRef":{"name":"test-valkey"},"toPort":6379,"type":"ingress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	valkeyCredentialsResJson     = `{"apiVersion":"v1","kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-credentials","namespace":"testspace"},"stringData":{"AUTH_TOKEN":"test-auth-token","PORT":"6379","PRIMARY_ENDPOINT":"test-valkey.eun1.cache.amazonaws.com","READER_ENDPOINT":"test-valkey-ro.eun1.cache.amazonaws.com","credentials.json":"{\"AUTH_TOKEN\": \"test-auth-token\", \"PORT\": \"6379\", \"PRIMARY_ENDPOINT\": \"test-valkey.eun1.cache.amazonaws.com\", \"READER_ENDPOINT\": \"test-valkey-ro.eun1.cache.amazonaws.com\"}"},"type":"Opaque"}`

	valkeyParameterGroupInputJson = `{"apiVersion":"database.entigo.com/v1alpha1","kind":"ValkeyInstance","metadata":{"name":"test-valkey","namespace":"testspace"},"spec":{"engineVersion":"8.2","instanceType":"cache.t4g.small","numCacheClusters":2,"autoMinorVersionUpgrade":true,"maintenanceWindow":"sun:05:00-sun:06:00","snapshotWindow":"03:00-05:00","snapshotRetentionLimit":7,"parameterGroupParameters":{"notify-keyspace-events":"Ex"}}}`
	valkeyPGResJson               = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-parametergroup-valkey8-811c9dc5"},"spec":{"forProvider":{"description":"Parameter group for Valkey test-valkey","family":"valkey8","name":"test-valkey-parametergroup-valkey8-811c9dc5","parameter":[{"name":"notify-keyspace-events","value":"Ex"}],"region":"eu-north-1","tags":{"Name":"test-valkey","entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	valkeyRGWithPGResJson         = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey"},"spec":{"forProvider":{"applyImmediately":true,"atRestEncryptionEnabled":"true","authTokenSecretRef":{"key":"auth-token","name":"test-valkey-auth-token"},"authTokenUpdateStrategy":"SET","autoGenerateAuthToken":true,"autoMinorVersionUpgrade":"true","automaticFailoverEnabled":true,"description":"test-valkey","engine":"valkey","engineVersion":"8.2","finalSnapshotIdentifier":"test-valkey-final-snapshot","kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","maintenanceWindow":"sun:05:00-sun:06:00","multiAzEnabled":true,"nodeType":"cache.t4g.small","numCacheClusters":2,"parameterGroupName":"test-valkey-parametergroup-valkey8-811c9dc5","region":"eu-north-1","securityGroupIdRefs":[{"name":"test-valkey"}],"snapshotRetentionLimit":7,"snapshotWindow":"03:00-05:00","subnetGroupName":"test-elasticache-sg","tags":{"Name":"test-valkey","entigo:zone":"zone-a"},"transitEncryptionEnabled":true},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"test-valkey"}},"status":{"atProvider":{}}}`

	valkeyNoEngineVersionInputJson = `{"apiVersion":"database.entigo.com/v1alpha1","kind":"ValkeyInstance","metadata":{"name":"test-valkey","namespace":"testspace"},"spec":{"instanceType":"cache.t4g.small","numCacheClusters":2,"autoMinorVersionUpgrade":true,"maintenanceWindow":"sun:05:00-sun:06:00","snapshotWindow":"03:00-05:00","snapshotRetentionLimit":7}}`
	valkeyRGNoEngineVersionResJson = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey"},"spec":{"forProvider":{"applyImmediately":true,"atRestEncryptionEnabled":"true","authTokenSecretRef":{"key":"auth-token","name":"test-valkey-auth-token"},"authTokenUpdateStrategy":"SET","autoGenerateAuthToken":true,"autoMinorVersionUpgrade":"true","automaticFailoverEnabled":true,"description":"test-valkey","engine":"valkey","finalSnapshotIdentifier":"test-valkey-final-snapshot","kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","maintenanceWindow":"sun:05:00-sun:06:00","multiAzEnabled":true,"nodeType":"cache.t4g.small","numCacheClusters":2,"region":"eu-north-1","securityGroupIdRefs":[{"name":"test-valkey"}],"snapshotRetentionLimit":7,"snapshotWindow":"03:00-05:00","subnetGroupName":"test-elasticache-sg","tags":{"Name":"test-valkey","entigo:zone":"zone-a"},"transitEncryptionEnabled":true},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"test-valkey"}},"status":{"atProvider":{}}}`

	valkeyNoEngineVersionParameterGroupInputJson = `{"apiVersion":"database.entigo.com/v1alpha1","kind":"ValkeyInstance","metadata":{"name":"test-valkey","namespace":"testspace"},"spec":{"instanceType":"cache.t4g.small","numCacheClusters":2,"autoMinorVersionUpgrade":true,"maintenanceWindow":"sun:05:00-sun:06:00","snapshotWindow":"03:00-05:00","snapshotRetentionLimit":7,"parameterGroupParameters":{"notify-keyspace-events":"Ex"}}}`
	valkeyRGObservedEngineVersionActualJson      = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"name":"test-valkey"},"status":{"atProvider":{"engineVersionActual":"8.2"}}}`
	valkeyRGNoEngineVersionWithPGResJson         = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey"},"spec":{"forProvider":{"applyImmediately":true,"atRestEncryptionEnabled":"true","authTokenSecretRef":{"key":"auth-token","name":"test-valkey-auth-token"},"authTokenUpdateStrategy":"SET","autoGenerateAuthToken":true,"autoMinorVersionUpgrade":"true","automaticFailoverEnabled":true,"description":"test-valkey","engine":"valkey","finalSnapshotIdentifier":"test-valkey-final-snapshot","kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:key/mrk-data123","maintenanceWindow":"sun:05:00-sun:06:00","multiAzEnabled":true,"nodeType":"cache.t4g.small","numCacheClusters":2,"parameterGroupName":"test-valkey-parametergroup-valkey8-811c9dc5","region":"eu-north-1","securityGroupIdRefs":[{"name":"test-valkey"}],"snapshotRetentionLimit":7,"snapshotWindow":"03:00-05:00","subnetGroupName":"test-elasticache-sg","tags":{"Name":"test-valkey","entigo:zone":"zone-a"},"transitEncryptionEnabled":true},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"},"writeConnectionSecretToRef":{"name":"test-valkey"}},"status":{"atProvider":{}}}`

	valkeyOldFamilyPGResJson = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-parametergroup-valkey9-811c9dc5"},"spec":{"forProvider":{"description":"Parameter group for Valkey test-valkey","family":"valkey9","name":"test-valkey-parametergroup-valkey9-811c9dc5","parameter":[{"name":"notify-keyspace-events","value":"Ex"}],"region":"eu-north-1","tags":{"Name":"test-valkey","entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`

	valkeyRGObservedStillOnOldPGJson    = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"name":"test-valkey"},"status":{"atProvider":{"parameterGroupName":"test-valkey-parametergroup-valkey9-811c9dc5"}}}`
	valkeyRGObservedSwitchedToNewPGJson = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ReplicationGroup","metadata":{"name":"test-valkey"},"status":{"atProvider":{"parameterGroupName":"test-valkey-parametergroup-valkey8-811c9dc5"}}}`

	valkeyParameterGroupUpdatedInputJson = `{"apiVersion":"database.entigo.com/v1alpha1","kind":"ValkeyInstance","metadata":{"name":"test-valkey","namespace":"testspace"},"spec":{"engineVersion":"8.2","instanceType":"cache.t4g.small","numCacheClusters":2,"autoMinorVersionUpgrade":true,"maintenanceWindow":"sun:05:00-sun:06:00","snapshotWindow":"03:00-05:00","snapshotRetentionLimit":7,"parameterGroupParameters":{"notify-keyspace-events":"KEA"}}}`
	valkeyPGUpdatedResJson               = `{"apiVersion":"elasticache.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"test-valkey-parametergroup-valkey8-811c9dc5"},"spec":{"forProvider":{"description":"Parameter group for Valkey test-valkey","family":"valkey8","name":"test-valkey-parametergroup-valkey8-811c9dc5","parameter":[{"name":"notify-keyspace-events","value":"KEA"}],"region":"eu-north-1","tags":{"Name":"test-valkey","entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
)

func valkeyRequiredResources() map[string]*fnv1.Resources {
	envData := map[string]interface{}{
		"awsProvider":            "aws-provider",
		"dataKMSKey":             "data",
		"configKMSKey":           "config",
		"vpc":                    "test-net-vpc",
		"subnetGroup":            "test-net-vpc",
		"elasticacheSubnetGroup": "test-elasticache-sg",
		"esClusterSecretStore":   "external-secrets",
		"backupRetentionPeriod":  float64(14),
	}
	return map[string]*fnv1.Resources{
		base.EnvironmentKey:               test.EnvironmentConfigResourceWithData(environmentName, envData),
		base.NamespaceKey:                 test.Namespace("testspace", "zone-a"),
		service.VPCKey:                    {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
		service.ElasticacheSubnetGroupKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(valkeyElasticacheSubnetGroupJson)}}},
		"KMSDataKey":                      test.KMSKeyResource("data", "aws-provider", "mrk-data123"),
		"KMSConfigKey":                    test.KMSKeyResource("config", "aws-provider", "mrk-config456"),
		service.ComputeSubnetsKey:         {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(valkeyComputeSubnetJson)}}},
	}
}

func valkeyExpectedRequirements() *fnv1.Requirements {
	reqResNs := "aws-provider"
	return &fnv1.Requirements{
		Resources: map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey:               base.RequiredEnvironmentConfig(environmentName),
			service.VPCKey:                    {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
			service.ElasticacheSubnetGroupKey: {Kind: "SubnetGroup", ApiVersion: "elasticache.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-elasticache-sg"}},
			"KMSDataKey":                      base.RequiredKMSKey("data", "aws-provider"),
			"KMSConfigKey":                    base.RequiredKMSKey("config", "aws-provider"),
			service.ComputeSubnetsKey: {Kind: "Subnet", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{
				MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": "compute"}},
			}},
		},
	}
}

func TestValkeyInstanceFunction(t *testing.T) {
	cases := map[string]test.Case{
		"ValkeyInstance/Stage 1: Create SecurityGroup": {
			Reason: "With no observed resources, SecurityGroup should be desired.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyInputJson)},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group": {Resource: resource.MustStructJSON(valkeySGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 2: Create ReplicationGroup when SecurityGroup is Ready": {
			Reason: "When SecurityGroup is ready, should also desire ReplicationGroup.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group": withReadyStatus(valkeySGResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":    {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"replication-group": {Resource: resource.MustStructJSON(valkeyRGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 3: All Ready": {
			Reason: "When all resources are ready with connection details, all resources desired with status.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group": withReadyStatus(valkeySGResJson),
							"replication-group": {
								Resource: resource.MustStructJSON(`{
									"apiVersion": "elasticache.aws.m.upbound.io/v1beta1", "kind": "ReplicationGroup",
									"metadata": {"name": "test-valkey"},
									"status": {"atProvider": {"primaryEndpointAddress": "test-valkey.eun1.cache.amazonaws.com", "port": 6379, "autoMinorVersionUpgrade": "true", "kmsKeyId": "arn:aws:kms:eu-north-1:111111111111:key/mrk-data123", "multiAzEnabled": true, "parameterGroupName": "default.valkey8"},
										"conditions": [{"type": "Ready", "status": "True"}]}
								}`),
								ConnectionDetails: map[string][]byte{
									"attribute.auth_token":     []byte("test-auth-token"),
									"port":                     []byte("6379"),
									"primary_endpoint_address": []byte("test-valkey.eun1.cache.amazonaws.com"),
									"reader_endpoint_address":  []byte("test-valkey-ro.eun1.cache.amazonaws.com"),
								},
							},
							"sg-ingress-compute-1a":          withReadyStatus(valkeySGRuleResJson),
							"credentials":                    withReadyStatus(valkeyCredentialsResJson),
							"secrets-manager-secret":         withReadyStatus(valkeySMSecretResJson),
							"secrets-manager-secret-version": withReadyStatus(valkeySMSecretVersionResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":                 {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"replication-group":              {Resource: resource.MustStructJSON(valkeyRGResJson), Ready: 1},
							"sg-ingress-compute-1a":          {Resource: resource.MustStructJSON(valkeySGRuleResJson), Ready: 1},
							"credentials":                    {Resource: resource.MustStructJSON(valkeyCredentialsResJson), Ready: 1},
							"secrets-manager-secret":         {Resource: resource.MustStructJSON(valkeySMSecretResJson), Ready: 1},
							"secrets-manager-secret-version": {Resource: resource.MustStructJSON(valkeySMSecretVersionResJson), Ready: 1},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 4: ParameterGroupParameters set creates ParameterGroup alongside SecurityGroup": {
			Reason: "With no observed resources and parameterGroupParameters set, ParameterGroup should be desired together with SecurityGroup.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyParameterGroupInputJson)},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson)},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 5: ReplicationGroup withheld while ParameterGroup is not yet Ready": {
			Reason: "SecurityGroup being ready is not enough; ReplicationGroup must also wait for the generated ParameterGroup to be Ready.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group": withReadyStatus(valkeySGResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 6: ReplicationGroup references generated ParameterGroup once both are Ready": {
			Reason: "Once SecurityGroup and ParameterGroup are both ready, ReplicationGroup should be desired with parameterGroupName set to the generated ParameterGroup.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group":          withReadyStatus(valkeySGResJson),
							"parameter-group-valkey8": withReadyStatus(valkeyPGResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGResJson), Ready: 1},
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGWithPGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 7: EngineVersion omitted leaves ReplicationGroup without an explicit version or parameterGroupName": {
			Reason: "engineVersion is optional; when unset, ReplicationGroup must not pin a version (AWS picks its own default/latest) and, since the family can't be computed yet, must not guess a parameterGroupName either.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyNoEngineVersionInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group": withReadyStatus(valkeySGResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":    {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"replication-group": {Resource: resource.MustStructJSON(valkeyRGNoEngineVersionResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 8: EngineVersion omitted with ParameterGroupParameters set withholds ParameterGroup until family is known": {
			Reason: "The ParameterGroup can't be built without a family, and the family can't be computed without an engine version (spec or observed engineVersionActual), so it must be withheld rather than created without one.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyNoEngineVersionParameterGroupInputJson)},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group": {Resource: resource.MustStructJSON(valkeySGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 9: ParameterGroup is created once ReplicationGroup reports its actual engine version": {
			Reason: "Once the ReplicationGroup exists and AWS has reported status.atProvider.engineVersionActual, the family can be derived from it, so the custom ParameterGroup gets built and the ReplicationGroup is wired to it.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyNoEngineVersionParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group":    withReadyStatus(valkeySGResJson),
							"replication-group": {Resource: resource.MustStructJSON(valkeyRGObservedEngineVersionActualJson)},
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGResJson)},
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGNoEngineVersionWithPGResJson), Ready: 2},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 10: EngineVersion changed to a different major version builds new ParameterGroup while keeping the old one alive": {
			Reason: "family is immutable on the AWS ParameterGroup and can't be replaced in place. AWS also refuses to delete a ParameterGroup while a ReplicationGroup still uses it, so the old one must keep being desired (kept alive, unchanged) alongside the newly-built one until ReplicationGroup's observed status confirms it switched away.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group":          withReadyStatus(valkeySGResJson),
							"parameter-group-valkey9": withReadyStatus(valkeyOldFamilyPGResJson),
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGObservedStillOnOldPGJson)},
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"parameter-group-valkey9": {Resource: resource.MustStructJSON(valkeyOldFamilyPGResJson), Ready: 1},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGResJson)},
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGWithPGResJson), Ready: 2},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 10b: old ParameterGroup is dropped once ReplicationGroup confirms the switch": {
			Reason: "Only the ReplicationGroup's observed status (not the desired spec we've already been setting) proves AWS actually stopped using the old ParameterGroup - only then is it safe to stop declaring it, so Crossplane deletes it.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group":          withReadyStatus(valkeySGResJson),
							"parameter-group-valkey9": withReadyStatus(valkeyOldFamilyPGResJson),
							"parameter-group-valkey8": withReadyStatus(valkeyPGResJson),
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGObservedSwitchedToNewPGJson)},
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGResJson), Ready: 1},
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGWithPGResJson), Ready: 2},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 11: Removing ParameterGroupParameters deletes ParameterGroup and reverts ReplicationGroup to the default": {
			Reason: "Clearing parameterGroupParameters must withhold the ParameterGroup so Crossplane deletes it, and ReplicationGroup must be pinned explicitly to default.<family> - parameterGroupName is Optional+Computed on the provider, so merely omitting it would leave the previously-applied custom group name untouched.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group":          withReadyStatus(valkeySGResJson),
							"parameter-group-valkey8": withReadyStatus(valkeyPGResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":    {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"replication-group": {Resource: resource.MustStructJSON(valkeyRGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
		"ValkeyInstance/Stage 12: Changing a ParameterGroupParameters value updates the existing ParameterGroup in place": {
			Reason: "family is unchanged, only the parameter value changed. parameter/value is Optional (not ForceNew) on the provider, so the same ParameterGroup must be updated in place, not withheld/recreated.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(valkeyParameterGroupUpdatedInputJson)},
						Resources: map[string]*fnv1.Resource{
							"security-group":          withReadyStatus(valkeySGResJson),
							"parameter-group-valkey8": withReadyStatus(valkeyPGResJson),
						},
					},
					RequiredResources: valkeyRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"security-group":          {Resource: resource.MustStructJSON(valkeySGResJson), Ready: 1},
							"parameter-group-valkey8": {Resource: resource.MustStructJSON(valkeyPGUpdatedResJson), Ready: 1},
							"replication-group":       {Resource: resource.MustStructJSON(valkeyRGWithPGResJson)},
						},
					},
					Requirements: valkeyExpectedRequirements(),
				},
			},
		},
	}

	test.AddZoneResources(cases, "testspace", "zone-a")
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, cases, "annotations", "force-sync", "lastTransitionTime")
}

// MariaDB test constants
const (
	mariadbInputJson         = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "MariaDBInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "11.4","instanceType": "db.t3.micro"}}`
	mariadbSnapshotInputJson = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "MariaDBInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "11.4","instanceType": "db.t3.micro","snapshotIdentifier":"rds:test-snapshot-id"}}`

	mysqlProviderConfigJson = `{"apiVersion":"mysql.sql.m.crossplane.io/v1alpha1","kind":"ProviderConfig","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"credentials":{"connectionSecretRef":{"name":"test-db-dbadmin"},"source":"MySQLConnectionSecret"},"tls":"preferred"},"status":{}}`

	mariadbInstanceResJson             = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"mariadb","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"mariadb","engineVersion":"11.4","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:mariadb-11-4","parameterGroupName":"default.mariadb11.4","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	mariadbInstanceWithSnapshotResJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"mariadb","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"mariadb","engineVersion":"11.4","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:mariadb-11-4","parameterGroupName":"default.mariadb11.4","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"snapshotIdentifier":"rds:test-snapshot-id","storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`

	mariadbParameterGroupInputJson                = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "MariaDBInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "11.4","instanceType": "db.t3.micro","parameterGroupParameters":{"max_connections":"200"}}}`
	mariadbParameterGroupUpdatedInputJson         = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "MariaDBInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "11.4","instanceType": "db.t3.micro","parameterGroupParameters":{"max_connections":"300"}}}`
	mariadbNoEngineVersionInputJson               = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "MariaDBInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"instanceType": "db.t3.micro"}}`
	mariadbNoEngineVersionParameterGroupInputJson = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "MariaDBInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"instanceType": "db.t3.micro","parameterGroupParameters":{"max_connections":"200"}}}`

	mariadbParameterGroupResJson          = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s"},"spec":{"forProvider":{"description":"Parameter group for MariaDB test-db","family":"mariadb11.4","parameter":[{"applyMethod":"immediate","name":"max_connections","value":"200"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	mariadbParameterGroupUpdatedResJson   = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s"},"spec":{"forProvider":{"description":"Parameter group for MariaDB test-db","family":"mariadb11.4","parameter":[{"applyMethod":"immediate","name":"max_connections","value":"300"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	mariadbOldFamilyParameterGroupResJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"ParameterGroup","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s"},"spec":{"forProvider":{"description":"Parameter group for MariaDB test-db","family":"mariadb10.11","parameter":[{"applyMethod":"immediate","name":"max_connections","value":"200"}],"region":"eu-north-1","tags":{"entigo:zone":"zone-a"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`

	mariadbInstanceWithParamGroupResJson        = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"mariadb","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"mariadb","engineVersion":"11.4","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:mariadb-11-4","parameterGroupName":"%s","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	mariadbInstanceNoEngineVersionResJson       = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"mariadb","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"mariadb","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	mariadbInstanceNoEngineVersionWithPGResJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"applyImmediately":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","backupRetentionPeriod":14,"dbName":"mariadb","dbSubnetGroupNameRef":{"name":"test-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"mariadb","finalSnapshotIdentifier":"%s","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"config","namespace":"aws-provider"},"multiAz":false,"optionGroupName":"default:mariadb-11-4","parameterGroupName":"%s","performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","tags":{"entigo:zone":"zone-a"},"username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`

	mariadbInstanceObservedEngineVersionActualJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"engine":"mariadb"}},"status":{"atProvider":{"engineVersionActual":"11.4"}}}`
	mariadbInstanceObservedStillOnOldPGJson        = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"engine":"mariadb"}},"status":{"atProvider":{"parameterGroupName":"%s"}}}`
	mariadbInstanceObservedSwitchedToNewPGJson     = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"engine":"mariadb"}},"status":{"atProvider":{"parameterGroupName":"%s"}}}`
)

func TestMariaDBInstanceFunction(t *testing.T) {
	var cr v1alpha1.MariaDBInstance
	if err := json.Unmarshal([]byte(mariadbInputJson), &cr); err != nil {
		t.Fatalf("Failed to unmarshal test composite resource: %v", err)
	}
	setHash := base.GenerateFNVHash(cr.UID)

	environmentData := map[string]interface{}{
		"awsProvider":            "aws-provider",
		"dataKMSKey":             "data",
		"configKMSKey":           "config",
		"vpc":                    "test-net-vpc",
		"subnetGroup":            "test-net-vpc",
		"elasticacheSubnetGroup": "test-elasticache-sg",
		"esClusterSecretStore":   "external-secrets",
		"backupRetentionPeriod":  float64(14),
	}

	instanceName := service.GetRDSInstanceName("test-db", setHash)
	sgName := service.GetSGName("test-db", setHash)
	sgIngressName := service.GetSGIngressName("test-db", setHash)
	sgEgressName := service.GetSGEgressName("test-db", setHash)
	snapshotName := service.GetRDSInstanceFinalSnapshotName("test-db", setHash)
	esName := service.GetESName("test-db", setHash)
	pcName := service.GetPCName("test-db")
	secretName := "test-db-dbadmin"
	ns := "testspace"
	reqResNs := "aws-provider"
	mariadbGroupName114 := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-parameterGroup-%s-%s", "test-db", "mariadb11.4", setHash))
	mariadbGroupName1011 := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-parameterGroup-%s-%s", "test-db", "mariadb10.11", setHash))
	mariadbGroupKey114 := "parameter-group-mariadb11.4"
	mariadbGroupKey1011 := "parameter-group-mariadb10.11"

	mariadbRequiredResources := func() map[string]*fnv1.Resources {
		return map[string]*fnv1.Resources{
			"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
			"KMSDataKey":    test.KMSKeyResource("data", reqResNs, "mrk-data123"),
			"KMSConfigKey":  test.KMSKeyResource("config", reqResNs, "mrk-config456"),
			"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
			"Secret":        {Items: []*fnv1.Resource{}},
		}
	}
	mariadbExpectedRequirements := func() *fnv1.Requirements {
		return &fnv1.Requirements{
			Resources: map[string]*fnv1.ResourceSelector{
				"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
				"KMSDataKey":    base.RequiredKMSKey(environmentData["dataKMSKey"].(string), reqResNs),
				"KMSConfigKey":  base.RequiredKMSKey(environmentData["configKMSKey"].(string), reqResNs),
				"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-net-vpc"}},
				"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &ns, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
			},
		}
	}

	cases := map[string]test.Case{
		"MariaDBInstance/Stage 1: Create Network when Secret is not found": {
			Reason: "When all requirements are met and no secret exists, desire the network stack.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbInputJson)},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/Stage 2: Create Instance when Network is Ready": {
			Reason: "When network is ready, should desire the network stack AND the RDS Instance.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceResJson, instanceName, snapshotName, instanceName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/Stage 3: Create ExternalSecret when Instance is Ready": {
			Reason: "When instance is ready, should desire all resources including the ExternalSecret.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "MariaDBInstance", "metadata": {"name":"test-db", "namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "11.4", "instanceType": "db.t3.micro"}, "status": {"storageType":"gp3"}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							instanceName:  withReadyStatus(fmt.Sprintf(mariadbInstanceResJson, instanceName, snapshotName, instanceName, sgName)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "MariaDBInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "11.4", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false,"storageType":"gp3","dbInstanceIdentifier":"%s"}}`, instanceName))},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceResJson, instanceName, snapshotName, instanceName, sgName)), Ready: 1},
							esName:        {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/Stage 4: Set Composite Ready when All Resources are Ready": {
			Reason: "When all composed resources are ready, the composite itself should become Ready.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "MariaDBInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "11.4", "instanceType": "db.t3.micro"}, "status": {"storageEncrypted":false}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							instanceName:  withReadyStatus(fmt.Sprintf(mariadbInstanceResJson, instanceName, snapshotName, instanceName, sgName)),
							esName:        withReadyStatus(fmt.Sprintf(esResJson, esName, secretName)),
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName))},
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "MariaDBInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "11.4", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false,"dbInstanceIdentifier":"%s"}}`, instanceName))},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceResJson, instanceName, snapshotName, instanceName, sgName)), Ready: 1},
							esName:        {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/WithSnapshot: snapshotIdentifier propagated to RDS Instance": {
			Reason: "When snapshotIdentifier is set in spec and network is ready, the RDS Instance should include snapshotIdentifier in forProvider.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbSnapshotInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceWithSnapshotResJson, instanceName, snapshotName, instanceName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 1: ParameterGroupParameters set creates ParameterGroup alongside network stack": {
			Reason: "With no observed resources and parameterGroupParameters set, ParameterGroup should be desired together with the SecurityGroup/ProviderConfig stack.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed:          &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbParameterGroupInputJson)}},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:             {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName:      {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgEgressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							pcName:             {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName))},
							mariadbGroupKey114: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 2: Instance withheld while ParameterGroup is not yet Ready": {
			Reason: "Network resources being ready is not enough; the RDS Instance must also wait for the generated ParameterGroup to be Ready.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:             {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:      {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:             {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							mariadbGroupKey114: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 3: Instance references generated ParameterGroup once network and ParameterGroup are Ready": {
			Reason: "Once the network stack and ParameterGroup are both ready, the RDS Instance should be desired with parameterGroupName set to the generated ParameterGroup.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:             withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:      withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:       withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:             withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
							mariadbGroupKey114: withReadyStatus(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:             {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:      {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:             {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							mariadbGroupKey114: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114)), Ready: 1},
							instanceName:       {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceWithParamGroupResJson, instanceName, snapshotName, instanceName, mariadbGroupName114, sgName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 4: EngineVersion omitted leaves Instance without an explicit version or parameterGroupName": {
			Reason: "engineVersion is optional; when unset, the RDS Instance must not pin a version and, since the family can't be computed yet, must not guess a parameterGroupName either.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbNoEngineVersionInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceNoEngineVersionResJson, instanceName, snapshotName, instanceName, sgName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 5: EngineVersion omitted with ParameterGroupParameters set withholds ParameterGroup until family is known": {
			Reason: "The ParameterGroup can't be built without a family, and the family can't be computed without an engine version, so it must be withheld rather than created without one.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed:          &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbNoEngineVersionParameterGroupInputJson)}},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName))},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName))},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName))},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 6: ParameterGroup is created once Instance reports its actual engine version": {
			Reason: "Once the RDS Instance exists and AWS has reported status.atProvider.engineVersionActual, the family can be derived from it, so the custom ParameterGroup gets built and the Instance is wired to it.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbNoEngineVersionParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:        withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName: withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:  withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:        withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceObservedEngineVersionActualJson, instanceName))},
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:             {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:      {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:             {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							mariadbGroupKey114: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114))},
							instanceName:       {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceNoEngineVersionWithPGResJson, instanceName, snapshotName, instanceName, mariadbGroupName114, sgName)), Ready: 2},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 7: EngineVersion change builds new ParameterGroup while keeping the old one alive": {
			Reason: "family is immutable on the AWS ParameterGroup. AWS refuses to delete a ParameterGroup while an Instance still uses it, so the old one must keep being desired alongside the newly-built one until the Instance's observed status confirms it switched away.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:              withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:       withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:        withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:              withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
							mariadbGroupKey1011: withReadyStatus(fmt.Sprintf(mariadbOldFamilyParameterGroupResJson, mariadbGroupName1011)),
							instanceName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceObservedStillOnOldPGJson, instanceName, mariadbGroupName1011))},
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:              {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:        {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:              {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							mariadbGroupKey1011: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbOldFamilyParameterGroupResJson, mariadbGroupName1011)), Ready: 1},
							mariadbGroupKey114:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114))},
							instanceName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceWithParamGroupResJson, instanceName, snapshotName, instanceName, mariadbGroupName114, sgName)), Ready: 2},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 7b: old ParameterGroup is dropped once Instance confirms the switch": {
			Reason: "Only the Instance's observed status proves AWS actually stopped using the old ParameterGroup - only then is it safe to stop declaring it, so Crossplane deletes it.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbParameterGroupInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:              withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:       withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:        withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:              withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
							mariadbGroupKey1011: withReadyStatus(fmt.Sprintf(mariadbOldFamilyParameterGroupResJson, mariadbGroupName1011)),
							mariadbGroupKey114:  withReadyStatus(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114)),
							instanceName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceObservedSwitchedToNewPGJson, instanceName, mariadbGroupName114))},
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:             {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:      {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:             {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							mariadbGroupKey114: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114)), Ready: 1},
							instanceName:       {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceWithParamGroupResJson, instanceName, snapshotName, instanceName, mariadbGroupName114, sgName)), Ready: 2},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 8: Removing ParameterGroupParameters deletes ParameterGroup and reverts Instance to the default": {
			Reason: "Clearing parameterGroupParameters must withhold the ParameterGroup so Crossplane deletes it, and the Instance must be pinned explicitly to default.<family>.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:             withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:      withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:       withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:             withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
							mariadbGroupKey114: withReadyStatus(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:        {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceResJson, instanceName, snapshotName, instanceName, sgName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
		"MariaDBInstance/ParameterGroup Stage 9: Changing a ParameterGroupParameters value updates the existing ParameterGroup in place": {
			Reason: "family is unchanged, only the parameter value changed, so the same ParameterGroup must be updated in place, not withheld/recreated.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(mariadbParameterGroupUpdatedInputJson)},
						Resources: map[string]*fnv1.Resource{
							sgName:             withReadyStatus(fmt.Sprintf(sgResJson, sgName, sgName)),
							sgIngressName:      withReadyStatus(fmt.Sprintf(ingressResJson, sgIngressName, sgName)),
							sgEgressName:       withReadyStatus(fmt.Sprintf(egressResJson, sgEgressName, sgName)),
							pcName:             withReadyStatus(fmt.Sprintf(mysqlProviderConfigJson, pcName)),
							mariadbGroupKey114: withReadyStatus(fmt.Sprintf(mariadbParameterGroupResJson, mariadbGroupName114)),
						},
					},
					RequiredResources: mariadbRequiredResources(),
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							sgName:             {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName:      {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:       {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							pcName:             {Resource: resource.MustStructJSON(fmt.Sprintf(mysqlProviderConfigJson, pcName)), Ready: 1},
							mariadbGroupKey114: {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbParameterGroupUpdatedResJson, mariadbGroupName114)), Ready: 1},
							instanceName:       {Resource: resource.MustStructJSON(fmt.Sprintf(mariadbInstanceWithParamGroupResJson, instanceName, snapshotName, instanceName, mariadbGroupName114, sgName))},
						},
					},
					Requirements: mariadbExpectedRequirements(),
				},
			},
		},
	}

	test.AddEnvironmentConfig(cases, environmentName, environmentData)
	test.AddZoneResources(cases, ns, "zone-a")
	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases, "annotations", "force-sync", "lastTransitionTime")
}
