package test

import (
	"context"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

// ── Orchestrator ──────────────────────────────────────────────────────────────

func testValkey(t *testing.T, ctx context.Context, cluster, argocd *terrak8s.KubectlOptions) {
	vkNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, ValkeyNamespaceName)
	defer cleanupValkey(t, cluster, argocd)

	if ctx.Err() != nil {
		return
	}
	applyFile(t, cluster, "./templates/valkey_test_application.yaml")
	syncWithRetry(t, argocd, ValkeyApplicationName)
	if ctx.Err() != nil {
		return
	}

	t.Run("instances", func(t *testing.T) {
		t.Run("MinimalValkeyInstance", func(t *testing.T) { t.Parallel(); testMinimalValkeyInstance(t, vkNs) })
		t.Run("CustomValkeyInstance", func(t *testing.T) { t.Parallel(); testCustomValkeyInstance(t, vkNs) })
	})
}

// ── Minimal ValkeyInstance ────────────────────────────────────────────────────

func testMinimalValkeyInstance(t *testing.T, vkNs *terrak8s.KubectlOptions) {
	t.Helper()

	// Create
	waitSyncedAndReady(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName, 120, 10*time.Second)
	if t.Failed() {
		return
	}

	// Sub-resources must be Synced+Ready
	t.Run("SubResources", func(t *testing.T) {
		t.Run("SecurityGroup", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, vkNs, SecurityGroupKind, ValkeyMinimalName, 60, 10*time.Second)
		})
		t.Run("ReplicationGroup", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, vkNs, ValkeyReplicationGroupKind, ValkeyMinimalName, 120, 10*time.Second)
		})
		t.Run("SecurityGroupRule", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, vkNs, SecurityGroupRuleKind, ValkeyMinimalName, 60, 10*time.Second)
		})
		t.Run("Secret", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, vkNs, ValkeySecretKind, ValkeyMinimalName, 60, 10*time.Second)
		})
	})
	if t.Failed() {
		return
	}

	// Read: status fields must be populated
	require.NotEmpty(t, getField(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName, ".status.endpoint.address"),
		"status.endpoint.address should be populated")
	require.NotEmpty(t, getField(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName, ".status.endpoint.port"),
		"status.endpoint.port should be populated")

	// Deletion protection: default is true, webhook must reject deletion
	require.Equal(t, "true", getField(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName, ".spec.deletionProtection"))
	testDeletionRejected(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName)

	// Update: patch snapshotRetentionLimit and verify it propagates to the ReplicationGroup
	rgName, err := getFirstByLabel(t, vkNs, ValkeyReplicationGroupKind, ValkeyMinimalName)
	require.NoError(t, err)
	require.NotEmpty(t, rgName)
	patchResource(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName, `{"spec":{"snapshotRetentionLimit":5}}`)
	waitFieldEquals(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.snapshotRetentionLimit", "5", 30, 10*time.Second)
}

// ── Custom ValkeyInstance ─────────────────────────────────────────────────────

func testCustomValkeyInstance(t *testing.T, vkNs *terrak8s.KubectlOptions) {
	t.Helper()

	// Create
	waitSyncedAndReady(t, vkNs, ValkeyInstanceKind, ValkeyCustomName, 120, 10*time.Second)
	if t.Failed() {
		return
	}

	rgName, err := getFirstByLabel(t, vkNs, ValkeyReplicationGroupKind, ValkeyCustomName)
	require.NoError(t, err)
	require.NotEmpty(t, rgName)

	// Read: verify custom spec fields propagated to the provider resource
	require.Equal(t, "cache.t4g.medium", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.nodeType"))
	require.Equal(t, "8.2", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.engineVersion"))
	require.Equal(t, "2", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.numCacheClusters"))
	require.Equal(t, "3", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.snapshotRetentionLimit"))
}

// ── Cleanup ───────────────────────────────────────────────────────────────────

func cleanupValkey(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	if t.Failed() {
		return
	}
	vkNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, ValkeyNamespaceName)

	defer func() {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocd, "delete", "application", ValkeyApplicationName, "--ignore-not-found")
	}()

	patchDeletionProtectionIfEnabled(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName)
	cleanupDeleteParallel(t, vkNs, ValkeyInstanceKind, ValkeyMinimalName, ValkeyCustomName)

	_, _ = terrak8s.RunKubectlAndGetOutputE(t, cluster, "delete", "namespace", ValkeyNamespaceName, "--ignore-not-found", "--wait=true")
}
