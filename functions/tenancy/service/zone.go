package service

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/argocd"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	policyv1 "github.com/kyverno/api/api/policies.kyverno.io/v1"
	istiov1alpha3 "istio.io/api/networking/v1alpha3"
	istiov1 "istio.io/client-go/pkg/apis/networking/v1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1alpha1 "k8s.io/api/admissionregistration/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	zonePoolAnnotation = "tenancy.entigo.com/zone-pool"
	PoolLabel          = "tenancy.entigo.com/pool"

	RoleContributor = "contributor"
	RoleMaintainer  = "maintainer"
	RoleObserver    = "observer"
	RoleCICD        = "cicd"

	NamespaceKey          = "Namespaces"
	VPCKey                = "VPC"
	KMSDataAliasKey       = "KMSDataAlias"
	KMSDataKeyKey         = "KMSDataKey"
	SecurityGroupKey      = "NodeSecurityGroup"
	ClusterKey            = "Cluster"
	SubnetsKey            = "Subnets"
	IngressKey            = "Ingresses"
	IngressClassKey       = "IngressClasses"
	IngressClassParamsKey = "IngressClassParams"
	ServiceKey            = "Services"

	HTTPRouteKey                 = "HTTPRoutes"
	GatewayKey                   = "Gateways"
	LoadBalancerConfigurationKey = "LoadBalancerConfigurations"

	subnetTypeLabel        = "subnet-type"
	externalNameAnnotation = "crossplane.io/external-name"

	ingressClassParamsGroup = "elbv2.k8s.aws"
	ingressClassParamsKind  = "IngressClassParams"

	gatewayGroup       = "gateway.networking.k8s.io"
	gatewayKind        = "Gateway"
	lbConfigGroup      = "gateway.k8s.aws"
	lbConfigKind       = "LoadBalancerConfiguration"
	serviceBackendKind = "Service"
)

type zoneGenerator struct {
	log logging.Logger
	// Inputs
	zone     v1alpha1.Zone
	observed map[resource.Name]resource.ObservedComposed
	required map[string][]resource.Required
	env      apis.Environment
	// Dependencies
	namespaces     []*corev1.Namespace
	ingressClasses []*networkingv1.IngressClass

	zoneAnnotations map[string]string
	zoneTags        map[string]*string
	egressExclude   base.Set[string]
	uqNamespaces    []string
	roleMappings    map[string][]string
}

func GenerateZoneObjects(
	zone v1alpha1.Zone,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
	log logging.Logger,
	cloudProvider apis.CloudProvider,
) (map[string]client.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}
	if err := env.ValidateFor(cloudProvider); err != nil {
		return nil, err
	}
	namespaces, err := base.ExtractResources[*corev1.Namespace](required, NamespaceKey)
	if err != nil {
		return nil, err
	}
	ingressClasses, err := base.ExtractResources[*networkingv1.IngressClass](required, IngressClassKey)
	if err != nil {
		return nil, err
	}
	tags := make(map[string]*string)
	maps.Copy(tags, env.Tags)
	tags[base.TenancyZoneLabel] = &zone.Name
	generator := zoneGenerator{
		log:            log,
		zone:           zone,
		observed:       observed,
		required:       required,
		env:            env,
		namespaces:     namespaces,
		ingressClasses: ingressClasses,
		zoneAnnotations: map[string]string{
			base.TenancyZoneLabel: zone.Name,
		},
		zoneTags:      tags,
		egressExclude: base.NewSet(env.GranularEgressExclude...),
		uqNamespaces:  GetUniqueNamespaces(zone, namespaces),
		roleMappings:  mergeRoleMappings(zone, env),
	}
	if cloudProvider == apis.CloudProviderOCI {
		oci, err := newOCIZoneGenerator(generator)
		if err != nil {
			return nil, err
		}
		return oci.generate()
	}
	aws, err := newAWSZoneGenerator(generator)
	if err != nil {
		return nil, err
	}
	return aws.generate()
}

