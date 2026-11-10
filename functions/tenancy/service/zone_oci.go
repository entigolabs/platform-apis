package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	TemplateNodePoolKey  = "TemplateNodePool"
	TemplateNodePoolName = "template"

	ociNodePoolNameMaxLength = 32
	ociBootVolumeSizeInGbs   = "50"

	requirementOCPUs    = "oci/ocpus"
	requirementMemoryGB = "oci/memory-gb"
	requirementImage    = "oci/image"

	ociIngressClassParamsGroup = "ingress.oraclecloud.com"
	ociIngressClassParamsKind  = "IngressClassParameters"
	gatewayNameLabel           = "gateway.networking.k8s.io/gateway-name"

	// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
	RoleContributor = "contributor"
	RoleMaintainer  = "maintainer"
	RoleObserver    = "observer"
	// crossplaneViewClusterRole is aggregated by the Crossplane rbac-manager from every installed
	// provider, so bound in a Namespace it reads the managed resources the platform APIs compose
	// there (kms, rds, ec2, ...) without the Zone having to know their API groups. Nothing is granted
	// while the rbac-manager is disabled, as the ClusterRole then does not exist.
	crossplaneViewClusterRole = "crossplane-view"
)

type ociZoneGenerator struct {
	zoneGenerator
	cluster      *unstructured.Unstructured
	nodeNSGID    string
	dataKeyID    string
	subnetIDs    []string
	templatePool *unstructured.Unstructured
	// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
	roleMappings map[string][]string
}

type ociPoolSpec struct {
	shape       string
	ocpus       *float64
	memoryGB    *float64
	image       string
	preemptible bool
	zones       base.Set[string]
	nsgIDs      []string
	minSize     float64
	maxSize     float64
}

func GetOCINodePoolKey(poolName string) string {
	return "nodepool-" + poolName
}

func newOCIZoneGenerator(common zoneGenerator) (ociZoneGenerator, error) {
	g := ociZoneGenerator{zoneGenerator: common, roleMappings: mergeRoleMappings(common.zone, common.env)}
	g.cluster = firstRequired(g.required, ClusterKey)
	if g.cluster == nil {
		return g, fmt.Errorf("%s not found in required resources", ClusterKey)
	}
	nsg := firstRequired(g.required, SecurityGroupKey)
	if nsg == nil {
		return g, fmt.Errorf("%s not found in required resources", SecurityGroupKey)
	}
	g.nodeNSGID = nestedString(nsg.Object, "status", "atProvider", "id")
	if key := firstRequired(g.required, KMSDataKeyKey); key != nil {
		g.dataKeyID = nestedString(key.Object, "status", "atProvider", "id")
	}
	g.subnetIDs = g.getComputeSubnetIDs()
	g.templatePool = firstRequired(g.required, TemplateNodePoolKey)
	return g, nil
}

func (g ociZoneGenerator) getComputeSubnetIDs() []string {
	var subnets []*unstructured.Unstructured
	for _, subnet := range g.required[SubnetsKey] {
		if subnet.Resource != nil && subnet.Resource.GetLabels()[subnetTypeLabel] == g.env.ComputeSubnetType {
			subnets = append(subnets, subnet.Resource)
		}
	}
	slices.SortFunc(subnets, func(a, b *unstructured.Unstructured) int {
		return strings.Compare(a.GetName(), b.GetName())
	})
	var ids []string
	for _, subnet := range subnets {
		if id := nestedString(subnet.Object, "status", "atProvider", "id"); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func firstRequired(required map[string][]resource.Required, key string) *unstructured.Unstructured {
	if len(required[key]) == 0 {
		return nil
	}
	return required[key][0].Resource
}

func (g ociZoneGenerator) generate() (map[string]client.Object, error) {
	objs, err := g.generateCommon()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, g.generateLegacyRBAC())
	nodePools, err := g.generateNodePools()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, nodePools)
	networkPolicies, err := g.generateTargetNetworkPolicies()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, networkPolicies)
	return objs, nil
}

