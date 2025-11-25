package service

import (
	"fmt"
	"maps"
	"strconv"
	"strings"

	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	policyv1alpha1 "github.com/kyverno/kyverno/api/policies.kyverno.io/v1alpha1"
	ec2v1beta1 "github.com/upbound/provider-aws/apis/cluster/ec2/v1beta1"
	eksv1beta1 "github.com/upbound/provider-aws/apis/cluster/eks/v1beta1"
	iamv1beta1 "github.com/upbound/provider-aws/apis/cluster/iam/v1beta1"
	kmsv1beta1 "github.com/upbound/provider-aws/apis/cluster/kms/v1beta1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	admissionregistrationv1alpha1 "k8s.io/api/admissionregistration/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	zoneAnnotation     = "tenancy.entigo.com/zone"
	zonePoolAnnotation = "tenancy.entigo.com/zone-pool"
)

type zoneGenerator struct {
	// Inputs
	zone     v1alpha1.Zone
	observed map[resource.Name]resource.ObservedComposed
	env      apis.Environment
	// Dependencies
	vpc           ec2v1beta1.VPC
	kmsDataAlias  kmsv1beta1.Alias
	securityGroup ec2v1beta1.SecurityGroup
	cluster       eksv1beta1.Cluster
	subnets       []*ec2v1beta1.Subnet

	zoneAnnotations map[string]string
	zoneTags        map[string]*string
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

	if err := base.ExtractRequiredResource(required, "VPC", &vpc); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "KMSDataAlias", &kmsDataAlias); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "NodeSecurityGroup", &securityGroup); err != nil {
		return nil, err
	}
	if err := base.ExtractRequiredResource(required, "Cluster", &cluster); err != nil {
		return nil, err
	}
	subnets, err := base.ExtractResources[*ec2v1beta1.Subnet](required, "Subnets")
	if err != nil {
		return nil, err
	}
	tags := map[string]*string{
		zoneAnnotation: &zone.Name,
	}
	maps.Copy(tags, env.Tags)
	generator := zoneGenerator{
		zone:          zone,
		observed:      observed,
		env:           env,
		vpc:           vpc,
		kmsDataAlias:  kmsDataAlias,
		securityGroup: securityGroup,
		cluster:       cluster,
		subnets:       subnets,
		zoneAnnotations: map[string]string{
			zoneAnnotation: zone.Name,
		},
		zoneTags: tags,
	}
	return generator.generate()
}

func (g zoneGenerator) generate() (map[string]runtime.Object, error) {
	objs := make(map[string]runtime.Object)
	namespaces := g.generateNamespaces()
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
	return objs, nil
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func GetNamespaceKey(zone string) string {
	return "namespace-" + zone
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

func GetNodeGroupKey(poolName, hash string) string {
	return "nodepool-" + poolName + "-" + hash
}

func GetAppProjectKey(zone string) string {
	return "appproject-" + zone
}

func (g zoneGenerator) generateNamespaces() map[string]runtime.Object {
	objs := make(map[string]runtime.Object)
	for _, ns := range g.zone.Spec.Namespaces {
		namespace := g.getNamespace(ns.Name)
		objs[GetNamespaceKey(g.zone.Name)] = namespace
		networkPolicy := g.getNetworkPolicy(ns.Name)
		objs[GetNetworkPolicyKey(g.zone.Name, ns.Name)] = networkPolicy
		allRole := g.getAllRole(ns.Name)
		objs[GetRBACRoleAllKey(g.zone.Name, ns.Name)] = allRole
		readRole := g.getReadRole(ns.Name)
		objs[GetRBACRoleReadKey(g.zone.Name, ns.Name)] = readRole
		contributorBinding := g.getRoleBinding(ns.Name, ns.Name+"-contributor", allRole.Name, "contributor")
		objs[GetRBContributorKey(g.zone.Name, ns.Name)] = contributorBinding
		maintainerBinding := g.getRoleBinding(ns.Name, ns.Name+"-maintainer", allRole.Name, "maintainer")
		objs[GetRBMaintainerKey(g.zone.Name, ns.Name)] = maintainerBinding
		observerBinding := g.getRoleBinding(ns.Name, ns.Name+"-observer", readRole.Name, "observer")
		objs[GetRBObserverKey(g.zone.Name, ns.Name)] = observerBinding
		mutatingPolicy := g.getMutatingPolicy(ns.Name, ns.Pool)
		objs[GetMutatingPolicyKey(g.zone.Name, ns.Name)] = mutatingPolicy
		validatingPolicy := g.getValidatingPolicy(ns.Name)
		objs[GetValidatingPolicyKey(g.zone.Name, ns.Name)] = validatingPolicy
	}
	return objs
}

func (g zoneGenerator) getNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Namespace",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				zoneAnnotation: g.zone.Name,
			},
			Annotations: g.zoneAnnotations,
		},
	}
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
				zoneAnnotation: g.zone.Name,
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
									zoneAnnotation: g.zone.Name,
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

