package test

import (
	"fmt"
	"testing"

	"github.com/entigolabs/kyverno-common"
)

const chartDir = "../../../helm"

var defaultHelmValues = map[string]string{
	"zone.install":                       "true",
	"zone.environmentConfig.podSecurity": "restricted",
}

func TestKyvernoPolicies(t *testing.T) {
	t.Run("NamespacePodSecurity", testNamespacePodSecurity)
	t.Run("ContributorCannotModifyNamespace", testContributorCannotModifyNamespace)
	t.Run("MaintainerCannotUseInfralibNamespace", testMaintainerCannotUseInfralibNamespace)
	t.Run("MaintainerCannotModifyInfralibZone", testMaintainerCannotModifyInfralibZone)
}

// NamespacePodSecurity — platform-apis-zone-namespace-pod-security
// Validates that namespaces have correct pod-security labels for the configured level.

func testNamespacePodSecurity(t *testing.T) {
	t.Run("pass: restricted enforce+warn with valid zone", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "pass",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("good-ns", "my-zone", "restricted", "restricted"),
		})
	})

	t.Run("fail: privileged enforce is denied when setting is restricted", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("bad-ns", "my-zone", "privileged", "restricted"),
		})
	})

	t.Run("fail: privileged warn is denied when setting is restricted", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("bad-ns", "my-zone", "restricted", "privileged"),
		})
	})

	t.Run("fail: baseline enforce is denied when setting is restricted", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("bad-ns", "my-zone", "baseline", "restricted"),
		})
	})

	t.Run("pass: baseline enforce+warn are allowed when setting is baseline", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "pass",
			HelmValues: map[string]string{
				"zone.install":                       "true",
				"zone.environmentConfig.podSecurity": "baseline",
			},
			ResourceYAML: namespace("good-ns", "my-zone", "baseline", "baseline"),
		})
	})

	t.Run("pass: namespace without zone label gets auto-assigned by mutation policy", func(t *testing.T) {
		t.Parallel()
		// The MutatingPolicy platform-apis-namespace-add-missing-zone-label automatically assigns
		// a zone label, so the ValidatingPolicy sees a namespace with a valid zone and passes.
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "pass",
			HelmValues:     defaultHelmValues,
			ResourceYAML: `
apiVersion: v1
kind: Namespace
metadata:
  name: no-zone-ns
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/warn: restricted
`,
		})
	})

	t.Run("fail: zone label referencing non-existent zone is denied", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("bad-ns", "non-existent-zone", "restricted", "restricted"),
		})
	})

	t.Run("pass: system namespace kube-system is excluded", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "pass",
			HelmValues:     defaultHelmValues,
			ResourceYAML: `
apiVersion: v1
kind: Namespace
metadata:
  name: kube-system
`,
		})
	})
}

// ContributorCannotModifyNamespace — platform-apis-zone-namespace-contributor-deny
// Contributors are never allowed to create, update, or delete namespaces.

func testContributorCannotModifyNamespace(t *testing.T) {
	t.Run("fail: contributor cannot create a namespace", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("some-ns", "my-zone", "restricted", "restricted"),
			UserInfoYAML:   userInfo("contributor"),
		})
	})
}

// MaintainerCannotUseInfralibNamespace — platform-apis-zone-namespace-maintainer-deny
// Maintainers cannot create or update namespaces that carry the infralib zone label.

func testMaintainerCannotUseInfralibNamespace(t *testing.T) {
	t.Run("fail: maintainer cannot create namespace with infralib zone", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("some-ns", "infralib", "restricted", "restricted"),
			UserInfoYAML:   userInfo("maintainer"),
		})
	})

	t.Run("pass: maintainer can create namespace without infralib zone", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "pass",
			HelmValues:     defaultHelmValues,
			ResourceYAML:   namespace("some-ns", "my-zone", "restricted", "restricted"),
			UserInfoYAML:   userInfo("maintainer"),
		})
	})
}

// MaintainerCannotModifyInfralibZone — platform-apis-zone-maintainer-infralib-zone-deny
// Maintainers cannot create or update the Zone named "infralib".

func testMaintainerCannotModifyInfralibZone(t *testing.T) {
	t.Run("fail: maintainer cannot create the infralib zone", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "fail",
			HelmValues:     defaultHelmValues,
			ResourceYAML: `
apiVersion: tenancy.entigo.com/v1alpha1
kind: Zone
metadata:
  name: infralib
`,
			UserInfoYAML: userInfo("maintainer"),
		})
	})

	t.Run("pass: maintainer can create a non-infralib zone", func(t *testing.T) {
		t.Parallel()
		kyverno.RunPolicyCheck(t, chartDir, kyverno.TestScenario{
			ExpectedAction: "pass",
			HelmValues:     defaultHelmValues,
			ResourceYAML: `
apiVersion: tenancy.entigo.com/v1alpha1
kind: Zone
metadata:
  name: my-zone
`,
			UserInfoYAML: userInfo("maintainer"),
		})
	})
}

// --- helpers ---

func namespace(name, zone, enforce, warn string) string {
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

func userInfo(groups ...string) string {
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