func mergeRoleMappings(zone v1alpha1.Zone, env apis.Environment) map[string][]string {
	merged := make(map[string][]string)
	for _, rm := range zone.Spec.RoleMapping {
		merged[rm.RoleRef] = append(merged[rm.RoleRef], rm.Groups...)
	}

	for _, rm := range env.RoleMapping {
		merged[rm.RoleRef] = append(merged[rm.RoleRef], rm.Groups...)
	}

	for roleRef, groups := range merged {
		if roleRef == RoleContributor {
			groups = mergeContributorGroups(zone, groups)
		}
		uqGroups := base.ToSet(groups).ToSlice()
		slices.Sort(uqGroups)
		merged[roleRef] = uqGroups
	}
	return merged
}

func mergeContributorGroups(zone v1alpha1.Zone, groups []string) []string {
	if zone.Spec.AppProject == nil || len(zone.Spec.AppProject.ContributorGroups) == 0 {
		return groups
	}
	return append(groups, zone.Spec.AppProject.ContributorGroups...)
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

func (g zoneGenerator) generateCommon() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	namespaces, err := g.generateNamespaces()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, namespaces)
	crb := g.getClusterRoleBinding(RoleContributor)
	if crb != nil {
		objs[GetClusterRoleBindingKey(g.zone.Name, RoleContributor)] = crb
	}
	objs[GetAppProjectKey(g.zone.Name)] = g.getAppProject()
	return objs, nil
}

func (g zoneGenerator) getPoolAnnotations(zonePool string) map[string]string {
	annotations := map[string]string{zonePoolAnnotation: zonePool}
	maps.Copy(annotations, g.zoneAnnotations)
	return annotations
}

