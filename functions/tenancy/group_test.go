package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/crossplane/function-sdk-go/response"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/authz"
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
		"apiVersion":"eks.aws.upbound.io/v1beta2","kind":"Cluster",
		"metadata":{"annotations":{"crossplane.io/external-name":"test-cluster"},"name":"test-cluster", "namespace":"aws-provider"},
		"status": {"atProvider": {"version": "1.34"}}
	}`
	requiredSubnetAJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-a", "labels": {"subnet-type": "compute"}, "annotations": {"crossplane.io/external-name": "subnet-a-id"}},
		"status": {"atProvider": {"availabilityZone": "eu-north-1a", "id": "subnet-a-id", "cidrBlock": "10.10.10.1"}}
	}`
	requiredSubnetBJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-b", "labels": {"subnet-type": "compute"}, "annotations": {"crossplane.io/external-name": "subnet-b-id"}},
		"status": {"atProvider": {"availabilityZone": "eu-north-1b", "id": "subnet-b-id", "cidrBlock": "10.10.10.2"}}
	}`
	requiredSubnetCJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-c", "labels": {"subnet-type": "control"}, "annotations": {"crossplane.io/external-name": "subnet-c-id"}},
		"status": {"atProvider": {"availabilityZone": "eu-north-1c", "id": "subnet-c-id", "cidrBlock": "10.10.10.3"}}
	}`
	requiredSubnetDJson = `{
		"apiVersion": "ec2.aws.upbound.io/v1beta1", "kind": "Subnet",
		"metadata": {"name": "subnet-d", "labels": {"subnet-type": "service"}, "annotations": {"crossplane.io/external-name": "subnet-d-id"}},
		"status": {"atProvider": {"availabilityZone": "eu-north-1a", "id": "subnet-d-id", "cidrBlock": "10.10.10.4"}}
	}`
	requiredServiceIngressClassJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"IngressClass",
        "metadata":{"name":"service"},
        "spec":{"controller":"ingress.k8s.aws/alb","parameters":{"apiGroup":"elbv2.k8s.aws","kind":"IngressClassParams","name":"service-params","scope":"Cluster"}}
    }`
	requiredAlbIngressClassJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"IngressClass",
        "metadata":{"name":"alb"},
        "spec":{"controller":"ingress.k8s.aws/alb","parameters":{"apiGroup":"elbv2.k8s.aws","kind":"IngressClassParams","name":"alb-params","scope":"Cluster"}}
    }`
	requiredExtIngressClassJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"IngressClass",
        "metadata":{"name":"external"},
        "spec":{"controller":"ingress.k8s.aws/alb","parameters":{"apiGroup":"elbv2.k8s.aws","kind":"IngressClassParams","name":"external-params","scope":"Cluster"}}
    }`
	requiredNginxIngressClassJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"IngressClass",
        "metadata":{"name":"nginx"},
        "spec":{"controller":"k8s.io/ingress-nginx"}
    }`
	requiredServiceIngressClassParamsJson = `{
        "apiVersion":"elbv2.k8s.aws/v1beta1","kind":"IngressClassParams",
        "metadata":{"name":"service-params"},
        "spec":{"scheme":"internal","subnets":{"ids":["subnet-d-id"]}}
    }`
	requiredAlbIngressClassParamsJson = `{
        "apiVersion":"elbv2.k8s.aws/v1beta1","kind":"IngressClassParams",
        "metadata":{"name":"alb-params"},
        "spec":{"scheme":"internal","subnets":{"ids":["subnet-c-id"]}}
    }`
	requiredExtIngressClassParamsJson = `{
        "apiVersion":"elbv2.k8s.aws/v1beta1","kind":"IngressClassParams",
        "metadata":{"name":"external-params"},
        "spec":{"scheme":"internet-facing","subnets":{"tags":{"subnet-type":["public"]}}}
    }`
	requiredIngressJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"Ingress",
        "metadata":{"name":"test-ingress","namespace":"test-app-ns"},
        "spec":{"ingressClassName":"service","rules":[{"host":"example.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"test-service","port":{"number":8080}}}}]}}]}
    }`
	requiredAlbIngressJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"Ingress",
        "metadata":{
            "name":"test-alb-ingress","namespace":"test-app-ns",
            "annotations":{
                "alb.ingress.kubernetes.io/actions.deny-access":"{\"type\":\"fixed-response\",\"fixedResponseConfig\":{\"contentType\":\"text/plain\",\"statusCode\":\"403\",\"messageBody\":\"Forbidden\"}}",
                "alb.ingress.kubernetes.io/actions.my-service":"{\"type\":\"forward\",\"forwardConfig\":{\"targetGroups\":[{\"serviceName\":\"test-service\",\"servicePort\":\"8080\",\"weight\":100}]}}"
            }
        },
        "spec":{"ingressClassName":"alb"}
    }`
	requiredServiceJson = `{
        "apiVersion": "v1", "kind": "Service",
        "metadata": { "name": "test-service", "namespace": "test-app-ns"},
        "spec": {"selector": {"app": "test-app"},"ports": [{"port": 8080, "targetPort": 8081}]}
    }`
	requiredExtIngressJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"Ingress",
        "metadata":{"name":"test-ext-ingress","namespace":"test-app-ext-ns"},
        "spec":{"ingressClassName":"external","rules":[{"host":"example.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"test-ext-service","port":{"number":443}}}}]}}]}
    }`
	requiredNginxIngressJson = `{
        "apiVersion":"networking.k8s.io/v1","kind":"Ingress",
        "metadata":{"name":"test-nginx-ingress","namespace":"test-app-ns"},
        "spec":{"ingressClassName":"nginx","rules":[{"host":"example.com","http":{"paths":[{"path":"/","pathType":"Prefix","backend":{"service":{"name":"test-service","port":{"number":8080}}}}]}}]}
    }`
	requiredServiceGatewayJson = `{
        "apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway",
        "metadata":{"name":"service","namespace":"aws-alb-biz"},
        "spec":{"gatewayClassName":"alb","infrastructure":{"parametersRef":{"group":"gateway.k8s.aws","kind":"LoadBalancerConfiguration","name":"service-lbconfig"}}}
    }`
	requiredExtGatewayJson = `{
        "apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway",
        "metadata":{"name":"external","namespace":"aws-alb-biz"},
        "spec":{"gatewayClassName":"alb","infrastructure":{"parametersRef":{"group":"gateway.k8s.aws","kind":"LoadBalancerConfiguration","name":"external-lbconfig"}}}
    }`
	requiredIstioGatewayJson = `{
        "apiVersion":"gateway.networking.k8s.io/v1","kind":"Gateway",
        "metadata":{"name":"istio","namespace":"istio-system"},
        "spec":{"gatewayClassName":"istio"}
    }`
	requiredServiceLBConfigJson = `{
        "apiVersion":"gateway.k8s.aws/v1beta1","kind":"LoadBalancerConfiguration",
        "metadata":{"name":"service-lbconfig","namespace":"aws-alb-biz"},
        "spec":{"scheme":"internal","loadBalancerSubnets":[{"identifier":"subnet-d-id"}]}
    }`
	requiredExtLBConfigJson = `{
        "apiVersion":"gateway.k8s.aws/v1beta1","kind":"LoadBalancerConfiguration",
        "metadata":{"name":"external-lbconfig","namespace":"aws-alb-biz"},
        "spec":{"scheme":"internet-facing","loadBalancerSubnets":[{"identifier":"subnet-public-id"}]}
    }`
	requiredHTTPRouteJson = `{
        "apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute",
        "metadata":{"name":"test-route","namespace":"test-app-ns"},
        "spec":{"parentRefs":[{"name":"service","namespace":"aws-alb-biz"}],"rules":[{"backendRefs":[{"name":"test-service","port":8080},{"name":"test-service2","port":9090}]}]}
    }`
	requiredIstioHTTPRouteJson = `{
        "apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute",
        "metadata":{"name":"test-istio-route","namespace":"test-app-ns"},
        "spec":{"parentRefs":[{"name":"istio","namespace":"istio-system"}],"rules":[{"backendRefs":[{"name":"test-service","port":8080}]}]}
    }`
	requiredExtHTTPRouteJson = `{
        "apiVersion":"gateway.networking.k8s.io/v1","kind":"HTTPRoute",
        "metadata":{"name":"test-ext-route","namespace":"test-app-ext-ns"},
        "spec":{"parentRefs":[{"name":"external","namespace":"aws-alb-biz"}],"rules":[{"backendRefs":[{"name":"test-ext-service","port":443}]}]}
    }`
	requiredService2Json = `{
        "apiVersion": "v1", "kind": "Service",
        "metadata": { "name": "test-service2", "namespace": "test-app-ns"},
        "spec": {"selector": {"app": "test-app2"},"ports": [{"port": 9090, "targetPort": 9091}]}
    }`
	requiredExtServiceJson = `{
        "apiVersion": "v1", "kind": "Service",
        "metadata": { "name": "test-ext-service", "namespace": "test-app-ext-ns"},
        "spec": {"selector": {"app": "test-app"},"ports": [{"port": 443, "targetPort": 8443}]}
    }`
	authzZoneJson            = `{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"name":"main"},"spec":{}}`
	authzZoneName            = "main"
	readEverythingRuleJson   = `{"apiGroups":["*"],"resources":["*"],"verbs":["get","list","watch"]}`
	zoneReadonlyRole         = "zone-readonly"
	zoneReadonlyPoliciesJson = `["p, proj:main:zone-readonly, applications, get, main/*, allow","p, proj:main:zone-readonly, applicationsets, get, main/*, allow","p, proj:main:zone-readonly, clusters, get, main/*, allow","p, proj:main:zone-readonly, exec, get, main/*, allow","p, proj:main:zone-readonly, logs, get, main/*, allow","p, proj:main:zone-readonly, repositories, get, main/*, allow"]`

	contractRolesJson = `[` +
		`{"name":"ct-n1","description":"RoleDefinition ct-n1","groups":["ct-n1@jev/jev"],"policies":["p, proj:main:ct-n1, applications, get, main/*, allow"]},` +
		`{"name":"ct-n10","description":"RoleDefinition ct-n10","groups":["ct-n10@jev/jev"],"policies":["p, proj:main:ct-n10, applications, *, main/*, allow","p, proj:main:ct-n10, applicationsets, *, main/*, allow","p, proj:main:ct-n10, clusters, *, main/*, allow","p, proj:main:ct-n10, exec, *, main/*, allow","p, proj:main:ct-n10, logs, *, main/*, allow","p, proj:main:ct-n10, repositories, *, main/*, allow"]},` +
		`{"name":"ct-n2","description":"RoleDefinition ct-n2","groups":["ct-n2@jev/jev"],"policies":["p, proj:main:ct-n2, applications, update, main/*, allow"]},` +
		`{"name":"ct-n3","description":"RoleDefinition ct-n3","groups":["ct-n3@jev/jev"],"policies":["p, proj:main:ct-n3, applicationsets, sync, main/*, allow"]},` +
		`{"name":"ct-n5","description":"RoleDefinition ct-n5","groups":["ct-n5@jev/jev"],"policies":["p, proj:main:ct-n5, logs, get, main/*, allow"]},` +
		`{"name":"ct-n6","description":"RoleDefinition ct-n6","groups":["ct-n6@jev/jev"],"policies":["p, proj:main:ct-n6, exec, create, main/*, allow"]},` +
		`{"name":"ct-n7","description":"RoleDefinition ct-n7","groups":["ct-n7@jev/jev"],"policies":["p, proj:main:ct-n7, repositories, *, main/*, allow"]},` +
		`{"name":"ct-n9","description":"RoleDefinition ct-n9","groups":["ct-n9@jev/jev"],"policies":["p, proj:main:ct-n9, applications, action/*, main/*, allow"]}` +
		`]`

	zoneInputJson                      = `{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"name":"test-zone"},"spec":{"clusterPermissions":false,"namespaces":[{"name":"test-app-ns"}],"pools":[{"name":"default","requirements":[{"key":"ami-type","value":"AL2_x86_64"},{"key":"instance-type","values":["t3.large"]},{"key":"capacity-type","value":"ON_DEMAND"},{"key":"min-size","value":1},{"key":"max-size","value":2},{"key":"security-groups","values":["sg-321"]}]}]}}`
	rbacRoleReadJson                   = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-read","namespace":"test-app-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}`
	extRbacRoleReadJson                = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-read","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":["*"],"resources":["*"],"verbs":["get","watch","list"]}]}`
	appProjectJson                     = `{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ext-ns","server":"https://kubernetes.default.svc"},{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"sourceNamespaces":["test-app-ext-ns","test-app-ns"],"sourceRepos":["*"]},"status":{}}`
	networkPolicyJson                  = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-zone","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`
	extNetworkPolicyJson               = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-zone","namespace":"test-app-ext-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"tenancy.entigo.com/zone":"test-zone"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}`
	targetNetworkPolicyJson            = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-ingress-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.4"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}`
	targetAlbNetworkPolicyJson         = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-alb-ingress-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.3"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}`
	targetRouteNetworkPolicyJson       = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-route-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.4"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}`
	targetRoute2NetworkPolicyJson      = `{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-route-test-service2-9091","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.4"}}],"ports":[{"port":9091,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app2"}},"policyTypes":["Ingress"]}}`
	rbMaintainerJson                   = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-maintainer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	extRBMaintainerJson                = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-maintainer","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	roleJson                           = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleECRProxyAttachmentJson         = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-proxy"},"spec":{"forProvider":{"policyArnRef":{"name":"ecr-proxy"},"roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleSSMAttachment                  = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ssm"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	namespaceRoleRulesJson             = `[{"apiGroups":[""],"resources":["pods","pods/log","services","endpoints","configmaps","secrets","persistentvolumeclaims","serviceaccounts","events"],"verbs":["*"]},{"apiGroups":[""],"resources":["pods/exec","pods/portforward","pods/eviction"],"verbs":["create"]},{"apiGroups":["apps","batch","autoscaling","policy"],"resources":["*"],"verbs":["*"]},{"apiGroups":["networking.k8s.io"],"resources":["ingresses"],"verbs":["*"]},{"apiGroups":["gateway.networking.k8s.io"],"resources":["httproutes","grpcroutes","tcproutes","tlsroutes"],"verbs":["*"]},{"apiGroups":["networking.istio.io"],"resources":["virtualservices","destinationrules","serviceentries"],"verbs":["*"]},{"apiGroups":["external-secrets.io"],"resources":["externalsecrets"],"verbs":["*"]},{"apiGroups":["artifact.entigo.com","database.entigo.com","mq.entigo.com","networking.entigo.com","queue.entigo.com","storage.entigo.com","workload.entigo.com"],"resources":["*"],"verbs":["*"]}]`
	rbacRoleAllJson                    = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-all","namespace":"test-app-ns"},"rules":[{"apiGroups":[""],"resources":["pods","pods/log","services","endpoints","configmaps","secrets","persistentvolumeclaims","serviceaccounts","events"],"verbs":["*"]},{"apiGroups":[""],"resources":["pods/exec","pods/portforward","pods/eviction"],"verbs":["create"]},{"apiGroups":["apps","batch","autoscaling","policy"],"resources":["*"],"verbs":["*"]},{"apiGroups":["networking.k8s.io"],"resources":["ingresses"],"verbs":["*"]},{"apiGroups":["gateway.networking.k8s.io"],"resources":["httproutes","grpcroutes","tcproutes","tlsroutes"],"verbs":["*"]},{"apiGroups":["networking.istio.io"],"resources":["virtualservices","destinationrules","serviceentries"],"verbs":["*"]},{"apiGroups":["external-secrets.io"],"resources":["externalsecrets"],"verbs":["*"]},{"apiGroups":["artifact.entigo.com","database.entigo.com","mq.entigo.com","networking.entigo.com","queue.entigo.com","storage.entigo.com","workload.entigo.com"],"resources":["*"],"verbs":["*"]}]}`
	extRbacRoleAllJson                 = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-all","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":[""],"resources":["pods","pods/log","services","endpoints","configmaps","secrets","persistentvolumeclaims","serviceaccounts","events"],"verbs":["*"]},{"apiGroups":[""],"resources":["pods/exec","pods/portforward","pods/eviction"],"verbs":["create"]},{"apiGroups":["apps","batch","autoscaling","policy"],"resources":["*"],"verbs":["*"]},{"apiGroups":["networking.k8s.io"],"resources":["ingresses"],"verbs":["*"]},{"apiGroups":["gateway.networking.k8s.io"],"resources":["httproutes","grpcroutes","tcproutes","tlsroutes"],"verbs":["*"]},{"apiGroups":["networking.istio.io"],"resources":["virtualservices","destinationrules","serviceentries"],"verbs":["*"]},{"apiGroups":["external-secrets.io"],"resources":["externalsecrets"],"verbs":["*"]},{"apiGroups":["artifact.entigo.com","database.entigo.com","mq.entigo.com","networking.entigo.com","queue.entigo.com","storage.entigo.com","workload.entigo.com"],"resources":["*"],"verbs":["*"]}]}`
	rbContributorJson                  = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-contributor","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	extRBContributorJson               = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-contributor","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	rbacRoleNamespaceJson              = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-namespace","namespace":"test-app-ns"},"rules":[{"apiGroups":[""],"resources":["namespaces"],"resourceNames":["test-app-ns"],"verbs":["update","patch","delete"]}]}`
	extRbacRoleNamespaceJson           = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-namespace","namespace":"test-app-ext-ns"},"rules":[{"apiGroups":[""],"resources":["namespaces"],"resourceNames":["test-app-ext-ns"],"verbs":["update","patch","delete"]}]}`
	rbMaintainerNamespaceJson          = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-maintainer-namespace","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-namespace"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	rbMaintainerCrossplaneViewJson     = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-maintainer-crossplane-view","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"crossplane-view"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	rbContributorCrossplaneViewJson    = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-contributor-crossplane-view","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"crossplane-view"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	extRBMaintainerCrossplaneViewJson  = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-maintainer-crossplane-view","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"crossplane-view"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	extRBContributorCrossplaneViewJson = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-contributor-crossplane-view","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"crossplane-view"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	extRBMaintainerNamespaceJson       = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-maintainer-namespace","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-namespace"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	launchTemplateJson                 = `{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-default","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-default","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-default","entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}},{"resourceType":"volume","tags":{"Name":"test-zone-default-root","entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}}],"tags":{"entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIds":["sg-123","sg-321"]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	roleWNAttachmentJson               = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-wn"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	mutatingPolicyJson                 = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-add-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\"}\n  }\n] : [\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	labelsMutatingPolicyJson           = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	rbObserverJson                     = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-observer","namespace":"test-app-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}`
	extRBObserverJson                  = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-observer","namespace":"test-app-ext-ns"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-app-ext-ns-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}`
	validatingPolicyJson               = `{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-validate-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"has(object.spec.nodeSelector) \u0026\u0026\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\" \u0026\u0026 (\n  !(\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector) ||\n  object.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n)","message":"Pod nodeSelector must use tenancy.entigo.com/zone with value test-zone, and tenancy.entigo.com/zone-pool, if set, with a valid value [test-zone-default]"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	roleECRROAttachment                = `{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"RolePolicyAttachment","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-ecr-ro"},"spec":{"forProvider":{"policyArn":"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly","roleRef":{"name":"test-zone"}},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	namespaceJson                      = `{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns"},"spec":{},"status":{}}`
	extNamespaceJson                   = `{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/pool":"default"},"name":"test-app-ext-ns"},"spec":{},"status":{}}`
	appsNamespaceJson                  = `{"apiVersion":"v1","kind":"Namespace","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"pod-security.kubernetes.io/enforce":"baseline","pod-security.kubernetes.io/warn":"baseline","tenancy.entigo.com/only-argocd-apps":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-apps"},"spec":{},"status":{}}`
	appsRbacRoleAllJson                = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-apps-all","namespace":"test-zone-apps"},"rules":[{"apiGroups":["argoproj.io"],"resources":["applications", "applicationsets"],"verbs":["*"]}]}`
	appsRbacRoleReadJson               = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-apps-read","namespace":"test-zone-apps"},"rules":[{"apiGroups":["argoproj.io"],"resources":["applications", "applicationsets"],"verbs":["get","watch","list"]}]}`
	appsRbMaintainerJson               = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-apps-maintainer","namespace":"test-zone-apps"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-zone-apps-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-maintainer"}]}`
	appsRbContributorJson              = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-apps-contributor","namespace":"test-zone-apps"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-zone-apps-all"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
	appsRbObserverJson                 = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"RoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-apps-observer","namespace":"test-zone-apps"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"Role","name":"test-zone-apps-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-observer"}]}`
	nodegroupJson                      = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default-15b0c82e"},"spec":{"forProvider":{"amiType":"AL2_x86_64","capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.large"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"launchTemplate":[{"name":"test-zone-default","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"maxSize":2,"minSize":1}],"subnetIds":["subnet-a-id","subnet-b-id"],"tags":{"entigo:zone":"test-zone","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateConfig":[{"maxUnavailable":1}],"version":"1.34"},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	accessEntryJson                    = `{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"AccessEntry","metadata":{"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"clusterNameRef":{"name":"test-cluster"},"principalArnFromRoleRef":{"name":"test-zone"},"region":null,"tags":{"entigo:zone":"test-zone"},"type":"EC2_LINUX"},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}`
	extMutatingPolicyJson              = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-add-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\", \"tenancy.entigo.com/zone-pool\": \"test-zone-default\"}\n  }\n] : [\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] + (\n  \"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector ? [] :\n  [\n    JSONPatch{\n      op: \"add\",\n      path: \"/spec/nodeSelector/tenancy.entigo.com~1zone-pool\",\n      value: \"test-zone-default\"\n    }\n  ]\n)"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	extLabelsMutatingPolicyJson        = `{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	extValidatingPolicyJson            = `{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-validate-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"has(object.spec.nodeSelector) \u0026\u0026\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\" \u0026\u0026 (\n  !(\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector) ||\n  object.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n)","message":"Pod nodeSelector must use tenancy.entigo.com/zone with value test-zone, and tenancy.entigo.com/zone-pool, if set, with a valid value [test-zone-default]"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}`
	crbContributorJson                 = `{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRoleBinding","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-contributor"},"roleRef":{"apiGroup":"rbac.authorization.k8s.io","kind":"ClusterRole","name":"zone-tenancy-c-read"},"subjects":[{"apiGroup":"rbac.authorization.k8s.io","kind":"Group","name":"group-contributor"}]}`
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
			"roleRef": service.RoleContributor,
			"groups":  []interface{}{"group-contributor"},
		},
		map[string]interface{}{
			"roleRef": service.RoleMaintainer,
			"groups":  []interface{}{"group-maintainer"},
		},
		map[string]interface{}{
			"roleRef": service.RoleObserver,
			"groups":  []interface{}{"group-observer"},
		},
	}
	var namespaceRoleRules []interface{}
	if err := json.Unmarshal([]byte(namespaceRoleRulesJson), &namespaceRoleRules); err != nil {
		t.Fatal(err)
	}
	environmentData := map[string]interface{}{
		"namespaceRole":     map[string]interface{}{"rules": namespaceRoleRules},
		"appProject":        appProject,
		"roleMapping":       roleMappings,
		"awsProvider":       "aws-provider",
		"argoCDNamespace":   "argocd",
		"cluster":           "test-cluster",
		"dataKMSAlias":      "data",
		"securityGroup":     "eks-node-sg",
		"computeSubnetType": "compute",
		"vpc":               "test-vpc",
		"podSecurity":       "baseline",
	}
	optEnvironmentData := map[string]interface{}{
		"tags": map[string]interface{}{
			"env": "test-environment",
		},
		"granularEgress":                 true,
		"granularEgressExclude":          []interface{}{"test-zone"},
		"granularNamespaceNetworkPolicy": true,
		"createAppsNamespace":            true,
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
	appsNsName := "test-zone-apps"
	poolName := "default"
	targetNetworkPolicyKey := service.GetTargetNetworkPolicyKey(nsName, "test-ingress", "test-service", intstr.FromInt32(8081))
	targetAlbNetworkPolicyKey := service.GetTargetNetworkPolicyKey(nsName, "test-alb-ingress", "test-service", intstr.FromInt32(8081))
	targetRouteNetworkPolicyKey := service.GetTargetNetworkPolicyKey(nsName, "test-route", "test-service", intstr.FromInt32(8081))
	targetRoute2NetworkPolicyKey := service.GetTargetNetworkPolicyKey(nsName, "test-route", "test-service2", intstr.FromInt32(9091))
	nodeGroupHash := service.GetInstanceTypesHash([]string{"t3.large"}, "ON_DEMAND")

	requiredResources := map[string]*fnv1.Resources{
		base.EnvironmentKey:      test.EnvironmentConfigResourceWithData(environmentName, environmentData),
		service.NamespaceKey:     {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(extNamespaceJson)}}},
		service.VPCKey:           {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredVPCjson)}}},
		service.KMSDataAliasKey:  {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredKMSAliasJson)}}},
		service.SecurityGroupKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSecurityGroupJson)}}},
		service.ClusterKey:       {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredClusterJson)}}},
		service.SubnetsKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSubnetAJson)},
			{Resource: resource.MustStructJSON(requiredSubnetBJson)}, {Resource: resource.MustStructJSON(requiredSubnetCJson)},
			{Resource: resource.MustStructJSON(requiredSubnetDJson)}}},
		service.IngressClassKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredServiceIngressClassJson)},
			{Resource: resource.MustStructJSON(requiredAlbIngressClassJson)}, {Resource: resource.MustStructJSON(requiredExtIngressClassJson)},
			{Resource: resource.MustStructJSON(requiredNginxIngressClassJson)}}},
		service.IngressClassParamsKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredServiceIngressClassParamsJson)},
			{Resource: resource.MustStructJSON(requiredAlbIngressClassParamsJson)}, {Resource: resource.MustStructJSON(requiredExtIngressClassParamsJson)}}},
		nsName + service.IngressKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredIngressJson)},
			{Resource: resource.MustStructJSON(requiredAlbIngressJson)}, {Resource: resource.MustStructJSON(requiredNginxIngressJson)}}},
		nsName + service.ServiceKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredServiceJson)},
			{Resource: resource.MustStructJSON(requiredService2Json)}}},
		extNsName + service.IngressKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredExtIngressJson)}}},
		extNsName + service.ServiceKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredExtServiceJson)}}},
		service.GatewayKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredServiceGatewayJson)},
			{Resource: resource.MustStructJSON(requiredExtGatewayJson)}, {Resource: resource.MustStructJSON(requiredIstioGatewayJson)}}},
		service.LoadBalancerConfigurationKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredServiceLBConfigJson)},
			{Resource: resource.MustStructJSON(requiredExtLBConfigJson)}}},
		nsName + service.HTTPRouteKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredHTTPRouteJson)},
			{Resource: resource.MustStructJSON(requiredIstioHTTPRouteJson)}}},
		extNsName + service.HTTPRouteKey: {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredExtHTTPRouteJson)}}},
	}
	tagsRequiredResources := make(map[string]*fnv1.Resources)
	maps.Copy(tagsRequiredResources, requiredResources)
	tagsRequiredResources[base.EnvironmentKey] = test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData)

	requirements := &fnv1.Requirements{
		Resources: map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey:                  base.RequiredEnvironmentConfig(environmentName),
			service.NamespaceKey:                 {Kind: "Namespace", ApiVersion: "v1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{base.TenancyZoneLabel: zoneName}}}},
			service.VPCKey:                       {Kind: "VPC", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-vpc"}},
			service.KMSDataAliasKey:              {Kind: "Alias", ApiVersion: "kms.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "data"}},
			service.SecurityGroupKey:             {Kind: "SecurityGroup", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "eks-node-sg"}},
			service.ClusterKey:                   {Kind: "Cluster", ApiVersion: "eks.aws.upbound.io/v1beta2", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-cluster"}},
			service.SubnetsKey:                   {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			service.IngressClassKey:              {Kind: "IngressClass", ApiVersion: "networking.k8s.io/v1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			service.IngressClassParamsKey:        {Kind: "IngressClassParams", ApiVersion: "elbv2.k8s.aws/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			nsName + service.IngressKey:          {Kind: "Ingress", ApiVersion: "networking.k8s.io/v1", Namespace: &nsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			nsName + service.ServiceKey:          {Kind: "Service", ApiVersion: "v1", Namespace: &nsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			extNsName + service.IngressKey:       {Kind: "Ingress", ApiVersion: "networking.k8s.io/v1", Namespace: &extNsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			extNsName + service.ServiceKey:       {Kind: "Service", ApiVersion: "v1", Namespace: &extNsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			service.GatewayKey:                   {Kind: "Gateway", ApiVersion: "gateway.networking.k8s.io/v1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			service.LoadBalancerConfigurationKey: {Kind: "LoadBalancerConfiguration", ApiVersion: "gateway.k8s.aws/v1beta1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			nsName + service.HTTPRouteKey:        {Kind: "HTTPRoute", ApiVersion: "gateway.networking.k8s.io/v1", Namespace: &nsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
			extNsName + service.HTTPRouteKey:     {Kind: "HTTPRoute", ApiVersion: "gateway.networking.k8s.io/v1", Namespace: &extNsName, Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}},
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
							targetAlbNetworkPolicyKey:                               {Resource: resource.MustStructJSON(targetAlbNetworkPolicyJson)},
							targetRouteNetworkPolicyKey:                             {Resource: resource.MustStructJSON(targetRouteNetworkPolicyJson)},
							targetRoute2NetworkPolicyKey:                            {Resource: resource.MustStructJSON(targetRoute2NetworkPolicyJson)},
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
							targetAlbNetworkPolicyKey:                               withReadyStatus(targetAlbNetworkPolicyJson),
							targetRouteNetworkPolicyKey:                             withReadyStatus(targetRouteNetworkPolicyJson),
							targetRoute2NetworkPolicyKey:                            withReadyStatus(targetRoute2NetworkPolicyJson),
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
							targetAlbNetworkPolicyKey:                               {Resource: resource.MustStructJSON(targetAlbNetworkPolicyJson), Ready: 1},
							targetRouteNetworkPolicyKey:                             {Resource: resource.MustStructJSON(targetRouteNetworkPolicyJson), Ready: 1},
							targetRoute2NetworkPolicyKey:                            {Resource: resource.MustStructJSON(targetRoute2NetworkPolicyJson), Ready: 1},
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
							service.GetNamespaceKey(nsName):                                     withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):                    withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                                  withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):                      withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):                withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):                    withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):                   withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName):             withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):                 withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):                       withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):                    withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                              withReadyStatus(targetNetworkPolicyJson),
							targetAlbNetworkPolicyKey:                                           withReadyStatus(targetAlbNetworkPolicyJson),
							targetRouteNetworkPolicyKey:                                         withReadyStatus(targetRouteNetworkPolicyJson),
							targetRoute2NetworkPolicyKey:                                        withReadyStatus(targetRoute2NetworkPolicyJson),
							service.GetRoleKey(zoneName):                                        withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):                            withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):                      withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):                         withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):                           withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):                        withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):                         withReadyStatus(rbacRoleAllJson),
							service.GetRBACRoleReadKey(zoneName, extNsName):                     withReadyStatus(extRbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, extNsName):                      withReadyStatus(extRbacRoleAllJson),
							service.GetRBACRoleNamespaceKey(zoneName, nsName):                   withReadyStatus(rbacRoleNamespaceJson),
							service.GetRBACRoleNamespaceKey(zoneName, extNsName):                withReadyStatus(extRbacRoleNamespaceJson),
							service.GetAccessentryKey(zoneName):                                 withReadyStatus(accessEntryJson),
							service.GetClusterRoleBindingKey(zoneName, service.RoleContributor): withReadyStatus(crbContributorJson),
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
							targetAlbNetworkPolicyKey:                               {Resource: resource.MustStructJSON(targetAlbNetworkPolicyJson), Ready: 1},
							targetRouteNetworkPolicyKey:                             {Resource: resource.MustStructJSON(targetRouteNetworkPolicyJson), Ready: 1},
							targetRoute2NetworkPolicyKey:                            {Resource: resource.MustStructJSON(targetRoute2NetworkPolicyJson), Ready: 1},
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
							service.GetAccessentryKey(zoneName):                     {Resource: resource.MustStructJSON(accessEntryJson), Ready: 1},
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
							service.GetNamespaceKey(nsName):                                     withReadyStatus(namespaceJson),
							service.GetNamespaceKey(appsNsName):                                 withReadyStatus(appsNamespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):                    withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                                  withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):                      withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):                withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):                    withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):                   withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName):             withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):                 withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):                       withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):                    withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                              withReadyStatus(targetNetworkPolicyJson),
							targetAlbNetworkPolicyKey:                                           withReadyStatus(targetAlbNetworkPolicyJson),
							targetRouteNetworkPolicyKey:                                         withReadyStatus(targetRouteNetworkPolicyJson),
							targetRoute2NetworkPolicyKey:                                        withReadyStatus(targetRoute2NetworkPolicyJson),
							service.GetRoleKey(zoneName):                                        withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):                            withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):                      withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):                         withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):                           withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):                        withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):                         withReadyStatus(rbacRoleAllJson),
							service.GetRBACRoleReadKey(zoneName, appsNsName):                    withReadyStatus(appsRbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, appsNsName):                     withReadyStatus(appsRbacRoleAllJson),
							service.GetRBKey(zoneName, nsName, service.RoleMaintainer):          withReadyStatus(rbMaintainerJson),
							service.GetRBKey(zoneName, nsName, service.RoleContributor):         withReadyStatus(rbContributorJson),
							service.GetRBKey(zoneName, nsName, service.RoleObserver):            withReadyStatus(rbObserverJson),
							service.GetRBKey(zoneName, appsNsName, service.RoleMaintainer):      withReadyStatus(appsRbMaintainerJson),
							service.GetRBKey(zoneName, appsNsName, service.RoleContributor):     withReadyStatus(appsRbContributorJson),
							service.GetRBKey(zoneName, appsNsName, service.RoleObserver):        withReadyStatus(appsRbObserverJson),
							service.GetNodeGroupKey(poolName, nodeGroupHash):                    withReadyStatus(nodegroupJson),
							service.GetClusterRoleBindingKey(zoneName, service.RoleContributor): withReadyStatus(crbContributorJson),
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
							service.GetNamespaceKey(appsNsName): {Resource: resource.MustStructJSON(appsNamespaceJson), Ready: 1},
							service.GetLaunchTemplateKey(zoneName, poolName): {Resource: resource.MustStructJSON(`
{"apiVersion":"ec2.aws.upbound.io/v1beta1","kind":"LaunchTemplate","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default"},"spec":{"deletionPolicy":"Delete","forProvider":{"description":"test-zone-default","disableApiStop":false,"disableApiTermination":false,"metadataOptions":[{"httpEndpoint":"enabled","httpProtocolIpv6":"","httpPutResponseHopLimit":1,"httpTokens":"required","instanceMetadataTags":""}],"name":"test-zone-default","region":"eu-north-1","tagSpecifications":[{"resourceType":"instance","tags":{"Name":"test-zone-default","entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}},{"resourceType":"volume","tags":{"Name":"test-zone-default-root","entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"}}],"tags":{"entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateDefaultVersion":true,"userData":"","vpcSecurityGroupIds":["sg-123","sg-321"]},"initProvider":{"blockDeviceMappings":[{"deviceName":"/dev/xvda","ebs":[{"deleteOnTermination":"true","encrypted":"true","iops":0,"kmsKeyId":"arn:aws:kms:eu-north-1:111111111111:alias/data","snapshotId":"","throughput":0,"volumeInitializationRate":0,"volumeSize":50,"volumeType":"gp3"}],"noDevice":"","virtualName":""}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ns-zone","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"test-app-ns"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}
`), Ready: 1},
							service.GetNetworkPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-app-ext-ns-zone","namespace":"test-app-ext-ns"},"spec":{"ingress":[{"from":[{"namespaceSelector":{"matchLabels":{"kubernetes.io/metadata.name":"test-app-ext-ns"}}}]}],"podSelector":{},"policyTypes":["Ingress"]}}