func (g ociZoneGenerator) generateNodePools() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	if len(g.zone.Spec.Pools) == 0 {
		return objs, nil
	}
	if g.templatePool == nil {
		return nil, fmt.Errorf("NodePool %s not found in required resources", TemplateNodePoolName)
	}
	for _, pool := range g.zone.Spec.Pools {
		np, err := g.getNodePool(pool)
		if err != nil {
			return nil, err
		}
		if np == nil {
			continue
		}
		objs[GetOCINodePoolKey(pool.Name)] = np
	}
	return objs, nil
}

func parseOCIPool(pool v1alpha1.Pool) (ociPoolSpec, error) {
	spec := ociPoolSpec{minSize: 1, maxSize: -1}
	for _, requirement := range pool.Requirements {
		switch requirement.Key {
		case "instance-type":
			if len(requirement.Values) != 1 {
				return spec, fmt.Errorf("pool %s: an OCI node pool takes exactly one instance-type", pool.Name)
			}
			spec.shape = requirement.Values[0]
		case "zone":
			spec.zones = base.ToSet(requirement.Values)
		case "capacity-type":
			spec.preemptible = requirement.Value.String() == "SPOT"
		case "min-size":
			val, err := getIntOrFloat(requirement.Value)
			if err != nil {
				return spec, err
			}
			spec.minSize = val
		case "max-size":
			val, err := getIntOrFloat(requirement.Value)
			if err != nil {
				return spec, err
			}
			spec.maxSize = val
		case "security-groups":
			spec.nsgIDs = append(spec.nsgIDs, requirement.Values...)
		case requirementOCPUs:
			val, err := getIntOrFloat(requirement.Value)
			if err != nil {
				return spec, err
			}
			spec.ocpus = &val
		case requirementMemoryGB:
			val, err := getIntOrFloat(requirement.Value)
			if err != nil {
				return spec, err
			}
			spec.memoryGB = &val
		case requirementImage:
			spec.image = requirement.Value.String()
		default:
			return spec, fmt.Errorf("pool %s: requirement %s is not supported on OCI", pool.Name, requirement.Key)
		}
	}
	if spec.shape == "" {
		return spec, fmt.Errorf("pool %s: instance-type is required on OCI", pool.Name)
	}
	if strings.HasSuffix(spec.shape, ".Flex") && spec.ocpus == nil {
		return spec, fmt.Errorf("pool %s: flexible shape %s requires %s", pool.Name, spec.shape, requirementOCPUs)
	}
	if spec.maxSize == -1 {
		spec.maxSize = spec.minSize
	}
	return spec, nil
}

func (g ociZoneGenerator) getNodePool(pool v1alpha1.Pool) (*unstructured.Unstructured, error) {
	spec, err := parseOCIPool(pool)
	if err != nil {
		return nil, err
	}
	if spec.maxSize < 1 {
		return nil, nil
	}
	zonePool := fmt.Sprintf("%s-%s", g.zone.GetName(), pool.Name)
	nodeSource, err := g.getNodeSource(pool.Name, spec)
	if err != nil {
		return nil, err
	}
	nodeConfig, err := g.getNodeConfig(pool.Name, spec)
	if err != nil {
		return nil, err
	}
	forProvider := map[string]any{
		"name":              getOCINodePoolName(zonePool),
		"clusterId":         nestedString(g.cluster.Object, "status", "atProvider", "id"),
		"compartmentId":     nestedString(g.cluster.Object, "status", "atProvider", "compartmentId"),
		"kubernetesVersion": nestedString(g.cluster.Object, "status", "atProvider", "kubernetesVersion"),
		"nodeShape":         spec.shape,
		"nodeSourceDetails": []any{nodeSource},
		"nodeConfigDetails": []any{nodeConfig},
		"initialNodeLabels": toOCINodeLabels(g.getPoolLabels(zonePool)),
		"freeformTags":      g.getNodePoolTags(zonePool, spec),
	}
	if shapeConfig := getShapeConfig(spec); shapeConfig != nil {
		forProvider["nodeShapeConfig"] = []any{shapeConfig}
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "containerengine.oci.upbound.io/v1alpha1",
		"kind":       "NodePool",
		"metadata": map[string]any{
			"name":        zonePool,
			"annotations": g.getNodePoolAnnotations(pool.Name, zonePool),
		},
		"spec": map[string]any{
			"managementPolicies": []any{"Observe", "Create", "Update", "Delete"},
			"providerConfigRef":  map[string]any{"name": g.env.OCIProvider},
			"forProvider":        forProvider,
			"initProvider": map[string]any{
				"nodeConfigDetails": []any{map[string]any{"size": spec.minSize}},
			},
		},
	}}, nil
}

