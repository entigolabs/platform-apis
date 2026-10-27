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

type oracleInfra struct {
	cluster       *unstructured.Unstructured
	nodeNSGID     string
	dataKeyID     string
	subnetIDs     []string
	referencePool *unstructured.Unstructured
}

type oraclePoolSpec struct {
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

func GetOracleNodePoolKey(poolName string) string {
	return "nodepool-" + poolName
}

func getOracleInfra(required map[string][]resource.Required, env apis.Environment) (oracleInfra, error) {
	var infra oracleInfra
	cluster := firstRequired(required, ClusterKey)
	if cluster == nil {
		return infra, fmt.Errorf("%s not found in required resources", ClusterKey)
	}
	infra.cluster = cluster
	nsg := firstRequired(required, SecurityGroupKey)
	if nsg == nil {
		return infra, fmt.Errorf("%s not found in required resources", SecurityGroupKey)
	}
	infra.nodeNSGID, _, _ = unstructured.NestedString(nsg.Object, "status", "atProvider", "id")
	if key := firstRequired(required, KMSDataKeyKey); key != nil {
		infra.dataKeyID, _, _ = unstructured.NestedString(key.Object, "status", "atProvider", "id")
	}
	var subnets []*unstructured.Unstructured
	for _, subnet := range required[SubnetsKey] {
		if subnet.Resource != nil && subnet.Resource.GetLabels()[subnetTypeLabel] == env.ComputeSubnetType {
			subnets = append(subnets, subnet.Resource)
		}
	}
	slices.SortFunc(subnets, func(a, b *unstructured.Unstructured) int {
		return strings.Compare(a.GetName(), b.GetName())
	})
	for _, subnet := range subnets {
		if id, _, _ := unstructured.NestedString(subnet.Object, "status", "atProvider", "id"); id != "" {
			infra.subnetIDs = append(infra.subnetIDs, id)
		}
	}
	for _, name := range env.GetReferenceNodePools() {
		if pool := firstRequired(required, GetReferenceNodePoolKey(name)); pool != nil {
			infra.referencePool = pool
			break
		}
	}
	return infra, nil
}

func firstRequired(required map[string][]resource.Required, key string) *unstructured.Unstructured {
	if len(required[key]) == 0 {
		return nil
	}
	return required[key][0].Resource
}

func (g zoneGenerator) generateOracleNodePools() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	if len(g.zone.Spec.Pools) == 0 {
		return objs, nil
	}
	if g.oracle.referencePool == nil {
		return nil, fmt.Errorf("none of the reference node pools %v is observed",
			g.env.GetReferenceNodePools())
	}
	for _, pool := range g.zone.Spec.Pools {
		np, err := g.getOracleNodePool(pool)
		if err != nil {
			return nil, err
		}
		if np == nil {
			continue
		}
		objs[GetOracleNodePoolKey(pool.Name)] = np
	}
	return objs, nil
}

