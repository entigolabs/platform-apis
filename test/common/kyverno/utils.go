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

// defaultUserInfo provides a non-privileged user context for tests that don't specify one.
// groups must be non-empty: kyverno CLI does not populate request.userInfo.groups from an
// empty list, causing CEL expressions like `"contributor" in request.userInfo.groups` to error
// instead of returning false. Using system:authenticated avoids matching any policy conditions.
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
	t.Log(output)

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

// applyPolicies renders policies, injects mocks, and runs kyverno apply. Returns the combined output.
// For built-in Kubernetes resource types (Namespace, Pod, …) it uses file-based offline mode.
// For custom resources (e.g. Zone) it spins up a minimal fake Kubernetes API server so that
// kyverno can resolve the CRD GVR without requiring a live cluster.
func applyPolicies(t *testing.T, chartDir string, scenario TestScenario) string {
	t.Helper()
	tmpDir := t.TempDir()

	rendered := injectOfflineMocks(renderHelm(t, chartDir, scenario.HelmValues))
	kind := resourceKind(scenario.ResourceYAML)
	policies := extractKyvernoPolicies(rendered, kind)
	if policies == "" {
		t.Fatalf("no Kyverno policies found for resource kind %q", kind)
	}

	policyFile := writeTempFile(t, tmpDir, "policy.yaml", policies)

	userInfoYAML := scenario.UserInfoYAML
	if userInfoYAML == "" {
		userInfoYAML = defaultUserInfo
	}
	userInfoFile := writeTempFile(t, tmpDir, "userinfo.yaml", userInfoYAML)

	var args []string
	var env []string

	if isBuiltinResource(resourceAPIVersion(scenario.ResourceYAML)) {
		// Offline file-based mode — no cluster access required.
		args = []string{"apply", policyFile,
			"--resource", writeTempFile(t, tmpDir, "resource.yaml", scenario.ResourceYAML),
		}
		if scenario.VariablesYAML != "" {
			args = append(args, "--values-file", writeTempFile(t, tmpDir, "values.yaml", scenario.VariablesYAML))
		}
		args = append(args, "--userinfo", userInfoFile)
		env = append(os.Environ(), "KUBECONFIG=/dev/null")
	} else {
		// Custom resource: spin up a minimal fake API server so kyverno can resolve the CRD GVR.
		kubeconfigFile := startFakeCluster(t, tmpDir, scenario.ResourceYAML)
		args = []string{"apply", policyFile,
			"--cluster",
			"--kubeconfig", kubeconfigFile,
		}
		args = append(args, "--userinfo", userInfoFile)
		env = os.Environ()
	}

	cmd := exec.Command("kyverno", args...)
	cmd.Env = env
	out, _ := cmd.CombinedOutput()
	output := string(out)

	if !strings.Contains(output, "pass:") {
		t.Fatalf("kyverno did not produce a summary — check policy syntax\n%s", output)
	}
	return output
}

// isBuiltinResource returns true when apiVersion belongs to a built-in Kubernetes group.
// Custom resources (e.g. tenancy.entigo.com/v1alpha1) require a live or fake cluster to resolve GVR.
func isBuiltinResource(apiVersion string) bool {
	for _, prefix := range []string{"v1", "apps/", "batch/", "networking.k8s.io/", "rbac.authorization.k8s.io/"} {
		if apiVersion == prefix || strings.HasPrefix(apiVersion, prefix) {
			return true
		}
	}
	return false
}

