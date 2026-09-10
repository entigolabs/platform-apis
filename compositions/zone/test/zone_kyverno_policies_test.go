package test

import (
	"testing"

	"github.com/entigolabs/static-common/kyverno"
)

const chartDir = "../../../helm"

func TestKyvernoPolicies(t *testing.T) {
	t.Run("NamespacePodSecurity", testNamespacePodSecurity)
	t.Run("ContributorDeny", testContributorDeny)
	t.Run("MaintainerNamespaceDeny", testMaintainerNamespaceDeny)
	t.Run("ZoneDeletionCheck", testZoneDeletionCheck)
	t.Run("ZoneNamespaceOwnership", testZoneNamespaceOwnership)
	t.Run("MaintainerInfralibZoneDeny", testMaintainerInfralibZoneDeny)
	t.Run("GenerateNamespaceFromArgoApp", testGenerateNamespaceFromArgoApp)
	t.Run("AppsNamespaceRestriction", testAppsNamespaceRestriction)
	t.Run("NamespaceArgoCDMetadata", testNamespaceArgoCDMetadata)
	t.Run("ApplicationNamespaceMetadata", testApplicationNamespaceMetadata)
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
				ResourceYAML:     kyverno.GenerateArgoApp("my-app", "", "my-project", "my-namespace"),
				ExpectedInOutput: "my-namespace",
			},
		},
		{
			name: "pass: ArgoApp with infralib project does not generate namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateArgoApp("infra-app", "", "infralib", "infra-namespace"),
			},
		},
	}
	runCases(t, cases)
}

