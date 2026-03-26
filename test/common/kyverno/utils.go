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
)

// TestScenario describes a single Kyverno policy test case
type TestScenario struct {
	HelmValues       map[string]string
	ResourceYAML     string
	VariablesYAML    string // values file (kind: Values) — policy variables and globalValues
	UserInfoYAML     string // user info file (kind: UserInfo) — request.userInfo.groups, username
	ExpectedAction   string // "pass" or "fail"
	ExpectedInOutput string // substring expected in kyverno output (e.g. for mutations)
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

	switch scenario.ExpectedAction {
	case "pass":
		if !passed {
			t.Errorf("expected policy PASS\n%s", output)
		}
	case "fail":
		if passed {
			t.Errorf("expected policy FAIL\n%s", output)
		}
	}

	if scenario.ExpectedInOutput != "" && !strings.Contains(output, scenario.ExpectedInOutput) {
		t.Errorf("expected %q in output\n%s", scenario.ExpectedInOutput, output)
	}
}

// applyPolicies orchestrator of policies rendering, mocks injecting, kyverno command. Returns the combined output.
func applyPolicies(t *testing.T, chartDir string, scenario TestScenario) string {
	t.Helper()
	tmpDir := t.TempDir()

	policyFile := preparePolicies(t, chartDir, tmpDir, scenario)
	userInfoFile := prepareUserInfo(t, tmpDir, scenario.UserInfoYAML)

	args, env := buildKyvernoCommand(t, tmpDir, policyFile, userInfoFile, scenario)

	cmd := exec.Command("kyverno", args...)
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	output := string(out)

	if !strings.Contains(output, "pass:") {
		t.Fatalf("kyverno did not produce a summary — check policy syntax\n%s", output)
	}
	t.Logf("kyverno args: %v\noutput:\n%s", args, output)
	return output
}

// preparePolicies renders policies and injects offline mocks
func preparePolicies(t *testing.T, chartDir, tmpDir string, s TestScenario) string {
	apiVersion, kind, _ := parseResourceMeta(s.ResourceYAML)
	// In file mode (builtin resources), inject a static inline namespace mock so that
	// zone-targeting policies with resource.List("v1","namespaces","") don't try to hit
	// the API server (KUBECONFIG=/dev/null). In cluster mode the fake server handles it.
	rendered := injectOfflineMocks(renderHelm(t, chartDir, s.HelmValues), isBuiltinResource(apiVersion))

	policies := extractKyvernoPolicies(rendered, kind)
	if policies == "" {
		t.Fatalf("no Kyverno policies found for resource kind %q (API: %s)", kind, apiVersion)
	}
	return writeTempFile(t, tmpDir, "policy.yaml", policies)
}

// prepareUserInfo prepares user info
func prepareUserInfo(t *testing.T, tmpDir, userYAML string) string {
	if userYAML == "" {
		userYAML = defaultUserInfo
	}
	return writeTempFile(t, tmpDir, "userinfo.yaml", userYAML)
}

// buildKyvernoCommand builds required for test kyverno CLI command
// For built-in Kubernetes resource types (Namespace, Pod, …) it uses file-based offline mode.
// For custom resources (e.g. Zone) it spins up a minimal fake Kubernetes API server so that
// kyverno can not resolve the CRD GVR without requiring a live cluster.
func buildKyvernoCommand(t *testing.T, tmpDir, policyFile, userInfoFile string, s TestScenario) ([]string, []string) {
	apiVersion, _, _ := parseResourceMeta(s.ResourceYAML)

	args := []string{"apply", policyFile, "--userinfo", userInfoFile}
	env := os.Environ()

	if isBuiltinResource(apiVersion) {
		// file based offline mode
		resourcePath := writeTempFile(t, tmpDir, "resource.yaml", s.ResourceYAML)
		args = append(args, "--resource", resourcePath)
		env = append(env, "KUBECONFIG=/dev/null")
	} else {
		// K8 API server mode
		kubeconfigFile := startFakeCluster(t, tmpDir, s.ResourceYAML)
		args = append(args, "--cluster", "--kubeconfig", kubeconfigFile)
	}
	if s.VariablesYAML != "" {
		varsPath := writeTempFile(t, tmpDir, "values.yaml", s.VariablesYAML)
		args = append(args, "--values-file", varsPath)
	}

	return args, env
}

// isBuiltinResource returns true when apiVersion belongs to a built-in Kubernetes group.
// Custom resources (e.g. tenancy.entigo.com/v1alpha1) require a live or fake cluster to resolve GVR.
func isBuiltinResource(apiVersion string) bool {
	prefixes := []string{"v1", "apps/", "batch/", "networking.k8s.io/", "rbac.authorization.k8s.io/"}
	for _, p := range prefixes {
		if strings.HasPrefix(apiVersion, p) {
			return true
		}
	}
	return false
}

