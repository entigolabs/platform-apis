package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/argocd"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	policyv1 "github.com/kyverno/api/api/policies.kyverno.io/v1"
	ec2v1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/ec2/v1beta1"
	eksv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/eks/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/iam/v1beta1"
	kmsv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/kms/v1beta1"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1alpha1 "k8s.io/api/admissionregistration/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	ZoneAnnotation     = "tenancy.entigo.com/zone"
	zonePoolAnnotation = "tenancy.entigo.com/zone-pool"
	PoolLabel          = "tenancy.entigo.com/pool"

	NamespaceKey      = "Namespaces"
	VPCKey            = "VPC"
	KMSDataAliasKey   = "KMSDataAlias"
	SecurityGroupKey  = "NodeSecurityGroup"
	ClusterKey        = "Cluster"
	ComputeSubnetsKey = "ComputeSubnets"
	ServiceSubnetsKey = "ServiceSubnets"
	PublicSubnetsKey  = "PublicSubnets"
	ControlSubnetsKey = "ControlSubnets"
	IngressKey        = "Ingresses"
	ServiceKey        = "Services"
)

var supportedIngressClasses = base.NewSet("service", "external", "alb")

type zoneGenerator struct {
	// Inputs
	zone     v1alpha1.Zone
	observed map[resource.Name]resource.ObservedComposed
	required map[string][]resource.Required
	env      apis.Environment
	// Dependencies
	namespaces     []*corev1.Namespace
	vpc            ec2v1beta1.VPC
	kmsDataAlias   kmsv1beta1.Alias
	securityGroup  ec2v1beta1.SecurityGroup
	cluster        eksv1beta1.Cluster
	computeSubnets []*ec2v1beta1.Subnet
	serviceSubnets []*ec2v1beta1.Subnet
	publicSubnets  []*ec2v1beta1.Subnet
	controlSubnets []*ec2v1beta1.Subnet

	zoneAnnotations map[string]string
	zoneTags        map[string]*string
	egressExclude   base.Set[string]
	uqNamespaces    []string
}

func GenerateZoneObjects(
	zone v1alpha1.Zone,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var vpc ec2v1beta1.VPC
	var kmsDataAlias kmsv1beta1.Alias
	var securityGroup ec2v1beta1.SecurityGroup
	var cluster eksv1beta1.Cluster

	namespaces, err := base.ExtractResources[*corev1.Namespace](required, NamespaceKey)
	if err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, VPCKey, &vpc); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, KMSDataAliasKey, &kmsDataAlias); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, SecurityGroupKey, &securityGroup); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, ClusterKey, &cluster); err != nil {
		return nil, err
	}
	computeSubnets, err := base.ExtractResources[*ec2v1beta1.Subnet](required, ComputeSubnetsKey)
	if err != nil {
		return nil, err
	}
	serviceSubnets, err := base.ExtractResources[*ec2v1beta1.Subnet](required, ServiceSubnetsKey)
	if err != nil {
		return nil, err
	}
	publicSubnets, err := base.ExtractResources[*ec2v1beta1.Subnet](required, PublicSubnetsKey)
	if err != nil {
		return nil, err
	}
	controlSubnets, err := base.ExtractResources[*ec2v1beta1.Subnet](required, ControlSubnetsKey)
	if err != nil {
		return nil, err
	}
	tags := map[string]*string{
		ZoneAnnotation: &zone.Name,
	}
	maps.Copy(tags, env.Tags)
	generator := zoneGenerator{
		zone:           zone,
		observed:       observed,
		required:       required,
		env:            env,
		namespaces:     namespaces,
		vpc:            vpc,
		kmsDataAlias:   kmsDataAlias,
		securityGroup:  securityGroup,
		cluster:        cluster,
		computeSubnets: computeSubnets,
		serviceSubnets: serviceSubnets,
		publicSubnets:  publicSubnets,
		controlSubnets: controlSubnets,
		zoneAnnotations: map[string]string{
			ZoneAnnotation: zone.Name,
		},
		zoneTags:      tags,
		egressExclude: base.NewSet(env.GranularEgressExclude...),
		uqNamespaces:  GetUniqueNamespaces(zone, namespaces),
	}
	return generator.generate()
}

func GetUniqueNamespaces(zone v1alpha1.Zone, namespaces []*corev1.Namespace) []string {
	uqNs := base.NewSet[string]()
	for _, ns := range namespaces {
		if ns.DeletionTimestamp != nil {
			continue
		}
		uqNs.Add(ns.Name)
	}
	for _, ns := range zone.Spec.Namespaces {
		uqNs.Add(ns.Name)
	}
	// Alphabetical order for consistency
	list := uqNs.ToSlice()
	slices.Sort(list)
	return list
}

func (g zoneGenerator) generate() (map[string]runtime.Object, error) {
	objs := make(map[string]runtime.Object)
	namespaces, err := g.generateNamespaces()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, namespaces)
	launchTemplates := g.generateLaunchTemplates()
	maps.Copy(objs, launchTemplates)
	nodePools, err := g.generateNodePools()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, nodePools)
	appProject := g.getAppProject()
	objs[GetAppProjectKey(g.zone.Name)] = appProject
	networkPolicies, err := g.generateTargetNetworkPolicies()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, networkPolicies)
	return objs, nil
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func GetNamespaceKey(namespace string) string {
	return "namespace-" + namespace
}