`), Ready: 1},
							targetNetworkPolicyKey:       {Resource: resource.MustStructJSON(targetNetworkPolicyJson), Ready: 1},
							targetAlbNetworkPolicyKey:    {Resource: resource.MustStructJSON(targetAlbNetworkPolicyJson), Ready: 1},
							targetRouteNetworkPolicyKey:  {Resource: resource.MustStructJSON(targetRouteNetworkPolicyJson), Ready: 1},
							targetRoute2NetworkPolicyKey: {Resource: resource.MustStructJSON(targetRoute2NetworkPolicyJson), Ready: 1},
							service.GetRoleKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
							`), Ready: 1},
							service.GetAppProjectKey(zoneName): {Resource: resource.MustStructJSON(`
{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ext-ns","server":"https://kubernetes.default.svc"},{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"namespaceResourceBlacklist":[{"group":"*.m.upbound.io","kind":"*"}],"namespaceResourceWhitelist":[{"group":"*.entigo.com","kind":"*"}],"sourceNamespaces":["test-app-ext-ns","test-app-ns"],"sourceRepos":["*"]},"status":{}}
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
							service.GetNodeGroupKey(poolName, nodeGroupHash): {Resource: resource.MustStructJSON(`
{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default-15b0c82e"},"spec":{"forProvider":{"amiType":"AL2_x86_64","capacityType":"ON_DEMAND","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.large"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"launchTemplate":[{"name":"test-zone-default","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"maxSize":2,"minSize":1}],"subnetIds":["subnet-a-id","subnet-b-id"],"tags":{"entigo:zone":"test-zone","env":"test-environment","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateConfig":[{"maxUnavailable":1}],"version":"1.34"},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
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
		{"apiVersion":"tenancy.entigo.com/v1alpha1","kind":"Zone","metadata":{"annotations":{"tags.entigo.com/foo":"bar"},"labels":{"tags.entigo.com/bar":"foo"},"name":"test-zone"},"spec":{"clusterPermissions":false,"namespaces":[{"name":"test-app-ns"}],"pools":[{"name":"default","requirements":[{"key":"instance-type","values":["t3.medium"]},{"key":"capacity-type","value":"SPOT"},{"key":"min-size","value":1},{"key":"max-size","value":2},{"key":"security-groups","values":["sg-321"]}]}]}}
		`)},
						Resources: map[string]*fnv1.Resource{
							service.GetNamespaceKey(nsName):                                     withReadyStatus(namespaceJson),
							service.GetLaunchTemplateKey(zoneName, poolName):                    withReadyStatus(launchTemplateJson),
							service.GetAppProjectKey(zoneName):                                  withReadyStatus(appProjectJson),
							service.GetMutatingPolicyKey(zoneName, nsName):                      withReadyStatus(mutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, nsName):                withReadyStatus(labelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, nsName):                    withReadyStatus(validatingPolicyJson),
							service.GetMutatingPolicyKey(zoneName, extNsName):                   withReadyStatus(extMutatingPolicyJson),
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName):             withReadyStatus(extLabelsMutatingPolicyJson),
							service.GetValidatingPolicyKey(zoneName, extNsName):                 withReadyStatus(extValidatingPolicyJson),
							service.GetNetworkPolicyKey(zoneName, nsName):                       withReadyStatus(networkPolicyJson),
							service.GetNetworkPolicyKey(zoneName, extNsName):                    withReadyStatus(extNetworkPolicyJson),
							targetNetworkPolicyKey:                                              withReadyStatus(targetNetworkPolicyJson),
							targetAlbNetworkPolicyKey:                                           withReadyStatus(targetAlbNetworkPolicyJson),
							targetRouteNetworkPolicyKey:                                         withReadyStatus(targetRouteNetworkPolicyJson),
							targetRoute2NetworkPolicyKey:                                        withReadyStatus(targetRoute2NetworkPolicyJson),
							service.GetRoleKey(zoneName):                                        withReadyStatus(roleJson),
							service.GetRoleWNAttachmentKey(zoneName):                            withReadyStatus(roleWNAttachmentJson),
							service.GetRoleECRProxyAttachmentKey(zoneName):                      withReadyStatus(roleECRProxyAttachmentJson),
							service.GetRoleECRROAttachmentKey(zoneName):                         withReadyStatus(roleECRROAttachment),
							service.GetRoleSSMAttachmentKey(zoneName):                           withReadyStatus(roleSSMAttachment),
							service.GetRBACRoleReadKey(zoneName, nsName):                        withReadyStatus(rbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, nsName):                         withReadyStatus(rbacRoleAllJson),
							service.GetRBACRoleReadKey(zoneName, extNsName):                     withReadyStatus(extRbacRoleReadJson),
							service.GetRBACRoleAllKey(zoneName, extNsName):                      withReadyStatus(extRbacRoleAllJson),
							service.GetRBACRoleNamespaceKey(zoneName, nsName):                   withReadyStatus(rbacRoleNamespaceJson),
							service.GetRBACRoleNamespaceKey(zoneName, extNsName):                withReadyStatus(extRbacRoleNamespaceJson),
							service.GetAccessentryKey(zoneName):                                 withReadyStatus(accessEntryJson),
							service.GetClusterRoleBindingKey(zoneName, service.RoleContributor): withReadyStatus(crbContributorJson),
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
		{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-ingress-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.4"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}
									`), Ready: 1},
							targetAlbNetworkPolicyKey: {Resource: resource.MustStructJSON(`
		{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-alb-ingress-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.3"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}
									`), Ready: 1},
							targetRouteNetworkPolicyKey: {Resource: resource.MustStructJSON(`
		{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-route-test-service-8081","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.4"}}],"ports":[{"port":8081,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app"}},"policyTypes":["Ingress"]}}
									`), Ready: 1},
							targetRoute2NetworkPolicyKey: {Resource: resource.MustStructJSON(`
		{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-route-test-service2-9091","namespace":"test-app-ns"},"spec":{"ingress":[{"from":[{"ipBlock":{"cidr":"10.10.10.4"}}],"ports":[{"port":9091,"protocol":"TCP"}]}],"podSelector":{"matchLabels":{"app":"test-app2"}},"policyTypes":["Ingress"]}}
									`), Ready: 1},
							service.GetRoleKey(zoneName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"iam.aws.upbound.io/v1beta1","kind":"Role","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"assumeRolePolicy":"{\n  \"Version\": \"2012-10-17\",\n  \"Statement\": [\n    {\n      \"Effect\": \"Allow\",\n      \"Principal\": {\n        \"Service\": \"ec2.amazonaws.com\"\n      },\n      \"Action\": \"sts:AssumeRole\"\n    }\n  ]\n}","tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone"}},"initProvider":{},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
									`), Ready: 1},
							service.GetAppProjectKey(zoneName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment","destinations":[{"namespace":"test-app-ext-ns","server":"https://kubernetes.default.svc"},{"namespace":"test-app-ns","server":"https://kubernetes.default.svc"}],"sourceNamespaces":["test-app-ext-ns","test-app-ns"],"sourceRepos":["*"]},"status":{}}
									`), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-add-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\"}\n  }\n] : [\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
									`), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
									`), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, nsName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ns-validate-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"has(object.spec.nodeSelector) \u0026\u0026\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\" \u0026\u0026 (\n  !(\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector) ||\n  object.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n)","message":"Pod nodeSelector must use tenancy.entigo.com/zone with value test-zone, and tenancy.entigo.com/zone-pool, if set, with a valid value [test-zone-default]"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
									`), Ready: 1},
							service.GetMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-add-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"mutations":[{"jsonPatch":{"expression":"!has(object.spec.nodeSelector) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector\",\n    value: {\"tenancy.entigo.com/zone\": \"test-zone\", \"tenancy.entigo.com/zone-pool\": \"test-zone-default\"}\n  }\n] : [\n  JSONPatch{\n    op: \"add\",\n    path: \"/spec/nodeSelector/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] + (\n  \"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector ? [] :\n  [\n    JSONPatch{\n      op: \"add\",\n      path: \"/spec/nodeSelector/tenancy.entigo.com~1zone-pool\",\n      value: \"test-zone-default\"\n    }\n  ]\n)"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
									`), Ready: 1},
							service.GetLabelsMutatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"policies.kyverno.io/v1","kind":"MutatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"reports.kyverno.io/disabled":"true","tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-labels"},"spec":{"evaluation":{"admission":{"enabled":true},"mutateExisting":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["services"]},{"apiGroups":["networking.k8s.io"],"apiVersions":["v1"],"operations":["CREATE"],"resources":["ingresses"]}]},"mutations":[{"jsonPatch":{"expression":"has(object.metadata.labels) ?\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels/tenancy.entigo.com~1zone\",\n    value: \"test-zone\"\n  }\n] :\n[\n  JSONPatch{\n    op: \"add\",\n    path: \"/metadata/labels\",\n    value: {\n      \"tenancy.entigo.com/zone\": \"test-zone\"\n    }\n  }\n]"},"patchType":"JSONPatch"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
									`), Ready: 1},
							service.GetValidatingPolicyKey(zoneName, extNsName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"policies.kyverno.io/v1","kind":"ValidatingPolicy","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-test-app-ext-ns-validate-nodeselector"},"spec":{"autogen":{"podControllers":{"controllers":[]}},"evaluation":{"admission":{"enabled":true},"background":{"enabled":false}},"matchConditions":[{"expression":"object.metadata.namespace == \"test-app-ext-ns\"","name":"namespace-filter"}],"matchConstraints":{"namespaceSelector":{"matchExpressions":[{"key":"tenancy.entigo.com/zone","operator":"Exists"}]},"resourceRules":[{"apiGroups":[""],"apiVersions":["v1"],"operations":["CREATE"],"resources":["pods"]}]},"validationActions":["Deny"],"validations":[{"expression":"has(object.spec.nodeSelector) \u0026\u0026\n\"tenancy.entigo.com/zone\" in object.spec.nodeSelector \u0026\u0026\nobject.spec.nodeSelector[\"tenancy.entigo.com/zone\"] == \"test-zone\" \u0026\u0026 (\n  !(\"tenancy.entigo.com/zone-pool\" in object.spec.nodeSelector) ||\n  object.spec.nodeSelector[\"tenancy.entigo.com/zone-pool\"] in [\"test-zone-default\"]\n)","message":"Pod nodeSelector must use tenancy.entigo.com/zone with value test-zone, and tenancy.entigo.com/zone-pool, if set, with a valid value [test-zone-default]"}]},"status":{"autogen":{},"conditionStatus":{"message":""},"generated":false}}
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
							service.GetAccessentryKey(zoneName): {Resource: resource.MustStructJSON(`
		{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"AccessEntry","metadata":{"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone"},"spec":{"forProvider":{"clusterNameRef":{"name":"test-cluster"},"principalArnFromRoleRef":{"name":"test-zone"},"region":null,"tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar"},"type":"EC2_LINUX"},"initProvider":{},"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
									`), Ready: 1},
							service.GetNodeGroupKey(poolName, service.GetInstanceTypesHash([]string{"t3.medium"}, "SPOT")): {Resource: resource.MustStructJSON(`
		{"apiVersion":"eks.aws.upbound.io/v1beta1","kind":"NodeGroup","metadata":{"annotations":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"labels":{"tags.entigo.com/bar":"foo","tenancy.entigo.com/zone":"test-zone"},"name":"test-zone-default-87b3db60"},"spec":{"forProvider":{"capacityType":"SPOT","clusterNameRef":{"name":"test-cluster"},"instanceTypes":["t3.medium"],"labels":{"tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"launchTemplate":[{"name":"test-zone-default","version":"1"}],"nodeRoleArnRef":{"name":"test-zone"},"region":null,"scalingConfig":[{"maxSize":2,"minSize":1}],"subnetIds":["subnet-a-id","subnet-b-id"],"tags":{"bar":"foo","entigo:zone":"test-zone","foo":"bar","tenancy.entigo.com/zone":"test-zone","tenancy.entigo.com/zone-pool":"test-zone-default"},"updateConfig":[{"maxUnavailable":1}],"version":"1.34"},"initProvider":{"scalingConfig":[{"desiredSize":1}]},"managementPolicies":["*"],"providerConfigRef":{"name":"aws-provider"}},"status":{"atProvider":{}}}
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

func roleDefinitionJson(name, rulesJson string) string {
	return fmt.Sprintf(`{"apiVersion":"authz.entigo.com/v1alpha1","kind":"RoleDefinition","metadata":{"name":%q},"spec":{"rules":[%s]}}`, name, rulesJson)
}

func roleGrantJson(name, role, labelsJson, targetsJson string) string {
	return fmt.Sprintf(`{"apiVersion":"authz.entigo.com/v1alpha1","kind":"RoleGrant","metadata":{"name":%q,"labels":{%s}},"spec":{"roleRef":%q,"subjects":[{"kind":"Group","name":"platform"}],"resources":[%s]}}`, name, labelsJson, role, targetsJson)
}

func targetJson(group, kind, name string) string {
	return fmt.Sprintf(`{"group":%q,"kind":%q,"name":%q}`, group, kind, name)
}

func zoneReadonlyRoleJson(groups ...string) string {
	quoted, _ := json.Marshal(groups)
	return fmt.Sprintf(`[{"name":%q,"description":"RoleDefinition %s","groups":%s,"policies":%s}]`,
		zoneReadonlyRole, zoneReadonlyRole, quoted, zoneReadonlyPoliciesJson)
}

func authzAppProjectJson(rolesJson string) string {
	roles := ""
	if rolesJson != "" {
		roles = fmt.Sprintf(`"roles":%s,`, rolesJson)
	}
	return fmt.Sprintf(`{"apiVersion":"argoproj.io/v1alpha1","kind":"AppProject","metadata":{"annotations":{"tenancy.entigo.com/zone":"main"},"labels":{"tenancy.entigo.com/zone":"main"},"name":"main","namespace":"argocd"},"spec":{"clusterResourceBlacklist":[{"group":"*","kind":"*"}],"description":"Security zone for isolated team deployment",%s"sourceRepos":["*"]},"status":{}}`, roles)
}

func authzItems(objects []string) *fnv1.Resources {
	items := make([]*fnv1.Resource, 0, len(objects))
	for _, object := range objects {
		items = append(items, &fnv1.Resource{Resource: resource.MustStructJSON(object)})
	}
	return &fnv1.Resources{Items: items}
}

func authzCase(reason string, definitions, grants []string, rolesJson string) test.Case {
	environment := map[string]interface{}{
		"namespaceRole":     map[string]interface{}{"rules": []interface{}{}},
		"appProject":        map[string]interface{}{"sourceRepos": []interface{}{"*"}},
		"awsProvider":       "aws-provider",
		"argoCDNamespace":   "argocd",
		"cluster":           "test-cluster",
		"securityGroup":     "eks-node-sg",
		"computeSubnetType": "compute",
		"vpc":               "test-vpc",
		"podSecurity":       "baseline",
	}
	everything := &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{}}}
	return test.Case{
		Reason: reason,
		Args: test.Args{Req: &fnv1.RunFunctionRequest{
			Observed: &fnv1.State{Composite: &fnv1.Resource{Resource: resource.MustStructJSON(authzZoneJson)}},
			RequiredResources: map[string]*fnv1.Resources{
				base.EnvironmentKey:                  test.EnvironmentConfigResourceWithData(environmentName, environment),
				service.NamespaceKey:                 {},
				service.VPCKey:                       {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredVPCjson)}}},
				service.SecurityGroupKey:             {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredSecurityGroupJson)}}},
				service.ClusterKey:                   {Items: []*fnv1.Resource{{Resource: resource.MustStructJSON(requiredClusterJson)}}},
				service.SubnetsKey:                   {},
				service.IngressClassKey:              {},
				service.IngressClassParamsKey:        {},
				service.GatewayKey:                   {},
				service.LoadBalancerConfigurationKey: {},
				service.RoleDefinitionKey:            authzItems(definitions),
				service.RoleGrantKey:                 authzItems(grants),
			},
		}},
		Want: test.Want{Rsp: &fnv1.RunFunctionResponse{
			Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
			Desired: &fnv1.State{Resources: map[string]*fnv1.Resource{
				service.GetAppProjectKey(authzZoneName): {Resource: resource.MustStructJSON(authzAppProjectJson(rolesJson))},
				service.GetRoleKey(authzZoneName):       {Resource: resource.MustStructJSON(strings.ReplaceAll(roleJson, "test-zone", authzZoneName))},
			}},
			Requirements: &fnv1.Requirements{Resources: map[string]*fnv1.ResourceSelector{
				base.EnvironmentKey:                  base.RequiredEnvironmentConfig(environmentName),
				service.NamespaceKey:                 {Kind: "Namespace", ApiVersion: "v1", Match: &fnv1.ResourceSelector_MatchLabels{MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{base.TenancyZoneLabel: authzZoneName}}}},
				service.VPCKey:                       {Kind: "VPC", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-vpc"}},
				service.SecurityGroupKey:             {Kind: "SecurityGroup", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: &fnv1.ResourceSelector_MatchName{MatchName: "eks-node-sg"}},
				service.ClusterKey:                   {Kind: "Cluster", ApiVersion: "eks.aws.upbound.io/v1beta2", Match: &fnv1.ResourceSelector_MatchName{MatchName: "test-cluster"}},
				service.SubnetsKey:                   {Kind: "Subnet", ApiVersion: "ec2.aws.upbound.io/v1beta1", Match: everything},
				service.IngressClassKey:              {Kind: "IngressClass", ApiVersion: "networking.k8s.io/v1", Match: everything},
				service.IngressClassParamsKey:        {Kind: "IngressClassParams", ApiVersion: "elbv2.k8s.aws/v1beta1", Match: everything},
				service.GatewayKey:                   {Kind: "Gateway", ApiVersion: "gateway.networking.k8s.io/v1", Match: everything},
				service.LoadBalancerConfigurationKey: {Kind: "LoadBalancerConfiguration", ApiVersion: "gateway.k8s.aws/v1beta1", Match: everything},
				service.RoleDefinitionKey:            {Kind: "RoleDefinition", ApiVersion: authz.APIVersion, Match: everything},
				service.RoleGrantKey:                 {Kind: "RoleGrant", ApiVersion: authz.APIVersion, Match: everything},
			}},
		}},
	}
}

