package kyverno

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestScenario describes a single Kyverno policy test case.
type TestScenario struct {
	HelmValues       map[string]string
	ResourceYAML     string
	VariablesYAML    string
	UserInfoYAML     string
	ExpectedAction   string
	ExpectedInOutput string
}

// K8sResource holds the fields parsed from a resource YAML needed for policy routing.
type K8sResource struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec map[string]interface{} `yaml:"spec"`
}

const defaultUserInfo = `
apiVersion: cli.kyverno.io/v1alpha1
kind: UserInfo
metadata:
  name: user-info
userInfo:
  username: test-user
  groups:
  - system:authenticated
roles: []
clusterRoles: []
`

// RunPolicyCheck renders the Helm chart, applies offline mocks, runs kyverno CLI, and asserts the result.
func RunPolicyCheck(t *testing.T, chartDir string, scenario TestScenario) {
	t.Helper()

	output := applyPolicies(t, chartDir, scenario)
	passed := strings.Contains(output, "fail: 0") && strings.Contains(output, "error: 0")

	assertAction(t, scenario.ExpectedAction, passed, output)
	assertOutputContains(t, scenario.ExpectedInOutput, output)
}

// applyPolicies renders policies, writes temp files, runs kyverno, and returns the combined output.
func applyPolicies(t *testing.T, chartDir string, scenario TestScenario) string {
	t.Helper()
	tmpDir := t.TempDir()

	policyFile := preparePolicies(t, chartDir, tmpDir, scenario)
	userInfoFile := prepareUserInfo(t, tmpDir, scenario.UserInfoYAML)
	args, env := buildKyvernoCommand(t, tmpDir, policyFile, userInfoFile, scenario)

	output := runCommand(t, env, "kyverno", args...)

	if !strings.Contains(output, "pass:") {
		t.Fatalf("kyverno did not produce a summary — check policy syntax\n%s", output)
	}
	return output
}

// buildKyvernoCommand assembles the kyverno apply arguments and environment.
// Built-in resources (Namespace, Pod, …) use file-based offline mode with KUBECONFIG=/dev/null.
// Custom resources (Zone, Application, …) require a fake Kubernetes API server to resolve the CRD GVR.
func buildKyvernoCommand(t *testing.T, tmpDir, policyFile, userInfoFile string, s TestScenario) ([]string, []string) {
	res := parseK8sYAML(t, s.ResourceYAML)
	args := []string{"apply", policyFile, "--userinfo", userInfoFile}
	env := os.Environ()

	if isBuiltinResource(res.APIVersion) {
		resourcePath := writeTempFile(t, tmpDir, "resource.yaml", s.ResourceYAML)
		args = append(args, "--resource", resourcePath)
		env = append(env, "KUBECONFIG=/dev/null")
	} else {
		kubeconfigFile := startFakeCluster(t, tmpDir, res)
		args = append(args, "--cluster", "--kubeconfig", kubeconfigFile)
	}

	if s.VariablesYAML != "" {
		varsPath := writeTempFile(t, tmpDir, "values.yaml", s.VariablesYAML)
		args = append(args, "--values-file", varsPath)
	}

	return args, env
}

// preparePolicies renders the Helm chart, injects offline mocks, extracts policies
// relevant to the tested resource kind, and writes them to a temp file.
func preparePolicies(t *testing.T, chartDir, tmpDir string, s TestScenario) string {
	res := parseK8sYAML(t, s.ResourceYAML)
	isBuiltin := isBuiltinResource(res.APIVersion)

	helmOutput := renderHelm(t, chartDir, s.HelmValues)
	rendered := injectOfflineMocks(helmOutput, isBuiltin)
	policies := extractKyvernoPolicies(rendered, res.Kind)

	if policies == "" {
		t.Fatalf("no Kyverno policies found for resource kind %q", res.Kind)
	}
	return writeTempFile(t, tmpDir, "policy.yaml", policies)
}

// prepareUserInfo writes the user info YAML to a temp file, using a default non-privileged user if none is provided.
func prepareUserInfo(t *testing.T, tmpDir, userYAML string) string {
	if userYAML == "" {
		userYAML = defaultUserInfo
	}
	return writeTempFile(t, tmpDir, "userinfo.yaml", userYAML)
}

// parseK8sYAML unmarshals a Kubernetes resource YAML string into a K8sResource.
func parseK8sYAML(t *testing.T, yamlStr string) K8sResource {
	t.Helper()
	var res K8sResource
	if err := yaml.Unmarshal([]byte(yamlStr), &res); err != nil {
		t.Fatalf("failed to parse resource YAML: %v", err)
	}
	return res
}

// isBuiltinResource reports whether the apiVersion belongs to a core Kubernetes group.
// Custom resources (e.g. tenancy.entigo.com/v1alpha1) return false and require a fake cluster.
func isBuiltinResource(apiVersion string) bool {
	prefixes := []string{"v1", "apps/", "batch/", "networking.k8s.io/", "rbac.authorization.k8s.io/"}
	for _, p := range prefixes {
		if strings.HasPrefix(apiVersion, p) {
			return true
		}
	}
	return false
}