func GetSidecarKey(zone, namespace string) string {
	return "sidecar-" + zone + "-" + namespace
}

func GetNetworkPolicyKey(zone, namespace string) string {
	return "netpol-" + zone + "-" + namespace
}

func GetRBACRoleAllKey(zone, namespace string) string {
	return "rbacrole-all-" + zone + "-" + namespace
}

func GetRBACRoleReadKey(zone, namespace string) string {
	return "rbacrole-read-" + zone + "-" + namespace
}

func GetRBContributorKey(zone, namespace string) string {
	return "rb-contributor-" + zone + "-" + namespace
}

func GetRBMaintainerKey(zone, namespace string) string {
	return "rb-maintainer-" + zone + "-" + namespace
}

func GetRBObserverKey(zone, namespace string) string {
	return "rb-observer-" + zone + "-" + namespace
}

func GetMutatingPolicyKey(zone, namespace string) string {
	return "kyverno-mutate-" + zone + "-" + namespace
}

func GetLabelsMutatingPolicyKey(zone, namespace string) string {
	return "kyverno-mutate-labels-" + zone + "-" + namespace
}

func GetValidatingPolicyKey(zone, namespace string) string {
	return "kyverno-validate-" + zone + "-" + namespace
}

func GetLaunchTemplateKey(zone, poolName string) string {
	return "launchtemplate-" + zone + "-" + poolName
}

func GetRoleKey(zone string) string {
	return "role-" + zone
}

func GetRoleWNAttachmentKey(zone string) string {
	return "rpa-wn-" + zone
}

func GetRoleECRROAttachmentKey(zone string) string {
	return "rpa-ecr-ro-" + zone
}

func GetRoleSSMAttachmentKey(zone string) string {
	return "rpa-ssm-" + zone
}

func GetRoleECRProxyAttachmentKey(zone string) string {
	return "rpa-ecr-proxy-" + zone
}

func GetAccessentryKey(zone string) string {
	return "ae-" + zone
}

func GetNodeGroupKey(poolName, hash string) string {
	return "nodepool-" + poolName + "-" + hash
}

func GetAppProjectKey(zone string) string {
	return "appproject-" + zone
}

func GetTargetNetworkPolicyKey(namespace, ingress, service string, port intstr.IntOrString) string {
	return "netpol-" + namespace + "-" + ingress + "-" + service + "-" + port.String()
}

func (g zoneGenerator) generateNamespaces() (map[string]runtime.Object, error) {
	objs := make(map[string]runtime.Object)
	zoneNs := base.NewSet[string]()
	for _, ns := range g.zone.Spec.Namespaces {
		namespace := g.getNamespace(ns)
		objs[GetNamespaceKey(ns.Name)] = namespace
		err := g.generateNamespace(objs, ns.Name, ns.Pool)
		if err != nil {
			return nil, err
		}
		zoneNs.Add(ns.Name)
	}
	for _, ns := range g.namespaces {
		if zoneNs.Contains(ns.Name) || ns.DeletionTimestamp != nil {
			continue
		}
		var pool string
		if ns.Labels != nil {
			pool = ns.Labels[PoolLabel]
		}
		err := g.generateNamespace(objs, ns.Name, pool)
		if err != nil {
			return nil, err
		}
	}
	return objs, nil
}

func (g zoneGenerator) generateNamespace(objs map[string]runtime.Object, name, pool string) error {
	if g.env.GranularEgress {
		sidecar, err := g.getSidecar(name)
		if err != nil {
			return err
		}
		objs[GetSidecarKey(g.zone.Name, name)] = sidecar
	}
	networkPolicy := g.getNetworkPolicy(name)
	objs[GetNetworkPolicyKey(g.zone.Name, name)] = networkPolicy
	allRole := g.getAllRole(name)
	objs[GetRBACRoleAllKey(g.zone.Name, name)] = allRole
	readRole := g.getReadRole(name)
	objs[GetRBACRoleReadKey(g.zone.Name, name)] = readRole
	contributorBinding := g.getRoleBinding(name, name+"-contributor", allRole.Name, "contributor")
	objs[GetRBContributorKey(g.zone.Name, name)] = contributorBinding
	maintainerBinding := g.getRoleBinding(name, name+"-maintainer", allRole.Name, "maintainer")
	objs[GetRBMaintainerKey(g.zone.Name, name)] = maintainerBinding
	observerBinding := g.getRoleBinding(name, name+"-observer", readRole.Name, "observer")
	objs[GetRBObserverKey(g.zone.Name, name)] = observerBinding
	mutatingPolicy := g.getMutatingPolicy(name, pool)
	objs[GetMutatingPolicyKey(g.zone.Name, name)] = mutatingPolicy
	labelsMutatingPolicy := g.getLabelsMutatingPolicy(name)
	objs[GetLabelsMutatingPolicyKey(g.zone.Name, name)] = labelsMutatingPolicy
	validatingPolicy := g.getValidatingPolicy(name)
	objs[GetValidatingPolicyKey(g.zone.Name, name)] = validatingPolicy
	return nil
}