func (g zoneGenerator) getMutatingPolicy(namespaceName, poolName string) *policyv1alpha1.MutatingPolicy {
	poolName = g.getPoolName(poolName)
	return &policyv1alpha1.MutatingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policies.kyverno.io/v1alpha1",
			Kind:       "MutatingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name + "-" + namespaceName + "-add-nodeselector",
			Annotations: g.zoneAnnotations,
		},
		Spec: policyv1alpha1.MutatingPolicySpec{
			MatchConstraints: &admissionregistrationv1alpha1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "kubernetes.io/metadata.name",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{namespaceName},
						},
					},
				},
				ResourceRules: []admissionregistrationv1alpha1.NamedRuleWithOperations{{
					RuleWithOperations: admissionregistrationv1alpha1.RuleWithOperations{
						Rule: admissionregistrationv1alpha1.Rule{
							APIGroups:   []string{""},
							APIVersions: []string{"v1"},
							Resources:   []string{"pods"},
						},
						Operations: []admissionregistrationv1alpha1.OperationType{admissionregistrationv1alpha1.Create},
					},
				}},
			},
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

func (g zoneGenerator) getValidatingPolicy(namespaceName string) *policyv1alpha1.ValidatingPolicy {
	var poolExprList, poolMsgList []string
	for _, pool := range g.zone.Spec.Pools {
		poolExprList = append(poolExprList, `"`+g.zone.Name+`-`+pool.Name+`"`)
		poolMsgList = append(poolMsgList, g.zone.Name+`-`+pool.Name)
	}
	return &policyv1alpha1.ValidatingPolicy{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "policies.kyverno.io/v1alpha1",
			Kind:       "ValidatingPolicy",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name + "-" + namespaceName + "-validate-nodeselector",
			Annotations: g.zoneAnnotations,
		},
		Spec: policyv1alpha1.ValidatingPolicySpec{
			MatchConstraints: &admissionregistrationv1.MatchResources{
				NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      "kubernetes.io/metadata.name",
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{namespaceName},
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
		zoneAnnotation: zoneName,
	}
	emptyString := ""

	for _, pool := range g.zone.Spec.Pools {
		zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)
		annotations := map[string]string{
			zonePoolAnnotation: zonePool,
		}
		maps.Copy(annotations, g.zoneAnnotations)
		tags := map[string]*string{
			zoneAnnotation:     &zoneName,
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
	for _, pool := range g.zone.Spec.Pools {
		launchTemplate, ok := g.observed[resource.Name(GetLaunchTemplateKey(g.zone.Name, pool.Name))]
		if !ok || launchTemplate.Resource == nil {
			continue
		}
		key, ng, err := g.getNodeGroup(pool, *launchTemplate.Resource)
		if err != nil {
			return nil, err
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

func (g zoneGenerator) getNodeGroup(pool v1alpha1.Pool, launchTemplate composed.Unstructured) (string, *eksv1beta1.NodeGroup, error) {
	zoneName := g.zone.GetName()
	zonePool := fmt.Sprintf("%s-%s", zoneName, pool.Name)

	var instanceTypes []*string
	var zoneFilter base.Set[string]
	var capacityType = "ON_DEMAND"
	var minSize float64 = 1
	var maxSize float64 = 1

	for _, requirement := range pool.Requirements {
		switch requirement.Key {
		case "instance-type":
			instanceTypes = append(instanceTypes, &requirement.Value)
		case "zone":
			zoneFilter = base.ToSet(requirement.Values)
		case "capacity-type":
			capacityType = requirement.Value
		case "min-size":
			val, err := strconv.ParseFloat(requirement.Value, 64)
			if err != nil {
				return "", nil, err
			}
			minSize = val
		case "max-size":
			val, err := strconv.ParseFloat(requirement.Value, 64)
			if err != nil {
				return "", nil, err
			}
			maxSize = val
		}
	}

	if maxSize < minSize {
		maxSize = minSize
	}

	hash := GetInstanceTypesHash(instanceTypes)
	name := fmt.Sprintf("%s-%s", zonePool, hash)

	var subnetRefs []v1.Reference
	for _, subnet := range g.subnets {
		subnetName := subnet.GetName()
		if zoneFilter.Size() > 0 {
			if subnet.Status.AtProvider.AvailabilityZone != nil && zoneFilter.Contains(*subnet.Status.AtProvider.AvailabilityZone) {
				subnetRefs = append(subnetRefs, v1.Reference{Name: subnetName})
			}
		} else {
			subnetRefs = append(subnetRefs, v1.Reference{Name: subnetName})
		}
	}

	version, _, _ := unstructured.NestedString(launchTemplate.Object, "status", "atProvider", "latestVersion")
	ltParams := eksv1beta1.LaunchTemplateParameters{
		Name: base.StringPtr(zonePool),
	}
	if version != "" {
		ltParams.Version = &version
	}

	annotations := map[string]string{
		zonePoolAnnotation: zonePool,
	}
	maps.Copy(annotations, g.zoneAnnotations)
	tags := map[string]*string{
		zoneAnnotation:     &zoneName,
		zonePoolAnnotation: &zonePool,
	}
	maps.Copy(tags, g.env.Tags)
	eksLabels := map[string]*string{
		zoneAnnotation:     &zoneName,
		zonePoolAnnotation: &zonePool,
	}

	return GetNodeGroupKey(pool.Name, hash), &eksv1beta1.NodeGroup{
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
				LaunchTemplate: []eksv1beta1.LaunchTemplateParameters{ltParams},
				ClusterNameRef: &v1.Reference{
					Name: g.cluster.GetName(),
				},
				Region:        g.vpc.Status.AtProvider.Region,
				UpdateConfig:  []eksv1beta1.UpdateConfigParameters{{MaxUnavailable: base.Float64Ptr(1)}},
				InstanceTypes: instanceTypes,
				SubnetIDRefs:  subnetRefs,
				Labels:        eksLabels,
				NodeRoleArnRef: &v1.Reference{
					Name: zoneName,
				},
				Tags:         tags,
				CapacityType: base.StringPtr(capacityType),
				ScalingConfig: []eksv1beta1.ScalingConfigParameters{{
					MaxSize: base.Float64Ptr(maxSize),
					MinSize: base.Float64Ptr(minSize),
				}},
			},
			InitProvider: eksv1beta1.NodeGroupInitParameters{
				ScalingConfig: []eksv1beta1.ScalingConfigInitParameters{{
					DesiredSize: base.Float64Ptr(minSize),
				}},
			},
		},
	}, nil
}

func (g zoneGenerator) getAppProject() runtime.Object {
	var destinations []argov1alpha1.ApplicationDestination
	var namespaces []string
	for _, ns := range g.zone.Spec.Namespaces {
		destinations = append(destinations, argov1alpha1.ApplicationDestination{
			Namespace: ns.Name,
			Server:    "https://kubernetes.default.svc",
		})
		namespaces = append(namespaces, ns.Name)
	}

	var whitelist, blacklist []metav1.GroupKind
	if g.zone.Spec.ClusterPermissions {
		whitelist = []metav1.GroupKind{{Group: "*", Kind: "*"}}
		blacklist = []metav1.GroupKind{}
	} else {
		whitelist = []metav1.GroupKind{}
		blacklist = []metav1.GroupKind{{Group: "*", Kind: "*"}}
	}

	return &argov1alpha1.AppProject{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "argoproj.io/v1alpha1",
			Kind:       "AppProject",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        g.zone.Name,
			Namespace:   g.env.ArgoCDNamespace,
			Annotations: g.zoneAnnotations,
		},
		Spec: argov1alpha1.AppProjectSpec{
			Description:              "Security zone for isolated team deployment",
			Destinations:             destinations,
			SourceRepos:              []string{"*"},
			SourceNamespaces:         namespaces,
			ClusterResourceWhitelist: whitelist,
			ClusterResourceBlacklist: blacklist,
			NamespaceResourceBlacklist: []metav1.GroupKind{
				{Group: "*.m.upbound.io", Kind: "*"},
			},
			Roles: []argov1alpha1.ProjectRole{
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
					Groups:      g.zone.Spec.AppProject.ContributorGroups,
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

func GetInstanceTypesHash(instanceTypes []*string) string {
	return base.GenerateHash([]byte(joinStringPtrs(instanceTypes, "-")))
}

func joinStringPtrs(ptrs []*string, sep string) string {
	vals := make([]string, len(ptrs))
	for i, p := range ptrs {
		if p != nil {
			vals[i] = *p
		} else {
			vals[i] = ""
		}
	}
	return strings.Join(vals, sep)
}