func contractDefinitions() []string {
	rules := []string{
		`{"apiGroups":["argoproj.io"],"resources":["applications"],"verbs":["get","list","watch"]}`,
		`{"apiGroups":["argoproj.io"],"resources":["applications"],"verbs":["patch","update"]}`,
		`{"apiGroups":["argoproj.io"],"resources":["applicationsets"],"verbs":["sync"]}`,
		`{"apiGroups":["argoproj.io"],"resources":["appprojects"],"verbs":["get"]}`,
		`{"apiGroups":["argocd.meta.entigo.com"],"resources":["logs"],"verbs":["get"]}`,
		`{"apiGroups":["argocd.meta.entigo.com"],"resources":["exec"],"verbs":["create"]}`,
		`{"apiGroups":["argocd.meta.entigo.com"],"resources":["repositories"],"verbs":["*"]}`,
		`{"apiGroups":["argocd.meta.entigo.com"],"resources":["certificates"],"verbs":["get"]}`,
		`{"apiGroups":["argoproj.io"],"resources":["applications"],"verbs":["action"]}`,
		`{"apiGroups":["*"],"resources":["*"],"verbs":["*"]}`,
		`{"apiGroups":["apps"],"resources":["deployments"],"verbs":["*"]}`,
		`{"apiGroups":["argoproj.io"],"resources":["applications"],"verbs":["deletecollection"]}`,
		`{"apiGroups":["argocd.meta.entigo.com"],"resources":["repositories"],"verbs":["get"],"scope":"Cluster"}`,
	}
	definitions := make([]string, 0, len(rules))
	for i, rule := range rules {
		definitions = append(definitions, roleDefinitionJson(fmt.Sprintf("ct-n%d", i+1), rule))
	}
	return definitions
}

