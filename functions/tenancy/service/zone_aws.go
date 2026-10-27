package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"strconv"
	"strings"

	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	ec2v1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/ec2/v1beta1"
	eksv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/eks/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/iam/v1beta1"
	kmsv1beta1 "github.com/upbound/provider-aws/v2/apis/cluster/kms/v1beta1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type awsZoneGenerator struct {
	zoneGenerator
	vpc            ec2v1beta1.VPC
	kmsDataAlias   kmsv1beta1.Alias
	securityGroup  ec2v1beta1.SecurityGroup
	cluster        eksv1beta1.Cluster
	subnets        []*ec2v1beta1.Subnet
	computeSubnets []*ec2v1beta1.Subnet
}

func newAWSZoneGenerator(common zoneGenerator) (awsZoneGenerator, error) {
	g := awsZoneGenerator{zoneGenerator: common}
	if err := base.ExtractRequiredResource(g.required, VPCKey, &g.vpc); err != nil {
		return g, err
	}
	if _, err := base.ExtractOptionalResource(g.required, KMSDataAliasKey, &g.kmsDataAlias); err != nil {
		return g, err
	}
	if err := base.ExtractRequiredResource(g.required, SecurityGroupKey, &g.securityGroup); err != nil {
		return g, err
	}
	if err := base.ExtractRequiredResource(g.required, ClusterKey, &g.cluster); err != nil {
		return g, err
	}
	subnets, err := base.ExtractResources[*ec2v1beta1.Subnet](g.required, SubnetsKey)
	if err != nil {
		return g, err
	}
	g.subnets = subnets
	g.computeSubnets = filterSubnetsByType(subnets, g.env.ComputeSubnetType)
	return g, nil
}

