package service

import (
	"testing"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/google/go-cmp/cmp"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/component-helpers/auth/rbac/validation"
)

func TestMergeRoleMappings(t *testing.T) {
	tests := map[string]struct {
		zone v1alpha1.Zone
		env  apis.Environment
		want map[string][]string
	}{
		"env only": {
			zone: v1alpha1.Zone{},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"env-contrib"}},
					{RoleRef: "maintainer", Groups: []string{"env-maintainer"}},
					{RoleRef: "observer", Groups: []string{"env-observer"}},
				},
			},
			want: map[string][]string{
				"contributor": {"env-contrib"},
				"maintainer":  {"env-maintainer"},
				"observer":    {"env-observer"},
			},
		},
		"zone only": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"zone-contrib"}},
						{RoleRef: "maintainer", Groups: []string{"zone-maintainer"}},
					},
				},
			},
			env: apis.Environment{},
			want: map[string][]string{
				"contributor": {"zone-contrib"},
				"maintainer":  {"zone-maintainer"},
			},
		},
		"both merged and deduplicated": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"group-a", "group-b"}},
						{RoleRef: "maintainer", Groups: []string{"group-m"}},
					},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"group-b", "group-c"}},
					{RoleRef: "maintainer", Groups: []string{"group-m", "group-n"}},
				},
			},
			want: map[string][]string{
				"contributor": {"group-a", "group-b", "group-c"},
				"maintainer":  {"group-m", "group-n"},
			},
		},
		"contributor includes AppProject groups": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					AppProject: &v1alpha1.AppProject{
						ContributorGroups: []string{"oidc-group"},
					},
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"zone-contrib"}},
					},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"env-contrib"}},
				},
			},
			want: map[string][]string{
				"contributor": {"env-contrib", "oidc-group", "zone-contrib"},
			},
		},
		"AppProject groups without env role mappings": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					AppProject: &v1alpha1.AppProject{
						ContributorGroups: []string{"oidc-group"},
					},
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "contributor", Groups: []string{"zone-contrib"}},
					},
				},
			},
			env: apis.Environment{},
			want: map[string][]string{
				"contributor": {"oidc-group", "zone-contrib"},
			},
		},
		"non-overlapping roles preserved": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					RoleMapping: []v1alpha1.RoleMapping{
						{RoleRef: "custom-role", Groups: []string{"custom-group"}},
					},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "maintainer", Groups: []string{"env-maintainer"}},
				},
			},
			want: map[string][]string{
				"custom-role": {"custom-group"},
				"maintainer":  {"env-maintainer"},
			},
		},
		"empty AppProject does not affect contributor": {
			zone: v1alpha1.Zone{
				Spec: v1alpha1.ZoneSpec{
					AppProject: &v1alpha1.AppProject{},
				},
			},
			env: apis.Environment{
				RoleMapping: []apis.RoleMapping{
					{RoleRef: "contributor", Groups: []string{"env-contrib"}},
				},
			},
			want: map[string][]string{
				"contributor": {"env-contrib"},
			},
		},
		"no role mappings anywhere": {
			zone: v1alpha1.Zone{},
			env:  apis.Environment{},
			want: map[string][]string{},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := mergeRoleMappings(tc.zone, tc.env)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("mergeRoleMappings() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGetNamespaceIstioInjection(t *testing.T) {
	tests := map[string]struct {
		granularEgress bool
		exclude        []string
		wantLabel      bool
	}{
		"granularEgress off": {
			granularEgress: false,
			wantLabel:      false,
		},
		"granularEgress on": {
			granularEgress: true,
			wantLabel:      true,
		},
		"granularEgress on, another zone excluded": {
			granularEgress: true,
			exclude:        []string{"other-zone"},
			wantLabel:      true,
		},
		// granularEgressExclude only relaxes the Zone's Sidecar to ALLOW_ANY. The Zone stays in
		// the mesh and its workloads still want mTLS, so it is still labelled.
		"granularEgress on, this zone excluded": {
			granularEgress: true,
			exclude:        []string{"test-zone"},
			wantLabel:      true,
		},
		"granularEgress off, this zone excluded": {
			granularEgress: false,
			exclude:        []string{"test-zone"},
			wantLabel:      false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			g := zoneGenerator{
				zone: v1alpha1.Zone{ObjectMeta: metav1.ObjectMeta{Name: "test-zone"}},
				env: apis.Environment{
					GranularEgress:        tc.granularEgress,
					GranularEgressExclude: tc.exclude,
					PodSecurity:           "baseline",
				},
				egressExclude: base.NewSet(tc.exclude...),
			}

			ns := g.getNamespace(v1alpha1.Namespace{Name: "test-app-ns"})
			value, found := ns.Labels["istio-injection"]

			if found != tc.wantLabel {
				t.Fatalf("istio-injection label present = %v, want %v", found, tc.wantLabel)
			}
			if tc.wantLabel && value != "enabled" {
				t.Fatalf("istio-injection = %q, want %q", value, "enabled")
			}
		})
	}
}

