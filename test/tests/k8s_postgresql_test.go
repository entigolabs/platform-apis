package test

import (
	"testing"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
)

const (
	PostgresqlConfigurationName = "platform-apis-postgresql"
	DatabaseFunctionName        = "platform-apis-database-fn"
	PostgresqlNamespaceName     = "test-postgresql"
	PostgresqlApplicationName   = "test-postgresql"
)

func testPostgresql(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	pgNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, PostgresqlNamespaceName)

	defer cleanupPostgresql(t, cluster, argocd)

	applyFile(t, cluster, "./templates/postgresql_test_application.yaml")
	syncWithRetry(t, argocd, PostgresqlApplicationName)

	testPostgresqlInstance(t, pgNs)
	if t.Failed() {
		return
	}

	// Admin user must be ready before databases can be verified (it is their owner)
	testPostgresqlAdminUser(t, pgNs)
	if t.Failed() {
		return
	}

	// Regular user and databases are independent of each other — run in parallel
	t.Run("users-and-databases", func(t *testing.T) {
		t.Run("regular-user", func(t *testing.T) {
			t.Parallel()
			testPostgresqlRegularUser(t, pgNs)
		})
		t.Run("database-one", func(t *testing.T) {
			t.Parallel()
			testDatabaseOne(t, pgNs)
		})
		t.Run("database-two", func(t *testing.T) {
			t.Parallel()
			testDatabaseTwo(t, pgNs)
		})
	})
	if t.Failed() {
		return
	}

	// Minimal database uses regular user as owner — must come after regular user is ready
	testMinimalDatabase(t, pgNs)
}
