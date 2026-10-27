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
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	referenceNodePoolKeyPrefix = "ReferenceNodePool-"

	ociNodePoolNameMaxLength = 32
	ociBootVolumeSizeInGbs   = "50"

	requirementOCPUs    = "oci/ocpus"
	requirementMemoryGB = "oci/memory-gb"
	requirementImage    = "oci/image"
)

type ociZoneGenerator struct {
	zoneGenerator
	cluster       *unstructured.Unstructured
	nodeNSGID     string
	dataKeyID     string
	subnetIDs     []string
	referencePool *unstructured.Unstructured
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

func GetReferenceNodePoolKey(name string) string {
	return referenceNodePoolKeyPrefix + name
}

func GetOCINodePoolKey(poolName string) string {
	return "nodepool-" + poolName
}

func newOCIZoneGenerator(common zoneGenerator) (ociZoneGenerator, error) {
	g := ociZoneGenerator{zoneGenerator: common}
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
	for _, name := range g.env.GetReferenceNodePools() {
		if pool := firstRequired(g.required, GetReferenceNodePoolKey(name)); pool != nil {
			g.referencePool = pool
			break
		}
	}
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
	nodePools, err := g.generateNodePools()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, nodePools)
	return objs, nil
}

func (g ociZoneGenerator) generateNodePools() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	if len(g.zone.Spec.Pools) == 0 {
		return objs, nil
	}
	if g.referencePool == nil {
		return nil, fmt.Errorf("none of the reference node pools %v is observed",
			g.env.GetReferenceNodePools())
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
			"providerConfigRef":  map[string]any{"name": g.env.OracleProvider},
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
		image = nestedSliceString(g.referencePool.Object, "imageId", "status", "atProvider", "nodeSourceDetails")
	}
	if image == "" {
		return nil, fmt.Errorf("pool %s: reference node pool %s has no image", poolName, g.referencePool.GetName())
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
	refNodeConfig := nestedSliceMap(g.referencePool.Object, "status", "atProvider", "nodeConfigDetails")
	if podNetwork, ok := refNodeConfig["nodePoolPodNetworkOptionDetails"]; ok {
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
		return nil, fmt.Errorf("pool %s: no availability domain of reference node pool %s matches the pool",
			poolName, g.referencePool.GetName())
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
	refNodeConfig := nestedSliceMap(g.referencePool.Object, "status", "atProvider", "nodeConfigDetails")
	refPlacements, _ := refNodeConfig["placementConfigs"].([]any)
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
