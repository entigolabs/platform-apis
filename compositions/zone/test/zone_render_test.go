package test

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
	"gopkg.in/yaml.v3"
)

const (
	composition     = "../apis/zone-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/tenancy"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"
	zoneResource    = "../examples/zone.yaml"
)

type renderedRule struct {
	APIGroups     []string `yaml:"apiGroups"`
	Resources     []string `yaml:"resources"`
	ResourceNames []string `yaml:"resourceNames"`
	Verbs         []string `yaml:"verbs"`
}

type renderedObject struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Rules []renderedRule `yaml:"rules"`
	Data  map[string]any `yaml:"data"`
	Spec  struct {
		MatchConditions []struct {
			Name string `yaml:"name"`
		} `yaml:"matchConditions"`
	} `yaml:"spec"`
}

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
	crossplane.AssertResourceCount(t, resources, "Namespace", 3)
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

	t.Log("Validating v1 Namespace fields")
	crossplane.AssertFieldValues(t, resources, "Namespace", "v1", map[string]string{
		"metadata.labels.tenancy\\.entigo\\.com/zone":             "testzone",
		"metadata.labels.tenancy\\.entigo\\.com/only-argocd-apps": "true",
		"metadata.labels.pod-security\\.kubernetes\\.io/enforce":  "baseline",
		"metadata.labels.pod-security\\.kubernetes\\.io/warn":     "baseline",
		"metadata.name":                         "testzone-apps",
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
	crossplane.AssertResourceCount(t, resources, "Namespace", 3)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)
	crossplane.AssertResourceCount(t, resources, "NetworkPolicy", 8) // 2 zone + 2 ingress target + 4 httproute target
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

	t.Log("Validating networking.k8s.io http route target NetworkPolicy fields")
	crossplane.AssertFieldValues(t, resources, "NetworkPolicy", "networking.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-route-abfe-svc-8081",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.podSelector.matchLabels.app":      "abfe-app",
		"spec.policyTypes.0":                    "Ingress",
		"spec.ingress.0.from.0.ipBlock.cidr":    "10.122.31.0/24",
		"spec.ingress.0.ports.0.port":           "8081",
		"spec.ingress.0.ports.0.protocol":       "TCP",
	})

	crossplane.AssertFieldValues(t, resources, "NetworkPolicy", "networking.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-route-abfe-svc2-9091",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.podSelector.matchLabels.app":      "abfe-app2",
		"spec.policyTypes.0":                    "Ingress",
		"spec.ingress.0.from.0.ipBlock.cidr":    "10.122.31.0/24",
		"spec.ingress.0.ports.0.port":           "9091",
		"spec.ingress.0.ports.0.protocol":       "TCP",
	})

	t.Log("Validating networking.k8s.io target NetworkPolicy fields")
	crossplane.AssertFieldValues(t, resources, "NetworkPolicy", "networking.k8s.io/v1", map[string]string{
		"metadata.name":                         "abfe-app-abfe-svc-8081",
		"metadata.namespace":                    "abfe",
		"metadata.ownerReferences.0.apiVersion": "tenancy.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Zone",
		"metadata.ownerReferences.0.name":       "testzone",
		"spec.podSelector.matchLabels.app":      "abfe-app",
		"spec.policyTypes.0":                    "Ingress",
		"spec.ingress.0.from.0.ipBlock.cidr":    "10.122.30.0/24",
		"spec.ingress.0.ports.0.port":           "8081",
		"spec.ingress.0.ports.0.protocol":       "TCP",
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
	crossplane.AssertResourceCount(t, resources, "Namespace", 3)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)
	crossplane.AssertResourceCount(t, resources, "NetworkPolicy", 8) // 2 zone + 2 ingress target + 4 httproute target
	crossplane.AssertResourceCount(t, resources, "ValidatingPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "AccessEntry", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 4)

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
		if res.GetKind() == "RolePolicyAttachment" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Zone", 1)
	crossplane.AssertResourceCount(t, resources, "Namespace", 3)
	crossplane.AssertResourceCount(t, resources, "LaunchTemplate", 2)
	crossplane.AssertResourceCount(t, resources, "MutatingPolicy", 4)
	crossplane.AssertResourceCount(t, resources, "AppProject", 1)
	crossplane.AssertResourceCount(t, resources, "NetworkPolicy", 8) // 2 zone + 2 ingress target + 4 httproute target
	crossplane.AssertResourceCount(t, resources, "ValidatingPolicy", 2)
	crossplane.AssertResourceCount(t, resources, "AccessEntry", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "RolePolicyAttachment", 4)
	crossplane.AssertResourceCount(t, resources, "RoleBinding", 0)
	crossplane.AssertResourceCount(t, resources, "ClusterRoleBinding", 0)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting tenancy.entigo.com Zone Ready Status")
	crossplane.AssertResourceReady(t, resources, "Zone", "tenancy.entigo.com/v1alpha1")
}

func TestZoneAuthzRendersTheSameOnEveryCloud(t *testing.T) {
	t.Parallel()
	for _, cloud := range []string{"aws", "oracle"} {
		t.Run(cloud, func(t *testing.T) {
			t.Parallel()
			objects := renderedObjects(t, "global.cloudProvider="+cloud)

			for _, gone := range []string{"ClusterRole/zone-tenancy-c-maintainer",
				"ClusterRoleBinding/zone-tenancy-maintainer", "ClusterRoleBinding/zone-tenancy-maintainer-c-read",
				"ClusterRoleBinding/zone-tenancy-observer", "ClusterRoleBinding/zone-tenancy-contributor"} {
				if _, found := objects[gone]; found {
					t.Errorf("%s is rendered, group RBAC is gone", gone)
				}
			}
			for _, kept := range []string{"ClusterRole/zone-tenancy-c-read", "ClusterRole/platform-apis-zone-authz-read",
				"ValidatingPolicy/platform-apis-zone-cluster-permissions"} {
				if _, found := objects[kept]; !found {
					t.Errorf("%s is not rendered", kept)
				}
			}
			if !hasRule(objects["ClusterRole/platform-apis-zone"], "rbac.authorization.k8s.io", "clusterroles", "crossplane-view", "bind") {
				t.Error("platform-apis-zone lost bind on crossplane-view, Crossplane cannot clean up the old RoleBindings")
			}
			if !hasRule(objects["ClusterRole/kyverno-zone-access"], "authorization.k8s.io", "subjectaccessreviews", "", "create") {
				t.Error("kyverno-zone-access cannot create SubjectAccessReviews the zone policies send")
			}
			for _, policy := range []string{"platform-apis-zone-namespace-contributor-deny",
				"platform-apis-zone-namespace-maintainer-deny", "platform-apis-zone-maintainer-infralib-zone-deny"} {
				conditions := objects["ValidatingPolicy/"+policy].Spec.MatchConditions
				if len(conditions) == 0 || conditions[0].Name != "oidc-users" {
					t.Errorf("%s does not judge oidc users by SubjectAccessReview: %+v", policy, conditions)
				}
			}
			environment := objects["EnvironmentConfig/platform-apis-zone"].Data
			for _, key := range []string{"namespaceRole", "roleMapping"} {
				if _, found := environment[key]; found {
					t.Errorf("EnvironmentConfig still carries %s", key)
				}
			}
		})
	}
}

func renderedObjects(t *testing.T, values ...string) map[string]renderedObject {
	t.Helper()
	decoder := yaml.NewDecoder(bytes.NewBufferString(helmTemplate(t, values...)))
	objects := make(map[string]renderedObject)
	for {
		var obj renderedObject
		err := decoder.Decode(&obj)
		if errors.Is(err, io.EOF) {
			return objects
		}
		if err != nil {
			t.Fatalf("decode rendered chart: %v", err)
		}
		if obj.Kind != "" {
			objects[obj.Kind+"/"+obj.Metadata.Name] = obj
		}
	}
}

func hasRule(obj renderedObject, group, resource, name, verb string) bool {
	return slices.ContainsFunc(obj.Rules, func(rule renderedRule) bool {
		return slices.Contains(rule.APIGroups, group) && slices.Contains(rule.Resources, resource) &&
			(name == "" || slices.Contains(rule.ResourceNames, name)) && slices.Contains(rule.Verbs, verb)
	})
}