func (g zoneGenerator) getNamespace(ns v1alpha1.Namespace) *corev1.Namespace {
	labels := map[string]string{
		ZoneAnnotation:                       g.zone.Name,
		"pod-security.kubernetes.io/enforce": g.env.PodSecurity,
		"pod-security.kubernetes.io/warn":    g.env.PodSecurity,
	}
	if ns.Pool != "" {
		labels[PoolLabel] = ns.Pool
	}
	if g.env.GranularEgress {
		labels["istio-injection"] = "enabled"
	}
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        ns.Name,
			Labels:      labels,
			Annotations: g.zoneAnnotations,
		},
	}
}

func (g zoneGenerator) getSidecar(name string) (runtime.Object, error) {
	hosts := []string{
		"*/*.svc.cluster.local",
		"istio-system/*",
		"kube-system/kube-dns.kube-system.svc.cluster.local",
	}
	for _, ns := range g.uqNamespaces {
		hosts = append(hosts, ns+"/*")
	}
	modeStr := istiov1alpha3.OutboundTrafficPolicy_Mode_name[int32(istiov1alpha3.OutboundTrafficPolicy_REGISTRY_ONLY)]
	if g.egressExclude.Contains(g.zone.Name) {
		modeStr = istiov1alpha3.OutboundTrafficPolicy_Mode_name[int32(istiov1alpha3.OutboundTrafficPolicy_ALLOW_ANY)]
	}
	sidecar := &istiov1.Sidecar{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.istio.io/v1",
			Kind:       "Sidecar",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name + "-egress",
			Namespace:   name,
			Annotations: g.zoneAnnotations,
		},
		Spec: istiov1alpha3.Sidecar{
			Egress: []*istiov1alpha3.IstioEgressListener{{
				Hosts: hosts,
			}},
		},
	}
	// This workaround is required because istio uses 0 for REGISTRY_ONLY enum and it converts to nil outboundTrafficPolicy
	u, err := base.ToUnstructured(sidecar)
	if err != nil {
		return nil, err
	}
	err = unstructured.SetNestedField(u.Object, modeStr, "spec", "outboundTrafficPolicy", "mode")
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (g zoneGenerator) getNetworkPolicy(nsName string) *networkingv1.NetworkPolicy {
	return &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "NetworkPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        nsName + "-zone",
			Namespace:   nsName,
			Annotations: g.zoneAnnotations,
			Labels: map[string]string{
				ZoneAnnotation: g.zone.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									ZoneAnnotation: g.zone.Name,
								},
							},
						},
					},
				},
			},
		},
	}
}

func (g zoneGenerator) getAllRole(nsName string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{{
		APIGroups: []string{"*"},
		Resources: []string{"*"},
		Verbs:     []string{"*"},
	}}
	return g.getRole(nsName, nsName+"-all", rules)
}

func (g zoneGenerator) getReadRole(nsName string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{{
		APIGroups: []string{"*"},
		Resources: []string{"*"},
		Verbs:     []string{"get", "watch", "list"},
	}}
	return g.getRole(nsName, nsName+"-read", rules)
}

func (g zoneGenerator) getRole(nsName, roleName string, rules []rbacv1.PolicyRule) *rbacv1.Role {
	return &rbacv1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        roleName,
			Namespace:   nsName,
			Annotations: g.zoneAnnotations,
		},
		Rules: rules,
	}
}

func (g zoneGenerator) getRoleBinding(nsName, bindingName, roleName, group string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "RoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        bindingName,
			Namespace:   nsName,
			Annotations: g.zoneAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "Group",
				Name:     group,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
	}
}

func (g zoneGenerator) getMutatingPolicy(namespaceName, poolName string) runtime.Object {
	poolName = g.getPoolName(poolName)
	return &policyv1.MutatingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policies.kyverno.io/v1",
			Kind:       "MutatingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name + "-" + namespaceName + "-add-nodeselector",
			Annotations: g.zoneAnnotations,
			Labels:      map[string]string{"reports.kyverno.io/disabled": "true"},
		},
		Spec: policyv1.MutatingPolicySpec{
			EvaluationConfiguration: &policyv1.MutatingPolicyEvaluationConfiguration{
				Admission:                   &policyv1.AdmissionConfiguration{Enabled: base.BoolPtr(true)},
				MutateExistingConfiguration: &policyv1.MutateExistingConfiguration{Enabled: base.BoolPtr(false)},
			},
			MatchConstraints: &admissionregistrationv1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "tenancy.entigo.com/zone",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1.RuleWithOperations{
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					},
				}},
			},
			MatchConditions: []admissionregistrationv1.MatchCondition{{
				Name:       "namespace-filter",
				Expression: `object.metadata.namespace == "` + namespaceName + `"`,
			}},
			Mutations: []admissionregistrationv1alpha1.Mutation{
				{
					PatchType: admissionregistrationv1alpha1.PatchTypeJSONPatch,
					JSONPatch: &admissionregistrationv1alpha1.JSONPatch{
						Expression: `!has(object.spec.nodeSelector) || size(object.spec.nodeSelector) == 0 ?
[
  JSONPatch{
    op: "add",
    path: "/spec/nodeSelector",
    value: {"tenancy.entigo.com/zone-pool": "` + g.zone.Name + `-` + poolName + `"}
  }
] : []`,
					},
				},
			},
		},
	}
}