func parseOraclePool(pool v1alpha1.Pool) (oraclePoolSpec, error) {
	spec := oraclePoolSpec{minSize: 1, maxSize: -1}
	for _, requirement := range pool.Requirements {
		switch requirement.Key {
		case "instance-type":
			if len(requirement.Values) != 1 {
				return spec, fmt.Errorf("pool %s: an Oracle node pool takes exactly one instance-type", pool.Name)
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
		}
	}
	if spec.shape == "" {
		return spec, fmt.Errorf("pool %s: instance-type is required on Oracle", pool.Name)
	}
	if strings.HasSuffix(spec.shape, ".Flex") && spec.ocpus == nil {
		return spec, fmt.Errorf("pool %s: flexible shape %s requires %s", pool.Name, spec.shape, requirementOCPUs)
	}
	if spec.maxSize == -1 {
		spec.maxSize = spec.minSize
	}
	return spec, nil
}

func (g zoneGenerator) getOracleNodePool(pool v1alpha1.Pool) (*unstructured.Unstructured, error) {
	spec, err := parseOraclePool(pool)
	if err != nil {
		return nil, err
	}
	if spec.maxSize < 1 {
		return nil, nil
	}
	zoneName := g.zone.GetName()
	zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)
	ref := g.oracle.referencePool.Object

	image := spec.image
	if image == "" {
		image = nestedSliceString(ref, "imageId", "status", "atProvider", "nodeSourceDetails")
	}
	if image == "" {
		return nil, fmt.Errorf("pool %s: reference node pool %s has no image", pool.Name, g.oracle.referencePool.GetName())
	}
	placements, err := g.getOraclePlacements(pool.Name, spec)
	if err != nil {
		return nil, err
	}

	nsgIDs := []any{}
	if g.oracle.nodeNSGID != "" {
		nsgIDs = append(nsgIDs, g.oracle.nodeNSGID)
	}
	for _, id := range spec.nsgIDs {
		nsgIDs = append(nsgIDs, id)
	}
	nodeConfig := map[string]any{
		"nsgIds":           nsgIDs,
		"placementConfigs": placements,
	}
	if g.oracle.dataKeyID != "" {
		nodeConfig["kmsKeyId"] = g.oracle.dataKeyID
	}
	refNodeConfig := nestedSliceMap(ref, "status", "atProvider", "nodeConfigDetails")
	if podNetwork, ok := refNodeConfig["nodePoolPodNetworkOptionDetails"]; ok {
		nodeConfig["nodePoolPodNetworkOptionDetails"] = podNetwork
	}

	tags := map[string]any{}
	for key, value := range g.env.Tags {
		if value != nil {
			tags[key] = *value
		}
	}
	for key, value := range base.GetObjectTags(&g.zone) {
		tags[key] = value
	}
	tags[base.TenancyZoneLabel] = zoneName
	tags[zonePoolAnnotation] = zonePool
	tags["cluster-autoscaler"] = "enabled"
	tags["minSize"] = strconv.FormatFloat(spec.minSize, 'f', -1, 64)
	tags["maxSize"] = strconv.FormatFloat(spec.maxSize, 'f', -1, 64)

	forProvider := map[string]any{
		"name":              getOracleNodePoolName(zonePool),
		"clusterId":         nestedString(g.oracle.cluster.Object, "status", "atProvider", "id"),
		"compartmentId":     nestedString(g.oracle.cluster.Object, "status", "atProvider", "compartmentId"),
		"kubernetesVersion": nestedString(g.oracle.cluster.Object, "status", "atProvider", "kubernetesVersion"),
		"nodeShape":         spec.shape,
		"nodeSourceDetails": []any{map[string]any{
			"sourceType":          "IMAGE",
			"imageId":             image,
			"bootVolumeSizeInGbs": ociBootVolumeSizeInGbs,
		}},
		"nodeConfigDetails": []any{nodeConfig},
		"initialNodeLabels": []any{
			map[string]any{"key": base.TenancyZoneLabel, "value": zoneName},
			map[string]any{"key": zonePoolAnnotation, "value": zonePool},
		},
		"freeformTags": tags,
	}
	if spec.ocpus != nil || spec.memoryGB != nil {
		shapeConfig := map[string]any{}
		if spec.ocpus != nil {
			shapeConfig["ocpus"] = *spec.ocpus
		}
		if spec.memoryGB != nil {
			shapeConfig["memoryInGbs"] = *spec.memoryGB
		}
		forProvider["nodeShapeConfig"] = []any{shapeConfig}
	}

	annotations := map[string]any{zonePoolAnnotation: zonePool}
	for key, value := range g.zoneAnnotations {
		annotations[key] = value
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "containerengine.oci.upbound.io/v1alpha1",
		"kind":       "NodePool",
		"metadata": map[string]any{
			"name":        zonePool,
			"annotations": annotations,
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

func (g zoneGenerator) getOraclePlacements(poolName string, spec oraclePoolSpec) ([]any, error) {
	if len(g.oracle.subnetIDs) == 0 {
		return nil, fmt.Errorf("pool %s: no %s subnets observed", poolName, g.env.ComputeSubnetType)
	}
	var domains []string
	refNodeConfig := nestedSliceMap(g.oracle.referencePool.Object, "status", "atProvider", "nodeConfigDetails")
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
	if len(domains) == 0 {
		return nil, fmt.Errorf("pool %s: no availability domain of reference node pool %s matches the pool",
			poolName, g.oracle.referencePool.GetName())
	}
	slices.Sort(domains)
	var placements []any
	for i, domain := range domains {
		placement := map[string]any{
			"availabilityDomain": domain,
			"subnetId":           g.oracle.subnetIDs[i%len(g.oracle.subnetIDs)],
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

func getOracleNodePoolName(zonePool string) string {
	if len(zonePool) <= ociNodePoolNameMaxLength {
		return zonePool
	}
	hash := sha256.Sum256([]byte(zonePool))
	suffix := hex.EncodeToString(hash[:])[:8]
	prefix := strings.TrimRight(zonePool[:ociNodePoolNameMaxLength-len(suffix)-1], "-")
	return prefix + "-" + suffix
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