// TestGetAllRole checks the zone namespace role bound to maintainers and contributors with the
// Kubernetes RBAC rule comparator, so a wildcard creeping back in fails the denied cases.
func TestGetAllRole(t *testing.T) {
	type access struct{ group, resource, verb string }
	allowed := []access{
		{"", "pods", "delete"},
		{"", "pods/log", "get"},
		{"", "pods/exec", "create"},
		{"", "pods/portforward", "create"},
		{"", "pods/eviction", "create"},
		{"", "services", "create"},
		{"", "configmaps", "update"},
		{"", "secrets", "create"},
		{"", "secrets", "delete"},
		{"", "persistentvolumeclaims", "create"},
		{"", "serviceaccounts", "create"},
		{"", "events", "list"},
		{"apps", "deployments", "patch"},
		{"apps", "statefulsets", "create"},
		{"apps", "deployments/scale", "update"},
		{"batch", "jobs", "create"},
		{"batch", "cronjobs", "delete"},
		{"autoscaling", "horizontalpodautoscalers", "create"},
		{"policy", "poddisruptionbudgets", "create"},
		{"networking.k8s.io", "ingresses", "create"},
		{"gateway.networking.k8s.io", "httproutes", "create"},
		{"networking.istio.io", "virtualservices", "create"},
		{"networking.istio.io", "destinationrules", "update"},
		{"networking.istio.io", "serviceentries", "create"},
		{"external-secrets.io", "externalsecrets", "create"},
		{"artifact.entigo.com", "repositories", "create"},
		{"database.entigo.com", "postgresqlinstances", "create"},
		{"mq.entigo.com", "rabbitmqbrokers", "delete"},
		{"networking.entigo.com", "webaccesses", "create"},
		{"queue.entigo.com", "topics", "create"},
		{"storage.entigo.com", "s3buckets", "create"},
		{"workload.entigo.com", "webapps", "update"},
	}
	denied := []access{
		// namespace-scoped bindings authorize the Namespace object itself, kubernetes/kubernetes#142024
		{"", "namespaces", "get"},
		{"", "namespaces", "update"},
		{"", "namespaces", "patch"},
		{"", "namespaces", "delete"},
		{"", "resourcequotas", "update"},
		{"", "limitranges", "delete"},
		// managed resources use the Crossplane provider credentials
		{"iam.aws.m.upbound.io", "rolepolicyattachments", "create"},
		{"iam.aws.m.upbound.io", "roles", "create"},
		{"s3.aws.m.upbound.io", "buckets", "create"},
		{"iam.aws.upbound.io", "policies", "create"},
		{"apiextensions.crossplane.io", "compositions", "create"},
		{"pkg.crossplane.io", "providers", "create"},
		{"rbac.authorization.k8s.io", "roles", "create"},
		{"rbac.authorization.k8s.io", "rolebindings", "create"},
		{"rbac.authorization.k8s.io", "rolebindings", "update"},
		{"networking.k8s.io", "networkpolicies", "update"},
		{"networking.k8s.io", "networkpolicies", "delete"},
		{"networking.istio.io", "sidecars", "update"},
		{"security.istio.io", "authorizationpolicies", "create"},
		{"security.istio.io", "peerauthentications", "create"},
		{"kyverno.io", "policies", "create"},
		{"policies.kyverno.io", "policyexceptions", "create"},
		{"argoproj.io", "applications", "create"},
		{"tenancy.entigo.com", "zones", "update"},
	}

	rules := zoneGenerator{}.getAllRole("test-app-ns").Rules
	allows := func(a access) bool {
		covered, _ := validation.Covers(rules, []rbacv1.PolicyRule{{
			APIGroups: []string{a.group},
			Resources: []string{a.resource},
			Verbs:     []string{a.verb},
		}})
		return covered
	}
	for _, a := range allowed {
		if !allows(a) {
			t.Errorf("want allowed: %s %s (group %q)", a.verb, a.resource, a.group)
		}
	}
	for _, a := range denied {
		if allows(a) {
			t.Errorf("want denied: %s %s (group %q)", a.verb, a.resource, a.group)
		}
	}
}