func (g zoneGenerator) getPoolLabels(zonePool string) map[string]string {
	return map[string]string{
		base.TenancyZoneLabel: g.zone.Name,
		zonePoolAnnotation:    zonePool,
	}
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

func GetRBACRoleNamespaceKey(zone, namespace string) string {
	return "rbacrole-namespace-" + zone + "-" + namespace
}

func GetRBKey(zone, namespace, roleRef string) string {
	return "rb-" + roleRef + "-" + zone + "-" + namespace
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

func GetAppProjectKey(zone string) string {
	return "appproject-" + zone
}

func GetClusterRoleBindingKey(zone, role string) string {
	return "crb-" + zone + "-" + role
}

func (g zoneGenerator) generateNamespaces() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	zoneNs := base.NewSet[string]()
	granularNetwork := g.env.GranularNamespaceNetworkPolicy
	if g.zone.Spec.GranularNamespaceNetworkPolicy != nil {
		granularNetwork = *g.zone.Spec.GranularNamespaceNetworkPolicy
	}
	if g.env.CreateAppsNamespace {
		name := g.zone.Name + "-apps"
		g.generateAppsNamespace(objs, name)
		zoneNs.Add(name)
	}
	for _, ns := range g.zone.Spec.Namespaces {
		if zoneNs.Contains(ns.Name) {
			continue
		}
		objs[GetNamespaceKey(ns.Name)] = g.getNamespace(ns)
		err := g.generateNamespace(objs, ns.Name, ns.Pool, granularNetwork)
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
		err := g.generateNamespace(objs, ns.Name, pool, granularNetwork)
		if err != nil {
			return nil, err
		}
	}
	return objs, nil
}

func (g zoneGenerator) generateNamespace(objs map[string]client.Object, name, pool string, granularNetwork bool) error {
	if g.env.GranularEgress {
		sidecar, err := g.getSidecar(name)
		if err != nil {
			return err
		}
		objs[GetSidecarKey(g.zone.Name, name)] = sidecar
	}
	objs[GetNetworkPolicyKey(g.zone.Name, name)] = g.getNetworkPolicy(name, granularNetwork)
	allRole := g.getAllRole(name)
	objs[GetRBACRoleAllKey(g.zone.Name, name)] = allRole
	readRole := g.getReadRole(name)
	objs[GetRBACRoleReadKey(g.zone.Name, name)] = readRole
	for role, groups := range g.roleMappings {
		roleName := allRole.Name
		if role == RoleObserver {
			roleName = readRole.Name
		}
		objs[GetRBKey(g.zone.Name, name, role)] = g.getRoleBinding(name, name+"-"+role, roleName, groups)
	}
	if groups, ok := g.roleMappings[RoleMaintainer]; ok {
		nsRole := g.getNamespaceRole(name)
		objs[GetRBACRoleNamespaceKey(g.zone.Name, name)] = nsRole
		objs[GetRBKey(g.zone.Name, name, RoleMaintainer+"-namespace")] =
			g.getRoleBinding(name, name+"-"+RoleMaintainer+"-namespace", nsRole.Name, groups)
	}
	objs[GetMutatingPolicyKey(g.zone.Name, name)] = g.getMutatingPolicy(name, pool)
	objs[GetLabelsMutatingPolicyKey(g.zone.Name, name)] = g.getLabelsMutatingPolicy(name)
	objs[GetValidatingPolicyKey(g.zone.Name, name)] = g.getValidatingPolicy(name)
	return nil
}

func (g zoneGenerator) generateAppsNamespace(objs map[string]client.Object, name string) {
	ns := v1alpha1.Namespace{Name: name}
	obj := g.getNamespace(ns)
	labels := obj.GetLabels()
	delete(labels, "istio-injection")
	labels["tenancy.entigo.com/only-argocd-apps"] = "true"
	obj.SetLabels(labels)
	objs[GetNamespaceKey(name)] = obj

	allRole := g.getRole(name, name+"-all", []rbacv1.PolicyRule{{
		APIGroups: []string{"argoproj.io"},
		Resources: []string{"applications", "applicationsets"},
		Verbs:     []string{"*"},
	}})
	objs[GetRBACRoleAllKey(g.zone.Name, name)] = allRole
	readRole := g.getRole(name, name+"-read", []rbacv1.PolicyRule{{
		APIGroups: []string{"argoproj.io"},
		Resources: []string{"applications", "applicationsets"},
		Verbs:     []string{"get", "watch", "list"},
	}})
	objs[GetRBACRoleReadKey(g.zone.Name, name)] = readRole
	for role, groups := range g.roleMappings {
		roleName := allRole.Name
		if role == RoleObserver {
			roleName = readRole.Name
		}
		objs[GetRBKey(g.zone.Name, name, role)] = g.getRoleBinding(name, name+"-"+role, roleName, groups)
	}
}

func (g zoneGenerator) getNamespace(ns v1alpha1.Namespace) *corev1.Namespace {
	labels := map[string]string{
		base.TenancyZoneLabel:                g.zone.Name,
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

func (g zoneGenerator) getSidecar(name string) (client.Object, error) {
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

func (g zoneGenerator) getNetworkPolicy(nsName string, granularNetwork bool) *networkingv1.NetworkPolicy {
	var matchLabels map[string]string
	if granularNetwork {
		matchLabels = map[string]string{"kubernetes.io/metadata.name": nsName}
	} else {
		matchLabels = map[string]string{base.TenancyZoneLabel: g.zone.Name}
	}
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
				base.TenancyZoneLabel: g.zone.Name,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeIngress},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{MatchLabels: matchLabels},
						},
					},
				},
			},
		},
	}
}

// getAllRole lists allowed resources explicitly instead of */*/*, plus the environment's
// namespaceRole.additionalRules for operators installed in the cluster. A wildcard would also grant
// managed resources (*.upbound.io), RBAC, NetworkPolicies and the Namespace object itself
// (namespace-scoped RoleBindings authorize /api/v1/namespaces/<ns>, kubernetes/kubernetes#142024).
func (g zoneGenerator) getAllRole(nsName string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "pods/log", "services", "endpoints", "configmaps", "secrets",
				"persistentvolumeclaims", "serviceaccounts", "events"},
			Verbs: []string{"*"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods/exec", "pods/portforward", "pods/eviction"},
			Verbs:     []string{"create"},
		},
		{
			APIGroups: []string{"apps", "batch", "autoscaling", "policy"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"gateway.networking.k8s.io"},
			Resources: []string{"httproutes", "grpcroutes", "tcproutes", "tlsroutes"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"networking.istio.io"},
			Resources: []string{"virtualservices", "destinationrules", "serviceentries"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"external-secrets.io"},
			Resources: []string{"externalsecrets"},
			Verbs:     []string{"*"},
		},
		{
			APIGroups: []string{"artifact.entigo.com", "database.entigo.com", "mq.entigo.com",
				"networking.entigo.com", "queue.entigo.com", "storage.entigo.com", "workload.entigo.com"},
			Resources: []string{"*"},
			Verbs:     []string{"*"},
		},
	}
	rules = append(rules, g.env.NamespaceRole.AdditionalRules...)
	return g.getRole(nsName, nsName+"-all", rules)
}

