package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"google.golang.org/protobuf/types/known/durationpb"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

const (
	requiredVPCjson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "VPC",
		"metadata": {"annotations": {"crossplane.io/external-name": "vpc-01cda48a237c4850f"}, "name": "test-vpc", "namespace":"aws-provider"},
		"spec": {"forProvider": {"region": "eu-north-1"}}
	}`
	requiredKMSAliasJson = `{
		"apiVersion":"kms.aws.upbound.io/v1beta1","kind":"Alias",
		"metadata":{"annotations":{"crossplane.io/external-name":"alias/data"},"name":"data", "namespace":"aws-provider"},
		"status": {"atProvider": {"arn": "arn:aws:kms:eu-north-1:111111111111:alias/data"}}
	}`
	requiredSecurityGroupJson = `{
		"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"SecurityGroup",
		"metadata":{"annotations":{"crossplane.io/external-name":"sg-123"},"name":"eks-node-sg", "namespace":"aws-provider"},
		"status": {"atProvider": {"id": "sg-123"}}
	}`
	requiredClusterJson = `{
		"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"Cluster",
		"metadata":{"annotations":{"crossplane.io/external-name":"test-cluster"},"name":"test-cluster", "namespace":"aws-provider"}
	}`
	requiredSubnetAJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-a"},
		"status": {"atProvider": {"availabilityZone": "eu-north-1a", "id": "subnet-a-id"}}
	}`
	requiredSubnetBJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-b"},
		"status": {"atProvider": {"availabilityZone": "eu-north-1b", "id": "subnet-b-id"}}
	}`
	zoneInputJson              = `{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"name":"test-zone","namespace":"default","uid":"a6b7c8d9-1234-5678-9012-f3e4d5c6b7a8"},"spec":{"clusterPermissions":false,"namespaces":[{"name":"test-app-ns","pool":"app-pool"}],"pools":[{"name":"app-pool","requirements":[{"key":"instance-type","value":"t3.medium"},{"key":"min-size","value":"1"},{"key":"max-size","value":"5"}]}],"appProject":{"contributorGroups":["group-contributor"],"observerGroups":["group-observer"],"maintainerGroups":["group-maintainer"]}}}`
	rbacRoleReadJson           = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-read","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}`
	appProjectJson             = `{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"namespaceResourceBlacklist":[{"group":"*.m.upbound.io","kind":"*"}],"roles":[{"description":"Maintainer permissions","groups":["group-maintainer"],"name":"maintainer","policies":["p, proj:test-zone:maintainer, applications, *, test-zone/*, allow","p, proj:test-zone:maintainer, repositories, *, test-zone/*, allow","p, proj:test-zone:maintainer, applicationsets, *, test-zone/*, allow","p, proj:test-zone:maintainer, logs, *, test-zone/*, allow","p, proj:test-zone:maintainer, exec, *, test-zone/*, allow"]},{"description":"Observer permissions","groups":["group-observer"],"name":"observer","policies":["p, proj:test-zone:observer, applications, get, test-zone/*, allow","p, proj:test-zone:observer, applicationsets, get, test-zone/*, allow"]},{"description":"Contributor permissions","groups":["group-contributor"],"name":"contributor","policies":["p, proj:test-zone:contributor, applications, *, test-zone/*, allow","p, proj:test-zone:contributor, repositories, *, test-zone/*, allow","p, proj:test-zone:contributor, applicationsets, *, test-zone/*, allow","p, proj:test-zone:contributor, logs, *, test-zone/*, allow","p, proj:test-zone:contributor, exec, *, test-zone/*, allow"]},{"description":"Use this role for your CI/CD pipelines","groups":["group-maintainer"],"name":"cicd","policies":["p, proj:test-zone:cicd, applications, sync, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, sync, test-zone/*, allow","p, proj:test-zone:cicd, applications, get, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, get, test-zone/*, allow"]}],"sourceNamespaces":["test-app-ns"],"sourceRepos":["*"]},"status":{}}`
	networkPolicyJson          = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-zone","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`
	rbMaintainerJson           = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-maintainer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"maintainer"}]}`
	roleJson                   = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleECRProxyAttachmentJson = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-proxy"},"spec":{"forProvider":{"policyArnRef":{"name":"ecr-proxy"},"roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleSSMAttachment          = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ssm"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	rbacRoleAllJson            = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-all","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}`
	rbContributorJson          = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-contributor","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"contributor"}]}`
	launchTemplateJson         = `{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-app-pool"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-app-pool","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-app-pool","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-app-pool","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"}},{"resourceType":"volume","tags":{"Name":"test-zone-app-pool-root","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"}}],"tags":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIdRefs":[{"name":"eks-node-sg"}]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleWNAttachmentJson       = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-wn"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	mutatingPolicyJson         = `{"apiVersion":"policies.kyverno.io/v1alpha1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-add-nodeselector"},"spec":{"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["test-app-ns"]}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) || size(object.spec.nodeSelector) == 0 ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone-pool\": \"test-zone-app-pool\"}\n  }\n] : []"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	rbObserverJson             = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-observer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"observer"}]}`
	validatingPolicyJson       = `{"apiVersion":"policies.kyverno.io/v1alpha1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-validate-nodeselector"},"spec":{"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"kubernetes.io/metadata.name","operator":"In","values":["test-app-ns"]}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE","UPDATE"],"resources":["pods"]}]},"validations":[{"expression":"has(object.spec.nodeSelector) \u0026\u0026\n\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-app-pool\"]","message":"Pod nodeSelector must use a valid nodeSelector label value for tenancy.entigo.com/zone-pool. Valid pools: test-zone-app-pool"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	roleECRROAttachment        = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-ro"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	namespaceJson              = `{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns"},"spec":{},"status":{}}`
	nodegroupJson              = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"name":"test-zone-app-pool-6a45af99"},"spec":{"forProvider":{"capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.medium"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"launchTemplate":[{"name":"test-zone-app-pool","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"desiredSize":null,"maxSize":5,"minSize":1}],"subnetIdRefs":[{"name":"subnet-a"},{"name":"subnet-b"}],"tags":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"updateConfig":[{"maxUnavailable":1}]},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	accessEntryJson            = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"AccessEntry","metadata":{"name":"test-zone"},"spec":{"forProvider":{"clusterNameRef":{"name":"test-cluster"},"principalArnFromRoleRef":{"name":"test-zone"},"region":null,"type":"EC2_LINUX"},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
)

func withReadyStatus(jsonStr string) *fnv1.Resource {
	u := &composed.Unstructured{}
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

	if u.GetKind() == "LaunchTemplate" {
		atProvider := map[string]interface{}{
			"latestVersion": "1",
		}
		if err := unstructured.SetNestedMap(u.Object, atProvider, "status", "atProvider"); err != nil {
			panic(fmt.Sprintf("failed set nested map to unstructured: %v", err))
		}
	}

	modifiedJSON, err := u.MarshalJSON()
	if err != nil {
		panic(fmt.Sprintf("failed to marshal modified unstructured object back to JSON: %v", err))
	}
	return &fnv1.Resource{Resource: resource.MustStructJSON(string(modifiedJSON))}
}

func TestZoneFunction(t *testing.T) {
	var cr struct {
		Spec     v1alpha1.ZoneSpec `json:"spec"`
		Metadata struct {
			UID types.UID `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(zoneInputJson), &cr); err != nil {
		t.Fatalf("Failed to unmarshal test composite resource: %v", err)
	}

	environmentData := map[string]interface{}{
		"appProject": map[string]interface{}{
			"maintainerGroups": []interface{}{"group-maintainer"},
			"observerGroups":   []interface{}{"group-observer"},
		},
		"awsProvider":     "aws-provider",
		"argoCDNamespace": "argocd",
		"cluster":         "test-cluster",
		"dataKMSAlias":    "data",
		"securityGroup":   "eks-node-sg",
		"subnetType":      "private",
		"vpc":             "test-vpc",
	}
	optEnvironmentData := map[string]interface{}{
		"tags": map[string]interface{}{
			"env": "test-environment",
		},
		"granularEgress":        true,
		"granularEgressExclude": []interface{}{"test-zone"},
	}
	maps.Copy(optEnvironmentData, environmentData)

	zoneName := "test-zone"
	nsName := "test-app-ns"
	poolName := "app-pool"
	nodeGroupHash := service.GetInstanceTypesHash([]*string{base.StringPtr("t3.medium")})

	requiredResources := map[string]*fnv1.Resources{
		base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
		"VPC":               {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredVPCjson)}}},
		"KMSDataAlias":      {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSAliasJson)}}},
		"NodeSecurityGroup": {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSecurityGroupJson)}}},
		"Cluster":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredClusterJson)}}},
		"Subnets":           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSubnetAJson)}, {Resource: resource.MustStructJSON(requiredSubnetBJson)}}},
	}
	tagsRequiredResources := make(map[string]*fnv1.Resources)
	maps.Copy(tagsRequiredResources, requiredResources)
	tagsRequiredResources[base.EnvironmentKey] = test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData)

	requirements := &fnv1.Requirements{
		Resources: map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
			"VPC":               {Kind: "VPC", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-vpc"}},
			"KMSDataAlias":      {Kind: "Alias", ApiVersion: "kms.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "data"}},
			"NodeSecurityGroup": {Kind: "SecurityGroup", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "eks-node-sg"}},
			"Cluster":           {Kind: "Cluster", ApiVersion: "eks.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-cluster"}},
			"Subnets":           {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": "private"}}}},
		},
	}

	cases := map[string]test.Case{
		"Zone/Stage 1: Generate initial objects": {
			Reason: "Should desire namespaces, kyverno policies and app project",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(zoneInputJson)},
					},
					RequiredResources: requiredResources,
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                {Resource: resource.MustStructJSON(namespaceJson)},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(launchTemplateJson)},
							service.GetAppProjectKey(zoneName):               {Resource: resource.MustStructJSON(appProjectJson)},
							service.GetMutatingPolicyKey(zoneName, nsName):   {Resource: resource.MustStructJSON(mutatingPolicyJson)},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(validatingPolicyJson)},
						},
					},
					Requirements: requirements,
				},
			},
		},
		"Zone/Stage 2: Generate NetworkPolicy and Role": {
			Reason: "Should desire the NetworkPolicy and Role once namespace/launchTemplates are ready",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(zoneInputJson)},
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName): withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):               withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):   withReadyStatus(mutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName): withReadyStatus(validatingPolicyJson),
						},
					},
					RequiredResources: requiredResources,
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                {Resource: resource.MustStructJSON(namespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(launchTemplateJson), Ready: 1},
							service.GetAppProjectKey(zoneName):               {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):   {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(networkPolicyJson)},
							service.GetRoleKey(zoneName):                     {Resource: resource.MustStructJSON(roleJson)},
						},
					},
					Requirements: requirements,
				},
			},
		},
		"Zone/Stage 3: Generate RBAC roles and attachments": {
			Reason: "Should desire the rbac roles and attachments once network policy and role is ready",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(zoneInputJson)},
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName): withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):               withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):   withReadyStatus(mutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName): withReadyStatus(validatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):    withReadyStatus(networkPolicyJson),
							service.GetRoleKey(zoneName):                     withReadyStatus(roleJson),
						},
					},
					RequiredResources: requiredResources,
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                {Resource: resource.MustStructJSON(namespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(launchTemplateJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(networkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName):                     {Resource: resource.MustStructJSON(roleJson), Ready: 1},
							service.GetAppProjectKey(zoneName):               {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):   {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName):         {Resource: resource.MustStructJSON(roleWNAttachmentJson)},
							service.GetRoleECRProxyAttachmentKey(zoneName):   {Resource: resource.MustStructJSON(roleECRProxyAttachmentJson)},
							service.GetRoleECRROAttachmentKey(zoneName):      {Resource: resource.MustStructJSON(roleECRROAttachment)},
							service.GetRoleSSMAttachmentKey(zoneName):        {Resource: resource.MustStructJSON(roleSSMAttachment)},
							service.GetRBACRoleReadKey(zoneName, nsName):     {Resource: resource.MustStructJSON(rbacRoleReadJson)},
							service.GetRBACRoleAllKey(zoneName, nsName):      {Resource: resource.MustStructJSON(rbacRoleAllJson)},
							service.GetAccessentryKey(zoneName):              {Resource: resource.MustStructJSON(accessEntryJson)},
						},
					},
					Requirements: requirements,
				},
			},
		},
		"Zone/Stage 4: Generate RoleBindings and NodeGroup": {
			Reason: "Should desire the role bindings and node group once rbac and attachments are ready",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(zoneInputJson)},
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName): withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):               withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):   withReadyStatus(mutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName): withReadyStatus(validatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):    withReadyStatus(networkPolicyJson),
							service.GetRoleKey(zoneName):                     withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):         withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):   withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):      withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):        withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):     withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):      withReadyStatus(rbacRoleAllJson),
							service.GetAccessentryKey(zoneName):              withReadyStatus(accessEntryJson),
						},
					},
					RequiredResources: requiredResources,
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                {Resource: resource.MustStructJSON(namespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(launchTemplateJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(networkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName):                     {Resource: resource.MustStructJSON(roleJson), Ready: 1},
							service.GetAppProjectKey(zoneName):               {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):   {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName):         {Resource: resource.MustStructJSON(roleWNAttachmentJson), Ready: 1},
							service.GetRoleECRProxyAttachmentKey(zoneName):   {Resource: resource.MustStructJSON(roleECRProxyAttachmentJson), Ready: 1},
							service.GetRoleECRROAttachmentKey(zoneName):      {Resource: resource.MustStructJSON(roleECRROAttachment), Ready: 1},
							service.GetRoleSSMAttachmentKey(zoneName):        {Resource: resource.MustStructJSON(roleSSMAttachment), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, nsName):     {Resource: resource.MustStructJSON(rbacRoleReadJson), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, nsName):      {Resource: resource.MustStructJSON(rbacRoleAllJson), Ready: 1},
							service.GetAccessentryKey(zoneName):              {Resource: resource.MustStructJSON(accessEntryJson), Ready: 1},
							service.GetRBMaintainerKey(zoneName, nsName):     {Resource: resource.MustStructJSON(rbMaintainerJson)},
							service.GetRBContributorKey(zoneName, nsName):    {Resource: resource.MustStructJSON(rbContributorJson)},
							service.GetRBObserverKey(zoneName, nsName):       {Resource: resource.MustStructJSON(rbObserverJson)},
							service.GetNodeGroupKey(poolName, nodeGroupHash): {Resource: resource.MustStructJSON(nodegroupJson)},
						},
					},
					Requirements: requirements,
				},
			},
		},
		"Zone/AllEnvData: Added optional environment data": {
			Reason: "When optional environment data is provided, the generated resources should include the optional data.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(zoneInputJson)},
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName):                withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName): withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):               withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):   withReadyStatus(mutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName): withReadyStatus(validatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):    withReadyStatus(networkPolicyJson),
							service.GetRoleKey(zoneName):                     withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):         withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):   withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):      withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):        withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):     withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):      withReadyStatus(rbacRoleAllJson),
							service.GetRBMaintainerKey(zoneName, nsName):     withReadyStatus(rbMaintainerJson),
							service.GetRBContributorKey(zoneName, nsName):    withReadyStatus(rbContributorJson),
							service.GetRBObserverKey(zoneName, nsName):       withReadyStatus(rbObserverJson),
							service.GetNodeGroupKey(poolName, nodeGroupHash): withReadyStatus(nodegroupJson),
						},
					},
					RequiredResources: tagsRequiredResources,
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"istio-injection":"enabled","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns"},"spec":{},"status":{}}
							`), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(`
{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-app-pool"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-app-pool","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-app-pool","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-app-pool","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"}},{"resourceType":"volume","tags":{"Name":"test-zone-app-pool-root","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"}}],"tags":{"env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIdRefs":[{"name":"eks-node-sg"}]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(networkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"env":"test-environment","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetAppProjectKey(zoneName):               {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):   {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName):         {Resource: resource.MustStructJSON(roleWNAttachmentJson), Ready: 1},
							service.GetRoleECRProxyAttachmentKey(zoneName):   {Resource: resource.MustStructJSON(roleECRProxyAttachmentJson), Ready: 1},
							service.GetRoleECRROAttachmentKey(zoneName):      {Resource: resource.MustStructJSON(roleECRROAttachment), Ready: 1},
							service.GetRoleSSMAttachmentKey(zoneName):        {Resource: resource.MustStructJSON(roleSSMAttachment), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, nsName):     {Resource: resource.MustStructJSON(rbacRoleReadJson), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, nsName):      {Resource: resource.MustStructJSON(rbacRoleAllJson), Ready: 1},
							service.GetRBMaintainerKey(zoneName, nsName):     {Resource: resource.MustStructJSON(rbMaintainerJson), Ready: 1},
							service.GetRBContributorKey(zoneName, nsName):    {Resource: resource.MustStructJSON(rbContributorJson), Ready: 1},
							service.GetRBObserverKey(zoneName, nsName):       {Resource: resource.MustStructJSON(rbObserverJson), Ready: 1},
							service.GetNodeGroupKey(poolName, nodeGroupHash): {Resource: resource.MustStructJSON(`
{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"name":"test-zone-app-pool-6a45af99"},"spec":{"forProvider":{"capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.medium"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"launchTemplate":[{"name":"test-zone-app-pool","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"desiredSize":null,"maxSize":5,"minSize":1}],"subnetIdRefs":[{"name":"subnet-a"},{"name":"subnet-b"}],"tags":{"env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-app-pool"},"updateConfig":[{"maxUnavailable":1}]},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetSidecarKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"Sidecar","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-egress","namespace":"test-app-ns"},"spec":{"egress":[{"hosts":["*/*.svc.cluster.local","istio-system/*","kube-system/kube-dns.kube-system.svc.cluster.local","test-app-ns/*"]}],"outboundTrafficPolicy":{"mode":"ALLOW_ANY"}},"status":{}}
							`)},
						},
					},
					Requirements: requirements,
				},
			},
		},
	}

	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases)
}