func (g ociZoneGenerator) getNodeSource(poolName string, spec ociPoolSpec) (map[string]any, error) {
	image := spec.image
	if image == "" {
		image = nestedSliceString(g.templatePool.Object, "imageId", "status", "atProvider", "nodeSourceDetails")
	}
	if image == "" {
		return nil, fmt.Errorf("pool %s: template node pool %s has no image", poolName, g.templatePool.GetName())
	}
	return map[string]any{
		"sourceType":          "IMAGE",
		"imageId":             image,
		"bootVolumeSizeInGbs": ociBootVolumeSizeInGbs,
	}, nil
}

func getShapeConfig(spec ociPoolSpec) map[string]any {
	if spec.ocpus == nil && spec.memoryGB == nil {
		return nil
	}
	shapeConfig := map[string]any{}
	if spec.ocpus != nil {
		shapeConfig["ocpus"] = *spec.ocpus
	}
	if spec.memoryGB != nil {
		shapeConfig["memoryInGbs"] = *spec.memoryGB
	}
	return shapeConfig
}

func (g ociZoneGenerator) getNodeConfig(poolName string, spec ociPoolSpec) (map[string]any, error) {
	placements, err := g.getPlacements(poolName, spec)
	if err != nil {
		return nil, err
	}
	nsgIDs := []any{}
	if g.nodeNSGID != "" {
		nsgIDs = append(nsgIDs, g.nodeNSGID)
	}
	for _, id := range spec.nsgIDs {
		nsgIDs = append(nsgIDs, id)
	}
	nodeConfig := map[string]any{
		"nsgIds":           nsgIDs,
		"placementConfigs": placements,
	}
	if g.dataKeyID != "" {
		nodeConfig["kmsKeyId"] = g.dataKeyID
	}
	templateNodeConfig := nestedSliceMap(g.templatePool.Object, "status", "atProvider", "nodeConfigDetails")
	if podNetwork, ok := templateNodeConfig["nodePoolPodNetworkOptionDetails"]; ok {
		nodeConfig["nodePoolPodNetworkOptionDetails"] = podNetwork
	}
	return nodeConfig, nil
}

func (g ociZoneGenerator) getPlacements(poolName string, spec ociPoolSpec) ([]any, error) {
	if len(g.subnetIDs) == 0 {
		return nil, fmt.Errorf("pool %s: no %s subnets observed", poolName, g.env.ComputeSubnetType)
	}
	domains := g.getAvailabilityDomains(spec)
	if len(domains) == 0 {
		return nil, fmt.Errorf("pool %s: no availability domain of template node pool %s matches the pool",
			poolName, g.templatePool.GetName())
	}
	var placements []any
	for i, domain := range domains {
		placement := map[string]any{
			"availabilityDomain": domain,
			"subnetId":           g.subnetIDs[i%len(g.subnetIDs)],
		}
		if spec.preemptible {
			placement["preemptibleNodeConfig"] = []any{map[string]any{
				"preemptionAction": []any{map[string]any{
					"type":                 "TERMINATE",
					"isPreserveBootVolume": false,
				}},
			}}
		}
		placements = append(placements, placement)
	}
	return placements, nil
}