// getNamespaceRole lets maintainers manage the zone namespace object itself. A RoleBinding in the
// namespace authorizes /api/v1/namespaces/<ns> (kubernetes/kubernetes#142024), so this stays limited
// to zone namespaces, unlike a cluster-wide grant.
func (g zoneGenerator) getNamespaceRole(nsName string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{{
		APIGroups:     []string{""},
		Resources:     []string{"namespaces"},
		ResourceNames: []string{nsName},
		Verbs:         []string{"update", "patch", "delete"},
	}}
	return g.getRole(nsName, nsName+"-namespace", rules)
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

func (g zoneGenerator) getRoleBinding(nsName, bindingName, roleName string, groups []string) *rbacv1.RoleBinding {
	subjects := make([]rbacv1.Subject, len(groups))
	for i, group := range groups {
		subjects[i] = rbacv1.Subject{
			Kind:     "Group",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     group,
		}
	}
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
		Subjects: subjects,
	}
}

func (g zoneGenerator) getClusterRoleBinding(role string) client.Object {
	groups := g.roleMappings[role]
	if len(groups) == 0 {
		return nil
	}
	subjects := make([]rbacv1.Subject, len(groups))
	for i, group := range groups {
		subjects[i] = rbacv1.Subject{
			Kind:     "Group",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     group,
		}
	}
	return &rbacv1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "rbac.authorization.k8s.io/v1",
			Kind:       "ClusterRoleBinding",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name + "-" + role,
			Annotations: g.zoneAnnotations,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "zone-tenancy-c-read",
		},
		Subjects: subjects,
	}
}

func (g zoneGenerator) getMutatingPolicy(namespaceName, pool string) client.Object {
	var key, value string
	if pool != "" {
		key = "tenancy.entigo.com/zone-pool"
		value = fmt.Sprintf("%s-%s", g.zone.Name, pool)
	} else {
		key = "tenancy.entigo.com/zone"
		value = g.zone.Name
	}
	escapedKey := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
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
						Expression: fmt.Sprintf(`!has(object.spec.nodeSelector) ?
[
  JSONPatch{
    op: "add",
    path: "/spec/nodeSelector",
    value: {"%s": "%s"}
  }
] : (
  "%s" in object.spec.nodeSelector ? [] :
  [
    JSONPatch{
      op: "add",
      path: "/spec/nodeSelector/%s",
      value: "%s"
    }
  ]
)`, key, value, key, escapedKey, value),
					},
				},
			},
		},
	}
}

func (g zoneGenerator) getLabelsMutatingPolicy(namespaceName string) client.Object {
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

func (g zoneGenerator) getValidatingPolicy(namespaceName string) client.Object {
	var poolExprList, poolMsgList []string
	for _, pool := range g.zone.Spec.Pools {
		poolExprList = append(poolExprList, `"`+g.zone.Name+`-`+pool.Name+`"`)
		poolMsgList = append(poolMsgList, g.zone.Name+`-`+pool.Name)
	}
	expression := fmt.Sprintf(`has(object.spec.nodeSelector) && (
  ("tenancy.entigo.com/zone-pool" in object.spec.nodeSelector &&
   object.spec.nodeSelector["tenancy.entigo.com/zone-pool"] in [%s]) ||
  ("tenancy.entigo.com/zone" in object.spec.nodeSelector &&
   object.spec.nodeSelector["tenancy.entigo.com/zone"] == "%s")
)`, strings.Join(poolExprList, ", "), g.zone.Name)
	message := fmt.Sprintf("Pod nodeSelector must either use tenancy.entigo.com/zone-pool with a valid value [%s]"+
		" or tenancy.entigo.com/zone with value %s", strings.Join(poolMsgList, ", "), g.zone.Name)
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
						Operations: []admissionregistrationv1.OperationType{admissionregistrationv1.Create},
					},
				}},
			},
			MatchConditions: []admissionregistrationv1.MatchCondition{{
				Name:       "namespace-filter",
				Expression: `object.metadata.namespace == "` + namespaceName + `"`,
			}},
			Validations: []admissionregistrationv1.Validation{
				{
					Expression: expression,
					Message:    message,
				},
			},
		},
	}
}

