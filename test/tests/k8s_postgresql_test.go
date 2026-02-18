package test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

const (
	PostgresqlConfigurationName = "platform-apis-postgresql"
	DatabaseFunctionName        = "platform-apis-database-fn"
	FunctionKind                = "function.pkg.crossplane.io"
	PostgresqlNamespaceName     = "test-postgresql"
)

func testPlatformApisPostgresql(t *testing.T, argocdNamespace string, clusterOptions *terrak8s.KubectlOptions, zonesReady <-chan struct{}, zonesReadySuccess *atomic.Bool) {

	defer func() {
		if t.Failed() {
			fmt.Printf("[%s] Cleanup: skipping due to failure\n", argocdNamespace)
			return
		}
		cleanupPostgresqlResources(t, argocdNamespace, clusterOptions)
	}()

	setupStart := time.Now()
	t.Run("setup", func(t *testing.T) {
		t.Parallel()
		t.Run("configuration", func(t *testing.T) {
			t.Parallel()
			waitForResourceHealthyAndInstalled(t, clusterOptions, ConfigurationKind, PostgresqlConfigurationName)
		})
		t.Run("function", func(t *testing.T) {
			t.Parallel()
			waitForResourceHealthyAndInstalled(t, clusterOptions, FunctionKind, DatabaseFunctionName)
		})
	})

	fmt.Printf("[%s] TIMING: PostgreSQL setup took %s\n", argocdNamespace, time.Since(setupStart))
	if t.Failed() {
		return
	}

	waitForZonesReady(t, argocdNamespace, zonesReady, zonesReadySuccess)

	namespaceOptions := terrak8s.NewKubectlOptions(clusterOptions.ContextName, clusterOptions.ConfigPath, PostgresqlNamespaceName)

	instanceStart := time.Now()
	testTestNamespaceCreated(t, clusterOptions)
	runPostgresqlInstanceTests(t, argocdNamespace, namespaceOptions)
	fmt.Printf("[%s] TIMING: PostgreSQL instance tests took %s\n", argocdNamespace, time.Since(instanceStart))

	userDbStart := time.Now()
	runPostgresqlUserAndDatabaseTests(t, argocdNamespace, namespaceOptions)
	fmt.Printf("[%s] TIMING: User and database tests took %s\n", argocdNamespace, time.Since(userDbStart))
}

func testTestNamespaceCreated(t *testing.T, clusterOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "create", "namespace", PostgresqlNamespaceName)
	if err != nil {
		_, err = terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "namespace", PostgresqlNamespaceName)
		require.NoError(t, err)
	}
}