func (g ociZoneGenerator) getAvailabilityDomains(spec ociPoolSpec) []string {
	var domains []string
	templateNodeConfig := nestedSliceMap(g.templatePool.Object, "status", "atProvider", "nodeConfigDetails")
	refPlacements, _ := templateNodeConfig["placementConfigs"].([]any)
	for _, placement := range refPlacements {
		p, ok := placement.(map[string]any)
		if !ok {
			continue
		}
		domain, _ := p["availabilityDomain"].(string)
		if domain == "" || slices.Contains(domains, domain) {
			continue
		}
		if spec.zones.Size() > 0 && !spec.zones.Contains(domain) {
			continue
		}
		domains = append(domains, domain)
	}
	slices.Sort(domains)
	return domains
}

func (g ociZoneGenerator) getNodePoolTags(zonePool string, spec ociPoolSpec) map[string]any {
	tags := map[string]any{}
	for key, value := range g.env.Tags {
		if value != nil {
			tags[ociTagKey(key)] = *value
		}
	}
	for key, value := range base.GetObjectTags(&g.zone) {
		tags[ociTagKey(key)] = value
	}
	tags[ociTagKey(base.TenancyZoneLabel)] = g.zone.GetName()
	tags[ociTagKey(zonePoolAnnotation)] = zonePool
	tags["cluster-autoscaler"] = "enabled"
	tags["minSize"] = strconv.FormatFloat(spec.minSize, 'f', -1, 64)
	tags["maxSize"] = strconv.FormatFloat(spec.maxSize, 'f', -1, 64)
	return tags
}

func (g ociZoneGenerator) getNodePoolAnnotations(poolName, zonePool string) map[string]any {
	annotations := map[string]any{}
	for key, value := range g.getPoolAnnotations(zonePool) {
		annotations[key] = value
	}
	if id := g.getObservedOCID(GetOCINodePoolKey(poolName)); id != "" {
		annotations[externalNameAnnotation] = id
	}
	return annotations
}

func toOCINodeLabels(labels map[string]string) []any {
	var nodeLabels []any
	for _, key := range slices.Sorted(maps.Keys(labels)) {
		nodeLabels = append(nodeLabels, map[string]any{"key": key, "value": labels[key]})
	}
	return nodeLabels
}

func (g ociZoneGenerator) getObservedOCID(key string) string {
	observed, ok := g.observed[resource.Name(key)]
	if !ok || observed.Resource == nil {
		return ""
	}
	if name := observed.Resource.GetAnnotations()[externalNameAnnotation]; name != "" {
		return name
	}
	return nestedString(observed.Resource.Object, "status", "atProvider", "id")
}

func getOCINodePoolName(zonePool string) string {
	if len(zonePool) <= ociNodePoolNameMaxLength {
		return zonePool
	}
	hash := sha256.Sum256([]byte(zonePool))
	suffix := hex.EncodeToString(hash[:])[:8]
	prefix := strings.TrimRight(zonePool[:ociNodePoolNameMaxLength-len(suffix)-1], "-")
	return prefix + "-" + suffix
}

func ociTagKey(key string) string {
	return strings.Map(func(r rune) rune {
		if r == '.' || r == '/' || r == ' ' {
			return '-'
		}
		return r
	}, key)
}

func nestedString(obj map[string]any, fields ...string) string {
	value, _, _ := unstructured.NestedString(obj, fields...)
	return value
}

func nestedSliceMap(obj map[string]any, fields ...string) map[string]any {
	items, _, _ := unstructured.NestedSlice(obj, fields...)
	if len(items) == 0 {
		return nil
	}
	item, _ := items[0].(map[string]any)
	return maps.Clone(item)
}

func nestedSliceString(obj map[string]any, key string, fields ...string) string {
	value, _ := nestedSliceMap(obj, fields...)[key].(string)
	return value
}