func contractGrants() []string {
	grants := make([]string, 0, 13)
	for i := 1; i <= 13; i++ {
		role := fmt.Sprintf("ct-n%d", i)
		grants = append(grants, roleGrantJson(role, role, "", ""))
	}
	return grants
}

func TestZoneAppProjectRoles(t *testing.T) {
	zone := targetJson(authz.ZoneGroup, authz.ZoneKind, authzZoneName)
	readonly := []string{roleDefinitionJson(zoneReadonlyRole, readEverythingRuleJson)}
	replicated := fmt.Sprintf(`%q:"jev"`, authz.ReplicatedFromLabel)

	cases := map[string]test.Case{
		"AppProject/Contract: every CT-N rule as its own role": authzCase(
			"Each contract rule renders the Casbin lines of the contract table, rules without lines give no role",
			contractDefinitions(), contractGrants(), contractRolesJson),
		"AppProject/A grant narrowed to this zone names the zone": authzCase(
			"The zone entry is <role>@<org>/<ws>/<zone>, lower case",
			readonly, []string{roleGrantJson("readers", zoneReadonlyRole, "", zone)}, zoneReadonlyRoleJson("zone-readonly@jev/jev/main")),
		"AppProject/A grant narrowed to another zone does not reach this zone": authzCase(
			"A grant narrowed to not-yet gives main no role",
			readonly, []string{roleGrantJson("readers", zoneReadonlyRole, "", targetJson(authz.ZoneGroup, authz.ZoneKind, "not-yet"))}, ""),
		"AppProject/An organization replica uses the organization entry": authzCase(
			"A replica without workspace targets reaches every workspace of the organization",
			readonly, []string{roleGrantJson("org-readers", zoneReadonlyRole, replicated, "")}, zoneReadonlyRoleJson("zone-readonly@jev")),
		"AppProject/A replica narrowed to this workspace uses the workspace entry": authzCase(
			"A replica naming the workspace is granted in that workspace only",
			readonly, []string{roleGrantJson("org-readers", zoneReadonlyRole, replicated, targetJson(authz.WorkspaceGroup, authz.WorkspaceKind, "jev"))},
			zoneReadonlyRoleJson("zone-readonly@jev/jev")),
		"AppProject/Grants of one role share one role": authzCase(
			"Groups of one role are sorted and unique",
			readonly, []string{roleGrantJson("b", zoneReadonlyRole, "", ""), roleGrantJson("a", zoneReadonlyRole, "", zone),
				roleGrantJson("c", zoneReadonlyRole, "", "")},
			zoneReadonlyRoleJson("zone-readonly@jev/jev", "zone-readonly@jev/jev/main")),
		"AppProject/A grant of a missing definition gives no role": authzCase(
			"Without its RoleDefinition a grant grants nothing",
			nil, []string{roleGrantJson("readers", zoneReadonlyRole, "", "")}, ""),
		"AppProject/A role with only cluster rules gives no project role": authzCase(
			"scope: Cluster rules belong to the global argocd-rbac-cm keys, not to the AppProject",
			[]string{roleDefinitionJson("repo-admin", `{"apiGroups":["argocd.meta.entigo.com"],"resources":["repositories","certificates"],"verbs":["*"],"scope":"Cluster"}`)},
			[]string{roleGrantJson("repos", "repo-admin", "", "")}, ""),
		"AppProject/An invalid role is skipped and the others stay": authzCase(
			"A role whose name breaks the Casbin line is left out, the rest of the project still renders",
			append([]string{roleDefinitionJson("a b", readEverythingRuleJson)}, readonly...),
			[]string{roleGrantJson("broken", "a b", "", ""), roleGrantJson("readers", zoneReadonlyRole, "", "")},
			zoneReadonlyRoleJson("zone-readonly@jev/jev")),
	}

	newService := func() base.GroupService {
		return &GroupImpl{cloudProvider: apis.CloudProviderAWS, authz: apis.Authz{Organization: "Jev", Workspace: "Jev"}}
	}
	test.RunFunctionCases(t, newService, cases)
}