func (g awsZoneGenerator) generate() (map[string]client.Object, error) {
	objs, err := g.generateCommon()
	if err != nil {
		return nil, err
	}
	maps.Copy(objs, g.generateLaunchTemplates())
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

const albActionAnnotationPrefix = "alb.ingress.kubernetes.io/actions."

type albAction struct {
	Type          string            `json:"type"`
	ForwardConfig *albForwardConfig `json:"forwardConfig"`
}

type albForwardConfig struct {
	TargetGroups []albTargetGroup `json:"targetGroups"`
}

type albTargetGroup struct {
	ServiceName string             `json:"serviceName"`
	ServicePort intstr.IntOrString `json:"servicePort"`
}

type httpRouteBackend struct {
	name string
	port int32
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

func GetTargetNetworkPolicyKey(namespace, ingress, service string, port intstr.IntOrString) string {
	return "netpol-" + namespace + "-" + ingress + "-" + service + "-" + port.String()
}

func (g awsZoneGenerator) generateLaunchTemplates() map[string]client.Object {
	objs := make(map[string]client.Object)
	zoneName := g.zone.GetName()
	labels := map[string]string{
		base.TenancyZoneLabel: zoneName,
	}
	emptyString := ""
	ZoneResourceTags := base.GetObjectTags(&g.zone)

	for _, pool := range g.zone.Spec.Pools {
		zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)
		annotations := g.getPoolAnnotations(zonePool)
		tags := make(map[string]*string)
		maps.Copy(tags, g.env.Tags)
		tags[base.TenancyZoneLabel] = &zoneName
		tags[zonePoolAnnotation] = &zonePool
		instanceTags := make(map[string]*string)
		maps.Copy(instanceTags, tags)
		instanceTags["Name"] = &zonePool
		instanceTags[base.TenancyZoneAWSTag] = &g.zone.Name
		volumeTags := make(map[string]*string)
		maps.Copy(volumeTags, tags)
		volumeTags["Name"] = base.StringPtr(zonePool + "-root")
		volumeTags[base.TenancyZoneAWSTag] = &g.zone.Name
		for key, value := range ZoneResourceTags {
			instanceTags[key] = &value
			volumeTags[key] = &value
		}

		var securityGroupIds []*string
		if g.securityGroup.Status.AtProvider.ID != nil {
			securityGroupIds = []*string{g.securityGroup.Status.AtProvider.ID}
		}
		for _, requirement := range pool.Requirements {
			switch requirement.Key {
			case "security-groups":
				for _, sg := range requirement.Values {
					securityGroupIds = append(securityGroupIds, &sg)
				}
			}
		}

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
					VPCSecurityGroupIds: securityGroupIds,
					Tags:                tags,
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

func (g awsZoneGenerator) generateNodePools() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	objs[GetRoleKey(g.zone.Name)] = g.getIAMRole()
	objs[GetRoleWNAttachmentKey(g.zone.Name)] = g.getIAMRolePolicyAttachment(g.zone.Name+"-wn",
		"arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy")
	objs[GetRoleECRROAttachmentKey(g.zone.Name)] = g.getIAMRolePolicyAttachment(g.zone.Name+"-ecr-ro",
		"arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly")
	objs[GetRoleSSMAttachmentKey(g.zone.Name)] = g.getIAMRolePolicyAttachment(g.zone.Name+"-ssm",
		"arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore")
	objs[GetRoleECRProxyAttachmentKey(g.zone.Name)] = g.getIAMRolePolicyAttachmentWithArnRef(g.zone.Name+"-ecr-proxy",
		"ecr-proxy")
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

func (g awsZoneGenerator) getIAMRole() *iamv1beta1.Role {
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

func (g awsZoneGenerator) getIAMRolePolicyAttachment(name, policyArn string) *iamv1beta1.RolePolicyAttachment {
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

func (g awsZoneGenerator) getIAMRolePolicyAttachmentWithArnRef(name, policyArnRef string) *iamv1beta1.RolePolicyAttachment {
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

func (g awsZoneGenerator) getAccessEntry(name string) client.Object {
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

func (g awsZoneGenerator) getNodeGroup(pool v1alpha1.Pool, launchTemplateObj ec2v1beta1.LaunchTemplate) (string, client.Object, error) {
	zoneName := g.zone.GetName()
	zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)
	version := strconv.FormatFloat(*launchTemplateObj.Status.AtProvider.LatestVersion, 'f', -1, 64)

	var instanceTypes []string
	var zoneFilter base.Set[string]
	var capacityType = "ON_DEMAND"
	var minSize float64 = 1
	var maxSize float64 = -1
	var amiType *string

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
		case "ami-type":
			value := requirement.Value.String()
			amiType = &value
		}
	}

	if maxSize == -1 {
		maxSize = minSize
	}
	if maxSize < 1 {
		return "", nil, nil
	}

	hash := GetInstanceTypesHash(instanceTypes, capacityType)
	name := fmt.Sprintf("%s-%s", zonePool, hash)

	var subnetIds []*string
	for _, subnet := range g.computeSubnets {
		subnetId := subnet.Status.AtProvider.ID
		if subnetId == nil {
			continue
		}
		if zoneFilter.Size() > 0 {
			if subnet.Status.AtProvider.AvailabilityZone != nil && zoneFilter.Contains(*subnet.Status.AtProvider.AvailabilityZone) {
				subnetIds = append(subnetIds, subnetId)
			}
		} else {
			subnetIds = append(subnetIds, subnetId)
		}
	}

	annotations := g.getPoolAnnotations(zonePool)
	tags := map[string]*string{
		base.TenancyZoneLabel: &zoneName,
		zonePoolAnnotation:    &zonePool,
	}
	maps.Copy(tags, g.env.Tags)
	eksLabels := make(map[string]*string)
	for key, value := range g.getPoolLabels(zonePool) {
		eksLabels[key] = &value
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
				AMIType:       amiType,
				InstanceTypes: toInstanceTypePointers(instanceTypes),
				SubnetIds:     subnetIds,
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

func toInstanceTypePointers(types []string) []*string {
	ptrTypes := make([]*string, len(types))
	for i, t := range types {
		ptrTypes[i] = &t
	}
	return ptrTypes
}

func (g awsZoneGenerator) generateTargetNetworkPolicies() (map[string]client.Object, error) {
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
					addTargetNetworkPolicy(objs, ns, ingress.Name, path.Backend.Service.Name,
						path.Backend.Service.Port.Name, path.Backend.Service.Port.Number, services, blocks)
				}
			}
			for _, tg := range g.collectAlbActionTargetGroups(ingress) {
				portName, portNumber, ok := albServicePortRef(tg.ServicePort)
				if !ok {
					continue
				}
				addTargetNetworkPolicy(objs, ns, ingress.Name, tg.ServiceName,
					portName, portNumber, services, blocks)
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

func (g awsZoneGenerator) collectAlbActionTargetGroups(ingress *networkingv1.Ingress) []albTargetGroup {
	var tgs []albTargetGroup
	for key, value := range ingress.Annotations {
		if !strings.HasPrefix(key, albActionAnnotationPrefix) {
			continue
		}
		var action albAction
		if err := json.Unmarshal([]byte(value), &action); err != nil {
			g.log.Debug("Failed to parse ALB action annotation", "ingress", ingress.Name, "key", key, "error", err)
			continue
		}
		if action.Type != "forward" || action.ForwardConfig == nil {
			continue
		}
		for _, tg := range action.ForwardConfig.TargetGroups {
			if tg.ServiceName == "" {
				continue
			}
			tgs = append(tgs, tg)
		}
	}
	return tgs
}

func albServicePortRef(p intstr.IntOrString) (name string, number int32, ok bool) {
	if p.Type == intstr.Int {
		return "", p.IntVal, p.IntVal != 0
	}
	if p.StrVal == "" {
		return "", 0, false
	}
	if n, err := strconv.Atoi(p.StrVal); err == nil {
		return "", int32(n), true
	}
	return p.StrVal, 0, true
}

// getIngressClassBlocks resolves ingress class name -> network policy peers by following
// IngressClass.spec.parameters -> IngressClassParams.spec.subnets.ids -> Subnet external-name.
// Classes without resolvable subnets are left out of the map.

func (g awsZoneGenerator) getIngressClassBlocks() map[string][]networkingv1.NetworkPolicyPeer {
	paramSubnetIds := g.getIngressClassParamsSubnetIds()
	subnetsByExternalName := g.subnetsByExternalName()
	classBlocks := make(map[string][]networkingv1.NetworkPolicyPeer)
	for _, ingressClass := range g.ingressClasses {
		params := ingressClass.Spec.Parameters
		if params == nil || params.Kind != ingressClassParamsKind ||
			params.APIGroup == nil || *params.APIGroup != ingressClassParamsGroup {
			continue
		}
		blocks := getSubnetsBlocks(resolveSubnets(subnetsByExternalName, paramSubnetIds[params.Name]))
		if len(blocks) == 0 {
			g.log.Debug("No subnets resolved for ingress class", "ingressClass", ingressClass.Name,
				"ingressClassParams", params.Name)
			continue
		}
		classBlocks[ingressClass.Name] = blocks
	}
	return classBlocks
}

func (g awsZoneGenerator) getIngressClassParamsSubnetIds() map[string][]string {
	subnetIds := make(map[string][]string)
	for _, params := range g.required[IngressClassParamsKey] {
		ids, found, err := unstructured.NestedStringSlice(params.Resource.Object, "spec", "subnets", "ids")
		if err != nil {
			g.log.Debug("Failed to read ingress class params subnet ids", "ingressClassParams",
				params.Resource.GetName(), "error", err)
			continue
		}
		if !found {
			continue
		}
		subnetIds[params.Resource.GetName()] = ids
	}
	return subnetIds
}

// getGatewayBlocks resolves gateway namespace/name -> network policy peers by following
// Gateway.spec.infrastructure.parametersRef -> LoadBalancerConfiguration.spec.loadBalancerSubnets
// -> Subnet external-name. Gateways without resolvable subnets are left out of the map.

func (g awsZoneGenerator) getGatewayBlocks() map[string][]networkingv1.NetworkPolicyPeer {
	configSubnetIds := g.getLoadBalancerConfigurationSubnetIds()
	subnetsByExternalName := g.subnetsByExternalName()
	gatewayBlocks := make(map[string][]networkingv1.NetworkPolicyPeer)
	for _, gateway := range g.required[GatewayKey] {
		obj := gateway.Resource.Object
		group, _, _ := unstructured.NestedString(obj, "spec", "infrastructure", "parametersRef", "group")
		kind, _, _ := unstructured.NestedString(obj, "spec", "infrastructure", "parametersRef", "kind")
		name, _, _ := unstructured.NestedString(obj, "spec", "infrastructure", "parametersRef", "name")
		if group != lbConfigGroup || kind != lbConfigKind || name == "" {
			continue
		}
		// parametersRef has no namespace, it always points inside the gateway namespace
		namespace := gateway.Resource.GetNamespace()
		subnetIds := configSubnetIds[getNamespacedName(namespace, name)]
		blocks := getSubnetsBlocks(resolveSubnets(subnetsByExternalName, subnetIds))
		if len(blocks) == 0 {
			g.log.Debug("No subnets resolved for gateway", "gateway", gateway.Resource.GetName(),
				"loadBalancerConfiguration", name)
			continue
		}
		gatewayBlocks[getNamespacedName(namespace, gateway.Resource.GetName())] = blocks
	}
	return gatewayBlocks
}

func (g awsZoneGenerator) getLoadBalancerConfigurationSubnetIds() map[string][]string {
	subnetIds := make(map[string][]string)
	for _, config := range g.required[LoadBalancerConfigurationKey] {
		subnets, found, err := unstructured.NestedSlice(config.Resource.Object, "spec", "loadBalancerSubnets")
		if err != nil {
			g.log.Debug("Failed to read load balancer configuration subnets", "loadBalancerConfiguration",
				config.Resource.GetName(), "error", err)
			continue
		}
		if !found {
			continue
		}
		var ids []string
		for _, subnet := range subnets {
			subnetMap, ok := subnet.(map[string]any)
			if !ok {
				continue
			}
			if id, ok := subnetMap["identifier"].(string); ok && id != "" {
				ids = append(ids, id)
			}
		}
		subnetIds[getNamespacedName(config.Resource.GetNamespace(), config.Resource.GetName())] = ids
	}
	return subnetIds
}

// getRouteBlocks unions the peers of every gateway the route lists as a parent, parents that aren't
// gateways or whose subnets couldn't be resolved contribute nothing.

func (g awsZoneGenerator) getRouteBlocks(route *unstructured.Unstructured,
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
	cidrs := base.NewSet[string]()
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
			if block.IPBlock == nil || cidrs.Contains(block.IPBlock.CIDR) {
				continue
			}
			cidrs.Add(block.IPBlock.CIDR)
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// getHTTPRouteBackends returns the service backends of every route rule. Backends in another
// namespace are skipped, their policy would have to be created in that namespace instead.

func (g awsZoneGenerator) getHTTPRouteBackends(route *unstructured.Unstructured, namespace string) []httpRouteBackend {
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

func (g awsZoneGenerator) subnetsByExternalName() map[string]*ec2v1beta1.Subnet {
	subnets := make(map[string]*ec2v1beta1.Subnet, len(g.subnets))
	for _, subnet := range g.subnets {
		if externalName := subnet.GetAnnotations()[externalNameAnnotation]; externalName != "" {
			subnets[externalName] = subnet
		}
	}
	return subnets
}

func resolveSubnets(subnetsByExternalName map[string]*ec2v1beta1.Subnet, subnetIds []string) []*ec2v1beta1.Subnet {
	var subnets []*ec2v1beta1.Subnet
	for _, subnetId := range subnetIds {
		if subnet, found := subnetsByExternalName[subnetId]; found {
			subnets = append(subnets, subnet)
		}
	}
	return subnets
}

func filterSubnetsByType(subnets []*ec2v1beta1.Subnet, subnetType string) []*ec2v1beta1.Subnet {
	var filtered []*ec2v1beta1.Subnet
	for _, subnet := range subnets {
		if subnet.GetLabels()[subnetTypeLabel] == subnetType {
			filtered = append(filtered, subnet)
		}
	}
	return filtered
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

func GetInstanceTypesHash(instanceTypes []string, capacityType string) string {
	hash := sha256.Sum256([]byte(strings.Join(instanceTypes, "-") + "-" + capacityType))
	return hex.EncodeToString(hash[:])[:8]
}
