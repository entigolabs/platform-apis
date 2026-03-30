package test

import (
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

const (
	PostgresqlAdminUserName     = "test-owner"
	PostgresqlAdminUserKind     = "postgresqlusers.database.entigo.com"
	PostgresqlAdminUserSpecName = "test_owner"
	PostgresqlRegularUserName   = "test-user"
	SqlRoleKind                 = "role.postgresql.sql.m.crossplane.io"

	RegularUserExpectedGrantName  = "grant-" + PostgresqlRegularUserName + "-test-owner-" + PostgresqlInstanceName
	RegularUserExpectedUsageName  = "usage-" + RegularUserExpectedGrantName
	RegularUserExpectedSecretName = PostgresqlInstanceName + "-" + PostgresqlRegularUserName

	AdminUserInstanceProtectionName   = PostgresqlAdminUserName + "-instance-protection"
	RegularUserInstanceProtectionName = PostgresqlRegularUserName + "-instance-protection"
)

func testPostgresqlAdminUser(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	// Create
	t.Run("admin-create", func(t *testing.T) {
		waitSyncedAndReady(t, pgNs, PostgresqlAdminUserKind, PostgresqlAdminUserName, 60, 10*time.Second)
		waitSyncedAndReadyByLabel(t, pgNs, SqlRoleKind, PostgresqlAdminUserName, 60, 10*time.Second)
	})
	if t.Failed() {
		return
	}

	// Read
	t.Run("admin-read", func(t *testing.T) {
		testRoleExternalName(t, pgNs, PostgresqlAdminUserName, PostgresqlAdminUserSpecName)
		testUsage(t, pgNs, AdminUserInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Role", PostgresqlAdminUserName)
	})
}

func testPostgresqlRegularUser(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	// Create
	t.Run("regular-user-create", func(t *testing.T) {
		waitSyncedAndReady(t, pgNs, PostgresqlAdminUserKind, PostgresqlRegularUserName, 60, 10*time.Second)
	})
	if t.Failed() {
		return
	}

	// Read: grant, usage, external-name fallback, privileges, connection secret
	t.Run("regular-user-read", func(t *testing.T) {
		testGrantReady(t, pgNs, RegularUserExpectedGrantName, PostgresqlRegularUserName, PostgresqlAdminUserSpecName)
		testUsage(t, pgNs, RegularUserExpectedUsageName, "Role", PostgresqlRegularUserName, "Grant", RegularUserExpectedGrantName)
		testUsage(t, pgNs, RegularUserInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Role", PostgresqlRegularUserName)

		// no spec.name → falls back to metadata.name
		testRoleExternalName(t, pgNs, PostgresqlRegularUserName, PostgresqlRegularUserName)
		testRolePrivileges(t, pgNs, PostgresqlRegularUserName)
		testConnectionSecretExists(t, pgNs, RegularUserExpectedSecretName)
	})

	// Update: verify Usage blocks role deletion (delete is attempted, Usage prevents it)
	t.Run("regular-user-update", func(t *testing.T) {
		roleName, err := getFirstByLabel(t, pgNs, SqlRoleKind, PostgresqlRegularUserName)
		require.NoError(t, err)
		require.NotEmpty(t, roleName)
		testUsageBlocksDeletion(t, pgNs, SqlRoleKind, roleName)
	})
}

func testRoleExternalName(t *testing.T, pgNs *terrak8s.KubectlOptions, composite, expectedExternalName string) {
	t.Helper()
	roleName, err := getFirstByLabel(t, pgNs, SqlRoleKind, composite)
	require.NoError(t, err)
	require.NotEmpty(t, roleName)

	actualName := getField(t, pgNs, SqlRoleKind, roleName, `.metadata.annotations.crossplane\.io/external-name`)
	require.Equal(t, expectedExternalName, actualName)
}

func testRolePrivileges(t *testing.T, pgNs *terrak8s.KubectlOptions, composite string) {
	t.Helper()
	roleName, err := getFirstByLabel(t, pgNs, SqlRoleKind, composite)
	require.NoError(t, err)
	require.NotEmpty(t, roleName)

	require.Equal(t, "false", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.createDb"))
	require.Equal(t, "true", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.login"))
	require.Equal(t, "false", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.createRole"))
	require.Equal(t, "true", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.inherit"))
}

func testConnectionSecretExists(t *testing.T, pgNs *terrak8s.KubectlOptions, secretName string) {
	t.Helper()
	waitResourceExists(t, pgNs, "secret", secretName, 60, 10*time.Second)
}
