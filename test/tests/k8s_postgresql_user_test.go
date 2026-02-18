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
	PostgresqlAdminUserName       = "test-admin"
	PostgresqlAdminUserKind       = "postgresqluser.database.entigo.com"
	PostgresqlAdminUserSpecName   = "test_admin"
	PostgresqlRegularUserName     = "test-user"
	SqlGrantKind                  = "grant.postgresql.sql.m.crossplane.io"
	RegularUserExpectedGrantName  = "grant-" + PostgresqlRegularUserName + "-test-admin-" + PostgresqlInstanceName
	RegularUserExpectedUsageName  = "usage-" + RegularUserExpectedGrantName
	RegularUserExpectedSecretName = PostgresqlInstanceName + "-" + PostgresqlRegularUserName
)

func runPostgresqlUserAndDatabaseTests(t *testing.T, argocdNamespace string, namespaceOptions *terrak8s.KubectlOptions) {

	testAdminUserApplied(t, namespaceOptions)

	t.Run("admin-resources-ready", func(t *testing.T) {
		t.Run("user", func(t *testing.T) {
			t.Parallel()
			waitForResourceSyncedAndReady(t, namespaceOptions, PostgresqlAdminUserKind, PostgresqlAdminUserName)
		})
		t.Run("role", func(t *testing.T) {
			t.Parallel()
			roleName := waitForSqlRoleName(t, namespaceOptions, PostgresqlAdminUserName)
			waitForResourceSyncedAndReady(t, namespaceOptions, SqlRoleKind, roleName)
			testAdminRoleExternalNameVerified(t, argocdNamespace, namespaceOptions)
		})
	})

	if t.Failed() {
		return
	}

	if t.Failed() {
		return
	}

	t.Run("parallel-user-and-db", func(t *testing.T) {
		t.Parallel()
		t.Run("regular-user", func(t *testing.T) {
			t.Parallel()
			testRegularUserApplied(t, namespaceOptions)

			t.Run("resources-ready", func(t *testing.T) {
				t.Run("user", func(t *testing.T) {
					t.Parallel()
					waitForResourceSyncedAndReady(t, namespaceOptions, PostgresqlAdminUserKind, PostgresqlRegularUserName)
				})
				t.Run("grant", func(t *testing.T) {
					t.Parallel()
					waitForResourceSyncedAndReady(t, namespaceOptions, SqlGrantKind, RegularUserExpectedGrantName)
					verifyGrantRoleMember(t, namespaceOptions, RegularUserExpectedGrantName, PostgresqlRegularUserName, PostgresqlAdminUserSpecName)
				})
				t.Run("usage", func(t *testing.T) {
					t.Parallel()
					testRegularUserUsageVerified(t, namespaceOptions)
				})
				t.Run("secret", func(t *testing.T) {
					t.Parallel()
					testRegularUserConnectionSecretCreated(t, namespaceOptions)
				})
			})

			if t.Failed() {
				return
			}

			testUserUsagePreventsRoleDeletion(t, namespaceOptions)
			testRegularUserExternalNameFallback(t, namespaceOptions)
		})

		t.Run("database", func(t *testing.T) {
			t.Parallel()
			testDatabaseApplied(t, namespaceOptions)

			t.Run("resources-ready", func(t *testing.T) {
				t.Run("db", func(t *testing.T) {
					t.Parallel()
					waitForResourceSyncedAndReady(t, namespaceOptions, PostgresqlDatabaseKind, PostgresqlDatabaseName)
				})
				t.Run("grant", func(t *testing.T) {
					t.Parallel()
					waitForResourceSyncedAndReady(t, namespaceOptions, SqlGrantKind, DatabaseGrantExpectedName)
				})
				t.Run("usage", func(t *testing.T) {
					t.Parallel()
					testDatabaseUsageVerified(t, namespaceOptions)
				})
			})

			testDatabaseOwnerFieldVerified(t, namespaceOptions)
			testDatabaseFieldsVerified(t, argocdNamespace, namespaceOptions)
		})
	})
	if t.Failed() {
		return
	}

	testMinimalDatabaseApplied(t, namespaceOptions)

	t.Run("minimal-db-ready-checks", func(t *testing.T) {
		t.Run("db-synced-ready", func(t *testing.T) {
			t.Parallel()
			waitForResourceSyncedAndReady(t, namespaceOptions, PostgresqlDatabaseKind, MinimalDatabaseName)
		})
		t.Run("usage", func(t *testing.T) {
			t.Parallel()
			testMinimalDatabaseUsageVerified(t, namespaceOptions)
		})
	})
	if t.Failed() {
		return
	}

	testMinimalDatabaseDefaultsVerified(t, namespaceOptions)
	testDatabaseUsagePreventsGrantDeletion(t, namespaceOptions)
}