// extractKyvernoPolicies returns the subset of YAML documents from fullYAML that are
// Kyverno policies targeting the given resource kind, preventing unrelated policy errors.
func extractKyvernoPolicies(fullYAML, kind string) string {
	resource := `"` + strings.ToLower(kind) + `s"`
	var policies []string
	for _, doc := range strings.Split(fullYAML, "---") {
		if strings.Contains(doc, "apiVersion: policies.kyverno.io") && strings.Contains(doc, resource) {
			policies = append(policies, strings.TrimSpace(doc))
		}
	}
	return strings.Join(policies, "\n---\n")
}

// injectOfflineMocks replaces resource.List() CEL calls with static data so tests run without cluster access.
func injectOfflineMocks(policies string, includeNamespaceMock bool) string {
	policies = strings.ReplaceAll(policies,
		`resource.List("tenancy.entigo.com/v1alpha1", "zones", "")`,
		`{"items": [{"metadata": {"name": "my-zone"}}, {"metadata": {"name": "default-zone-name"}}]}`)

	if includeNamespaceMock {
		policies = strings.ReplaceAll(policies,
			`resource.List("v1", "namespaces", "")`,
			`{"items": [{"metadata": {"name": dyn("attached-ns"), "labels": dyn({"tenancy.entigo.com/zone": "my-zone"})}}, {"metadata": {"name": dyn("stolen-ns"), "labels": dyn({})}}]}`)
	} else {
		policies = strings.ReplaceAll(policies,
			"  matchConditions:\n  - name: delete-only\n    expression: request.operation == \"DELETE\"\n",
			"")
		policies = strings.ReplaceAll(policies, "oldObject.", "object.")
	}
	return policies
}

// renderHelm runs helm template with the given values and returns the rendered YAML.
func renderHelm(t *testing.T, chartPath string, values map[string]string) string {
	t.Helper()
	args := []string{"template", "test-release", chartPath}
	for k, v := range values {
		args = append(args, "--set", fmt.Sprintf("%s=%s", k, v))
	}
	out, err := exec.Command("helm", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %s\n%s", err, out)
	}
	return string(out)
}

// startFakeCluster launches an in-process HTTP server that mimics the Kubernetes API for the
// given CRD resource, returning a path to a kubeconfig that points at it.
// The server is stopped when the test ends via t.Cleanup.
func startFakeCluster(t *testing.T, tmpDir string, res K8sResource) string {
	t.Helper()

	mux := http.NewServeMux()
	registerDiscoveryRoutes(mux, res)
	registerResourceRoutes(mux, res)
	registerStaticNamespaceRoutes(mux)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	return writeKubeconfig(t, tmpDir, server.URL)
}

