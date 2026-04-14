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
	"k8s.io/apimachinery/pkg/util/intstr"

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
		"metadata":{"annotations":{"crossplane.io/external-name":"test-cluster"},"name":"test-cluster", "namespace":"aws-provider"},
		"status": {"atProvider": {"version": "1.34"}}
	}`
	requiredSubnetAJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-a"},
		"status": {"atProvider": {"availabilityZone": "eu-north-1a", "id": "subnet-a-id", "cidrBlock": "10.10.10.1"}}
	}`
	requiredSubnetBJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-b"},
		"status": {"atProvider": {"availabilityZone": "eu-north-1b", "id": "subnet-b-id", "cidrBlock": "10.10.10.2"}}
	}`
	requiredSubnetCJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-c"},
		"status": {"atProvider": {"availabilityZone": "eu-north-1c", "id": "subnet-c-id", "cidrBlock": "10.10.10.3"}}
	}`
	requiredIngressJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"Ingress",
        "metadata":{"name":"test-ingress","namespace":"test-app-ns"},
        "spec":{"ingressClassName":"service","rules":[{"host":"example.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"test-service","port":{"number":8080}}}}]}}]}
    }`
	requiredServiceJson = `{
        "apiVersion": "v1", "kind": "Service",
        "metadata": { "name": "test-service", "namespace": "test-app-ns"},
        "spec": {"selector": {"app": "test-app"},"ports": [{"port": 8080, "targetPort": 8081}]}
    }`
	requiredExtIngressJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"Ingress",
        "metadata":{"name":"test-ext-ingress","namespace":"test-app-ext-ns"},
        "spec":{"ingressClassName":"external","rules":[{"host":"example.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"test-service","port":{"number":8080}}}}]}}]}
    }`
	requiredExtServiceJson = `{
        "apiVersion": "v1", "kind": "Service",
        "metadata": { "name": "test-ext-service", "namespace": "test-app-ext-ns"},
        "spec": {"selector": {"app": "test-app"},"ports": [{"port": 443, "targetPort": 8443}]}
    }`
	zoneInputJson               = `{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"name":"test-zone"},"spec":{"clusterPermissions":false,"namespaces":[{"name":"test-app-ns"}],"pools":[{"name":"default","requirements":[{"key":"instance-type","values":["t3.large"]},{"key":"capacity-type","value":"ON_DEMAND"},{"key":"min-size","value":1},{"key":"max-size","value":2},{"key":"security-groups","values":["sg-321"]}]}]}}`
	rbacRoleReadJson            = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-read","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}`
	extRbacRoleReadJson         = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-read","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}`
	appProjectJson              = `{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ext-ns","server":"https://kubernetes.default.svc"},{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"roles":[{"description":"Maintainer permissions","groups":["group-maintainer"],"name":"maintainer","policies":["p, proj:test-zone:maintainer, applications, *, test-zone/*, allow","p, proj:test-zone:maintainer, repositories, *, test-zone/*, allow","p, proj:test-zone:maintainer, applicationsets, *, test-zone/*, allow","p, proj:test-zone:maintainer, logs, *, test-zone/*, allow","p, proj:test-zone:maintainer, exec, *, test-zone/*, allow"]},{"description":"Observer permissions","groups":["group-observer"],"name":"observer","policies":["p, proj:test-zone:observer, applications, get, test-zone/*, allow","p, proj:test-zone:observer, applicationsets, get, test-zone/*, allow"]},{"description":"Contributor permissions","groups":["group-contributor"],"name":"contributor","policies":["p, proj:test-zone:contributor, applications, *, test-zone/*, allow","p, proj:test-zone:contributor, repositories, *, test-zone/*, allow","p, proj:test-zone:contributor, applicationsets, *, test-zone/*, allow","p, proj:test-zone:contributor, logs, *, test-zone/*, allow","p, proj:test-zone:contributor, exec, *, test-zone/*, allow"]},{"description":"Use this role for your CI/CD pipelines","groups":["group-maintainer"],"name":"cicd","policies":["p, proj:test-zone:cicd, applications, sync, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, sync, test-zone/*, allow","p, proj:test-zone:cicd, applications, get, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, get, test-zone/*, allow"]}],"sourceNamespaces":["test-app-ext-ns","test-app-ns"],"sourceRepos":["*"]},"status":{}}`
	networkPolicyJson           = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-zone","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`
	extNetworkPolicyJson        = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-zone","namespace":"test-app-ext-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`
	targetNetworkPolicyJson     = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-ingress-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.1"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}`
	rbMaintainerJson            = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-maintainer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	extRBMaintainerJson         = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-maintainer","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	roleJson                    = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleECRProxyAttachmentJson  = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-proxy"},"spec":{"forProvider":{"policyArnRef":{"name":"ecr-proxy"},"roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleSSMAttachment           = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ssm"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	rbacRoleAllJson             = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-all","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}`
	extRbacRoleAllJson          = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-all","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}`
	rbContributorJson           = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-contributor","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	extRBContributorJson        = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-contributor","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	launchTemplateJson          = `{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-default","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-default","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-default","entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}},{"resourceType":"volume","tags":{"Name":"test-zone-default-root","entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}}],"tags":{"entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIds":["sg-123","sg-321"]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleWNAttachmentJson        = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-wn"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	mutatingPolicyJson          = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-add-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) || size(object.spec.nodeSelector) == 0 ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\"}\n  }\n] : []"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	labelsMutatingPolicyJson    = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	rbObserverJson              = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-observer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}`
	extRBObserverJson           = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-observer","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}`
	validatingPolicyJson        = `{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-validate-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE","UPDATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"\nhas(object.spec.nodeSelector) \u0026\u0026\n(\n\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n) || (\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\"\n)","message":"Pod nodeSelector must either use tenancy.entigo.com/zone-pool with a valid value [test-zone-default] or tenancy.entigo.com/zone with value test-zone"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	roleECRROAttachment         = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-ro"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	namespaceJson               = `{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns"},"spec":{},"status":{}}`
	extNamespaceJson            = `{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns"},"spec":{},"status":{}}`
	nodegroupJson               = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default-050c2b39"},"spec":{"forProvider":{"capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.large"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"launchTemplate":[{"name":"test-zone-default","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"maxSize":2,"minSize":1}],"subnetIds":["subnet-a-id","subnet-b-id"],"tags":{"entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateConfig":[{"maxUnavailable":1}],"version":"1.34"},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	accessEntryJson             = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"AccessEntry","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"clusterNameRef":{"name":"test-cluster"},"principalArnFromRoleRef":{"name":"test-zone"},"region":null,"tags":{"entigo:zone":"test-zone"},"type":"EC2_LINUX"},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	extMutatingPolicyJson       = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-add-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) || size(object.spec.nodeSelector) == 0 ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\"}\n  }\n] : []"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	extLabelsMutatingPolicyJson = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	extValidatingPolicyJson     = `{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-validate-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE","UPDATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"\nhas(object.spec.nodeSelector) \u0026\u0026\n(\n\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n) || (\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\"\n)","message":"Pod nodeSelector must either use tenancy.entigo.com/zone-pool with a valid value [test-zone-default] or tenancy.entigo.com/zone with value test-zone"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
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
			"latestVersion": float64(1),
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
	var cr v1alpha1.Zone
	if err := json.Unmarshal([]byte(zoneInputJson), &cr); err != nil {
		t.Fatalf("Failed to unmarshal test composite resource: %v", err)
	}

	appProject := map[string]interface{}{
		"sourceRepos":      []interface{}{"*"},
		"maintainerGroups": []interface{}{"group-maintainer"},
		"observerGroups":   []interface{}{"group-observer"},
	}
	roleMappings := []interface{}{
		map[string]interface{}{
			"roleRef": "contributor",
			"groups":  []interface{}{"group-contributor"},
		},
		map[string]interface{}{
			"roleRef": "maintainer",
			"groups":  []interface{}{"group-maintainer"},
		},
		map[string]interface{}{
			"roleRef": "observer",
			"groups":  []interface{}{"group-observer"},
		},
	}
	environmentData := map[string]interface{}{
		"appProject":        appProject,
		"roleMapping":       roleMappings,
		"awsProvider":       "aws-provider",
		"argoCDNamespace":   "argocd",
		"cluster":           "test-cluster",
		"dataKMSAlias":      "data",
		"securityGroup":     "eks-node-sg",
		"computeSubnetType": "compute",
		"serviceSubnetType": "service",
		"publicSubnetType":  "public",
		"controlSubnetType": "control",
		"vpc":               "test-vpc",
		"podSecurity":       "baseline",
	}
	optEnvironmentData := map[string]interface{}{
		"tags": map[string]interface{}{
			"env": "test-environment",
		},
		"granularEgress":        true,
		"granularEgressExclude": []interface{}{"test-zone"},
	}
	maps.Copy(optEnvironmentData, environmentData)
	optAppProject := map[string]interface{}{
		"namespaceResourceBlacklist": []interface{}{map[string]interface{}{"group": "*.m.upbound.io", "kind": "*"}},
		"namespaceResourceWhitelist": []interface{}{map[string]interface{}{"group": "*.entigo.com", "kind": "*"}},
	}
	maps.Copy(optAppProject, appProject)
	optEnvironmentData["appProject"] = optAppProject

	zoneName := "test-zone"
	nsName := "test-app-ns"
	extNsName := "test-app-ext-ns"
	poolName := "default"
	targetNetworkPolicyKey := service.GetTargetNetworkPolicyKey(nsName, "test-ingress", "test-service", intstr.FromInt32(8081))
	nodeGroupHash := service.GetInstanceTypesHash([]string{"t3.large"})

	requiredResources := map[string]*fnv1.Resources{
		base.EnvironmentKey:            test.EnvironmentConfigResourceWithData(environmentName, environmentData),
		service.NamespaceKey:           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(extNamespaceJson)}}},
		service.VPCKey:                 {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredVPCjson)}}},
		service.KMSDataAliasKey:        {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSAliasJson)}}},
		service.SecurityGroupKey:       {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSecurityGroupJson)}}},
		service.ClusterKey:             {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredClusterJson)}}},
		service.ComputeSubnetsKey:      {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSubnetAJson)}, {Resource: resource.MustStructJSON(requiredSubnetBJson)}}},
		service.ServiceSubnetsKey:      {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSubnetAJson)}}},
		service.PublicSubnetsKey:       {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSubnetBJson)}}},
		service.ControlSubnetsKey:      {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSubnetCJson)}}},
		nsName + service.IngressKey:    {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredIngressJson)}}},
		nsName + service.ServiceKey:    {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredServiceJson)}}},
		extNsName + service.IngressKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredExtIngressJson)}}},
		extNsName + service.ServiceKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredExtServiceJson)}}},
	}
	tagsRequiredResources := make(map[string]*fnv1.Resources)
	maps.Copy(tagsRequiredResources, requiredResources)
	tagsRequiredResources[base.EnvironmentKey] = test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData)

	requirements := &fnv1.Requirements{
		Resources: map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey:            base.RequiredEnvironmentConfig(environmentName),
			service.NamespaceKey:           {Kind: "Namespace", ApiVersion: "v1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{base.TenancyZoneLabel: zoneName}}}},
			service.VPCKey:                 {Kind: "VPC", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-vpc"}},
			service.KMSDataAliasKey:        {Kind: "Alias", ApiVersion: "kms.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "data"}},
			service.SecurityGroupKey:       {Kind: "SecurityGroup", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "eks-node-sg"}},
			service.ClusterKey:             {Kind: "Cluster", ApiVersion: "eks.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-cluster"}},
			service.ComputeSubnetsKey:      {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": "compute"}}}},
			service.ServiceSubnetsKey:      {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": "service"}}}},
			service.PublicSubnetsKey:       {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": "public"}}}},
			service.ControlSubnetsKey:      {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{"subnet-type": "control"}}}},
			nsName + service.IngressKey:    {Kind: "Ingress", ApiVersion: "networking.k8s.io/v1", Namespace: &nsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			nsName + service.ServiceKey:    {Kind: "Service", ApiVersion: "v1", Namespace: &nsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			extNsName + service.IngressKey: {Kind: "Ingress", ApiVersion: "networking.k8s.io/v1", Namespace: &extNsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			extNsName + service.ServiceKey: {Kind: "Service", ApiVersion: "v1", Namespace: &extNsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
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
							service.GetNamespaceKey(nsName):                         {Resource: resource.MustStructJSON(namespaceJson)},
							service.GetLaunchTemplateKey(zoneName, poolName):        {Resource: resource.MustStructJSON(launchTemplateJson)},
							service.GetAppProjectKey(zoneName):                      {Resource: resource.MustStructJSON(appProjectJson)},
							service.GetMutatingPolicyKey(zoneName, nsName):          {Resource: resource.MustStructJSON(mutatingPolicyJson)},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(labelsMutatingPolicyJson)},
							service.GetMutatingPolicyKey(zoneName, extNsName):       {Resource: resource.MustStructJSON(extMutatingPolicyJson)},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(extLabelsMutatingPolicyJson)},
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
							service.GetNamespaceKey(nsName):                         withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):        withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                      withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):          withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):        withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):       withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):     withReadyStatus(extValidatingPolicyJson),
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
							service.GetNamespaceKey(nsName):                         {Resource: resource.MustStructJSON(namespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName):        {Resource: resource.MustStructJSON(launchTemplateJson), Ready: 1},
							service.GetAppProjectKey(zoneName):                      {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):          {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(labelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName):        {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, extNsName):       {Resource: resource.MustStructJSON(extMutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(extLabelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, extNsName):     {Resource: resource.MustStructJSON(extValidatingPolicyJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):           {Resource: resource.MustStructJSON(networkPolicyJson)},
							service.GetNetworkPolicyKey(zoneName, extNsName):        {Resource: resource.MustStructJSON(extNetworkPolicyJson)},
							targetNetworkPolicyKey:                                  {Resource: resource.MustStructJSON(targetNetworkPolicyJson)},
							service.GetRoleKey(zoneName):                            {Resource: resource.MustStructJSON(roleJson)},
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
							service.GetNamespaceKey(nsName):                         withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):        withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                      withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):          withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):        withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):       withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):     withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):           withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):        withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                  withReadyStatus(targetNetworkPolicyJson),
							service.GetRoleKey(zoneName):                            withReadyStatus(roleJson),
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
							service.GetNamespaceKey(nsName):                         {Resource: resource.MustStructJSON(namespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName):        {Resource: resource.MustStructJSON(launchTemplateJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):           {Resource: resource.MustStructJSON(networkPolicyJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, extNsName):        {Resource: resource.MustStructJSON(extNetworkPolicyJson), Ready: 1},
							targetNetworkPolicyKey:                                  {Resource: resource.MustStructJSON(targetNetworkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName):                            {Resource: resource.MustStructJSON(roleJson), Ready: 1},
							service.GetAppProjectKey(zoneName):                      {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):          {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(labelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName):        {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, extNsName):       {Resource: resource.MustStructJSON(extMutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(extLabelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, extNsName):     {Resource: resource.MustStructJSON(extValidatingPolicyJson), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName):                {Resource: resource.MustStructJSON(roleWNAttachmentJson)},
							service.GetRoleECRProxyAttachmentKey(zoneName):          {Resource: resource.MustStructJSON(roleECRProxyAttachmentJson)},
							service.GetRoleECRROAttachmentKey(zoneName):             {Resource: resource.MustStructJSON(roleECRROAttachment)},
							service.GetRoleSSMAttachmentKey(zoneName):               {Resource: resource.MustStructJSON(roleSSMAttachment)},
							service.GetRBACRoleReadKey(zoneName, nsName):            {Resource: resource.MustStructJSON(rbacRoleReadJson)},
							service.GetRBACRoleAllKey(zoneName, nsName):             {Resource: resource.MustStructJSON(rbacRoleAllJson)},
							service.GetRBACRoleReadKey(zoneName, extNsName):         {Resource: resource.MustStructJSON(extRbacRoleReadJson)},
							service.GetRBACRoleAllKey(zoneName, extNsName):          {Resource: resource.MustStructJSON(extRbacRoleAllJson)},
							service.GetAccessentryKey(zoneName):                     {Resource: resource.MustStructJSON(accessEntryJson)},
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
							service.GetNamespaceKey(nsName):                         withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):        withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                      withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):          withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):        withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):       withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):     withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):           withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):        withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                  withReadyStatus(targetNetworkPolicyJson),
							service.GetRoleKey(zoneName):                            withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):                withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):          withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):             withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):               withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):            withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):             withReadyStatus(rbacRoleAllJson),
							service.GetRBACRoleReadKey(zoneName, extNsName):         withReadyStatus(extRbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, extNsName):          withReadyStatus(extRbacRoleAllJson),
							service.GetAccessentryKey(zoneName):                     withReadyStatus(accessEntryJson),
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
							service.GetNamespaceKey(nsName):                         {Resource: resource.MustStructJSON(namespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName):        {Resource: resource.MustStructJSON(launchTemplateJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):           {Resource: resource.MustStructJSON(networkPolicyJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, extNsName):        {Resource: resource.MustStructJSON(extNetworkPolicyJson), Ready: 1},
							targetNetworkPolicyKey:                                  {Resource: resource.MustStructJSON(targetNetworkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName):                            {Resource: resource.MustStructJSON(roleJson), Ready: 1},
							service.GetAppProjectKey(zoneName):                      {Resource: resource.MustStructJSON(appProjectJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):          {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(labelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName):        {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, extNsName):       {Resource: resource.MustStructJSON(extMutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(extLabelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, extNsName):     {Resource: resource.MustStructJSON(extValidatingPolicyJson), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName):                {Resource: resource.MustStructJSON(roleWNAttachmentJson), Ready: 1},
							service.GetRoleECRProxyAttachmentKey(zoneName):          {Resource: resource.MustStructJSON(roleECRProxyAttachmentJson), Ready: 1},
							service.GetRoleECRROAttachmentKey(zoneName):             {Resource: resource.MustStructJSON(roleECRROAttachment), Ready: 1},
							service.GetRoleSSMAttachmentKey(zoneName):               {Resource: resource.MustStructJSON(roleSSMAttachment), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, nsName):            {Resource: resource.MustStructJSON(rbacRoleReadJson), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, nsName):             {Resource: resource.MustStructJSON(rbacRoleAllJson), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, extNsName):         {Resource: resource.MustStructJSON(extRbacRoleReadJson), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, extNsName):          {Resource: resource.MustStructJSON(extRbacRoleAllJson), Ready: 1},
							service.GetAccessentryKey(zoneName):                     {Resource: resource.MustStructJSON(accessEntryJson), Ready: 1},
							service.GetRBMaintainerKey(zoneName, nsName):            {Resource: resource.MustStructJSON(rbMaintainerJson)},
							service.GetRBContributorKey(zoneName, nsName):           {Resource: resource.MustStructJSON(rbContributorJson)},
							service.GetRBObserverKey(zoneName, nsName):              {Resource: resource.MustStructJSON(rbObserverJson)},
							service.GetRBMaintainerKey(zoneName, extNsName):         {Resource: resource.MustStructJSON(extRBMaintainerJson)},
							service.GetRBContributorKey(zoneName, extNsName):        {Resource: resource.MustStructJSON(extRBContributorJson)},
							service.GetRBObserverKey(zoneName, extNsName):           {Resource: resource.MustStructJSON(extRBObserverJson)},
							service.GetNodeGroupKey(poolName, nodeGroupHash):        {Resource: resource.MustStructJSON(nodegroupJson)},
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
							service.GetNamespaceKey(nsName):                         withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):        withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                      withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):          withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):        withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):       withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):     withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):           withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):        withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                  withReadyStatus(targetNetworkPolicyJson),
							service.GetRoleKey(zoneName):                            withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):                withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):          withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):             withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):               withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):            withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):             withReadyStatus(rbacRoleAllJson),
							service.GetRBMaintainerKey(zoneName, nsName):            withReadyStatus(rbMaintainerJson),
							service.GetRBContributorKey(zoneName, nsName):           withReadyStatus(rbContributorJson),
							service.GetRBObserverKey(zoneName, nsName):              withReadyStatus(rbObserverJson),
							service.GetNodeGroupKey(poolName, nodeGroupHash):        withReadyStatus(nodegroupJson),
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
							service.GetNamespaceKey(nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"istio-injection":"enabled","pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns"},"spec":{},"status":{}}
							`), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(`
{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-default","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-default","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-default","entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}},{"resourceType":"volume","tags":{"Name":"test-zone-default-root","entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}}],"tags":{"entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIds":["sg-123","sg-321"]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(networkPolicyJson), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(extNetworkPolicyJson), Ready: 1},
							targetNetworkPolicyKey:                           {Resource: resource.MustStructJSON(targetNetworkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetAppProjectKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ext-ns","server":"https://kubernetes.default.svc"},{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"namespaceResourceBlacklist":[{"group":"*.m.upbound.io","kind":"*"}],"namespaceResourceWhitelist":[{"group":"*.entigo.com","kind":"*"}],"roles":[{"description":"Maintainer permissions","groups":["group-maintainer"],"name":"maintainer","policies":["p, proj:test-zone:maintainer, applications, *, test-zone/*, allow","p, proj:test-zone:maintainer, repositories, *, test-zone/*, allow","p, proj:test-zone:maintainer, applicationsets, *, test-zone/*, allow","p, proj:test-zone:maintainer, logs, *, test-zone/*, allow","p, proj:test-zone:maintainer, exec, *, test-zone/*, allow"]},{"description":"Observer permissions","groups":["group-observer"],"name":"observer","policies":["p, proj:test-zone:observer, applications, get, test-zone/*, allow","p, proj:test-zone:observer, applicationsets, get, test-zone/*, allow"]},{"description":"Contributor permissions","groups":["group-contributor"],"name":"contributor","policies":["p, proj:test-zone:contributor, applications, *, test-zone/*, allow","p, proj:test-zone:contributor, repositories, *, test-zone/*, allow","p, proj:test-zone:contributor, applicationsets, *, test-zone/*, allow","p, proj:test-zone:contributor, logs, *, test-zone/*, allow","p, proj:test-zone:contributor, exec, *, test-zone/*, allow"]},{"description":"Use this role for your CI/CD pipelines","groups":["group-maintainer"],"name":"cicd","policies":["p, proj:test-zone:cicd, applications, sync, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, sync, test-zone/*, allow","p, proj:test-zone:cicd, applications, get, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, get, test-zone/*, allow"]}],"sourceNamespaces":["test-app-ext-ns","test-app-ns"],"sourceRepos":["*"]},"status":{}}
							`), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName):          {Resource: resource.MustStructJSON(mutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    {Resource: resource.MustStructJSON(labelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName):        {Resource: resource.MustStructJSON(validatingPolicyJson), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, extNsName):       {Resource: resource.MustStructJSON(extMutatingPolicyJson), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(extLabelsMutatingPolicyJson), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, extNsName):     {Resource: resource.MustStructJSON(extValidatingPolicyJson), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName):                {Resource: resource.MustStructJSON(roleWNAttachmentJson), Ready: 1},
							service.GetRoleECRProxyAttachmentKey(zoneName):          {Resource: resource.MustStructJSON(roleECRProxyAttachmentJson), Ready: 1},
							service.GetRoleECRROAttachmentKey(zoneName):             {Resource: resource.MustStructJSON(roleECRROAttachment), Ready: 1},
							service.GetRoleSSMAttachmentKey(zoneName):               {Resource: resource.MustStructJSON(roleSSMAttachment), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, nsName):            {Resource: resource.MustStructJSON(rbacRoleReadJson), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, nsName):             {Resource: resource.MustStructJSON(rbacRoleAllJson), Ready: 1},
							service.GetRBMaintainerKey(zoneName, nsName):            {Resource: resource.MustStructJSON(rbMaintainerJson), Ready: 1},
							service.GetRBContributorKey(zoneName, nsName):           {Resource: resource.MustStructJSON(rbContributorJson), Ready: 1},
							service.GetRBObserverKey(zoneName, nsName):              {Resource: resource.MustStructJSON(rbObserverJson), Ready: 1},
							service.GetNodeGroupKey(poolName, nodeGroupHash): {Resource: resource.MustStructJSON(`
{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default-050c2b39"},"spec":{"forProvider":{"capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.large"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"launchTemplate":[{"name":"test-zone-default","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"maxSize":2,"minSize":1}],"subnetIds":["subnet-a-id","subnet-b-id"],"tags":{"entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateConfig":[{"maxUnavailable":1}],"version":"1.34"},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetSidecarKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"Sidecar","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-egress","namespace":"test-app-ns"},"spec":{"egress":[{"hosts":["*/*.svc.cluster.local","istio-system/*","kube-system/kube-dns.kube-system.svc.cluster.local","test-app-ext-ns/*","test-app-ns/*"]}],"outboundTrafficPolicy":{"mode":"ALLOW_ANY"}},"status":{}}
							`)},
							service.GetSidecarKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"Sidecar","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-egress","namespace":"test-app-ext-ns"},"spec":{"egress":[{"hosts":["*/*.svc.cluster.local","istio-system/*","kube-system/kube-dns.kube-system.svc.cluster.local","test-app-ext-ns/*","test-app-ns/*"]}],"outboundTrafficPolicy":{"mode":"ALLOW_ANY"}},"status":{}}
							`)},
						},
					},
					Requirements: requirements,
				},
			},
		},
		"Zone/LabelAnnotationTags: Added Entigo tags to zone labels and annotations": {
			Reason: "Should desire the role bindings and node group once rbac and attachments are ready",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(`
{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"annotations":{"tags.entigo.com/foo":"bar"},"labels":{"tags.entigo.com/bar":"foo"},"name":"test-zone"},"spec":{"clusterPermissions":false,"namespaces":[{"name":"test-app-ns"}],"pools":[{"name":"default","requirements":[{"key":"instance-type","values":["t3.large"]},{"key":"capacity-type","value":"ON_DEMAND"},{"key":"min-size","value":1},{"key":"max-size","value":2},{"key":"security-groups","values":["sg-321"]}]}]}}
`)},
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(nsName):                         withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):        withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                      withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):          withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):    withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):        withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):       withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):     withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):           withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):        withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                  withReadyStatus(targetNetworkPolicyJson),
							service.GetRoleKey(zoneName):                            withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):                withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):          withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):             withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):               withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):            withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):             withReadyStatus(rbacRoleAllJson),
							service.GetRBACRoleReadKey(zoneName, extNsName):         withReadyStatus(extRbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, extNsName):          withReadyStatus(extRbacRoleAllJson),
							service.GetAccessentryKey(zoneName):                     withReadyStatus(accessEntryJson),
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
							service.GetNamespaceKey(nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns"},"spec":{},"status":{}}
							`), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(`
{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-default","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-default","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-default","bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}},{"resourceType":"volume","tags":{"Name":"test-zone-default-root","bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}}],"tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIds":["sg-123","sg-321"]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-zone","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}
							`), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-zone","namespace":"test-app-ext-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}
							`), Ready: 1},
							targetNetworkPolicyKey: {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-ingress-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.1"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}
							`), Ready: 1},
							service.GetRoleKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetAppProjectKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ext-ns","server":"https://kubernetes.default.svc"},{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"roles":[{"description":"Maintainer permissions","groups":["group-maintainer"],"name":"maintainer","policies":["p, proj:test-zone:maintainer, applications, *, test-zone/*, allow","p, proj:test-zone:maintainer, repositories, *, test-zone/*, allow","p, proj:test-zone:maintainer, applicationsets, *, test-zone/*, allow","p, proj:test-zone:maintainer, logs, *, test-zone/*, allow","p, proj:test-zone:maintainer, exec, *, test-zone/*, allow"]},{"description":"Observer permissions","groups":["group-observer"],"name":"observer","policies":["p, proj:test-zone:observer, applications, get, test-zone/*, allow","p, proj:test-zone:observer, applicationsets, get, test-zone/*, allow"]},{"description":"Contributor permissions","groups":["group-contributor"],"name":"contributor","policies":["p, proj:test-zone:contributor, applications, *, test-zone/*, allow","p, proj:test-zone:contributor, repositories, *, test-zone/*, allow","p, proj:test-zone:contributor, applicationsets, *, test-zone/*, allow","p, proj:test-zone:contributor, logs, *, test-zone/*, allow","p, proj:test-zone:contributor, exec, *, test-zone/*, allow"]},{"description":"Use this role for your CI/CD pipelines","groups":["group-maintainer"],"name":"cicd","policies":["p, proj:test-zone:cicd, applications, sync, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, sync, test-zone/*, allow","p, proj:test-zone:cicd, applications, get, test-zone/*, allow","p, proj:test-zone:cicd, applicationsets, get, test-zone/*, allow"]}],"sourceNamespaces":["test-app-ext-ns","test-app-ns"],"sourceRepos":["*"]},"status":{}}
							`), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-add-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) || size(object.spec.nodeSelector) == 0 ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\"}\n  }\n] : []"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
							`), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
							`), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-validate-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE","UPDATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"\nhas(object.spec.nodeSelector) \u0026\u0026\n(\n\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n) || (\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\"\n)","message":"Pod nodeSelector must either use tenancy.entigo.com/zone-pool with a valid value [test-zone-default] or tenancy.entigo.com/zone with value test-zone"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
							`), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-add-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) || size(object.spec.nodeSelector) == 0 ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\"}\n  }\n] : []"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
							`), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
							`), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-validate-nodeselector"},"spec":{"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE","UPDATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"\nhas(object.spec.nodeSelector) \u0026\u0026\n(\n\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n) || (\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\"\n)","message":"Pod nodeSelector must either use tenancy.entigo.com/zone-pool with a valid value [test-zone-default] or tenancy.entigo.com/zone with value test-zone"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
							`), Ready: 1},
							service.GetRoleWNAttachmentKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-wn"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetRoleECRProxyAttachmentKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-proxy"},"spec":{"forProvider":{"policyArnRef":{"name":"ecr-proxy"},"roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetRoleECRROAttachmentKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-ro"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetRoleSSMAttachmentKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ssm"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-read","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}
							`), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-all","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}
							`), Ready: 1},
							service.GetRBACRoleReadKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-read","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}
							`), Ready: 1},
							service.GetRBACRoleAllKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-all","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}]}
							`), Ready: 1},
							service.GetAccessentryKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"AccessEntry","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"clusterNameRef":{"name":"test-cluster"},"principalArnFromRoleRef":{"name":"test-zone"},"region":null,"tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar"},"type":"EC2_LINUX"},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetRBMaintainerKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-maintainer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}
							`)},
							service.GetRBContributorKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-contributor","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}
							`)},
							service.GetRBObserverKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-observer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}
							`)},
							service.GetRBMaintainerKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-maintainer","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}
							`)},
							service.GetRBContributorKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-contributor","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}
							`)},
							service.GetRBObserverKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-observer","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}
							`)},
							service.GetNodeGroupKey(poolName, nodeGroupHash): {Resource: resource.MustStructJSON(`
{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default-050c2b39"},"spec":{"forProvider":{"capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.large"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"launchTemplate":[{"name":"test-zone-default","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"maxSize":2,"minSize":1}],"subnetIds":["subnet-a-id","subnet-b-id"],"tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateConfig":[{"maxUnavailable":1}],"version":"1.34"},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`)},
						},
					},
					Requirements: requirements,
				},
			},
		},
		"Zone/Infralib: Skip": {
			Reason: "When reconciling for infralib zone, the function should skip processing and return no desired resources.",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{Resource: resource.MustStructJSON(fmt.Sprintf(`
{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"name":"%s"},"spec":{"clusterPermissions":false,"namespaces":[{"name":"test-app-ns"}],"pools":[{"name":"default"}]}}
`, infralibZone))},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
				},
			},
		},
	}

	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases)
}