func (g zoneGenerator) getLabelsMutatingPolicy(namespaceName string) runtime.Object {
	return &policyv1.MutatingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policies.kyverno.io/v1",
			Kind:       "MutatingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name + "-" + namespaceName + "-labels",
			Annotations: g.zoneAnnotations,
			Labels:      map[string]string{"reports.kyverno.io/disabled": "true"},
		},
		Spec: policyv1.MutatingPolicySpec{
			EvaluationConfiguration: &policyv1.MutatingPolicyEvaluationConfiguration{
				Admission:                   &policyv1.AdmissionConfiguration{Enabled: base.BoolPtr(true)},
				MutateExistingConfiguration: &policyv1.MutateExistingConfiguration{Enabled: base.BoolPtr(false)},
			},
			MatchConstraints: &admissionregistrationv1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "tenancy.entigo.com/zone",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1.RuleWithOperations{
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"services"},
						},
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					},
				}, {
					RuleWithOperations: admissionregistrationv1.RuleWithOperations{
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{"networking.k8s.io"},
							APIVersions: []string{"v1"},
							Resources:   []string{"ingresses"},
						},
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					},
				}},
			},
			MatchConditions: []admissionregistrationv1.MatchCondition{{
				Name:       "namespace-filter",
				Expression: `object.metadata.namespace == "` + namespaceName + `"`,
			}},
			Mutations: []admissionregistrationv1alpha1.Mutation{
				{
					PatchType: admissionregistrationv1alpha1.PatchTypeJSONPatch,
					JSONPatch: &admissionregistrationv1alpha1.JSONPatch{
						Expression: fmt.Sprintf(`has(object.metadata.labels) ?
[
  JSONPatch{
    op: "add",
    path: "/metadata/labels/tenancy.entigo.com~1zone",
    value: "%s"
  }
] :
[
  JSONPatch{
    op: "add",
    path: "/metadata/labels",
    value: {
      "tenancy.entigo.com/zone": "%s"
    }
  }
]`, g.zone.Name, g.zone.Name),
					},
				},
			},
		},
	}
}

func (g zoneGenerator) getValidatingPolicy(namespaceName string) runtime.Object {
	var poolExprList, poolMsgList []string
	for _, pool := range g.zone.Spec.Pools {
		poolExprList = append(poolExprList, `"`+g.zone.Name+`-`+pool.Name+`"`)
		poolMsgList = append(poolMsgList, g.zone.Name+`-`+pool.Name)
	}
	return &policyv1.ValidatingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policies.kyverno.io/v1",
			Kind:       "ValidatingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name + "-" + namespaceName + "-validate-nodeselector",
			Annotations: g.zoneAnnotations,
		},
		Spec: policyv1.ValidatingPolicySpec{
			EvaluationConfiguration: &policyv1.EvaluationConfiguration{
				Admission:  &policyv1.AdmissionConfiguration{Enabled: base.BoolPtr(true)},
				Background: &policyv1.BackgroundConfiguration{Enabled: base.BoolPtr(false)},
			},
			ValidationAction: []admissionregistrationv1.ValidationAction{admissionregistrationv1.Deny},
			MatchConstraints: &admissionregistrationv1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "tenancy.entigo.com/zone",
							Operator: metav1.LabelSelectorOpExists,
						},
					},
				},
				ResourceRules: []admissionregistrationv1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1.RuleWithOperations{
						Rule: admissionregistrationv1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create, admissionregistrationv1.Update},
					},
				}},
			},
			MatchConditions: []admissionregistrationv1.MatchCondition{{
				Name:       "namespace-filter",
				Expression: `object.metadata.namespace == "` + namespaceName + `"`,
			}},
			Validations: []admissionregistrationv1.Validation{
				{
					Expression: `has(object.spec.nodeSelector) &&
"tenancy.entigo.com/zone-pool" in object.spec.nodeSelector &&
object.spec.nodeSelector["tenancy.entigo.com/zone-pool"] in [` + strings.Join(poolExprList, ", ") + `]`,
					Message: "Pod nodeSelector must use a valid nodeSelector label value for tenancy.entigo.com/zone-pool. Valid pools: " + strings.Join(poolMsgList, ", "),
				},
			},
		},
	}
}

func (g zoneGenerator) getPoolName(poolName string) string {
	if poolName != "" {
		return poolName
	}
	pools := g.zone.Spec.Pools
	if len(pools) > 0 {
		return pools[0].Name
	}
	return "default"
}

