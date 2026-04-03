package test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"text/template"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

// ── Template data types ───────────────────────────────────────────────────────

type kyvernoNsData struct {
	Name, Zone, Enforce, Warn string
}

type kyvernoZoneData struct {
	Name       string
	Namespaces []string
}

type kyvernoArgoAppData struct {
	Name, Namespace, DestNamespace, Project string
}

// ── Setup helpers ─────────────────────────────────────────────────────────────

// ensureKyvernoTestNamespace creates the shared kyverno-test-e2e namespace if it does not exist.
// The namespace is labeled tenancy.entigo.com/zone=a so that zone-deletion and ownership tests
// can rely on it being attached to zone "a".
func ensureKyvernoTestNamespace(t *testing.T, cluster *terrak8s.KubectlOptions) {
	t.Helper()
	applyFile(t, cluster, writeTempYAML(t, nsYAML(t, kyvernoNsData{
		Name: KyvernoTestNSName, Zone: ZoneAName, Enforce: "baseline", Warn: "baseline",
	})))
}

// roleKubectlOptions builds a temporary kubeconfig that authenticates to the same EKS cluster
// using the provided AWS IAM credentials, enabling role-based webhook testing.
// The context name must be an EKS ARN: arn:aws:eks:<region>:<account>:cluster/<name>.
func roleKubectlOptions(t *testing.T, base *terrak8s.KubectlOptions, keyID, secret string) *terrak8s.KubectlOptions {
	t.Helper()

	// Parse region and cluster name from EKS ARN context.
	// Format: arn:aws:eks:<region>:<account>:cluster/<name>
	parts := strings.SplitN(base.ContextName, ":", 6)
	region := parts[3]
	clusterName := parts[5]
	if idx := strings.LastIndex(clusterName, "/"); idx >= 0 {
		clusterName = clusterName[idx+1:]
	}

	ca, err := terrak8s.RunKubectlAndGetOutputE(t, base,
		"config", "view", "--raw", "--minify", "-o", "jsonpath={.clusters[0].cluster.certificate-authority-data}")
	require.NoError(t, err, "read cluster CA from kubeconfig")

	server, err := terrak8s.RunKubectlAndGetOutputE(t, base,
		"config", "view", "--minify", "-o", "jsonpath={.clusters[0].cluster.server}")
	require.NoError(t, err, "read cluster server from kubeconfig")

	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: role-cluster
contexts:
- context:
    cluster: role-cluster
    user: role-user
  name: role-context
current-context: role-context
users:
- name: role-user
  user:
    exec:
      apiVersion: client.authentication.k8s.io/v1beta1
      command: aws
      args:
      - eks
      - get-token
      - --cluster-name
      - %s
      - --region
      - %s
      env:
      - name: AWS_ACCESS_KEY_ID
        value: %s
      - name: AWS_SECRET_ACCESS_KEY
        value: %s
      interactiveMode: Never
`, ca, server, clusterName, region, keyID, secret)

	f, ferr := os.CreateTemp("", "kubeconfig-role-*.yaml")
	require.NoError(t, ferr)
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	_, err = f.WriteString(kubeconfig)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	return terrak8s.NewKubectlOptions("role-context", f.Name(), "")
}

// ── Kyverno dry-run helpers ───────────────────────────────────────────────────

// kyvernoDryRunApply applies yamlStr with --dry-run=server so admission webhooks fire
// but no resource is persisted. Returns the combined stdout output and any error.
func kyvernoDryRunApply(t *testing.T, opts *terrak8s.KubectlOptions, yamlStr string) (string, error) {
	t.Helper()
	return terrak8s.RunKubectlAndGetOutputE(t, opts, "apply", "-f", writeTempYAML(t, yamlStr), "--dry-run=server")
}

// writeTempYAML writes content to a temp file, schedules removal with t.Cleanup, and returns the path.
func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "kyverno-test-*.yaml")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

// renderTemplate loads templatePath, executes it with data, and returns the rendered string.
func renderTemplate(t *testing.T, templatePath string, data interface{}) string {
	t.Helper()
	raw, err := os.ReadFile(templatePath)
	require.NoError(t, err, "read template %s", templatePath)
	tmpl, err := template.New("").Parse(string(raw))
	require.NoError(t, err, "parse template %s", templatePath)
	var buf strings.Builder
	require.NoError(t, tmpl.Execute(&buf, data), "render template %s", templatePath)
	return buf.String()
}

// nsYAML renders the kyverno_namespace.yaml template.
// When Zone is empty the zone label is omitted (MutatingPolicy auto-assign test).
func nsYAML(t *testing.T, data kyvernoNsData) string {
	return renderTemplate(t, "./templates/kyverno_namespace.yaml", data)
}

// zoneYAML renders the kyverno_zone.yaml template.
func zoneYAML(t *testing.T, data kyvernoZoneData) string {
	return renderTemplate(t, "./templates/kyverno_zone.yaml", data)
}

// argoAppYAML renders the kyverno_argoapp.yaml template.
func argoAppYAML(t *testing.T, data kyvernoArgoAppData) string {
	return renderTemplate(t, "./templates/kyverno_argoapp.yaml", data)
}

// ── Assertion helpers ─────────────────────────────────────────────────────────

// assertKyvernoDenied asserts that the request was explicitly rejected by a Kyverno policy
// (combined output must contain "denied").
func assertKyvernoDenied(t *testing.T, out string, err error) {
	t.Helper()
	combined := out
	if err != nil {
		combined += " " + err.Error()
	}
	require.Error(t, err, "expected Kyverno to deny the request; output: %s", combined)
	require.Contains(t, strings.ToLower(combined), "denied",
		"rejection should mention 'denied'; output: %s", combined)
}

// assertKyvernoAllowed asserts that Kyverno did not deny the request.
// A non-nil err is acceptable provided it does not contain "denied" (e.g. CRD validation or
// resource-already-exists — anything that is not a policy denial).
func assertKyvernoAllowed(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	require.NotContains(t, strings.ToLower(err.Error()), "denied",
		"expected Kyverno to allow the request but got a denial: %v", err)
}

// assertForbidden asserts that the operation was blocked by either RBAC ("forbidden") or
// Kyverno ("denied"). Used for role-based tests where the specific rejection layer may vary
// depending on the cluster's RBAC configuration.
func assertForbidden(t *testing.T, out string, err error) {
	t.Helper()
	combined := strings.ToLower(out)
	if err != nil {
		combined += " " + strings.ToLower(err.Error())
	}
	require.Error(t, err, "expected operation to be blocked (RBAC or Kyverno); output: %s", combined)
	require.True(t, strings.Contains(combined, "denied") || strings.Contains(combined, "forbidden"),
		"rejection should mention 'denied' or 'forbidden'; output: %s", combined)
}
