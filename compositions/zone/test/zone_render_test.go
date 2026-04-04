package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	composition     = "../apis/zone-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/tenancy"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"
	zoneResource    = "../examples/zone.yaml"
)

func TestZoneCrossplaneRender(t *testing.T) {
	t.Logf("Starting tenancy function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Zone", 1)
	crossplane.AssertResourceCount(t, resources, "Namespace", 2)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)

	t.Log("Validating tenancy.entigo.com Zone fields")
	crossplane.AssertFieldValues(t, resources, "Zone", "tenancy.entigo.com/v1alpha1", map[string]string{
		"metadata.name":          "testzone",
		"spec.namespaces.0.name": "abfe",
		"spec.namespaces.1.name": "abbe",
		"spec.namespaces.1.pool": "spot",
		"spec.pools.0.name":      "default",
		"spec.pools.1.name":      "spot",
	})

	t.Log("Validating argoproj.io AppProject fields")
	crossplane.AssertFieldValues(t, resources, "AppProject", "argoproj.io/v1alpha1", map[string]string{
		"metadata.name":                         "testzone",
		"metadata.namespace":                    "argocd",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.clusterResourceBlacklist.0.group": "*",
		"spec.clusterResourceBlacklist.0.kind":  "*",
		"spec.destinations.0.namespace":         "abbe",
		"spec.destinations.1.namespace":         "abfe",
		"spec.roles.0.name":                     "maintainer",
		"spec.roles.1.name":                     "observer",
		"spec.roles.2.name":                     "contributor",
		"spec.roles.3.name":                     "cicd",
		"spec.sourceNamespaces.0":               "abbe",
		"spec.sourceNamespaces.1":               "abfe",
		"spec.sourceRepos.0":                    "*",
	})

	t.Log("Validating policies.kyverno.io MutatingPolicy fields")
	crossplane.AssertFieldValues(t, resources, "MutatingPolicy", "policies.kyverno.io/v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":     "testzone",
		"metadata.labels.reports\\.kyverno\\.io/disabled": "true",
		"metadata.name":                         "testzone-abbe-labels",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.matchConditions.0.expression":     "object.metadata.namespace == \"abbe\"",
		"spec.matchConditions.0.name":           "namespace-filter",
	})

	t.Log("Validating policies.kyverno.io MutatingPolicy fields")
	crossplane.AssertFieldValues(t, resources, "MutatingPolicy", "policies.kyverno.io/v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":     "testzone",
		"metadata.labels.reports\\.kyverno\\.io/disabled": "true",
		"metadata.name":                         "testzone-abfe-labels",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.matchConditions.0.expression":     "object.metadata.namespace == \"abfe\"",
		"spec.matchConditions.0.name":           "namespace-filter",
	})

	t.Log("Validating policies.kyverno.io MutatingPolicy fields")
	crossplane.AssertFieldValues(t, resources, "MutatingPolicy", "policies.kyverno.io/v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":     "testzone",
		"metadata.labels.reports\\.kyverno\\.io/disabled": "true",
		"metadata.name":                         "testzone-abbe-add-nodeselector",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.matchConditions.0.expression":     "object.metadata.namespace == \"abbe\"",
		"spec.matchConditions.0.name":           "namespace-filter",
	})

	t.Log("Validating policies.kyverno.io MutatingPolicy fields")
	crossplane.AssertFieldValues(t, resources, "MutatingPolicy", "policies.kyverno.io/v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":     "testzone",
		"metadata.labels.reports\\.kyverno\\.io/disabled": "true",
		"metadata.name":                         "testzone-abfe-add-nodeselector",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.matchConditions.0.expression":     "object.metadata.namespace == \"abfe\"",
		"spec.matchConditions.0.name":           "namespace-filter",
	})

	t.Log("Validating ec2.aws.upbound.io LaunchTemplate fields")
	crossplane.AssertFieldValues(t, resources, "LaunchTemplate", "ec2.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone-default",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.description":          "testzone-default",
	})

	t.Log("Validating ec2.aws.upbound.io LaunchTemplate fields")
	crossplane.AssertFieldValues(t, resources, "LaunchTemplate", "ec2.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone-spot",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.description":          "testzone-spot",
	})

	t.Log("Validating v1 Namespace fields")
	crossplane.AssertFieldValues(t, resources, "Namespace", "v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":            "testzone",
		"metadata.labels.pod-security\\.kubernetes\\.io/enforce": "baseline",
		"metadata.labels.pod-security\\.kubernetes\\.io/warn":    "baseline",
		"metadata.labels.tenancy\\.entigo\\.com/pool":            "spot",
		"metadata.name":                         "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
	})

	t.Log("Validating v1 Namespace fields")
	crossplane.AssertFieldValues(t, resources, "Namespace", "v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":            "testzone",
		"metadata.labels.pod-security\\.kubernetes\\.io/enforce": "baseline",
		"metadata.labels.pod-security\\.kubernetes\\.io/warn":    "baseline",
		"metadata.name":                         "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "LaunchTemplate" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		} else {
			crossplane.AppendToResources(t, observed, res)
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Zone", 1)
	crossplane.AssertResourceCount(t, resources, "Namespace", 2)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)
	crossplane.AssertResourceCount(t, resources, "NetworkPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "ValidatingPolicy", 2)

	t.Log("Validating policies.kyverno.io ValidatingPolicy fields")
	crossplane.AssertFieldValues(t, resources, "ValidatingPolicy", "policies.kyverno.io/v1", map[string]string{
		"metadata.annotations.tenancy\\.entigo\\.com/zone": "testzone",
		"metadata.name":                         "testzone-abbe-validate-nodeselector",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.matchConditions.0.expression":     "object.metadata.namespace == \"abbe\"",
		"spec.matchConditions.0.name":           "namespace-filter",
	})

	t.Log("Validating policies.kyverno.io ValidatingPolicy fields")
	crossplane.AssertFieldValues(t, resources, "ValidatingPolicy", "policies.kyverno.io/v1", map[string]string{
		"metadata.annotations.tenancy\\.entigo\\.com/zone": "testzone",
		"metadata.name":                         "testzone-abfe-validate-nodeselector",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.matchConditions.0.expression":     "object.metadata.namespace == \"abfe\"",
		"spec.matchConditions.0.name":           "namespace-filter",
	})

	t.Log("Validating networking.k8s.io NetworkPolicy fields")
	crossplane.AssertFieldValues(t, resources, "NetworkPolicy", "networking.k8s.io/v1", map[string]string{
		"metadata.annotations.tenancy\\.entigo\\.com/zone": "testzone",
		"metadata.name":                         "abbe-zone",
		"metadata.namespace":                    "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.ingress.0.from.0.namespaceSelector.matchLabels.tenancy\\.entigo\\.com/zone": "testzone",
	})

	t.Log("Validating networking.k8s.io NetworkPolicy fields")
	crossplane.AssertFieldValues(t, resources, "NetworkPolicy", "networking.k8s.io/v1", map[string]string{
		"metadata.annotations.tenancy\\.entigo\\.com/zone": "testzone",
		"metadata.name":                         "abfe-zone",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.ingress.0.from.0.namespaceSelector.matchLabels.tenancy\\.entigo\\.com/zone": "testzone",
	})

	t.Log("Validating iam.aws.upbound.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.assumeRolePolicy":     "*",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if (res.GetAPIVersion() == "networking.k8s.io/v1" && res.GetKind() == "NetworkPolicy") || (res.GetAPIVersion() == "policies.kyverno.io/v1" && res.GetKind() == "ValidatingPolicy") {
			crossplane.AppendToResources(t, observed, res)
		}
	}
	crossplane.AppendToResources(t, observed, crossplane.MockByKind(t, resources, "Role", "iam.aws.upbound.io/v1beta1", true, nil))

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Zone", 1)
	crossplane.AssertResourceCount(t, resources, "Namespace", 2)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)
	crossplane.AssertResourceCount(t, resources, "NetworkPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "ValidatingPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "AccessEntry", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 5)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 4)

	t.Log("Validating rbac.authorization.k8s.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abbe-all",
		"metadata.namespace":                    "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"rules.0.apiGroups.0":                   "*",
		"rules.0.resources.0":                   "*",
		"rules.0.verbs.0":                       "*",
	})

	t.Log("Validating rbac.authorization.k8s.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abbe-read",
		"metadata.namespace":                    "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"rules.0.apiGroups.0":                   "*",
		"rules.0.resources.0":                   "*",
		"rules.0.verbs.0":                       "get",
		"rules.0.verbs.1":                       "watch",
		"rules.0.verbs.2":                       "list",
	})

	t.Log("Validating rbac.authorization.k8s.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-all",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"rules.0.apiGroups.0":                   "*",
		"rules.0.resources.0":                   "*",
		"rules.0.verbs.0":                       "*",
	})

	t.Log("Validating rbac.authorization.k8s.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-read",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"rules.0.apiGroups.0":                   "*",
		"rules.0.resources.0":                   "*",
		"rules.0.verbs.0":                       "get",
		"rules.0.verbs.1":                       "watch",
		"rules.0.verbs.2":                       "list",
	})

	t.Log("Validating iam.aws.upbound.io RolePolicyAttachment fields")
	crossplane.AssertFieldValues(t, resources, "RolePolicyAttachment", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone-ecr-proxy",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.policyArnRef.name":    "ecr-proxy",
		"spec.forProvider.roleRef.name":         "testzone",
	})

	t.Log("Validating iam.aws.upbound.io RolePolicyAttachment fields")
	crossplane.AssertFieldValues(t, resources, "RolePolicyAttachment", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone-ecr-ro",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.policyArn":            "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly",
		"spec.forProvider.roleRef.name":         "testzone",
	})

	t.Log("Validating iam.aws.upbound.io RolePolicyAttachment fields")
	crossplane.AssertFieldValues(t, resources, "RolePolicyAttachment", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone-ssm",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.policyArn":            "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore",
		"spec.forProvider.roleRef.name":         "testzone",
	})

	t.Log("Validating iam.aws.upbound.io RolePolicyAttachment fields")
	crossplane.AssertFieldValues(t, resources, "RolePolicyAttachment", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "testzone-wn",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.forProvider.policyArn":            "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy",
		"spec.forProvider.roleRef.name":         "testzone",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed, crossplane.MockByKind(t, resources, "AccessEntry", "eks.aws.upbound.io/v1beta1", true, nil))
	for _, res := range resources {
		if res.GetKind() == "Role" && res.GetAPIVersion() == "rbac.authorization.k8s.io/v1" {
			crossplane.AppendToResources(t, observed, res)
		}
		if res.GetKind() == "RolePolicyAttachment" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Zone", 1)
	crossplane.AssertResourceCount(t, resources, "Namespace", 2)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)
	crossplane.AssertResourceCount(t, resources, "NetworkPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "ValidatingPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "AccessEntry", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 5)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 4)
	crossplane.AssertResourceCount(t, resources, "RoleBinding", 6)

	t.Log("Validating rbac.authorization.k8s.io RoleBinding fields")
	crossplane.AssertFieldValues(t, resources, "RoleBinding", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abbe-contributor",
		"metadata.namespace":                    "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"roleRef.apiGroup":                      "rbac.authorization.k8s.io",
		"roleRef.kind":                          "Role",
		"roleRef.name":                          "abbe-all",
		"subjects.0.apiGroup":                   "rbac.authorization.k8s.io",
		"subjects.0.kind":                       "Group",
		"subjects.0.name":                       "contributor",
	})

	t.Log("Validating rbac.authorization.k8s.io RoleBinding fields")
	crossplane.AssertFieldValues(t, resources, "RoleBinding", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-contributor",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"roleRef.apiGroup":                      "rbac.authorization.k8s.io",
		"roleRef.kind":                          "Role",
		"roleRef.name":                          "abfe-all",
		"subjects.0.apiGroup":                   "rbac.authorization.k8s.io",
		"subjects.0.kind":                       "Group",
		"subjects.0.name":                       "contributor",
	})

	t.Log("Validating rbac.authorization.k8s.io RoleBinding fields")
	crossplane.AssertFieldValues(t, resources, "RoleBinding", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abbe-maintainer",
		"metadata.namespace":                    "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"roleRef.apiGroup":                      "rbac.authorization.k8s.io",
		"roleRef.kind":                          "Role",
		"roleRef.name":                          "abbe-all",
		"subjects.0.apiGroup":                   "rbac.authorization.k8s.io",
		"subjects.0.kind":                       "Group",
		"subjects.0.name":                       "maintainer",
	})

	t.Log("Validating rbac.authorization.k8s.io RoleBinding fields")
	crossplane.AssertFieldValues(t, resources, "RoleBinding", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-maintainer",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"roleRef.apiGroup":                      "rbac.authorization.k8s.io",
		"roleRef.kind":                          "Role",
		"roleRef.name":                          "abfe-all",
		"subjects.0.apiGroup":                   "rbac.authorization.k8s.io",
		"subjects.0.kind":                       "Group",
		"subjects.0.name":                       "maintainer",
	})

	t.Log("Validating rbac.authorization.k8s.io RoleBinding fields")
	crossplane.AssertFieldValues(t, resources, "RoleBinding", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abbe-observer",
		"metadata.namespace":                    "abbe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"roleRef.apiGroup":                      "rbac.authorization.k8s.io",
		"roleRef.kind":                          "Role",
		"roleRef.name":                          "abbe-read",
		"subjects.0.apiGroup":                   "rbac.authorization.k8s.io",
		"subjects.0.kind":                       "Group",
		"subjects.0.name":                       "observer",
	})

	t.Log("Validating rbac.authorization.k8s.io RoleBinding fields")
	crossplane.AssertFieldValues(t, resources, "RoleBinding", "rbac.authorization.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-observer",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"roleRef.apiGroup":                      "rbac.authorization.k8s.io",
		"roleRef.kind":                          "Role",
		"roleRef.name":                          "abfe-read",
		"subjects.0.apiGroup":                   "rbac.authorization.k8s.io",
		"subjects.0.kind":                       "Group",
		"subjects.0.name":                       "observer",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "RoleBinding" {
			crossplane.AppendToResources(t, observed, res)
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting tenancy.entigo.com Zone Ready Status")
	crossplane.AssertResourceReady(t, resources, "Zone", "tenancy.entigo.com/v1alpha1")
}