func (g zoneGenerator) generateLaunchTemplates() map[string]runtime.Object {
	objs := make(map[string]runtime.Object)
	zoneName := g.zone.GetName()
	labels := map[string]string{
		ZoneAnnotation: zoneName,
	}
	emptyString := ""

	for _, pool := range g.zone.Spec.Pools {
		zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)
		annotations := map[string]string{
			zonePoolAnnotation: zonePool,
		}
		maps.Copy(annotations, g.zoneAnnotations)
		tags := map[string]*string{
			ZoneAnnotation:     &zoneName,
			zonePoolAnnotation: &zonePool,
		}
		maps.Copy(tags, g.env.Tags)
		instanceTags := map[string]*string{
			"Name": &zonePool,
		}
		maps.Copy(instanceTags, tags)
		volumeTags := map[string]*string{
			"Name": base.StringPtr(zonePool + "-root"),
		}
		maps.Copy(volumeTags, tags)

		lt := &ec2v1beta1.LaunchTemplate{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "ec2.aws.upbound.io/v1beta1",
				Kind:       "LaunchTemplate",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:        zonePool,
				Annotations: annotations,
				Labels:      labels,
			},
			Spec: ec2v1beta1.LaunchTemplateSpec{
				ResourceSpec: v1.ResourceSpec{
					ManagementPolicies: []v1.ManagementAction{v1.ManagementActionAll},
					ProviderConfigReference: &v1.Reference{
						Name: g.env.AWSProvider,
					},
					DeletionPolicy: "Delete",
				},
				ForProvider: ec2v1beta1.LaunchTemplateParameters_2{
					Name:                  &zonePool,
					UserData:              &emptyString,
					Description:           &zonePool,
					Region:                g.vpc.Spec.ForProvider.Region,
					DisableAPITermination: base.BoolPtr(false),
					DisableAPIStop:        base.BoolPtr(false),
					UpdateDefaultVersion:  base.BoolPtr(true),
					MetadataOptions: []ec2v1beta1.LaunchTemplateMetadataOptionsParameters{{
						HTTPEndpoint:            base.StringPtr("enabled"),
						HTTPProtocolIPv6:        &emptyString,
						HTTPPutResponseHopLimit: base.Float64Ptr(1),
						HTTPTokens:              base.StringPtr("required"),
						InstanceMetadataTags:    &emptyString,
					}},
					VPCSecurityGroupIDRefs: []v1.Reference{{
						Name: g.securityGroup.GetName(),
					}},
					Tags: tags,
					TagSpecifications: []ec2v1beta1.TagSpecificationsParameters{
						{
							ResourceType: base.StringPtr("instance"),
							Tags:         instanceTags,
						},
						{
							ResourceType: base.StringPtr("volume"),
							Tags:         volumeTags,
						},
					},
				},
				InitProvider: ec2v1beta1.LaunchTemplateInitParameters_2{
					BlockDeviceMappings: []ec2v1beta1.BlockDeviceMappingsInitParameters{{
						DeviceName: base.StringPtr("/dev/xvda"),
						EBS: []ec2v1beta1.EBSInitParameters{{
							DeleteOnTermination:      base.StringPtr("true"),
							VolumeSize:               base.Float64Ptr(50),
							Throughput:               base.Float64Ptr(0),
							VolumeInitializationRate: base.Float64Ptr(0),
							SnapshotID:               &emptyString,
							Iops:                     base.Float64Ptr(0),
							VolumeType:               base.StringPtr("gp3"),
							Encrypted:                base.StringPtr("true"),
							KMSKeyID:                 g.kmsDataAlias.Status.AtProvider.Arn,
						}},
						NoDevice:    &emptyString,
						VirtualName: &emptyString,
					}},
				},
			},
		}
		objs[GetLaunchTemplateKey(g.zone.Name, pool.Name)] = lt
	}
	return objs
}

func (g zoneGenerator) generateNodePools() (map[string]runtime.Object, error) {
	objs := make(map[string]runtime.Object)
	iamRole := g.getIAMRole()
	objs[GetRoleKey(g.zone.Name)] = iamRole
	wnRoleAttachment := g.getIAMRolePolicyAttachment(g.zone.Name+"-wn", "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy")
	objs[GetRoleWNAttachmentKey(g.zone.Name)] = wnRoleAttachment
	ecrRORoleAttachment := g.getIAMRolePolicyAttachment(g.zone.Name+"-ecr-ro", "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly")
	objs[GetRoleECRROAttachmentKey(g.zone.Name)] = ecrRORoleAttachment
	ssmRoleAttachment := g.getIAMRolePolicyAttachment(g.zone.Name+"-ssm", "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore")
	objs[GetRoleSSMAttachmentKey(g.zone.Name)] = ssmRoleAttachment
	ecrProxyRoleAttachment := g.getIAMRolePolicyAttachmentWithArnRef(g.zone.Name+"-ecr-proxy", "ecr-proxy")
	objs[GetRoleECRProxyAttachmentKey(g.zone.Name)] = ecrProxyRoleAttachment
	accessEntry := g.getAccessEntry(g.zone.Name)
	objs[GetAccessentryKey(g.zone.Name)] = accessEntry
	for _, pool := range g.zone.Spec.Pools {
		launchTemplate, ok := g.observed[resource.Name(GetLaunchTemplateKey(g.zone.Name, pool.Name))]
		if !ok || launchTemplate.Resource == nil || launchTemplate.Resource.Object == nil {
			continue
		}
		var launchTemplateObj ec2v1beta1.LaunchTemplate
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(launchTemplate.Resource.Object, &launchTemplateObj); err != nil {
			return nil, err
		}
		if launchTemplateObj.Status.AtProvider.LatestVersion == nil {
			continue
		}
		key, ng, err := g.getNodeGroup(pool, launchTemplateObj)
		if err != nil {
			return nil, err
		}
		if ng == nil {
			continue
		}
		objs[key] = ng
	}
	return objs, nil
}

