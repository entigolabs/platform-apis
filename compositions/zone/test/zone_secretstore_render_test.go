package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

// The store objects are not part of the Zone sequence, so they are expected in the first render.
func TestZoneSecretStoreRender(t *testing.T) {
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")

	envConfig := crossplane.ParseYamlFileToUnstructured(t, env)
	crossplane.AppendToResources(t, extra, crossplane.MockByKind(t, envConfig, "EnvironmentConfig", "apiextensions.crossplane.io/v1beta1", false, map[string]interface{}{
		"data.secretStoreNamespace": "external-secrets",
	}))
	for _, res := range crossplane.ParseYamlFileToUnstructured(t, required) {
		if res.GetKind() == "Cluster" {
			res = crossplane.Mock(t, res, false, map[string]interface{}{
				"status.atProvider.arn": "arn:aws:eks:eu-north-1:111111111111:cluster/eks",
				"status.atProvider.identity": []interface{}{map[string]interface{}{
					"oidc": []interface{}{map[string]interface{}{"issuer": "https://oidc.eks.eu-north-1.amazonaws.com/id/ABC123"}},
				}},
			})
		}
		crossplane.AppendToResources(t, extra, res)
	}

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
