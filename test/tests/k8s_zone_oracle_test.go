package test

import (
	"testing"
	"time"

	"github.com/entigolabs/entigo-infralib-test/env"
	"github.com/entigolabs/entigo-infralib-test/k8s"
	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

// testPlatformApisOracle creates a Zone with one OKE node pool and a namespace pinned to it, and
// checks that a Pod in the namespace runs on a node of that pool.
func testPlatformApisOracle(t *testing.T, e *env.Environment) {
	c := k8s.Connect(t, e)
	cluster, _ := setupKubectlClients(c.Context, e.Prefix)

	t.Run("packages", func(t *testing.T) {
		checkPlatformApisHaveRequiredPackages(t, cluster, ZoneConfigurationName, TenancyFunctionName)
	})
	if t.Failed() {
		return
	}

	t.Run("zone", func(t *testing.T) {
		applyFile(t, cluster, "./templates/oracle_zone.yaml")
		waitSyncedAndReady(t, cluster, ZoneKind, OracleZoneName, 60, 10*time.Second)
		waitSyncedAndReadyByLabel(t, cluster, NodePoolKind, OracleZoneName, 60, 10*time.Second)
		waitZoneNodesReady(t, cluster, OracleZoneName)
	})
	if t.Failed() {
		return
	}

	t.Run("pod-on-pool", func(t *testing.T) {
		applyFile(t, cluster, "./templates/oracle_zone_namespace.yaml")
		waitNamespacePoliciesReady(t, cluster, OracleZoneName, OracleZoneNamespace)
		if t.Failed() {
			return
		}
		applyFile(t, cluster, "./templates/oracle_zone_pod.yaml")
		nsOpts := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, OracleZoneNamespace)
		zonePool := OracleZoneName + "-" + OracleZonePool

		testPodsRunning(t, nsOpts, OracleZonePodName)
		testPodNodeSelector(t, nsOpts, OracleZonePodName, "tenancy.entigo.com/zone-pool", zonePool)

		pod, err := terrak8s.GetPodE(t, nsOpts, OracleZonePodName)
		require.NoError(t, err)
		nodePool, err := terrak8s.RunKubectlAndGetOutputE(t, cluster, "get", "node", pod.Spec.NodeName,
			"-o", `jsonpath={.metadata.labels.tenancy\.entigo\.com/zone-pool}`)
		require.NoError(t, err)
		require.Equal(t, zonePool, nodePool, "pod %s/%s runs on node %s outside pool %s",
			OracleZoneNamespace, OracleZonePodName, pod.Spec.NodeName, zonePool)
	})
}