func (g zoneGenerator) getIAMRole() *iamv1beta1.Role {
	return &iamv1beta1.Role{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "iam.aws.upbound.io/v1beta1",
			Kind:       "Role",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name,
			Annotations: g.zoneAnnotations,
		},
		Spec: iamv1beta1.RoleSpec{
			ResourceSpec: v1.ResourceSpec{
				ManagementPolicies: []v1.ManagementAction{v1.ManagementActionAll},
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
			ForProvider: iamv1beta1.RoleParameters{
				AssumeRolePolicy: base.StringPtr(`{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "ec2.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}`),
				Tags: g.zoneTags,
			},
		},
	}
}

func (g zoneGenerator) getIAMRolePolicyAttachment(name, policyArn string) *iamv1beta1.RolePolicyAttachment {
	return &iamv1beta1.RolePolicyAttachment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "iam.aws.upbound.io/v1beta1",
			Kind:       "RolePolicyAttachment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: g.zoneAnnotations,
		},
		Spec: iamv1beta1.RolePolicyAttachmentSpec{
			ResourceSpec: v1.ResourceSpec{
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
			ForProvider: iamv1beta1.RolePolicyAttachmentParameters{
				PolicyArn: &policyArn,
				RoleRef: &v1.Reference{
					Name: g.zone.Name,
				},
			},
		},
	}
}

func (g zoneGenerator) getIAMRolePolicyAttachmentWithArnRef(name, policyArnRef string) *iamv1beta1.RolePolicyAttachment {
	return &iamv1beta1.RolePolicyAttachment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "iam.aws.upbound.io/v1beta1",
			Kind:       "RolePolicyAttachment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: g.zoneAnnotations,
		},
		Spec: iamv1beta1.RolePolicyAttachmentSpec{
			ForProvider: iamv1beta1.RolePolicyAttachmentParameters{
				PolicyArnRef: &v1.Reference{Name: policyArnRef},
				RoleRef: &v1.Reference{
					Name: g.zone.Name,
				},
			},
			ResourceSpec: v1.ResourceSpec{
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
		},
	}
}

func (g zoneGenerator) getAccessEntry(name string) runtime.Object {
	return &eksv1beta1.AccessEntry{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "eks.aws.upbound.io/v1beta1",
			Kind:       "AccessEntry",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: eksv1beta1.AccessEntrySpec{
			ResourceSpec: v1.ResourceSpec{
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
			ForProvider: eksv1beta1.AccessEntryParameters{
				ClusterNameRef: &v1.Reference{
					Name: g.cluster.GetName(),
				},
				PrincipalArnFromRoleRef: &v1.Reference{
					Name: name,
				},
				Type:   base.StringPtr("EC2_LINUX"),
				Region: g.vpc.Status.AtProvider.Region,
			},
		},
	}
}

func (g zoneGenerator) getNodeGroup(pool v1alpha1.Pool, launchTemplateObj ec2v1beta1.LaunchTemplate) (string, runtime.Object, error) {
	zoneName := g.zone.GetName()
	zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)
	version := strconv.FormatFloat(*launchTemplateObj.Status.AtProvider.LatestVersion, 'f', -1, 64)

	var instanceTypes []string
	var zoneFilter base.Set[string]
	var capacityType = "ON_DEMAND"
	var minSize float64 = 1
	var maxSize float64 = -1

	for _, requirement := range pool.Requirements {
		switch requirement.Key {
		case "instance-type":
			instanceTypes = requirement.Values
		case "zone":
			zoneFilter = base.ToSet(requirement.Values)
		case "capacity-type":
			capacityType = requirement.Value.String()
		case "min-size":
			val, err := getIntOrFloat(requirement.Value)
			if err != nil {
				return "", nil, err
			}
			minSize = val
		case "max-size":
			val, err := getIntOrFloat(requirement.Value)
			if err != nil {
				return "", nil, err
			}
			maxSize = val
		}
	}

	if maxSize == -1 {
		maxSize = minSize
	}
	if maxSize < 1 {
		return "", nil, nil
	}

	hash := GetInstanceTypesHash(instanceTypes)
	name := fmt.Sprintf("%s-%s", zonePool, hash)

	var subnetRefs []v1.Reference
	for _, subnet := range g.computeSubnets {
		subnetName := subnet.GetName()
		if zoneFilter.Size() > 0 {
			if subnet.Status.AtProvider.AvailabilityZone != nil && zoneFilter.Contains(*subnet.Status.AtProvider.AvailabilityZone) {
				subnetRefs = append(subnetRefs, v1.Reference{Name: subnetName})
			}
		} else {
			subnetRefs = append(subnetRefs, v1.Reference{Name: subnetName})
		}
	}

	annotations := map[string]string{
		zonePoolAnnotation: zonePool,
	}
	maps.Copy(annotations, g.zoneAnnotations)
	tags := map[string]*string{
		ZoneAnnotation:     &zoneName,
		zonePoolAnnotation: &zonePool,
	}
	maps.Copy(tags, g.env.Tags)
	eksLabels := map[string]*string{
		ZoneAnnotation:     &zoneName,
		zonePoolAnnotation: &zonePool,
	}

	ng := &eksv1beta1.NodeGroup{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "eks.aws.upbound.io/v1beta1",
			Kind:       "NodeGroup",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Annotations: annotations,
		},
		Spec: eksv1beta1.NodeGroupSpec{
			ResourceSpec: v1.ResourceSpec{
				ManagementPolicies: []v1.ManagementAction{v1.ManagementActionAll},
				ProviderConfigReference: &v1.Reference{
					Name: g.env.AWSProvider,
				},
			},
			ForProvider: eksv1beta1.NodeGroupParameters{
				LaunchTemplate: []eksv1beta1.LaunchTemplateParameters{{
					Name:    base.StringPtr(zonePool),
					Version: &version,
				}},
				ClusterNameRef: &v1.Reference{
					Name: g.cluster.GetName(),
				},
				Version:       g.cluster.Status.AtProvider.Version,
				Region:        g.vpc.Status.AtProvider.Region,
				UpdateConfig:  []eksv1beta1.UpdateConfigParameters{{MaxUnavailable: base.Float64Ptr(1)}},
				InstanceTypes: toInstanceTypePointers(instanceTypes),
				SubnetIDRefs:  subnetRefs,
				Labels:        eksLabels,
				NodeRoleArnRef: &v1.Reference{
					Name: zoneName,
				},
				Tags:         tags,
				CapacityType: base.StringPtr(capacityType),
				ScalingConfig: []eksv1beta1.ScalingConfigParameters{{
					MaxSize: &maxSize,
					MinSize: &minSize,
				}},
			},
			InitProvider: eksv1beta1.NodeGroupInitParameters{
				ScalingConfig: []eksv1beta1.ScalingConfigInitParameters{{
					DesiredSize: &minSize,
				}},
			},
		},
	}
	u, err := base.ToUnstructured(ng)
	if err != nil {
		return "", nil, err
	}
	configs, found, _ := unstructured.NestedSlice(u.Object, "spec", "forProvider", "scalingConfig")
	if found {
		for _, cfg := range configs {
			if c, ok := cfg.(map[string]any); ok {
				if val, exists := c["desiredSize"]; exists && val == nil {
					delete(c, "desiredSize")
				}
			}
		}
		err = unstructured.SetNestedSlice(u.Object, configs, "spec", "forProvider", "scalingConfig")
		if err != nil {
			return "", nil, err
		}
	}
	return GetNodeGroupKey(pool.Name, hash), u, nil
}

