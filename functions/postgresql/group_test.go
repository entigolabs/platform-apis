package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/platform-apis/model/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/types/known/durationpb"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

const (
	requiredResVPCjson = `{ 
		"apiVersion": "ec2.aws.m.upbound.io/v1beta1", "kind": "VPC",
		"metadata": {"annotations": {"crossplane.io/external-name": "vpc-01cda48a237c4850f"}, "name": "test-net-vpc", "namespace":"aws-provider"},
		"spec": {"deletionPolicy": "Orphan", "forProvider": {"cidrBlock": "10.138.0.0/16", "enableDnsHostNames": true, "enableDnsSupport": true, "enableNetworkAddressUsageMetrics": false, "instanceTenancy": "default","region": "eu-north-1"}, "managementPolicies": ["Observe"], "providerConfigRef": {"name": "aws-provider"}},
		"status": {"atProvider": {"cidrBlockAssociationSet": [{"associationId": "vpc-cidr-assoc-004d60c89a29ef659", "cidrBlock": "10.138.0.0/16", "cidrBlockState": {"state": "associated"}}], "dhcpOptionsId": "dopt-0104bc556d993f0bb", "ownerId": "207567774345", "vpcId": "vpc-01cda48a237c4850f", "vpcState": "available"},
			"conditions": [{"lastTransitionTime": "2025-07-25T08:59:28Z", "reason": "ReconcileSuccess", "status": "True", "type": "Synced"}, {"lastTransitionTime": "2025-07-25T08:59:29Z", "reason": "Available", "status": "True", "type": "Ready"}]}
	}`
	requiredKMSKeyJson = `{"apiVersion":"kms.aws.m.upbound.io/v1beta1","kind":"Key",
		"metadata":{"annotations":{"argocd.argoproj.io/sync-options":"SkipDryRunOnMissingResource=true","argocd.argoproj.io/sync-wave":"10","argocd.argoproj.io/tracking-id":"crossplane-aws:kms.aws.upbound.io/Key:crossplane-aws/biz-data","crossplane.io/external-name":"arn:aws:kms:eu-north-1:877483565445:key/mrk-6c709a49a34940a48025f3bbc412827e"},"name":"biz-data", "namespace":"aws-provider"},
		"spec":{"deletionPolicy":"Orphan","forProvider":{"region":"eu-north-1","tags":{"created-by":"entigo-infralib"}},"managementPolicies":["Observe"],"providerConfigRef":{"name":"aws-provider"}}
	}`
	requiredDBSubnetGroupJson = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"SubnetGroup",
		"metadata":{"annotations":{"argocd.argoproj.io/sync-options":"SkipDryRunOnMissingResource=true","argocd.argoproj.io/sync-wave":"10","argocd.argoproj.io/tracking-id":"crossplane-aws:rds.aws.upbound.io/SubnetGroup:crossplane-aws/biz-net-vpc","crossplane.io/external-name":"biz-net-vpc"},"name":"biz-net-vpc", "namespace":"aws-provider"},
		"spec":{"deletionPolicy":"Orphan","forProvider":{"description":"Subnet group for PostgreSQL instance","region":"eu-north-1","subnetIds":["subnet-06d0eed9e63d945e3","subnet-003df49492fc669cd"],"tags":{"created-by":"entigo-infralib"}},"managementPolicies":["Observe"],"providerConfigRef":{"name":"aws-provider"}}
	}`
	postgresInputJson = `{"apiVersion": "database.entigo.com/v1alpha1","kind": "PostgreSQLInstance","metadata": {"name":"test-db", "namespace":"testspace"},"spec": {"allocatedStorage":20,"engineVersion": "17.2","instanceType": "db.t3.micro"}}`
	sgResJson         = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroup","metadata":{"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"description":"allow traffic from vpc","region":"eu-north-1","tags":{"Name":"%s"}, "vpcIdRef":{"name":"test-net-vpc","namespace":"aws-provider"}},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	ingressResJson    = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow traffic from vpc","fromPort":5432,"protocol":"tcp","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":5432,"type":"ingress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	egressResJson     = `{"apiVersion":"ec2.aws.m.upbound.io/v1beta1","kind":"SecurityGroupRule","metadata":{"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"forProvider":{"cidrBlocks":["0.0.0.0/0"],"description":"allow traffic from vpc","fromPort":0,"protocol":"-1","region":"eu-north-1","securityGroupIdRef":{"name":"%s"},"toPort":0,"type":"egress"},"initProvider":{},"providerConfigRef":{"kind":"ClusterProviderConfig","name":"aws-provider"}},"status":{"atProvider":{}}}`
	instanceResJson   = `{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","dbName":"postgres","dbSubnetGroupNameRef":{"name":"biz-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"biz-data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"biz-data","namespace":"aws-provider"},"multiAz":false,"performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`
	esResJson         = `{"apiVersion":"external-secrets.io/v1","kind":"ExternalSecret","metadata":{"annotations":{"force-sync":"1756119533"},"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"data":[{"remoteRef":{"key":"arn:aws:secretsmanager:eu-north-1:123456789012:secret:test-db-secret-xyz","property":"password","version":"AWSCURRENT"},"secretKey":"password"}],"refreshInterval":"15m0s","refreshPolicy":"Periodic","secretStoreRef":{"kind":"ClusterSecretStore","name":"external-secrets"},"target":{"creationPolicy":"Owner","deletionPolicy":"Delete","name":"%s"}},"status":{"binding":{},"refreshTime":null}}`
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

func TestRunFunction(t *testing.T) {
	type compositeResource struct {
		Spec     v1alpha1.PostgreSQLInstanceSpec `json:"spec"`
		Metadata struct {
			UID types.UID `json:"uid"`
		} `json:"metadata"`
	}

	var cr compositeResource
	if err := json.Unmarshal([]byte(postgresInputJson), &cr); err != nil {
		t.Fatalf("Failed to unmarshal test composite resource: %v", err)
	}
	setHash := base.GenerateFNVHash(cr.Metadata.UID)

	sgName := fmt.Sprintf("test-db-sg-%s", setHash)
	sgIngressName := fmt.Sprintf("test-db-sg-ingress-%s", setHash)
	sgEgressName := fmt.Sprintf("test-db-sg-egress-%s", setHash)
	instanceName := fmt.Sprintf("test-db-instance-%s", setHash)
	esName := fmt.Sprintf("test-db-es-%s", setHash)
	secretName := "test-db-dbadmin"
	secretNs := "testspace"
	reqResNs := "aws-provider"

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
						"KMSKey":        {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSKeyJson)}}},
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
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"KMSKey":        {Kind: "Key", ApiVersion: "kms.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &secretNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
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
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSKey":        {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSKeyJson)}}},
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
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceResJson, instanceName, instanceName, sgName))},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"KMSKey":        {Kind: "Key", ApiVersion: "kms.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &secretNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
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
							instanceName:  withReadyStatus(fmt.Sprintf(`{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","dbName":"postgres","dbSubnetGroupNameRef":{"name":"biz-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"biz-data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"biz-data","namespace":"aws-provider"},"multiAz":false,"performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`, instanceName, instanceName, sgName)),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSKey":        {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSKeyJson)}}},
						"DBSubnetGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredDBSubnetGroupJson)}}},
						"Secret":        {Items: []*fnv1.Resource{}},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"conditions": [{"type": "Ready", "status": "False", "reason": "Creating", "lastTransitionTime": "2025-09-17T11:44:45Z"}],"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false,"storageType":"gp3"}}`)},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(`{"apiVersion":"rds.aws.m.upbound.io/v1beta1","kind":"Instance","metadata":{"creationTimestamp":null,"name":"%s","namespace":"testspace"},"spec":{"providerConfigRef":{"name":"aws-provider","kind":"ClusterProviderConfig"},"managementPolicies":["*"],"forProvider":{"allocatedStorage":20,"allowMajorVersionUpgrade":false,"autoMinorVersionUpgrade":false,"availabilityZone":"eu-north-1a","dbName":"postgres","dbSubnetGroupNameRef":{"name":"biz-net-vpc","namespace":"aws-provider"},"deletionProtection":false,"engine":"postgres","engineVersion":"17.2","identifier":"%s","instanceClass":"db.t3.micro","kmsKeyIdRef":{"name":"biz-data","namespace":"aws-provider"},"manageMasterUserPassword":true,"masterUserSecretKmsKeyIdRef":{"name":"biz-data","namespace":"aws-provider"},"multiAz":false,"performanceInsightsEnabled":false,"publiclyAccessible":false,"region":"eu-north-1","skipFinalSnapshot":false,"storageEncrypted":true,"storageType":"gp3","username":"dbadmin","vpcSecurityGroupIdRefs":[{"name":"%s"}]},"initProvider":{}},"status":{"atProvider":{}}}`, instanceName, instanceName, sgName)), Ready: 1},
							esName:        {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName))},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"KMSKey":        {Kind: "Key", ApiVersion: "kms.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &secretNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
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
							instanceName:  withReadyStatus(fmt.Sprintf(instanceResJson, instanceName, instanceName, sgName)),
							esName:        withReadyStatus(fmt.Sprintf(esResJson, esName, secretName)),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"VPC":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredResVPCjson)}}},
						"KMSKey":        {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSKeyJson)}}},
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
							Resource: resource.MustStructJSON(`{"apiVersion": "database.entigo.com/v1alpha1", "kind": "PostgreSQLInstance", "metadata": {"name":"test-db","namespace":"testspace"}, "spec": {"allocatedStorage":20, "engineVersion": "17.2", "instanceType": "db.t3.micro"}, "status": {"allowMajorVersionUpgrade": false,"autoMinorVersionUpgrade":false,"conditions": [{"type": "Ready", "status": "True", "reason": "Available", "lastTransitionTime": "2025-09-17T11:44:45Z"}],"endpoint":{"address":"test.rds.amazonaws.com","hostedZoneId":"Z12345","port":5432},"storageEncrypted":false}}`),
						},
						Resources: map[string]*fnv1.Resource{
							sgName:        {Resource: resource.MustStructJSON(fmt.Sprintf(sgResJson, sgName, sgName)), Ready: 1},
							sgIngressName: {Resource: resource.MustStructJSON(fmt.Sprintf(ingressResJson, sgIngressName, sgName)), Ready: 1},
							sgEgressName:  {Resource: resource.MustStructJSON(fmt.Sprintf(egressResJson, sgEgressName, sgName)), Ready: 1},
							instanceName:  {Resource: resource.MustStructJSON(fmt.Sprintf(instanceResJson, instanceName, instanceName, sgName)), Ready: 1},
							esName:        {Resource: resource.MustStructJSON(fmt.Sprintf(esResJson, esName, secretName)), Ready: 1},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							"VPC":           {Kind: "VPC", ApiVersion: "ec2.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"KMSKey":        {Kind: "Key", ApiVersion: "kms.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"DBSubnetGroup": {Kind: "SubnetGroup", ApiVersion: "rds.aws.m.upbound.io/v1beta1", Namespace: &reqResNs, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{}}},
							"Secret":        {Kind: "Secret", ApiVersion: "v1", Namespace: &secretNs, Match: &fnv1.ResourceSelector_MatchName{MatchName: secretName}},
						},
					},
				},
			},
		},
	}

	newService := func() base.GroupService {
		return NewGroupImpl("aws-provider")
	}
	test.RunFunctionCases(t, newService, cases, "force-sync", "lastTransitionTime")
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
								"spec":       map[string]interface{}{},
								"status": map[string]interface{}{
									"atProvider": map[string]interface{}{
										"address":                  "testdb.c1k4qme2k72a.eu-north-1.rds.amazonaws.com",
										"allowMajorVersionUpgrade": false,
										"autoMinorVersionUpgrade":  true,
										"backupWindow":             "02:00-02:30",
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
				"endpoint": map[string]interface{}{
					"address":      "testdb.c1k4qme2k72a.eu-north-1.rds.amazonaws.com",
					"hostedZoneId": "TESTHOSTEDZONE",
					"port":         float64(5432),
				},
				"iops":                 float64(3000),
				"kmsKeyId":             "arn:aws:kms:eu-north-1:111111111111:key/test",
				"latestRestorableTime": "2025-01-01T00:00:00Z",
				"maintenanceWindow":    "wed:06:00-wed:06:30",
				"parameterGroupName":   "default.postgres17",
				"resourceId":           "db-TESTRESID",
				"status":               "available",
				"storageEncrypted":     true,
				"storageThroughput":    float64(125),
				"vpcSecurityGroupIds": []interface{}{
					"sg-00000000000000000",
				},
			},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			service := NewGroupImpl("aws-provider")
			got, err := service.GetObservedStatus(tc.observed["db-instance-test"].Resource)
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
			service := NewGroupImpl("aws-provider")
			if got := service.GetReadyStatus(tc.observed); !cmp.Equal(got, tc.want, cmpopts.EquateEmpty()) {
				t.Errorf("getReadyStatus() = %v, want %v", got, tc.want)
			}
		})
	}
}