// parseResourceMeta combined search and parsing of resource metadata
func parseResourceMeta(yaml string) (apiVersion, kind, name string) {
	inMetadata := false
	for _, line := range strings.Split(yaml, "\n") {
		trimmed := strings.TrimSpace(line)
		parts := strings.SplitN(trimmed, ":", 2)

		if len(parts) == 2 {
			key, val := parts[0], strings.TrimSpace(parts[1])
			switch key {
			case "apiVersion":
				apiVersion = val
			case "kind":
				kind = val
			case "metadata":
				inMetadata = true
			case "name":
				if inMetadata {
					name = val
				}
			}
		} else if trimmed != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			inMetadata = false
		}
	}
	return apiVersion, kind, name
}

// extractKyvernoPolicies returns policies from fullYAML that target the given resource kind.
// Filtering prevents errors from policies targeting unrelated resource types (e.g. Zone policies
// erroring when applied to a Namespace test resource).
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

// injectOfflineMocks replaces resource.List() CEL calls with static inline data so tests run without cluster access.
// includeNamespaceMock should be true in file mode (builtin resources like Namespace) where there is no fake cluster
// to serve /api/v1/namespaces. In cluster mode, the fake server handles the namespace list and has() works correctly
// on the real JSON objects it returns.
//
// When NOT in file mode (i.e. cluster mode for Zone tests) the delete-only matchCondition is also stripped
// because kyverno CLI always simulates CREATE and cannot simulate DELETE operations.
func injectOfflineMocks(policies string, includeNamespaceMock bool) string {
	policies = strings.ReplaceAll(policies,
		`resource.List("tenancy.entigo.com/v1alpha1", "zones", "")`,
		`{"items": [{"metadata": {"name": "my-zone"}}, {"metadata": {"name": "default-zone-name"}}]}`)
	if includeNamespaceMock {
		// CEL map literals require uniform value types. Both name and labels are wrapped in dyn() so the
		// metadata map has type map(string,dyn). Note: has() does not work reliably on dyn()-typed values;
		// these zone-targeting policies are skipped for Namespace resources so has() is never evaluated.
		policies = strings.ReplaceAll(policies,
			`resource.List("v1", "namespaces", "")`,
			`{"items": [{"metadata": {"name": dyn("attached-ns"), "labels": dyn({"tenancy.entigo.com/zone": "my-zone"})}}, {"metadata": {"name": dyn("stolen-ns"), "labels": dyn({})}}]}`)
	} else {
		// kyverno CLI always simulates CREATE; strip the delete-only matchCondition so the zone deletion
		// check policy executes and tests the underlying namespace-filtering logic.
		policies = strings.ReplaceAll(policies,
			"  matchConditions:\n  - name: delete-only\n    expression: request.operation == \"DELETE\"\n",
			"")
		// For DELETE operations kyverno uses oldObject, but the CLI only provides object (CREATE).
		// Replace oldObject references so the test uses the submitted resource's metadata/spec.
		policies = strings.ReplaceAll(policies, "oldObject.", "object.")
	}
	return policies
}

// renderHelm renders helm charts
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

// writeTempFile writes content to temporary file
func writeTempFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	content = strings.TrimSpace(strings.ReplaceAll(content, "\t", "  "))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", filename, err)
	}
	return path
}

// GenerateZone generates Zone for tests
func GenerateZone(name string) string {
	return fmt.Sprintf(`
apiVersion: tenancy.entigo.com/v1alpha1
kind: Zone
metadata:
  name: %s
`, name)
}

// GenerateNamespace generates Namespace for tests
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

// GenerateUserInfo generates userInfo for tests
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

// GenerateZoneWithNamespaces generates a Zone resource YAML with spec.namespaces.
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

// GenerateArgoApp generates an ArgoCD Application resource YAML for tests.
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

// GenerateOperationValues returns a kyverno Values YAML file overriding request.operation.
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

// parseSpecNamespaces extracts the spec: section from a YAML resource string and returns it
// as a nested map[string]interface{} for use in the fake cluster JSON responses.
func parseSpecNamespaces(resourceYAML string) interface{} {
	lines := strings.Split(strings.TrimSpace(resourceYAML), "\n")
	specStart := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "spec:" {
			specStart = i + 1
			break
		}
	}
	if specStart < 0 || specStart >= len(lines) {
		return nil
	}
	var specLines []string
	for _, line := range lines[specStart:] {
		cleaned := strings.TrimRight(line, " \t\r")
		if strings.TrimSpace(cleaned) == "" {
			continue
		}
		if len(cleaned) > 0 && cleaned[0] != ' ' && cleaned[0] != '\t' {
			break
		}
		specLines = append(specLines, cleaned)
	}
	if len(specLines) == 0 {
		return nil
	}
	m := yamlLinesToMap(specLines, yamlIndent(specLines[0]))
	if len(m) == 0 {
		return nil
	}
	return m
}

