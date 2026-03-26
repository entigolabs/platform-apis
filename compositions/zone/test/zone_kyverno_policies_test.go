package test

import (
	"testing"

	"github.com/entigolabs/kyverno-common"
)

const chartDir = "../../../helm"

var defaultHelmValues = map[string]string{
	"zone.install":                       "true",
	"zone.environmentConfig.podSecurity": "restricted",
}

func TestKyvernoPolicies(t *testing.T) {
	t.Run("pass: restricted enforce+warn when setting is restricted", testPassRestrictedEnforceWarnWithSettingRestricted)
	t.Run("fail: privileged enforce is denied when setting is restricted", testFailPrivilegedEnforceWithSettingRestricted)
	t.Run("fail: privileged warn is denied when setting is restricted", testFailPrivilegedWarnWithSettingRestricted)
	t.Run("pass: baseline enforce+warn are allowed when setting is baseline", testPassBaselineEnforceWarnWithSettingBaseline)
	t.Run("pass: namespace without zone label gets auto-assigned", testPassNamespaceWithoutZoneLabelAutoAssigned)
	t.Run("fail: zone label referencing non-existent zone is denied", testFailZoneLabelRefNonExistentZoneDenied)
	t.Run("pass: system namespace kube-system is excluded", testPassSystemNamespaceKubeSystemExcluded)

	t.Run("fail: contributor cannot create a namespace", testContributorCannotCreateNamespace)
	t.Run("fail: contributor cannot update a namespace", testContributorCannotUpdateNamespace)
	t.Run("fail: contributor cannot delete a namespace", testContributorCannotDeleteNamespace)

	t.Run("fail: maintainer cannot create namespace with infralib zone", testMaintainerCannotCreateNamespaceWithInfralibZone)
	t.Run("fail: maintainer cannot update namespace with infralib zone", testMaintainerCannotUpdateNamespaceWithInfralibZone)
	t.Run("pass: maintainer can create namespace without infralib zone", testMaintainerCanCreateNamespaceWithoutInfralibZone)

	t.Run("fail: zone deletion blocked when namespaces still attached", testZoneDeletionBlockedWhenNamespacesAttached)
	t.Run("pass: zone deletion allowed when no namespaces attached", testZoneDeletionAllowedWhenNoNamespacesAttached)

	t.Run("fail: cannot create default zone", testCannotCreateDefaultZone)
	t.Run("fail: cannot claim namespace owned by another zone", testCannotClaimNamespaceOwnedByAnotherZone)
	t.Run("fail: cannot claim namespace without zone label", testCannotClaimNamespaceWithoutZoneLabel)
	t.Run("pass: zone can manage its own namespace", testZoneCanManageItsOwnNamespace)

	t.Run("fail: maintainer cannot create the infralib zone", testMaintainerCannotCreateInfralibZone)
	t.Run("fail: maintainer cannot update the infralib zone", testMaintainerCannotUpdateInfralibZone)
	t.Run("pass: maintainer can create a non-infralib zone", testMaintainerCanCreateNonInfralibZone)

	t.Run("pass: ArgoApp generates namespace", testArgoAppGeneratesNamespace)
	t.Run("pass: ArgoApp with infralib project does not generate namespace", testArgoAppInfralibProjectSkipped)
}

// NamespacePodSecurity — platform-apis-zone-namespace-pod-security
// Validates that namespaces have correct pod-security labels for the configured level.