// registerDiscoveryRoutes adds /version, /apis, and /apis/<group>/<version> handlers
// so kyverno can discover and resolve the CRD GVR for the given resource.
func registerDiscoveryRoutes(mux *http.ServeMux, res K8sResource) {
	parts := strings.SplitN(res.APIVersion, "/", 2)
	group, version := res.APIVersion, "v1"
	if len(parts) == 2 {
		group, version = parts[0], parts[1]
	}
	plural := strings.ToLower(res.Kind) + "s"

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, map[string]interface{}{"major": "1", "minor": "28", "gitVersion": "v1.28.0"})
	})

	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, map[string]interface{}{
			"kind": "APIGroupList", "apiVersion": "v1",
			"groups": []interface{}{
				map[string]interface{}{
					"name":             group,
					"versions":         []interface{}{map[string]interface{}{"groupVersion": res.APIVersion, "version": version}},
					"preferredVersion": map[string]interface{}{"groupVersion": res.APIVersion, "version": version},
				},
			},
		})
	})

	mux.HandleFunc("/apis/"+res.APIVersion, func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, map[string]interface{}{
			"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": res.APIVersion,
			"resources": []interface{}{
				map[string]interface{}{
					"name": plural, "singularName": strings.ToLower(res.Kind),
					"namespaced": false, "kind": res.Kind,
					"verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})
}

// registerResourceRoutes adds list and get endpoints for the tested resource under /apis/<group>/<version>/<plural>.
func registerResourceRoutes(mux *http.ServeMux, res K8sResource) {
	plural := strings.ToLower(res.Kind) + "s"

	if res.Spec == nil {
		res.Spec = map[string]interface{}{}
	}

	resourceObj := map[string]interface{}{
		"apiVersion": res.APIVersion,
		"kind":       res.Kind,
		"metadata":   map[string]interface{}{"name": res.Metadata.Name},
		"spec":       res.Spec,
	}
	resourceList := map[string]interface{}{
		"apiVersion": res.APIVersion,
		"kind":       res.Kind + "List",
		"metadata":   map[string]interface{}{},
		"items":      []interface{}{resourceObj},
	}

	mux.HandleFunc("/apis/"+res.APIVersion+"/"+plural, func(w http.ResponseWriter, r *http.Request) { sendJSON(w, resourceList) })
	mux.HandleFunc("/apis/"+res.APIVersion+"/"+plural+"/"+res.Metadata.Name, func(w http.ResponseWriter, r *http.Request) { sendJSON(w, resourceObj) })
}

// registerStaticNamespaceRoutes adds /api/v1 discovery and /api/v1/namespaces endpoints.
func registerStaticNamespaceRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		sendJSON(w, map[string]interface{}{
			"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": "v1",
			"resources": []interface{}{
				map[string]interface{}{
					"name": "namespaces", "singularName": "namespace",
					"namespaced": false, "kind": "Namespace",
					"verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})

	namespaceList := map[string]interface{}{
		"apiVersion": "v1", "kind": "NamespaceList", "metadata": map[string]interface{}{},
		"items": []interface{}{
			map[string]interface{}{
				"apiVersion": "v1", "kind": "Namespace",
				"metadata": map[string]interface{}{
					"name":   "attached-ns",
					"labels": map[string]interface{}{"tenancy.entigo.com/zone": "my-zone"},
				},
			},
			map[string]interface{}{
				"apiVersion": "v1", "kind": "Namespace",
				"metadata": map[string]interface{}{
					"name":   "stolen-ns",
					"labels": map[string]interface{}{},
				},
			},
		},
	}
	mux.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) { sendJSON(w, namespaceList) })
}

// writeKubeconfig writes a minimal kubeconfig pointing at serverURL and returns its path.
func writeKubeconfig(t *testing.T, tmpDir, serverURL string) string {
	kubeconfig := fmt.Sprintf(`
apiVersion: v1
kind: Config
clusters:
- cluster:
    server: %s
  name: fake
contexts:
- context:
    cluster: fake
    user: fake
  name: fake
current-context: fake
users:
- name: fake
  user:
    token: fake`, serverURL)

	return writeTempFile(t, tmpDir, "kubeconfig.yaml", kubeconfig)
}

// sendJSON encodes v as JSON and writes it to w with the application/json content type.
func sendJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// assertAction fails the test if the actual pass/fail outcome does not match expected.
func assertAction(t *testing.T, expected string, passed bool, output string) {
	t.Helper()
	if expected == "pass" && !passed {
		t.Errorf("expected policy PASS\n%s", output)
	} else if expected == "fail" && passed {
		t.Errorf("expected policy FAIL\n%s", output)
	}
}

// assertOutputContains fails the test if expected is non-empty and not found in output.
func assertOutputContains(t *testing.T, expected string, output string) {
	t.Helper()
	if expected != "" && !strings.Contains(output, expected) {
		t.Errorf("expected %q in output\n%s", expected, output)
	}
}

// runCommand executes the named command with args and returns its combined stdout+stderr.
func runCommand(t *testing.T, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	return string(out)
}

// writeTempFile writes content to dir/filename, normalising tabs to spaces, and returns the full path.
func writeTempFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	content = strings.TrimSpace(strings.ReplaceAll(content, "\t", "  "))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", filename, err)
	}
	return path
}

// GenerateZone returns a Zone resource YAML with the given name.
func GenerateZone(name string) string {
	return fmt.Sprintf(`
apiVersion: tenancy.entigo.com/v1alpha1
kind: Zone
metadata:
  name: %s
`, name)
}

// GenerateZoneWithNamespaces returns a Zone resource YAML with spec.namespaces populated.
func GenerateZoneWithNamespaces(name string, namespaces []string) string {
	var nsLines string
	for _, ns := range namespaces {
		nsLines += "\n    - name: " + ns
	}
	return fmt.Sprintf(`
apiVersion: tenancy.entigo.com/v1alpha1
kind: Zone
metadata:
  name: %s
spec:
  namespaces:%s
`, name, nsLines)
}

// GenerateNamespace returns a Namespace resource YAML with zone and pod-security labels.
func GenerateNamespace(name, zone, enforce, warn string) string {
	return fmt.Sprintf(`
apiVersion: v1
kind: Namespace
metadata:
  name: %s
  labels:
    tenancy.entigo.com/zone: %s
    pod-security.kubernetes.io/enforce: %s
    pod-security.kubernetes.io/warn: %s
`, name, zone, enforce, warn)
}

// GenerateUserInfo returns a kyverno UserInfo YAML placing the given group names in request.userInfo.groups.
func GenerateUserInfo(groups ...string) string {
	list := ""
	for _, g := range groups {
		list += "\n  - " + g
	}
	return fmt.Sprintf(`
apiVersion: cli.kyverno.io/v1alpha1
kind: UserInfo
metadata:
  name: user-info
userInfo:
  username: test-user
  groups:%s
roles: []
clusterRoles: []
`, list)
}

// GenerateArgoApp returns an ArgoCD Application resource YAML for the given name, project, and destination namespace.
func GenerateArgoApp(name, project, destNamespace string) string {
	return fmt.Sprintf(`
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: %s
spec:
  project: %s
  destination:
    namespace: %s
`, name, project, destNamespace)
}

// GenerateOperationValues returns a kyverno Values YAML that sets request.operation to the given value.
func GenerateOperationValues(operation string) string {
	return fmt.Sprintf(`
apiVersion: cli.kyverno.io/v1alpha1
kind: Values
metadata:
  name: values
globalValues:
  request.operation: %s
`, operation)
}