func TestOCIZoneRequiresRoleGrantsOnlyWhenAuthzIsOn(t *testing.T) {
	xr := composite.New()
	if err := json.Unmarshal([]byte(authzZoneJson), &xr.Object); err != nil {
		t.Fatal(err)
	}
	required := map[string][]resource.Required{
		base.EnvironmentKey: {{Resource: &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apiextensions.crossplane.io/v1beta1",
			"kind":       "EnvironmentConfig",
			"metadata":   map[string]any{"name": environmentName},
			"data": map[string]any{
				"oracleProvider":    "crossplane-oracle",
				"argoCDNamespace":   "argocd",
				"cluster":           "oke",
				"securityGroup":     "node",
				"computeSubnetType": "private",
				"podSecurity":       "baseline",
				"namespaceRole":     map[string]any{"rules": []any{}},
			},
		}}}},
	}
	for name, settings := range map[string]apis.Authz{"on": {Organization: "Jev", Workspace: "Jev"}, "off": {}} {
		t.Run(name, func(t *testing.T) {
			g := &GroupImpl{cloudProvider: apis.CloudProviderOCI, authz: settings}
			selectors, err := g.GetRequiredResources(xr, required)
			if err != nil {
				t.Fatal(err)
			}
			_, definitions := selectors[service.RoleDefinitionKey]
			_, grants := selectors[service.RoleGrantKey]
			if definitions != settings.Enabled() || grants != settings.Enabled() {
				t.Errorf("authz %v: RoleDefinitions requested %v, RoleGrants requested %v", settings.Enabled(), definitions, grants)
			}
		})
	}
}
