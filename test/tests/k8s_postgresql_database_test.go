package test

import (
	"fmt"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	PostgresqlDatabaseName           = "database-test"
	PostgresqlDatabaseKind           = "postgresqldatabase.database.entigo.com"
	SqlDatabaseKind                  = "database.postgresql.sql.m.crossplane.io"
	SqlRoleKind                      = "role.postgresql.sql.m.crossplane.io"
	UsageKind                        = "usage.protection.crossplane.io"
	MinimalDatabaseName              = "database-minimal-test"
	DatabaseGrantExpectedName        = "test-admin-to-dbadmin-grant"
	DatabaseUsageExpectedName        = PostgresqlDatabaseName + "-grant-usage"
	MinimalDatabaseUsageExpectedName = MinimalDatabaseName + "-grant-usage"
)

func testDatabaseApplied(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "apply", "-f", "./templates/postgresql_test_database.yaml", "-n", PostgresqlNamespaceName)
	require.NoError(t, err)
}

func testDatabaseOwnerFieldVerified(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	dbName, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlDatabaseName), "-o", "jsonpath={.items[0].metadata.name}")
	owner, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, dbName, "-o", "jsonpath={.spec.forProvider.owner}")
	require.Equal(t, PostgresqlAdminUserSpecName, owner)
}

func testDatabaseFieldsVerified(t *testing.T, argocdNamespace string, namespaceOptions *terrak8s.KubectlOptions) {
	dbName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlDatabaseName), "-o", "jsonpath={.items[0].metadata.name}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to find SQL Database", argocdNamespace))
	require.NotEmpty(t, dbName, fmt.Sprintf("[%s] No SQL Database found for composite '%s'", argocdNamespace, PostgresqlDatabaseName))

	encoding, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, dbName, "-o", "jsonpath={.spec.forProvider.encoding}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get encoding", argocdNamespace))
	require.Equal(t, "UTF8", encoding, fmt.Sprintf("[%s] SQL Database '%s' encoding mismatch", argocdNamespace, dbName))

	lcCType, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, dbName, "-o", "jsonpath={.spec.forProvider.lcCType}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get lcCType", argocdNamespace))
	require.Equal(t, "et_EE.UTF-8", lcCType, fmt.Sprintf("[%s] SQL Database '%s' lcCType mismatch", argocdNamespace, dbName))

	lcCollate, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, dbName, "-o", "jsonpath={.spec.forProvider.lcCollate}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get lcCollate", argocdNamespace))
	require.Equal(t, "et_EE.UTF-8", lcCollate, fmt.Sprintf("[%s] SQL Database '%s' lcCollate mismatch", argocdNamespace, dbName))

	template, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, dbName, "-o", "jsonpath={.spec.forProvider.template}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get template", argocdNamespace))
	require.Equal(t, "template0", template, fmt.Sprintf("[%s] SQL Database '%s' template mismatch", argocdNamespace, dbName))
}

func testMinimalDatabaseApplied(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "apply", "-f", "./templates/postgresql_test_database_minimal.yaml", "-n", PostgresqlNamespaceName)
	require.NoError(t, err)
}

func testMinimalDatabaseDefaultsVerified(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	dbName, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", MinimalDatabaseName), "-o", "jsonpath={.items[0].metadata.name}")
	owner, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlDatabaseKind, dbName, "-o", "jsonpath={.spec.forProvider.owner}")
	require.Equal(t, PostgresqlRegularUserName, owner)
}

func testDatabaseUsageVerified(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for DB Usage", 30, 10*time.Second, func() (string, error) {
		return terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", UsageKind, DatabaseUsageExpectedName, "-o", "jsonpath={.metadata.name}")
	})
	require.NoError(t, err)
}

func testMinimalDatabaseUsageVerified(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for Minimal DB Usage", 30, 10*time.Second, func() (string, error) {
		return terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", UsageKind, MinimalDatabaseUsageExpectedName, "-o", "jsonpath={.metadata.name}")
	})
	require.NoError(t, err)
}

func testDatabaseUsagePreventsGrantDeletion(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "delete", SqlGrantKind, DatabaseGrantExpectedName, "--wait=false")
	time.Sleep(10 * time.Second)
	// Grant should still exist
	out, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlGrantKind, DatabaseGrantExpectedName, "--ignore-not-found")
	require.NoError(t, err)
	require.NotEmpty(t, out, "Grant should not have been deleted")
}