// testAppsNamespaceRestriction covers platform-apis-zone-apps-namespace-restriction (ValidatingPolicy).
// In a namespace labeled tenancy.entigo.com/only-argocd-apps=true, only ArgoCD Application/ApplicationSet
// and rbac Role/RoleBinding resources may be created or updated; everything else is denied.
func testAppsNamespaceRestriction(t *testing.T) {
	t.Parallel()
	// The label belongs to a Zone's own apps namespace, so the simulated namespace is named like one.
	const restrictedNs = "my-zone-apps"
	restrictedLabels := kyverno.GenerateNamespaceLabelsValues(restrictedNs, map[string]string{
		"tenancy.entigo.com/only-argocd-apps": "true",
	})
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "pass: ConfigMap allowed in restricted namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateConfigMap("my-cm", restrictedNs),
				VariablesYAML:  restrictedLabels,
			},
		},
		{
			name: "fail: Service denied in restricted namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML: `
apiVersion: v1
kind: Service
metadata:
  name: my-svc
  namespace: my-zone-apps
spec:
  ports:
  - port: 80`,
				VariablesYAML: restrictedLabels,
			},
		},
		{
			name: "pass: Role allowed in restricted namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: `
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: my-role
  namespace: my-zone-apps
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get"]`,
				VariablesYAML: restrictedLabels,
			},
		},
		{
			name: "pass: ServiceAccount allowed in restricted namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: `
apiVersion: v1
kind: ServiceAccount
metadata:
  name: default
  namespace: my-zone-apps`,
				VariablesYAML: restrictedLabels,
			},
		},
		{
			name: "pass: RoleBinding allowed in restricted namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: `
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-rb
  namespace: my-zone-apps
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: my-role
subjects:
- kind: User
  name: someone
  apiGroup: rbac.authorization.k8s.io`,
				VariablesYAML: restrictedLabels,
			},
		},
		{
			name: "pass: ArgoCD Application allowed in restricted namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateArgoApp("my-app", restrictedNs, "my-project", "target-ns"),
				VariablesYAML:  restrictedLabels,
			},
		},
		{
			name: "pass: ConfigMap in non-restricted namespace is unaffected",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   kyverno.GenerateConfigMap("my-cm", "normal-ns"),
			},
		},
	}
	runCases(t, cases)
}

// testNamespaceArgoCDMetadata covers platform-apis-namespace-argocd-metadata (MutatingPolicy).
// The ArgoCD Application is the trigger and the Namespace it deploys to is the target, so every
// scenario states the Namespace it expects after the patch and the CLI compares the two in full.
func testNamespaceArgoCDMetadata(t *testing.T) {
	t.Parallel()
	const zone = "my-zone"
	const policy = "platform-apis-namespace-argocd-metadata"
	userLabels := map[string]string{"team": "platform", "istio-injection": "enabled"}
	userAnnotations := map[string]string{"owner": "ops"}
	namespace := kyverno.GenerateNamespace("my-namespace", zone, "baseline", "baseline")
	// patchedNamespace returns "my-namespace" with the given extra labels and annotations, which is
	// what the policy is expected to leave behind. No extras means the Namespace is untouched.
	patchedNamespace := func(labels, annotations map[string]string) string {
		return kyverno.GenerateNamespaceWithMetadata(
			"my-namespace", zone, "baseline", "baseline", labels, annotations)
	}
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "pass: labels and annotations are copied to the namespace",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", zone, "my-namespace", userLabels, userAnnotations),
				TargetResourceYAML:        namespace,
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: patchedNamespace(userLabels, userAnnotations),
			},
		},
		{
			name: "pass: the zone label of the application is ignored",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", zone, "my-namespace",
					map[string]string{"tenancy.entigo.com/zone": "other-zone"}, nil),
				TargetResourceYAML:        namespace,
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: patchedNamespace(nil, nil),
			},
		},
		{
			name: "pass: the pool label of the application is copied",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", zone, "my-namespace",
					map[string]string{"tenancy.entigo.com/pool": "myspot"}, nil),
				TargetResourceYAML: namespace,
				MutatingPolicyName: policy,
				ExpectedPatchedTargetYAML: patchedNamespace(
					map[string]string{"tenancy.entigo.com/pool": "myspot"}, nil),
			},
		},
		{
			name: "pass: the pool key is exempt as a label only, not as an annotation",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", zone, "my-namespace", nil,
					map[string]string{"tenancy.entigo.com/pool": "myspot"}),
				TargetResourceYAML:        namespace,
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: patchedNamespace(nil, nil),
			},
		},
		{
			name: "pass: a stricter pod security label is copied",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", zone, "my-namespace",
					map[string]string{"pod-security.kubernetes.io/enforce": "restricted"}, nil),
				TargetResourceYAML: namespace,
				MutatingPolicyName: policy,
				ExpectedPatchedTargetYAML: patchedNamespace(
					map[string]string{"pod-security.kubernetes.io/enforce": "restricted"}, nil),
			},
		},
		{
			name: "pass: an application of another project is ignored",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", "other-zone", "my-namespace", userLabels, userAnnotations),
				TargetResourceYAML:        namespace,
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: patchedNamespace(nil, nil),
			},
		},
		{
			name: "pass: the apps namespace of the zone is left alone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", zone, zone+"-apps", userLabels, userAnnotations),
				TargetResourceYAML:        kyverno.GenerateNamespace(zone+"-apps", zone, "baseline", "baseline"),
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: kyverno.GenerateNamespace(zone+"-apps", zone, "baseline", "baseline"),
			},
		},
		{
			name: "pass: an application without namespace metadata patches nothing",
			scenario: kyverno.TestScenario{
				ExpectedAction:            "pass",
				ResourceYAML:              kyverno.GenerateArgoApp("my-app", "", zone, "my-namespace"),
				TargetResourceYAML:        namespace,
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: patchedNamespace(nil, nil),
			},
		},
		{
			name: "pass: namespaces of the infralib zone are left alone",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", "infralib", "infra-namespace", userLabels, userAnnotations),
				TargetResourceYAML:        kyverno.GenerateNamespace("infra-namespace", "infralib", "baseline", "baseline"),
				MutatingPolicyName:        policy,
				ExpectedPatchedTargetYAML: kyverno.GenerateNamespace("infra-namespace", "infralib", "baseline", "baseline"),
			},
		},
	}
	runCases(t, cases)
}

// testApplicationNamespaceMetadata covers platform-apis-application-namespace-metadata
// (ValidatingPolicy). A Pod Security level looser than the zone floor is rejected on the
// Application, where the person who wrote it sees the error.
func testApplicationNamespaceMetadata(t *testing.T) {
	t.Parallel()
	const zone = "my-zone"
	app := func(labels map[string]string) string {
		return kyverno.GenerateArgoAppWithNamespaceMetadata("my-app", zone, "my-namespace", labels, nil)
	}
	cases := []struct {
		name     string
		scenario kyverno.TestScenario
	}{
		{
			name: "fail: a privileged enforce label is denied",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   app(map[string]string{"pod-security.kubernetes.io/enforce": "privileged"}),
			},
		},
		{
			name: "fail: a privileged warn label is denied",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				ResourceYAML:   app(map[string]string{"pod-security.kubernetes.io/warn": "privileged"}),
			},
		},
		{
			name: "pass: a restricted enforce label is allowed",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   app(map[string]string{"pod-security.kubernetes.io/enforce": "restricted"}),
			},
		},
		{
			name: "pass: labels without a pod security level are allowed",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML:   app(map[string]string{"team": "platform"}),
			},
		},
		{
			name: "fail: a baseline enforce label is denied when the setting is restricted",
			scenario: kyverno.TestScenario{
				ExpectedAction: "fail",
				HelmValues: map[string]string{
					"zone.install":                       "true",
					"zone.environmentConfig.podSecurity": "restricted",
				},
				ResourceYAML: app(map[string]string{"pod-security.kubernetes.io/enforce": "baseline"}),
			},
		},
		{
			name: "pass: an application of the infralib project is not checked",
			scenario: kyverno.TestScenario{
				ExpectedAction: "pass",
				ResourceYAML: kyverno.GenerateArgoAppWithNamespaceMetadata(
					"my-app", "infralib", "infra-namespace",
					map[string]string{"pod-security.kubernetes.io/enforce": "privileged"}, nil),
			},
		},
	}
	runCases(t, cases)
}

// runCases iterates scenarios and runs each as a parallel subtest.
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