func testPassRestrictedEnforceWarnWithSettingRestricted(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML:   kyverno.GenerateNamespace("good-ns", "my-zone", "restricted", "restricted"),
	}
	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testFailPrivilegedEnforceWithSettingRestricted(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "my-zone", "privileged", "restricted"),
	}
	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testFailPrivilegedWarnWithSettingRestricted(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "my-zone", "restricted", "privileged"),
	}
	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testPassBaselineEnforceWarnWithSettingBaseline(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		HelmValues: map[string]string{
			"zone.install":                       "true",
			"zone.environmentConfig.podSecurity": "baseline",
		},
		ResourceYAML: kyverno.GenerateNamespace("good-ns", "my-zone", "baseline", "baseline"),
	}
	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testPassNamespaceWithoutZoneLabelAutoAssigned(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML: `
apiVersion: v1
kind: Namespace
metadata:
  name: no-zone-ns
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/warn: restricted`,
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testFailZoneLabelRefNonExistentZoneDenied(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("bad-ns", "non-existent-zone", "restricted", "restricted"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testPassSystemNamespaceKubeSystemExcluded(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML: `
apiVersion: v1
kind: Namespace
metadata:
  name: kube-system`,
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

// ContributorCannotModifyNamespace — platform-apis-zone-namespace-contributor-deny
// Contributors are never allowed to create, update, or delete namespaces.

func testContributorCannotCreateNamespace(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
		UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

// MaintainerCannotUseInfralibNamespace — platform-apis-zone-namespace-maintainer-deny
// Maintainers cannot create or update namespaces that carry the infralib zone label.
// DELETE is excluded from this policy.

func testMaintainerCannotCreateNamespaceWithInfralibZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("some-ns", "infralib", "restricted", "restricted"),
		UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testContributorCannotUpdateNamespace(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
		UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
		VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testContributorCannotDeleteNamespace(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
		UserInfoYAML:   kyverno.GenerateUserInfo("contributor"),
		VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testMaintainerCannotUpdateNamespaceWithInfralibZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateNamespace("some-ns", "infralib", "restricted", "restricted"),
		UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
		VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testMaintainerCanCreateNamespaceWithoutInfralibZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML:   kyverno.GenerateNamespace("some-ns", "my-zone", "restricted", "restricted"),
		UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

// ZoneDeletionCheckNamespaces — platform-apis-zone-deletion-check-namespaces
// Prevents deleting a Zone that still has namespaces assigned to it (outside spec.namespaces).

func testZoneDeletionBlockedWhenNamespacesAttached(t *testing.T) {
	t.Parallel()

	// "my-zone" is referenced by "attached-ns" in the offline namespace mock.
	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateZone("my-zone"),
		VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testZoneDeletionAllowedWhenNoNamespacesAttached(t *testing.T) {
	t.Parallel()

	// "other-zone" has no namespaces in the offline namespace mock.
	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML:   kyverno.GenerateZone("other-zone"),
		VariablesYAML:  kyverno.GenerateOperationValues("DELETE"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

// ZoneNamespaceOwnership — platform-apis-zone-namespace-ownership
// Zones cannot be named "default", cannot claim namespaces owned by other zones,
// and cannot claim namespaces that have no zone label at all.

func testCannotCreateDefaultZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateZone("default"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testCannotClaimNamespaceOwnedByAnotherZone(t *testing.T) {
	t.Parallel()

	// "attached-ns" is labeled "my-zone" in the offline mock; claiming it from "new-zone" must fail.
	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateZoneWithNamespaces("new-zone", []string{"attached-ns"}),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testCannotClaimNamespaceWithoutZoneLabel(t *testing.T) {
	t.Parallel()

	// "stolen-ns" has no zone label in the offline mock; it must be labeled manually first.
	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateZoneWithNamespaces("new-zone", []string{"stolen-ns"}),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testZoneCanManageItsOwnNamespace(t *testing.T) {
	t.Parallel()

	// "attached-ns" is labeled "my-zone" in the offline mock; the owner zone may manage it.
	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML:   kyverno.GenerateZoneWithNamespaces("my-zone", []string{"attached-ns"}),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

// MaintainerCannotModifyInfralibZone — platform-apis-zone-maintainer-infralib-zone-deny
// Maintainers cannot create or update the Zone named "infralib".

func testMaintainerCannotCreateInfralibZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateZone("infralib"),
		UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testMaintainerCannotUpdateInfralibZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "fail",
		ResourceYAML:   kyverno.GenerateZone("infralib"),
		UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
		VariablesYAML:  kyverno.GenerateOperationValues("UPDATE"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testMaintainerCanCreateNonInfralibZone(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML:   kyverno.GenerateZone("maintainer-zone"),
		UserInfoYAML:   kyverno.GenerateUserInfo("maintainer"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

// GenerateNamespaceFromArgoApp — generate-namespace-from-argocd-app
// Generates a labeled namespace when an ArgoCD Application is created with a non-infralib project.

func testArgoAppGeneratesNamespace(t *testing.T) {
	t.Parallel()

	scenario := kyverno.TestScenario{
		ExpectedAction:   "pass",
		ResourceYAML:     kyverno.GenerateArgoApp("my-app", "my-project", "my-namespace"),
		ExpectedInOutput: "my-namespace",
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}

func testArgoAppInfralibProjectSkipped(t *testing.T) {
	t.Parallel()

	// infralib project is excluded by matchCondition — policy must not deny it.
	scenario := kyverno.TestScenario{
		ExpectedAction: "pass",
		ResourceYAML:   kyverno.GenerateArgoApp("infra-app", "infralib", "infra-namespace"),
	}

	kyverno.RunPolicyCheck(t, chartDir, scenario)
}