func getIntOrFloat(value intstr.IntOrString) (float64, error) {
	if value.Type == intstr.Int {
		return float64(value.IntValue()), nil
	}
	return strconv.ParseFloat(value.StrVal, 64)
}

func toInstanceTypePointers(types []string) []*string {
	ptrTypes := make([]*string, len(types))
	for i, t := range types {
		ptrTypes[i] = &t
	}
	return ptrTypes
}

func (g zoneGenerator) getAppProject() runtime.Object {
	var destinations []argocd.ApplicationDestination
	for _, ns := range g.uqNamespaces {
		destinations = append(destinations, argocd.ApplicationDestination{
			Namespace: ns,
			Server:    "https://kubernetes.default.svc",
		})
	}

	var whitelist, blacklist []argocd.ClusterResourceRestrictionItem
	if g.zone.Spec.ClusterPermissions {
		whitelist = []argocd.ClusterResourceRestrictionItem{{Group: "*", Kind: "*"}}
		blacklist = []argocd.ClusterResourceRestrictionItem{}
	} else {
		whitelist = []argocd.ClusterResourceRestrictionItem{}
		blacklist = []argocd.ClusterResourceRestrictionItem{{Group: "*", Kind: "*"}}
	}
	var contributorGroups []string
	if g.zone.Spec.AppProject != nil {
		contributorGroups = g.zone.Spec.AppProject.ContributorGroups
	}

	return &argocd.AppProject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "AppProject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name,
			Namespace:   g.env.ArgoCDNamespace,
			Annotations: g.zoneAnnotations,
		},
		Spec: argocd.AppProjectSpec{
			Description:                "Security zone for isolated team deployment",
			Destinations:               destinations,
			SourceRepos:                g.env.AppProject.SourceRepos,
			SourceNamespaces:           g.uqNamespaces,
			ClusterResourceWhitelist:   whitelist,
			ClusterResourceBlacklist:   blacklist,
			NamespaceResourceBlacklist: g.env.AppProject.NamespaceResourceBlacklist,
			NamespaceResourceWhitelist: g.env.AppProject.NamespaceResourceWhitelist,
			Roles: []argocd.ProjectRole{
				{
					Name:        "maintainer",
					Description: "Maintainer permissions",
					Groups:      g.env.AppProject.MaintainerGroups,
					Policies: []string{
						fmt.Sprintf("p, proj:%s:maintainer, applications, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:maintainer, repositories, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:maintainer, applicationsets, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:maintainer, logs, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:maintainer, exec, *, %s/*, allow", g.zone.Name, g.zone.Name),
					},
				},
				{
					Name:        "observer",
					Description: "Observer permissions",
					Groups:      g.env.AppProject.ObserverGroups,
					Policies: []string{
						fmt.Sprintf("p, proj:%s:observer, applications, get, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:observer, applicationsets, get, %s/*, allow", g.zone.Name, g.zone.Name),
					},
				},
				{
					Name:        "contributor",
					Description: "Contributor permissions",
					Groups:      contributorGroups,
					Policies: []string{
						fmt.Sprintf("p, proj:%s:contributor, applications, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:contributor, repositories, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:contributor, applicationsets, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:contributor, logs, *, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:contributor, exec, *, %s/*, allow", g.zone.Name, g.zone.Name),
					},
				},
				{
					Name:        "cicd",
					Description: "Use this role for your CI/CD pipelines",
					Groups:      g.env.AppProject.MaintainerGroups,
					Policies: []string{
						fmt.Sprintf("p, proj:%s:cicd, applications, sync, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:cicd, applicationsets, sync, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:cicd, applications, get, %s/*, allow", g.zone.Name, g.zone.Name),
						fmt.Sprintf("p, proj:%s:cicd, applicationsets, get, %s/*, allow", g.zone.Name, g.zone.Name),
					},
				},
			},
		},
	}
}