func getIntOrFloat(value intstr.IntOrString) (float64, error) {
	if value.Type == intstr.Int {
		return float64(value.IntValue()), nil
	}
	return strconv.ParseFloat(value.StrVal, 64)
}

func (g zoneGenerator) getAppProject() client.Object {
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
					Name:        RoleMaintainer,
					Description: "Maintainer permissions",
					Groups:      g.roleMappings[RoleMaintainer],
					Policies: []string{
						fmt.Sprintf("p, proj:%s:%s, applications, *, %s/*, allow", g.zone.Name, RoleMaintainer, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, repositories, *, %s/*, allow", g.zone.Name, RoleMaintainer, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, applicationsets, *, %s/*, allow", g.zone.Name, RoleMaintainer, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, logs, *, %s/*, allow", g.zone.Name, RoleMaintainer, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, exec, *, %s/*, allow", g.zone.Name, RoleMaintainer, g.zone.Name),
					},
				},
				{
					Name:        RoleObserver,
					Description: "Observer permissions",
					Groups:      g.roleMappings[RoleObserver],
					Policies: []string{
						fmt.Sprintf("p, proj:%s:%s, applications, get, %s/*, allow", g.zone.Name, RoleObserver, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, applicationsets, get, %s/*, allow", g.zone.Name, RoleObserver, g.zone.Name),
					},
				},
				{
					Name:        RoleContributor,
					Description: "Contributor permissions",
					Groups:      g.roleMappings[RoleContributor],
					Policies: []string{
						fmt.Sprintf("p, proj:%s:%s, applications, *, %s/*, allow", g.zone.Name, RoleContributor, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, repositories, *, %s/*, allow", g.zone.Name, RoleContributor, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, applicationsets, *, %s/*, allow", g.zone.Name, RoleContributor, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, logs, *, %s/*, allow", g.zone.Name, RoleContributor, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, exec, *, %s/*, allow", g.zone.Name, RoleContributor, g.zone.Name),
					},
				},
				{
					Name:        RoleCICD,
					Description: "Use this role for your CI/CD pipelines",
					Groups:      g.roleMappings[RoleMaintainer],
					Policies: []string{
						fmt.Sprintf("p, proj:%s:%s, applications, sync, %s/*, allow", g.zone.Name, RoleCICD, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, applicationsets, sync, %s/*, allow", g.zone.Name, RoleCICD, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, applications, get, %s/*, allow", g.zone.Name, RoleCICD, g.zone.Name),
						fmt.Sprintf("p, proj:%s:%s, applicationsets, get, %s/*, allow", g.zone.Name, RoleCICD, g.zone.Name),
					},
				},
			},
		},
	}
}

type httpRouteBackend struct {
	name string
	port int32
}

func GetTargetNetworkPolicyKey(namespace, ingress, service string, port intstr.IntOrString) string {
	return "netpol-" + namespace + "-" + ingress + "-" + service + "-" + port.String()
}

func addTargetNetworkPolicy(objs map[string]client.Object, ns, ingressName, serviceName string,
	portName string, portNumber int32, services []*corev1.Service, blocks []networkingv1.NetworkPolicyPeer) {
	if serviceName == "" {
		return
	}
	var service *corev1.Service
	var targetPort *intstr.IntOrString
	for _, svc := range services {
		if svc.Name != serviceName {
			continue
		}
		service = svc
		for _, port := range svc.Spec.Ports {
			if portName != "" && port.Name == portName ||
				portNumber != 0 && port.Port == portNumber {
				tp := port.TargetPort
				targetPort = &tp
				break
			}
		}
		break
	}
	if service == nil || targetPort == nil {
		return
	}
	matchLabels := make(map[string]string)
	for key, value := range service.Spec.Selector {
		matchLabels[key] = value
	}
	protocol := corev1.ProtocolTCP
	objs[GetTargetNetworkPolicyKey(ns, ingressName, serviceName, *targetPort)] = &networkingv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "networking.k8s.io/v1",
			Kind:       "NetworkPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%s", ingressName, serviceName, targetPort.String()),
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

// getRouteBlocks unions the peers of every gateway the route lists as a parent, parents that aren't
// gateways or whose peers couldn't be resolved contribute nothing.

func (g zoneGenerator) getRouteBlocks(route *unstructured.Unstructured,
	gatewayBlocks map[string][]networkingv1.NetworkPolicyPeer, namespace string) []networkingv1.NetworkPolicyPeer {
	parents, found, err := unstructured.NestedSlice(route.Object, "spec", "parentRefs")
	if err != nil {
		g.log.Debug("Failed to read http route parent refs", "httpRoute", route.GetName(), "error", err)
		return nil
	}
	if !found {
		return nil
	}
	var blocks []networkingv1.NetworkPolicyPeer
	for _, parent := range parents {
		parentMap, ok := parent.(map[string]any)
		if !ok {
			continue
		}
		name := getOptionalString(parentMap, "name", "")
		if name == "" || getOptionalString(parentMap, "kind", gatewayKind) != gatewayKind ||
			getOptionalString(parentMap, "group", gatewayGroup) != gatewayGroup {
			continue
		}
		parentNamespace := getOptionalString(parentMap, "namespace", namespace)
		for _, block := range gatewayBlocks[getNamespacedName(parentNamespace, name)] {
			if slices.ContainsFunc(blocks, func(b networkingv1.NetworkPolicyPeer) bool {
				return reflect.DeepEqual(b, block)
			}) {
				continue
			}
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// getHTTPRouteBackends returns the service backends of every route rule. Backends in another
// namespace are skipped, their policy would have to be created in that namespace instead.

func (g zoneGenerator) getHTTPRouteBackends(route *unstructured.Unstructured, namespace string) []httpRouteBackend {
	rules, found, err := unstructured.NestedSlice(route.Object, "spec", "rules")
	if err != nil {
		g.log.Debug("Failed to read http route rules", "httpRoute", route.GetName(), "error", err)
		return nil
	}
	if !found {
		return nil
	}
	var backends []httpRouteBackend
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]any)
		if !ok {
			continue
		}
		refs, ok := ruleMap["backendRefs"].([]any)
		if !ok {
			continue
		}
		for _, ref := range refs {
			refMap, ok := ref.(map[string]any)
			if !ok {
				continue
			}
			name := getOptionalString(refMap, "name", "")
			if name == "" || getOptionalString(refMap, "kind", serviceBackendKind) != serviceBackendKind ||
				getOptionalString(refMap, "group", "") != "" {
				continue
			}
			if refNamespace := getOptionalString(refMap, "namespace", namespace); refNamespace != namespace {
				g.log.Debug("Skipping cross namespace http route backend", "httpRoute", route.GetName(),
					"service", name, "namespace", refNamespace)
				continue
			}
			port, ok := getNumber(refMap["port"])
			if !ok {
				g.log.Debug("Skipping http route backend without a port", "httpRoute", route.GetName(),
					"service", name)
				continue
			}
			backends = append(backends, httpRouteBackend{name: name, port: port})
		}
	}
	return backends
}

func getOptionalString(obj map[string]any, key, defaultValue string) string {
	value, ok := obj[key].(string)
	if !ok {
		return defaultValue
	}
	return value
}

// getNumber handles both representations, unstructured objects coming from crossplane are decoded
// from protobuf structs where every number is a float

func getNumber(value any) (int32, bool) {
	switch number := value.(type) {
	case int64:
		return int32(number), true
	case float64:
		return int32(number), true
	default:
		return 0, false
	}
}

func getNamespacedName(namespace, name string) string {
	return namespace + "/" + name
}