// startFakeCluster launches an in-process fake Kubernetes API server that registers the CRD for
// the given resource so that kyverno can resolve its GVR. It returns the path to a kubeconfig
// that points at that server; the server is stopped when the test ends.
func startFakeCluster(t *testing.T, tmpDir, resourceYAML string) (kubeconfigPath string) {
	t.Helper()

	apiVersion := resourceAPIVersion(resourceYAML)
	kind := resourceKind(resourceYAML)
	name := resourceName(resourceYAML)
	plural := strings.ToLower(kind) + "s"

	parts := strings.SplitN(apiVersion, "/", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected apiVersion format %q", apiVersion)
	}
	group, version := parts[0], parts[1]

	// Build the single resource object served by the fake cluster.
	resourceObj := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name},
		"spec":       map[string]interface{}{},
	}
	resourceList := map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind + "List",
		"metadata":   map[string]interface{}{},
		"items":      []interface{}{resourceObj},
	}

	mustJSON := func(v interface{}) []byte {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("json.Marshal: %v", err)
		}
		return b
	}

	mux := http.NewServeMux()

	respond := func(w http.ResponseWriter, v interface{}) {
		b := mustJSON(v)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(b)
	}

	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{"major": "1", "minor": "28", "gitVersion": "v1.28.0"})
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{"kind": "APIVersions", "versions": []string{"v1"}})
	})
	mux.HandleFunc("/api/v1", func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]interface{}{
			"kind": "APIResourceList", "apiVersion": "v1", "groupVersion": "v1", "resources": []interface{}{},
		})
	})
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
	mux.HandleFunc("/apis/"+apiVersion+"/"+plural, func(w http.ResponseWriter, r *http.Request) {
		respond(w, resourceList)
	})
	mux.HandleFunc("/apis/"+apiVersion+"/"+plural+"/"+name, func(w http.ResponseWriter, r *http.Request) {
		respond(w, resourceObj)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	kubeconfig := fmt.Sprintf(`apiVersion: v1
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

// injectOfflineMocks replaces resource.List() CEL calls with static inline data so tests run without cluster access.
// All items in each list have identical field structure — CEL rejects lists with mixed value types.
func injectOfflineMocks(policies string) string {
	policies = strings.ReplaceAll(policies,
		`resource.List("tenancy.entigo.com/v1alpha1", "zones", "")`,
		`{"items": [{"metadata": {"name": "my-zone"}}, {"metadata": {"name": "default-zone-name"}}]}`)
	policies = strings.ReplaceAll(policies,
		`resource.List("v1", "namespaces", "")`,
		`{"items": [{"metadata": {"name": "attached-ns"}}, {"metadata": {"name": "stolen-ns"}}]}`)
	return policies
}

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

// extractKyvernoPolicies returns policies from fullYAML that target the given resource kind.
// Filtering prevents errors from policies targeting unrelated resource types (e.g. Zone policies
// erroring when applied to a Namespace test resource).
func extractKyvernoPolicies(fullYAML, kind string) string {
	// Simple pluralisation covers all resource kinds used here (Namespace→namespaces, Zone→zones).
	resource := `"` + strings.ToLower(kind) + `s"`
	var policies []string
	for _, doc := range strings.Split(fullYAML, "---") {
		if strings.Contains(doc, "apiVersion: policies.kyverno.io") && strings.Contains(doc, resource) {
			policies = append(policies, strings.TrimSpace(doc))
		}
	}
	return strings.Join(policies, "\n---\n")
}

// resourceAPIVersion parses the `apiVersion:` field from a YAML resource manifest.
func resourceAPIVersion(resourceYAML string) string {
	for _, line := range strings.Split(resourceYAML, "\n") {
		if kv := strings.SplitN(strings.TrimSpace(line), ":", 2); len(kv) == 2 && kv[0] == "apiVersion" {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

// resourceKind parses the `kind:` field from a YAML resource manifest.
func resourceKind(resourceYAML string) string {
	for _, line := range strings.Split(resourceYAML, "\n") {
		if kv := strings.SplitN(strings.TrimSpace(line), ":", 2); len(kv) == 2 && kv[0] == "kind" {
			return strings.TrimSpace(kv[1])
		}
	}
	return ""
}

// resourceName parses the `metadata.name:` field from a YAML resource manifest.
func resourceName(resourceYAML string) string {
	inMetadata := false
	for _, line := range strings.Split(resourceYAML, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "metadata:" {
			inMetadata = true
			continue
		}
		if inMetadata {
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && trimmed != "" {
				inMetadata = false
				continue
			}
			if kv := strings.SplitN(trimmed, ":", 2); len(kv) == 2 && kv[0] == "name" {
				return strings.TrimSpace(kv[1])
			}
		}
	}
	return ""
}

func writeTempFile(t *testing.T, dir, filename, content string) string {
	t.Helper()
	path := filepath.Join(dir, filename)
	content = strings.TrimSpace(strings.ReplaceAll(content, "\t", "  "))
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", filename, err)
	}
	return path
}
