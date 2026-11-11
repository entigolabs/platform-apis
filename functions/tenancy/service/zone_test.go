package service

import (
	"reflect"
	"strings"
	"testing"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/go-cmp/cmp"
	policyv1 "github.com/kyverno/api/api/policies.kyverno.io/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/cel/common"
	"k8s.io/apiserver/pkg/cel/mutation"
)

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

// TestNodeSelectorPolicies runs a pod through the zone's add-nodeselector mutation and then its
// validate-nodeselector validation, as admission does. Zone "team" with pool "b-default" has the
// pool label "team-b-default", the same as zone "team-b" with pool "default", so only the zone
// label keeps the pod on its own zone's nodes.
func TestNodeSelectorPolicies(t *testing.T) {
	const (
		zoneKey = "tenancy.entigo.com/zone"
		poolKey = "tenancy.entigo.com/zone-pool"
	)
	tests := map[string]struct {
		nsPool       string
		nodeSelector map[string]string
		want         map[string]string
		rejected     bool
	}{
		"no pool, no nodeSelector": {
			want: map[string]string{zoneKey: "team"},
		},
		"no pool, unrelated nodeSelector": {
			nodeSelector: map[string]string{"kubernetes.io/arch": "arm64"},
			want:         map[string]string{"kubernetes.io/arch": "arm64", zoneKey: "team"},
		},
		"no pool, other zone": {
			nodeSelector: map[string]string{zoneKey: "team-b"},
			want:         map[string]string{zoneKey: "team"},
		},
		"no pool, own pool": {
			nodeSelector: map[string]string{poolKey: "team-b-default"},
			want:         map[string]string{zoneKey: "team", poolKey: "team-b-default"},
		},
		"pool, no nodeSelector": {
			nsPool: "b-default",
			want:   map[string]string{zoneKey: "team", poolKey: "team-b-default"},
		},
		"pool, unrelated nodeSelector": {
			nsPool:       "b-default",
			nodeSelector: map[string]string{"kubernetes.io/arch": "arm64"},
			want:         map[string]string{"kubernetes.io/arch": "arm64", zoneKey: "team", poolKey: "team-b-default"},
		},
		"pool, other zone": {
			nsPool:       "b-default",
			nodeSelector: map[string]string{zoneKey: "team-b"},
			want:         map[string]string{zoneKey: "team", poolKey: "team-b-default"},
		},
		"pool, pod's own pool kept": {
			nsPool:       "b-default",
			nodeSelector: map[string]string{poolKey: "team-spot"},
			want:         map[string]string{zoneKey: "team", poolKey: "team-spot"},
		},
		"no pool, other zone's pool": {
			nodeSelector: map[string]string{poolKey: "other-default"},
			want:         map[string]string{zoneKey: "team", poolKey: "other-default"},
			rejected:     true,
		},
		"pool, other zone's pool": {
			nsPool:       "b-default",
			nodeSelector: map[string]string{poolKey: "other-default"},
			want:         map[string]string{zoneKey: "team", poolKey: "other-default"},
			rejected:     true,
		},
		"pool, other zone and its pool": {
			nsPool:       "b-default",
			nodeSelector: map[string]string{zoneKey: "other", poolKey: "other-default"},
			want:         map[string]string{zoneKey: "team", poolKey: "other-default"},
			rejected:     true,
		},
	}

	env, err := cel.NewEnv(
		common.ResolverEnvOption(&mutation.DynamicTypeResolver{}),
		cel.Variable("object", cel.DynType),
	)
	if err != nil {
		t.Fatalf("cel env: %v", err)
	}
	eval := func(t *testing.T, expression string, object map[string]any) ref.Val {
		t.Helper()
		ast, iss := env.Compile(expression)
		if iss.Err() != nil {
			t.Fatalf("compile: %v", iss.Err())
		}
		prg, err := env.Program(ast)
		if err != nil {
			t.Fatalf("program: %v", err)
		}
		out, _, err := prg.Eval(map[string]any{"object": object})
		if err != nil {
			t.Fatalf("eval: %v", err)
		}
		return out
	}
	g := zoneGenerator{zone: v1alpha1.Zone{
		ObjectMeta: metav1.ObjectMeta{Name: "team"},
		Spec:       v1alpha1.ZoneSpec{Pools: []v1alpha1.Pool{{Name: "b-default"}, {Name: "spot"}}},
	}}
	validation := g.getValidatingPolicy("app").(*policyv1.ValidatingPolicy).Spec.Validations[0].Expression

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mutate := g.getMutatingPolicy("app", tc.nsPool).(*policyv1.MutatingPolicy).Spec.Mutations[0].JSONPatch.Expression
			spec := map[string]any{}
			if tc.nodeSelector != nil {
				nodeSelector := map[string]any{}
				for k, v := range tc.nodeSelector {
					nodeSelector[k] = v
				}
				spec["nodeSelector"] = nodeSelector
			}
			patches, err := eval(t, mutate, map[string]any{"spec": spec}).ConvertToNative(reflect.TypeFor[[]*mutation.JSONPatchVal]())
			if err != nil {
				t.Fatalf("convert patches: %v", err)
			}

			got := map[string]string{}
			for k, v := range tc.nodeSelector {
				got[k] = v
			}
			for _, patch := range patches.([]*mutation.JSONPatchVal) {
				if patch.Op != "add" {
					t.Fatalf("patch op = %q, want add", patch.Op)
				}
				if patch.Path == "/spec/nodeSelector" {
					value, err := patch.Val.ConvertToNative(reflect.TypeFor[map[string]string]())
					if err != nil {
						t.Fatalf("convert nodeSelector: %v", err)
					}
					got = value.(map[string]string)
					continue
				}
				key := strings.TrimPrefix(patch.Path, "/spec/nodeSelector/")
				key = strings.ReplaceAll(strings.ReplaceAll(key, "~1", "/"), "~0", "~")
				got[key] = patch.Val.Value().(string)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("nodeSelector after mutation (-want +got):\n%s", diff)
			}

			nodeSelector := map[string]any{}
			for k, v := range got {
				nodeSelector[k] = v
			}
			admitted := eval(t, validation, map[string]any{"spec": map[string]any{"nodeSelector": nodeSelector}}).Value()
			if admitted != !tc.rejected {
				t.Errorf("admitted = %v, want %v", admitted, !tc.rejected)
			}
		})
	}
}
