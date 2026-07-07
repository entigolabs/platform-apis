package test

import (
	"context"
	"strings"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

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
		t.Run("ValkeyLifecycle", func(t *testing.T) { testValkeyLifecycle(t, vkNs) })
	})
}

func testValkeyLifecycle(t *testing.T, vkNs *terrak8s.KubectlOptions) {
	t.Helper()

	waitSyncedAndReady(t, vkNs, ValkeyInstanceKind, ValkeyLifecycleName, 120, 10*time.Second)
	if t.Failed() {
		return
	}

	rgName, err := getFirstByLabel(t, vkNs, ValkeyReplicationGroupKind, ValkeyLifecycleName)
	require.NoError(t, err)
	require.NotEmpty(t, rgName)

	require.Equal(t, "cache.t4g.medium", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.nodeType"))
	require.Equal(t, "2", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.numCacheClusters"))
	require.Equal(t, "3", getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.snapshotRetentionLimit"))

	_, err = getFirstByLabel(t, vkNs, ValkeyParameterGroupKind, ValkeyLifecycleName)
	require.Error(t, err, "no ParameterGroup should exist while engineVersion is unset and parameterGroupParameters is unset")

	actualVersion := waitFieldNonEmpty(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.engineVersion", 60, 10*time.Second)
	actualFamily := "valkey" + strings.SplitN(actualVersion, ".", 2)[0]

	patchResource(t, vkNs, ValkeyInstanceKind, ValkeyLifecycleName, `{"spec":{"parameterGroupParameters":{"notify-keyspace-events":"Ex"}}}`)

	pgName := waitSyncedAndReadyByLabel(t, vkNs, ValkeyParameterGroupKind, ValkeyLifecycleName, 60, 10*time.Second)
	require.NotEmpty(t, pgName)
	require.Equal(t, actualFamily, getField(t, vkNs, ValkeyParameterGroupKind, pgName, ".spec.forProvider.family"))
	require.Equal(t, "notify-keyspace-events", getField(t, vkNs, ValkeyParameterGroupKind, pgName, ".spec.forProvider.parameter[0].name"))
	require.Equal(t, "Ex", getField(t, vkNs, ValkeyParameterGroupKind, pgName, ".spec.forProvider.parameter[0].value"))
	waitFieldEquals(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.parameterGroupName", pgName, 60, 10*time.Second)

	patchResource(t, vkNs, ValkeyInstanceKind, ValkeyLifecycleName, `{"spec":{"parameterGroupParameters":{"notify-keyspace-events":"KEA"}}}`)

	waitFieldEquals(t, vkNs, ValkeyParameterGroupKind, pgName, ".spec.forProvider.parameter[0].value", "KEA", 60, 10*time.Second)
	require.Equal(t, pgName, getField(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.parameterGroupName"),
		"ParameterGroup should update in place, not be recreated under a different name")

	newVersion, newFamily := "9.1", "valkey9"
	if actualFamily == "valkey9" {
		newVersion, newFamily = "8.2", "valkey8"
	}
	patchResource(t, vkNs, ValkeyInstanceKind, ValkeyLifecycleName, `{"spec":{"engineVersion":"`+newVersion+`"}}`)

	recreatedPgName := waitSyncedAndReadyByLabelWhere(t, vkNs, ValkeyParameterGroupKind, ValkeyLifecycleName, ".spec.forProvider.family", newFamily, 60, 10*time.Second)
	require.NotEqual(t, pgName, recreatedPgName)
	waitFieldEquals(t, vkNs, ValkeyReplicationGroupKind, rgName, ".status.atProvider.parameterGroupName", recreatedPgName, 120, 10*time.Second)
	waitFieldEquals(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.parameterGroupName", recreatedPgName, 60, 10*time.Second)
	waitFieldEquals(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.engineVersion", newVersion, 60, 10*time.Second)

	cleanupWaitGone(t, vkNs, ValkeyParameterGroupKind, pgName, 30)

	patchResource(t, vkNs, ValkeyInstanceKind, ValkeyLifecycleName, `{"spec":{"parameterGroupParameters":null}}`)

	cleanupWaitGone(t, vkNs, ValkeyParameterGroupKind, recreatedPgName, 30)
	waitFieldEquals(t, vkNs, ValkeyReplicationGroupKind, rgName, ".spec.forProvider.parameterGroupName", "default."+newFamily, 60, 10*time.Second)
}

func cleanupValkey(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	if t.Failed() {
		return
	}
	vkNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, ValkeyNamespaceName)

	cleanupDeleteParallel(t, vkNs, ValkeyInstanceKind, ValkeyLifecycleName)

	_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocd, "delete", "application", ValkeyApplicationName, "--ignore-not-found")
}
