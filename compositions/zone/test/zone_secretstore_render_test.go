package test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

// TestZoneSecretStoreRender renders a Zone with secretStore set in the EnvironmentConfig. The
// store objects are not part of the Zone sequence, so they are expected in the first render.
func TestZoneSecretStoreRender(t *testing.T) {
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	envWithStore := filepath.Join(tmpDir, "environment-config.yaml")
	requiredWithArn := filepath.Join(tmpDir, "required-resources.yaml")

	writeWithEdit(t, env, envWithStore, func(s string) string {
		return s + "\n  secretStore:\n    namespace: external-secrets\n"
	})
	writeWithEdit(t, required, requiredWithArn, func(s string) string {
		return strings.Replace(s, "    version: \"1.32\"",
			"    version: \"1.32\"\n    arn: arn:aws:eks:eu-north-1:111111111111:cluster/eks\n"+
				"    identity:\n    - oidc:\n      - issuer: https://oidc.eks.eu-north-1.amazonaws.com/id/ABC123", 1)
	})
	crossplane.AppendYamlToResources(t, envWithStore, extra)
	crossplane.AppendYamlToResources(t, requiredWithArn, extra)

	resources := crossplane.CrossplaneRender(t, zoneResource, composition, functionsConfig, crossplane.Ptr(extra), nil)

	crossplane.AssertResourceCount(t, resources, "ClusterSecretStore", 1)
	crossplane.AssertResourceCount(t, resources, "RolePolicy", 1)
	crossplane.AssertResourceCount(t, resources, "ServiceAccount", 1)

	crossplane.AssertFieldValues(t, resources, "ClusterSecretStore", "external-secrets.io/v1", map[string]string{
		"metadata.name":                   "zone-testzone",
		"metadata.ownerReferences.0.kind": "Zone",
		"metadata.ownerReferences.0.name": "testzone",
		"spec.conditions.0.namespaceSelector.matchLabels.tenancy\\.entigo\\.com/zone": "testzone",
		"spec.provider.aws.service":                              "SecretsManager",
		"spec.provider.aws.region":                               "eu-north-1",
		"spec.provider.aws.auth.jwt.serviceAccountRef.name":      "zone-testzone",
		"spec.provider.aws.auth.jwt.serviceAccountRef.namespace": "external-secrets",
	})
	crossplane.AssertFieldValues(t, resources, "ServiceAccount", "v1", map[string]string{
		"metadata.name":      "zone-testzone",
		"metadata.namespace": "external-secrets",
		"metadata.annotations.eks\\.amazonaws\\.com/role-arn": "arn:aws:iam::111111111111:role/testzone-secrets",
	})
	crossplane.AssertFieldValues(t, resources, "Role", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                     "testzone-secrets",
		"spec.forProvider.tags.entigo:zone": "testzone",
		"spec.providerConfigRef.name":       "crossplane-aws",
		"spec.forProvider.assumeRolePolicy": "*",
	})
	crossplane.AssertFieldValues(t, resources, "RolePolicy", "iam.aws.upbound.io/v1beta1", map[string]string{
		"metadata.name":                 "testzone-secrets",
		"spec.forProvider.roleRef.name": "testzone-secrets",
		"spec.forProvider.policy":       "*",
	})
}

func writeWithEdit(t *testing.T, src, dst string, edit func(string) string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	if err := os.WriteFile(dst, []byte(edit(string(data))), 0o600); err != nil {
		t.Fatalf("write %s: %v", dst, err)
	}
}
