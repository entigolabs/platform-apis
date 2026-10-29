package test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/google/cel-go/cel"
	"gopkg.in/yaml.v3"
)

// TestZoneManagedResourceDeny covers platform-apis-zone-managed-resource-deny (ValidatingPolicy).
// The kyverno CLI cannot evaluate a wildcard rule against a custom resource offline (it has no CRD
// to map the kind), so the rendered matchConditions are evaluated with CEL directly: the request is
// denied when every condition matches. The namespaceSelector (Zone Namespaces except infralib) is
// checked on its rendered form.
func TestZoneManagedResourceDeny(t *testing.T) {
	spec := renderManagedResourcePolicy(t, "crossplane-system")

	selector := spec.MatchConstraints.NamespaceSelector.MatchExpressions
	if len(selector) != 2 ||
		selector[0].Key != "tenancy.entigo.com/zone" || selector[0].Operator != "Exists" ||
		selector[1].Key != "tenancy.entigo.com/zone" || selector[1].Operator != "NotIn" ||
		len(selector[1].Values) != 1 || selector[1].Values[0] != "infralib" {
		t.Fatalf("namespaceSelector must select Zone Namespaces except infralib, got %+v", selector)
	}
	rules := spec.MatchConstraints.ResourceRules
	if len(rules) != 1 || strings.Join(rules[0].Operations, ",") != "CREATE,UPDATE" {
		t.Fatalf("policy must match CREATE and UPDATE only (deletes stay allowed), got %+v", rules)
	}

	cases := []struct {
		name, group, username string
		deny                  bool
	}{
		{"contributor creates an AWS managed resource", "iam.aws.m.upbound.io", "attacker", true},
		{"ArgoCD creates an AWS managed resource", "iam.aws.m.upbound.io", "system:serviceaccount:argocd:argocd-application-controller", true},
		{"contributor creates a Kafka managed resource", "acl.kafka.m.crossplane.io", "attacker", true},
		{"contributor creates an SQL managed resource", "postgresql.sql.m.crossplane.io", "attacker", true},
		{"contributor creates a namespaced ProviderConfig", "aws.m.upbound.io", "attacker", true},
		{"service account of another namespace", "iam.aws.m.upbound.io", "system:serviceaccount:crossplane-system-fake:crossplane", true},
		{"Crossplane creates an AWS managed resource", "iam.aws.m.upbound.io", "system:serviceaccount:crossplane-system:crossplane", false},
		{"a provider updates a Kafka managed resource", "acl.kafka.m.crossplane.io", "system:serviceaccount:crossplane-system:provider-kafka-0123456789ab", false},
		{"the garbage collector updates owner references", "iam.aws.m.upbound.io", "system:serviceaccount:kube-system:generic-garbage-collector", false},
		{"the controller manager updates owner references", "iam.aws.m.upbound.io", "system:kube-controller-manager", false},
		{"contributor creates a platform API object", "database.entigo.com", "attacker", false},
		{"contributor creates a cluster-scoped style group", "iam.aws.upbound.io", "attacker", false},
		{"contributor creates a Deployment", "apps", "attacker", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := map[string]any{
				"kind":     map[string]any{"group": tc.group, "kind": "Test"},
				"userInfo": map[string]any{"username": tc.username},
			}
			if got := allConditionsMatch(t, spec, request); got != tc.deny {
				t.Errorf("denied = %v, want %v (group %s, user %s)", got, tc.deny, tc.group, tc.username)
			}
		})
	}

	t.Run("crossplaneNamespace value is used", func(t *testing.T) {
		custom := renderManagedResourcePolicy(t, "cp-other")
		request := func(username string) map[string]any {
			return map[string]any{
				"kind":     map[string]any{"group": "iam.aws.m.upbound.io", "kind": "Test"},
				"userInfo": map[string]any{"username": username},
			}
		}
		if allConditionsMatch(t, custom, request("system:serviceaccount:cp-other:crossplane")) {
			t.Error("Crossplane in the configured namespace must be allowed")
		}
		if !allConditionsMatch(t, custom, request("system:serviceaccount:crossplane-system:crossplane")) {
			t.Error("the default namespace must not be exempt once crossplaneNamespace is changed")
		}
	})
}

type managedResourcePolicySpec struct {
	MatchConstraints struct {
		NamespaceSelector struct {
			MatchExpressions []struct {
				Key      string   `yaml:"key"`
				Operator string   `yaml:"operator"`
				Values   []string `yaml:"values"`
			} `yaml:"matchExpressions"`
		} `yaml:"namespaceSelector"`
		ResourceRules []struct {
			Operations []string `yaml:"operations"`
		} `yaml:"resourceRules"`
	} `yaml:"matchConstraints"`
	MatchConditions []struct {
		Name       string `yaml:"name"`
		Expression string `yaml:"expression"`
	} `yaml:"matchConditions"`
}

func renderManagedResourcePolicy(t *testing.T, crossplaneNamespace string) managedResourcePolicySpec {
	t.Helper()
	out, err := exec.Command("helm", "template", "test", chartDir,
		"--set", "zone.crossplaneNamespace="+crossplaneNamespace).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	for _, doc := range strings.Split(string(out), "\n---") {
		var obj struct {
			Metadata struct {
				Name string `yaml:"name"`
			} `yaml:"metadata"`
			Spec managedResourcePolicySpec `yaml:"spec"`
		}
		if yaml.Unmarshal([]byte(doc), &obj) == nil && obj.Metadata.Name == "platform-apis-zone-managed-resource-deny" {
			if len(obj.Spec.MatchConditions) == 0 {
				t.Fatal("policy has no matchConditions")
			}
			return obj.Spec
		}
	}
	t.Fatal("platform-apis-zone-managed-resource-deny not found in rendered chart")
	return managedResourcePolicySpec{}
}

func allConditionsMatch(t *testing.T, spec managedResourcePolicySpec, request map[string]any) bool {
	t.Helper()
	env, err := cel.NewEnv(cel.Variable("request", cel.DynType))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range spec.MatchConditions {
		ast, iss := env.Compile(c.Expression)
		if iss.Err() != nil {
			t.Fatalf("condition %s: %v", c.Name, iss.Err())
		}
		prg, err := env.Program(ast)
		if err != nil {
			t.Fatalf("condition %s: %v", c.Name, err)
		}
		val, _, err := prg.Eval(map[string]any{"request": request})
		if err != nil {
			t.Fatalf("condition %s: %v", c.Name, err)
		}
		if matched, ok := val.Value().(bool); !ok || !matched {
			return false
		}
	}
	return true
}