func (g ociZoneGenerator) generateTargetNetworkPolicies() (map[string]client.Object, error) {
	classBlocks := g.getIngressClassBlocks()
	gatewayBlocks := g.getGatewayBlocks()
	objs := make(map[string]client.Object)
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
			if ingress.Spec.IngressClassName == nil {
				continue
			}
			blocks := classBlocks[*ingress.Spec.IngressClassName]
			if len(blocks) == 0 {
				continue
			}
			for _, rule := range ingress.Spec.Rules {
				if rule.HTTP == nil {
					continue
				}
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service == nil {
						continue
					}
					addTargetNetworkPolicy(objs, ns, ingress.Name, path.Backend.Service.Name,
						path.Backend.Service.Port.Name, path.Backend.Service.Port.Number, services, blocks)
				}
			}
		}
		for _, route := range g.required[ns+HTTPRouteKey] {
			blocks := g.getRouteBlocks(route.Resource, gatewayBlocks, ns)
			if len(blocks) == 0 {
				continue
			}
			for _, backend := range g.getHTTPRouteBackends(route.Resource, ns) {
				addTargetNetworkPolicy(objs, ns, route.Resource.GetName(), backend.name,
					"", backend.port, services, blocks)
			}
		}
	}
	return objs, nil
}

// getIngressClassBlocks resolves ingress class name -> network policy peers by following
// IngressClass.spec.parameters -> IngressClassParameters.spec.subnetId -> Subnet id. The native
// ingress controller's load balancer sends traffic to the pods from that subnet.
func (g ociZoneGenerator) getIngressClassBlocks() map[string][]networkingv1.NetworkPolicyPeer {
	paramSubnetIDs := make(map[string]string)
	for _, params := range g.required[IngressClassParamsKey] {
		if params.Resource == nil {
			continue
		}
		if id := nestedString(params.Resource.Object, "spec", "subnetId"); id != "" {
			paramSubnetIDs[getNamespacedName(params.Resource.GetNamespace(), params.Resource.GetName())] = id
		}
	}
	subnetCIDRs := make(map[string]string)
	for _, subnet := range g.required[SubnetsKey] {
		if subnet.Resource == nil {
			continue
		}
		id := nestedString(subnet.Resource.Object, "status", "atProvider", "id")
		if cidr := nestedString(subnet.Resource.Object, "status", "atProvider", "cidrBlock"); id != "" && cidr != "" {
			subnetCIDRs[id] = cidr
		}
	}
	classBlocks := make(map[string][]networkingv1.NetworkPolicyPeer)
	for _, ingressClass := range g.ingressClasses {
		params := ingressClass.Spec.Parameters
		if params == nil || params.APIGroup == nil || *params.APIGroup != ociIngressClassParamsGroup ||
			!strings.EqualFold(params.Kind, ociIngressClassParamsKind) || params.Namespace == nil {
			continue
		}
		cidr := subnetCIDRs[paramSubnetIDs[getNamespacedName(*params.Namespace, params.Name)]]
		if cidr == "" {
			g.log.Debug("No subnet resolved for ingress class", "ingressClass", ingressClass.Name,
				"ingressClassParameters", params.Name)
			continue
		}
		classBlocks[ingressClass.Name] = []networkingv1.NetworkPolicyPeer{{
			IPBlock: &networkingv1.IPBlock{CIDR: cidr},
		}}
	}
	return classBlocks
}