func (g zoneGenerator) generateTargetNetworkPolicies() (map[string]runtime.Object, error) {
	serviceBlocks := getSubnetsBlocks(g.serviceSubnets)
	publicBlocks := getSubnetsBlocks(g.publicSubnets)
	controlBlocks := getSubnetsBlocks(g.controlSubnets)
	protocol := corev1.ProtocolTCP
	objs := make(map[string]runtime.Object)
	for _, ns := range g.uqNamespaces {
		if ns == "" {
			continue
		}
		ingresses, err := base.ExtractResources[*networkingv1.Ingress](g.required, ns+IngressKey)
		if err != nil {
			return nil, err
		}
		services, err := base.ExtractResources[*corev1.Service](g.required, ns+ServiceKey)
		if err != nil {
			return nil, err
		}
		for _, ingress := range ingresses {
			if ingress.Spec.Rules == nil || ingress.Spec.IngressClassName == nil ||
				!supportedIngressClasses.Contains(*ingress.Spec.IngressClassName) {
				continue
			}
			for _, rule := range ingress.Spec.Rules {
				if rule.HTTP == nil {
					continue
				}
				for _, path := range rule.HTTP.Paths {
					var targetPort *intstr.IntOrString
					serviceName := path.Backend.Service.Name
					var service corev1.Service
					for _, svc := range services {
						if svc.Name != serviceName {
							continue
						}
						service = *svc
						for _, port := range svc.Spec.Ports {
							if path.Backend.Service.Port.Name != "" && port.Name == path.Backend.Service.Port.Name ||
								path.Backend.Service.Port.Number != 0 && port.Port == path.Backend.Service.Port.Number {
								targetPort = &port.TargetPort
								break
							}
						}
					}
					if service.Name == "" || targetPort == nil {
						continue
					}
					matchLabels := make(map[string]string)
					for key, value := range service.Spec.Selector {
						matchLabels[key] = value
					}
					var blocks []networkingv1.NetworkPolicyPeer
					switch *ingress.Spec.IngressClassName {
					case "service":
						blocks = serviceBlocks
					case "external":
						blocks = publicBlocks
					case "alb":
						// TODO Improved ALB support based on annotations
						blocks = controlBlocks
					}
					objs[GetTargetNetworkPolicyKey(ns, ingress.Name, serviceName, *targetPort)] = &networkingv1.NetworkPolicy{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "networking.k8s.io/v1",
							Kind:       "NetworkPolicy",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      fmt.Sprintf("%s-%s-%s", ingress.Name, serviceName, targetPort.String()),
							Namespace: ns,
						},
						Spec: networkingv1.NetworkPolicySpec{
							PodSelector: metav1.LabelSelector{MatchLabels: matchLabels},
							PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
							Ingress: []networkingv1.NetworkPolicyIngressRule{{
								From: blocks,
								Ports: []networkingv1.NetworkPolicyPort{{
									Protocol: &protocol,
									Port:     targetPort,
								}},
							}},
						},
					}
				}
			}
		}
	}
	return objs, nil
}

func getSubnetsBlocks(subnets []*ec2v1beta1.Subnet) []networkingv1.NetworkPolicyPeer {
	var blocks []networkingv1.NetworkPolicyPeer
	for _, subnet := range subnets {
		if cidr := subnet.Status.AtProvider.CidrBlock; cidr != nil {
			blocks = append(blocks, networkingv1.NetworkPolicyPeer{
				IPBlock: &networkingv1.IPBlock{CIDR: *cidr},
			})
		}
	}

	return blocks
}

func GetInstanceTypesHash(instanceTypes []string) string {
	hash := sha256.Sum256([]byte(strings.Join(instanceTypes, "-")))
	return hex.EncodeToString(hash[:])[:8]
}