// yamlLinesToMap parses indented YAML lines into a map at the given indent depth.
// It handles scalar values, nested maps, and lists of single-key-value maps.
func yamlLinesToMap(lines []string, depth int) map[string]interface{} {
	result := map[string]interface{}{}
	i := 0
	for i < len(lines) {
		line := lines[i]
		ind := yamlIndent(line)
		if ind != depth {
			i++
			continue
		}
		trimmed := strings.TrimSpace(line)
		colon := strings.Index(trimmed, ":")
		if colon < 0 {
			i++
			continue
		}
		key := strings.TrimSpace(trimmed[:colon])
		val := strings.TrimSpace(trimmed[colon+1:])
		i++
		if val != "" {
			result[key] = val
			continue
		}
		var children []string
		for i < len(lines) {
			if yamlIndent(lines[i]) > depth {
				children = append(children, lines[i])
				i++
			} else {
				break
			}
		}
		if len(children) == 0 {
			continue
		}
		childDepth := yamlIndent(children[0])
		if strings.HasPrefix(strings.TrimSpace(children[0]), "- ") {
			var items []interface{}
			for _, child := range children {
				ct := strings.TrimSpace(child)
				if !strings.HasPrefix(ct, "- ") {
					continue
				}
				ct = strings.TrimPrefix(ct, "- ")
				c := strings.Index(ct, ":")
				if c < 0 {
					items = append(items, ct)
				} else {
					items = append(items, map[string]interface{}{
						strings.TrimSpace(ct[:c]): strings.TrimSpace(ct[c+1:]),
					})
				}
			}
			result[key] = items
		} else {
			result[key] = yamlLinesToMap(children, childDepth)
		}
	}
	return result
}

// yamlIndent returns the number of leading spaces in s.
func yamlIndent(s string) int {
	for i, c := range s {
		if c != ' ' && c != '\t' {
			return i
		}
	}
	return 0
}

// startFakeCluster launches an in-process fake Kubernetes API server that registers the CRD for
// the given resource so that kyverno can resolve its GVR. It returns the path to a kubeconfig
// that points at that server; the server is stopped when the test ends.
func startFakeCluster(t *testing.T, tmpDir, resourceYAML string) (kubeconfigPath string) {
	t.Helper()

	apiVersion, kind, name := parseResourceMeta(resourceYAML)
	plural := strings.ToLower(kind) + "s"
	parts := strings.SplitN(apiVersion, "/", 2)
	group, version := parts[0], parts[1]

	spec := parseSpecNamespaces(resourceYAML)
	if spec == nil {
		spec = map[string]interface{}{}
	}
	resourceObj := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec":       spec,
	}
	resourceList := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind + "List",
		"metadata":   map[string]interface{}{},
		"items":      []interface{}{resourceObj},
	}

	respond := func(w http.ResponseWriter, v interface{}) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(v)
	}

	// Static namespace list served to policies that call resource.List("v1","namespaces","").
	// Using real JSON objects (not inline CEL) ensures that has(ns.metadata.labels) works correctly.
	namespaceList := map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "NamespaceList",
		"metadata":   map[string]interface{}{},
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

	mux := http.NewServeMux()
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{"major": "1", "minor": "28", "gitVersion": "v1.28.0"})
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{
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
	mux.HandleFunc("/api/v1/namespaces", func(w http.ResponseWriter, r *http.Request) { respond(w, namespaceList) })
	mux.HandleFunc("/apis", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{
			"kind": "APIGroupList", "apiVersion": "v1",
			"groups": []interface{}{
				map[string]interface{}{
					"name":             group,
					"versions":         []interface{}{map[string]interface{}{"groupVersion": apiVersion, "version": version}},
					"preferredVersion": map[string]interface{}{"groupVersion": apiVersion, "version": version},
				},
			},
		})
	})
	mux.HandleFunc("/apis/"+apiVersion, func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{
			"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": apiVersion,
			"resources": []interface{}{
				map[string]interface{}{
					"name": plural, "singularName": strings.ToLower(kind),
					"namespaced": false, "kind": kind,
					"verbs": []string{"create", "delete", "get", "list", "patch", "update", "watch"},
				},
			},
		})
	})
	mux.HandleFunc("/apis/"+apiVersion+"/"+plural, func(w http.ResponseWriter, r *http.Request) { respond(w, resourceList) })
	mux.HandleFunc("/apis/"+apiVersion+"/"+plural+"/"+name, func(w http.ResponseWriter, r *http.Request) { respond(w, resourceObj) })
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

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
    token: fake`, server.URL)

	return writeTempFile(t, tmpDir, "kubeconfig.yaml", kubeconfig)
}