// getGatewayBlocks resolves gateway namespace/name -> network policy peers. Oracle gateways run
// in-cluster behind a native ingress load balancer, so routed traffic comes from the gateway's pods.
func (g ociZoneGenerator) getGatewayBlocks() map[string][]networkingv1.NetworkPolicyPeer {
	gatewayBlocks := make(map[string][]networkingv1.NetworkPolicyPeer)
	for _, gateway := range g.required[GatewayKey] {
		if gateway.Resource == nil {
			continue
		}
		namespace := gateway.Resource.GetNamespace()
		name := gateway.Resource.GetName()
		gatewayBlocks[getNamespacedName(namespace, name)] = []networkingv1.NetworkPolicyPeer{{
			NamespaceSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				corev1.LabelMetadataName: namespace,
			}},
			PodSelector: &metav1.LabelSelector{MatchLabels: map[string]string{
				gatewayNameLabel: name,
			}},
		}}
	}
	return gatewayBlocks
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) generateLegacyRBAC() map[string]client.Object {
	objs := make(map[string]client.Object)
	if crb := g.getClusterRoleBinding(RoleContributor); crb != nil {
		objs[GetClusterRoleBindingKey(g.zone.Name, RoleContributor)] = crb
	}
	appsNamespace := ""
	if g.env.CreateAppsNamespace {
		appsNamespace = g.zone.Name + "-apps"
		g.addAppsNamespaceRoles(objs, appsNamespace)
	}
	for _, name := range g.uqNamespaces {
		if name != appsNamespace {
			g.addNamespaceRoles(objs, name)
		}
	}
	return objs
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) addNamespaceRoles(objs map[string]client.Object, name string) {
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
	for role, groups := range g.roleMappings {
		if role == RoleObserver {
			continue // the read Role already covers every kind in the Namespace
		}
		objs[GetRBKey(g.zone.Name, name, role+"-"+crossplaneViewClusterRole)] =
			g.getClusterRoleRoleBinding(name, name+"-"+role+"-"+crossplaneViewClusterRole, crossplaneViewClusterRole, groups)
	}
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) addAppsNamespaceRoles(objs map[string]client.Object, name string) {
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

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
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

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func mergeContributorGroups(zone v1alpha1.Zone, groups []string) []string {
	if zone.Spec.AppProject == nil || len(zone.Spec.AppProject.ContributorGroups) == 0 {
		return groups
	}
	return append(groups, zone.Spec.AppProject.ContributorGroups...)
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func GetRBACRoleAllKey(zone, namespace string) string {
	return "rbacrole-all-" + zone + "-" + namespace
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func GetRBACRoleReadKey(zone, namespace string) string {
	return "rbacrole-read-" + zone + "-" + namespace
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func GetRBACRoleNamespaceKey(zone, namespace string) string {
	return "rbacrole-namespace-" + zone + "-" + namespace
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func GetRBKey(zone, namespace, roleRef string) string {
	return "rb-" + roleRef + "-" + zone + "-" + namespace
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func GetClusterRoleBindingKey(zone, role string) string {
	return "crb-" + zone + "-" + role
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) getAllRole(nsName string) *rbacv1.Role {
	rules := slices.Concat(g.env.NamespaceRole.Rules, g.env.NamespaceRole.AdditionalRules)
	return g.getRole(nsName, nsName+"-all", rules)
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
// getNamespaceRole lets maintainers manage the zone namespace object itself.
func (g ociZoneGenerator) getNamespaceRole(nsName string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{{
		APIGroups:     []string{""},
		Resources:     []string{"namespaces"},
		ResourceNames: []string{nsName},
		Verbs:         []string{"update", "patch", "delete"},
	}}
	return g.getRole(nsName, nsName+"-namespace", rules)
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) getReadRole(nsName string) *rbacv1.Role {
	rules := []rbacv1.PolicyRule{{
		APIGroups: []string{"*"},
		Resources: []string{"*"},
		Verbs:     []string{"get", "watch", "list"},
	}}
	return g.getRole(nsName, nsName+"-read", rules)
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) getRole(nsName, roleName string, rules []rbacv1.PolicyRule) *rbacv1.Role {
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

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) getRoleBinding(nsName, bindingName, roleName string, groups []string) *rbacv1.RoleBinding {
	return g.getRoleBindingRef(nsName, bindingName, "Role", roleName, groups)
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
// getClusterRoleRoleBinding binds a ClusterRole in one Namespace, which scopes its rules to that Namespace.
func (g ociZoneGenerator) getClusterRoleRoleBinding(nsName, bindingName, clusterRoleName string, groups []string) *rbacv1.RoleBinding {
	return g.getRoleBindingRef(nsName, bindingName, "ClusterRole", clusterRoleName, groups)
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) getRoleBindingRef(nsName, bindingName, roleKind, roleName string, groups []string) *rbacv1.RoleBinding {
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
			Kind:     roleKind,
			Name:     roleName,
		},
		Subjects: subjects,
	}
}

// TODO: remove once every cloud is on RoleDefinition/RoleGrant; only non-AWS clouds still use this
func (g ociZoneGenerator) getClusterRoleBinding(role string) client.Object {
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