func testAdminUserApplied(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "apply", "-f", "./templates/postgresql_test_admin_user.yaml", "-n", PostgresqlNamespaceName)
	require.NoError(t, err)
}

func waitForSqlRoleName(t *testing.T, options *terrak8s.KubectlOptions, compositeName string) string {
	var roleName string
	_, err := retry.DoWithRetryE(t, "Find SQL Role", 60, 5*time.Second, func() (string, error) {
		name, err := terrak8s.RunKubectlAndGetOutputE(t, options, "get", SqlRoleKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", compositeName), "-o", "jsonpath={.items[0].metadata.name}")
		if err != nil || name == "" {
			return "", fmt.Errorf("role not found")
		}
		roleName = name
		return name, nil
	})
	require.NoError(t, err)
	return roleName
}

func verifyGrantRoleMember(t *testing.T, options *terrak8s.KubectlOptions, grantName, expectedRole, expectedMemberOf string) {
	role, _ := terrak8s.RunKubectlAndGetOutputE(t, options, "get", SqlGrantKind, grantName, "-o", "jsonpath={.spec.forProvider.role}")
	memberOf, _ := terrak8s.RunKubectlAndGetOutputE(t, options, "get", SqlGrantKind, grantName, "-o", "jsonpath={.spec.forProvider.memberOf}")
	require.Equal(t, expectedRole, role)
	require.Equal(t, expectedMemberOf, memberOf)
}

func testAdminRoleExternalNameVerified(t *testing.T, argocdNamespace string, namespaceOptions *terrak8s.KubectlOptions) {
	roleName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlRoleKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlAdminUserName), "-o", "jsonpath={.items[0].metadata.name}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to find SQL Role", argocdNamespace))
	require.NotEmpty(t, roleName, fmt.Sprintf("[%s] No SQL Role found for composite '%s'", argocdNamespace, PostgresqlAdminUserName))

	externalName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlRoleKind, roleName, "-o", `jsonpath={.metadata.annotations.crossplane\.io/external-name}`)
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get crossplane.io/external-name annotation", argocdNamespace))
	require.Equal(t, PostgresqlAdminUserSpecName, externalName, fmt.Sprintf("[%s] SQL Role '%s' crossplane.io/external-name expected '%s', got '%s'", argocdNamespace, roleName, PostgresqlAdminUserSpecName, externalName))
}

func testRegularUserApplied(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "apply", "-f", "./templates/postgresql_test_user.yaml", "-n", PostgresqlNamespaceName)
	require.NoError(t, err)
}

func testRegularUserExternalNameFallback(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	roleName := waitForSqlRoleName(t, namespaceOptions, PostgresqlRegularUserName)
	extName, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlRoleKind, roleName, "-o", `jsonpath={.metadata.annotations.crossplane\.io/external-name}`)
	require.Equal(t, PostgresqlRegularUserName, extName)
}

func testRegularUserConnectionSecretCreated(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for Secret", 60, 10*time.Second, func() (string, error) {
		return terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", "secret", RegularUserExpectedSecretName, "-n", PostgresqlNamespaceName, "-o", "jsonpath={.metadata.name}")
	})
	require.NoError(t, err)
}

func testRegularUserUsageVerified(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for Usage", 30, 10*time.Second, func() (string, error) {
		return terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", UsageKind, RegularUserExpectedUsageName, "-o", "jsonpath={.metadata.name}")
	})
	require.NoError(t, err)
}

func testUserUsagePreventsRoleDeletion(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	roleName := waitForSqlRoleName(t, namespaceOptions, PostgresqlRegularUserName)
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "delete", SqlRoleKind, roleName, "--wait=false")
	time.Sleep(10 * time.Second)
	// Role should still exist
	out, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlRoleKind, roleName, "--ignore-not-found")
	require.NoError(t, err)
	require.NotEmpty(t, out, "Role should not have been deleted")
}
