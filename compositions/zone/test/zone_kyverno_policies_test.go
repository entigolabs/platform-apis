package test

import (
	"testing"

	"github.com/entigolabs/static-common/kyverno"
)

const chartDir = "../../../helm"

// TestKyvernoPolicies runs all Kyverno policy tests for the zone composition.
// Each subtest covers one policy; within each subtest the cases run in parallel.
func TestKyvernoPolicies(t *testing.T) {
	t.Run("NamespacePodSecurity", testNamespacePodSecurity)
	t.Run("ContributorDeny", testContributorDeny)
	t.Run("MaintainerNamespaceDeny", testMaintainerNamespaceDeny)
	t.Run("ZoneDeletionCheck", testZoneDeletionCheck)
	t.Run("ZoneNamespaceOwnership", testZoneNamespaceOwnership)
	t.Run("MaintainerInfralibZoneDeny", testMaintainerInfralibZoneDeny)
	t.Run("GenerateNamespaceFromArgoApp", testGenerateNamespaceFromArgoApp)
}

// testNamespacePodSecurity covers platform-apis-zone-namespace-pod-security (ValidatingPolicy)
// and platform-apis-namespace-add-missing-zone-label (MutatingPolicy).
func testNamespacePodSecurity(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "pass: restricted enforce+warn when setting is restricted",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateNamespace("good-ns", "my-zone", "restricted", "restricted"),
			},
		},
		{
			name: "fail: privileged enforce is denied when setting is restricted",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "my-zone", "privileged", "restricted"),
			},
		},
		{
			name: "fail: privileged warn is denied when setting is restricted",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "my-zone", "restricted", "privileged"),
			},
		},
		{
			name: "pass: baseline enforce+warn are allowed when setting is baseline",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				HelmValues: map[string]string{
					"zone.install":                       "true",
					"zone.environmentConfig.podSecurity": "baseline",
				},
				ResourceYAML: kyverno.GenerateNamespace("good-ns", "my-zone", "baseline", "baseline"),
			},
		},
		{
			// MutatingPolicy adds the zone label; ValidatingPolicy then accepts the mutated resource.
			name: "pass: namespace without zone label gets auto-assigned",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: `
apiVersion: v1
kind: Namespace
metadata:
  name: no-zone-ns
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/warn: restricted`,
			},
		},
		{
			name: "fail: zone label referencing non-existent zone is denied",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "non-existent-zone", "restricted", "restricted"),
			},
		},
		{
			name: "pass: system namespace kube-system is excluded",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: `
apiVersion: v1
kind: Namespace
metadata:
  name: kube-system`,
			},
		},
	}
	runCases(t, cases)
}

// testContributorDeny covers platform-apis-zone-namespace-contributor-deny (ValidatingPolicy).
// Contributors are denied all namespace operations regardless of operation type.
func testContributorDeny(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "fail: contributor cannot create a namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
				UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
			},
		},
		{
			name: "fail: contributor cannot update a namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
				UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
				VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
			},
		},
		{
			name: "fail: contributor cannot delete a namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
				UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
				VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
			},
		},
	}
	runCases(t, cases)
}

// testMaintainerNamespaceDeny covers platform-apis-zone-namespace-maintainer-deny (ValidatingPolicy).
// Maintainers cannot create or update namespaces that carry the infralib zone label.
func testMaintainerNamespaceDeny(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "fail: maintainer cannot create namespace with infralib zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("some-ns", "infralib", "restricted", "restricted"),
				UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
			},
		},
		{
			name: "fail: maintainer cannot update namespace with infralib zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateNamespace("some-ns", "infralib", "restricted", "restricted"),
				UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
				VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
			},
		},
		{
			name: "pass: maintainer can create namespace without infralib zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
				UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
			},
		},
	}
	runCases(t, cases)
}

// testZoneDeletionCheck covers platform-apis-zone-deletion-check-namespaces (ValidatingPolicy).
// "my-zone" is referenced by "attached-ns" in the offline namespace mock.
// "other-zone" has no namespaces in the offline namespace mock.
func testZoneDeletionCheck(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "fail: zone deletion blocked when namespaces still attached",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateZone("my-zone"),
				VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
			},
		},
		{
			name: "pass: zone deletion allowed when no namespaces attached",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateZone("other-zone"),
				VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
			},
		},
	}
	runCases(t, cases)
}

// testZoneNamespaceOwnership covers platform-apis-zone-namespace-ownership (ValidatingPolicy).
// "attached-ns" is labeled tenancy.entigo.com/zone=my-zone in the offline mock.
// "stolen-ns" has no zone label in the offline mock.
func testZoneNamespaceOwnership(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "fail: cannot create default zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateZone("default"),
			},
		},
		{
			name: "fail: cannot claim namespace owned by another zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateZoneWithNamespaces("new-zone", []string{"attached-ns"}),
			},
		},
		{
			name: "fail: cannot claim namespace without zone label",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateZoneWithNamespaces("new-zone", []string{"stolen-ns"}),
			},
		},
		{
			name: "pass: zone can manage its own namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateZoneWithNamespaces("my-zone", []string{"attached-ns"}),
			},
		},
	}
	runCases(t, cases)
}

// testMaintainerInfralibZoneDeny covers platform-apis-zone-maintainer-infralib-zone-deny (ValidatingPolicy).
// Maintainers cannot create or update the Zone named "infralib".
func testMaintainerInfralibZoneDeny(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "fail: maintainer cannot create the infralib zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateZone("infralib"),
				UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
			},
		},
		{
			name: "fail: maintainer cannot update the infralib zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   kyverno.GenerateZone("infralib"),
				UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
				VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
			},
		},
		{
			name: "pass: maintainer can create a non-infralib zone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateZone("maintainer-zone"),
				UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
			},
		},
	}
	runCases(t, cases)
}

// testGenerateNamespaceFromArgoApp covers generate-namespace-from-argocd-app (GeneratingPolicy).
// The infralib project is excluded by matchCondition and must not trigger generation.
func testGenerateNamespaceFromArgoApp(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "pass: ArgoApp generates namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction:   "pass",
				ResourceYAML:     kyverno.GenerateArgoApp("my-app", "my-project", "my-namespace"),
				ExpectedInOutput: "my-namespace",
			},
		},
		{
			name: "pass: ArgoApp with infralib project does not generate namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateArgoApp("infra-app", "infralib", "infra-namespace"),
			},
		},
	}
	runCases(t, cases)
}

// runCases iterates a table of named scenarios and runs each as a parallel subtest.
func runCases(t *testing.T, cases []struct {
	name     string
	scenario kyverno.TestScenario
}) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			kyverno.RunPolicyCheck(t, chartDir, tc.scenario)
		})
	}
}
