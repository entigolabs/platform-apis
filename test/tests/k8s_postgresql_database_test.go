package test

import (
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/require"
)

const (
	PostgresqlDatabaseKind = "postgresqldatabases.database.entigo.com"
	SqlDatabaseKind        = "database.postgresql.sql.m.crossplane.io"
	SqlExtensionKind       = "extension.postgresql.sql.m.crossplane.io"
	SqlGrantKind           = "grant.postgresql.sql.m.crossplane.io"
	UsageKind              = "usage.protection.crossplane.io"

	DatabaseOneName     = "database-one-test"
	DatabaseOneSpecName = "database_one_test"
	DatabaseTwoName     = "database-two-test"
	MinimalDatabaseName = "database-minimal-test"

	DatabaseGrantExpectedName    = DatabaseOneName + "-grant-owner-to-dbadmin"
	DatabaseTwoGrantExpectedName = DatabaseTwoName + "-grant-owner-to-dbadmin"

	DatabaseOneOwnerProtectionName     = DatabaseOneName + "-owner-protection"
	DatabaseTwoOwnerProtectionName     = DatabaseTwoName + "-owner-protection"
	MinimalDatabaseOwnerProtectionName = MinimalDatabaseName + "-owner-protection"

	DatabaseOneInstanceProtectionName     = DatabaseOneName + "-instance-protection"
	DatabaseTwoInstanceProtectionName     = DatabaseTwoName + "-instance-protection"
	MinimalDatabaseInstanceProtectionName = MinimalDatabaseName + "-instance-protection"
)

func testDatabaseOne(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	// Create
	t.Run("create", func(t *testing.T) {
		waitSyncedAndReady(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName, 60, 10*time.Second)
	})
	if t.Failed() {
		return
	}

	// Read
	t.Run("read", func(t *testing.T) {
		testDatabaseOwner(t, pgNs, DatabaseOneName, PostgresqlAdminUserSpecName)
		testDatabaseLocale(t, pgNs, DatabaseOneName)
		testDatabaseExternalName(t, pgNs, DatabaseOneName, DatabaseOneSpecName)
		testGrantReady(t, pgNs, DatabaseGrantExpectedName, "dbadmin", PostgresqlAdminUserSpecName)

		testUsage(t, pgNs, DatabaseOneOwnerProtectionName, "PostgreSQLUser", PostgresqlAdminUserName, "Database", DatabaseOneName)
		testUsage(t, pgNs, DatabaseOneInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Database", DatabaseOneName)

		testDatabaseExtensions(t, pgNs, DatabaseOneName, DatabaseOneSpecName)
	})

	t.Run("protection-checks", func(t *testing.T) {
		// deletion protection is enabled by default and blocks webhook-level delete
		require.Equal(t, "true", getField(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName, ".spec.deletionProtection"))
		testDeletionRejected(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName)

		// Usage blocks grant deletion
		testUsageBlocksDeletion(t, pgNs, SqlGrantKind, DatabaseGrantExpectedName)
	})
}

func testDatabaseTwo(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	// Create
	t.Run("create", func(t *testing.T) {
		waitSyncedAndReady(t, pgNs, PostgresqlDatabaseKind, DatabaseTwoName, 60, 10*time.Second)
	})
	if t.Failed() {
		return
	}

	// Read
	t.Run("read", func(t *testing.T) {
		testDatabaseOwner(t, pgNs, DatabaseTwoName, PostgresqlAdminUserSpecName)
		testDatabaseLocale(t, pgNs, DatabaseTwoName)
		testGrantReady(t, pgNs, DatabaseTwoGrantExpectedName, "dbadmin", PostgresqlAdminUserSpecName)

		testUsage(t, pgNs, DatabaseTwoOwnerProtectionName, "PostgreSQLUser", PostgresqlAdminUserName, "Database", DatabaseTwoName)
		testUsage(t, pgNs, DatabaseTwoInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Database", DatabaseTwoName)
	})
}

func testMinimalDatabase(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	// Create
	t.Run("create", func(t *testing.T) {
		waitSyncedAndReady(t, pgNs, PostgresqlDatabaseKind, MinimalDatabaseName, 60, 10*time.Second)
	})
	if t.Failed() {
		return
	}

	// Read
	t.Run("read", func(t *testing.T) {
		testDatabaseOwner(t, pgNs, MinimalDatabaseName, PostgresqlRegularUserName)

		testUsage(t, pgNs, MinimalDatabaseOwnerProtectionName, "PostgreSQLUser", PostgresqlRegularUserName, "Database", MinimalDatabaseName)
		testUsage(t, pgNs, MinimalDatabaseInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Database", MinimalDatabaseName)
	})
}

// ── Field assertion helpers ───────────────────────────────────────────────────

func testDatabaseOwner(t *testing.T, pgNs *terrak8s.KubectlOptions, composite, expected string) {
	t.Helper()
	sqlDbName, err := getFirstByLabel(t, pgNs, SqlDatabaseKind, composite)
	require.NoError(t, err)
	require.Equal(t, expected, getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.owner"))
}

func testDatabaseLocale(t *testing.T, pgNs *terrak8s.KubectlOptions, composite string) {
	t.Helper()
	sqlDbName, err := getFirstByLabel(t, pgNs, SqlDatabaseKind, composite)
	require.NoError(t, err)

	require.Equal(t, "UTF8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.encoding"))
	require.Equal(t, "et_EE.UTF-8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.lcCType"))
	require.Equal(t, "et_EE.UTF-8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.lcCollate"))
	require.Equal(t, "template0", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.template"))
}

func testDatabaseExternalName(t *testing.T, pgNs *terrak8s.KubectlOptions, composite, expected string) {
	t.Helper()
	sqlDbName, err := getFirstByLabel(t, pgNs, SqlDatabaseKind, composite)
	require.NoError(t, err)
	require.Equal(t, expected, getField(t, pgNs, SqlDatabaseKind, sqlDbName, `.metadata.annotations.crossplane\.io/external-name`))
}

func testGrantReady(t *testing.T, pgNs *terrak8s.KubectlOptions, grantName, expectedRole, expectedMemberOf string) {
	t.Helper()
	waitSyncedAndReady(t, pgNs, SqlGrantKind, grantName, 60, 10*time.Second)
	require.Equal(t, expectedRole, getField(t, pgNs, SqlGrantKind, grantName, ".spec.forProvider.role"))
	require.Equal(t, expectedMemberOf, getField(t, pgNs, SqlGrantKind, grantName, ".spec.forProvider.memberOf"))
}

func testDatabaseExtensions(t *testing.T, pgNs *terrak8s.KubectlOptions, composite, dbSpecName string) {
	t.Helper()
	type ext struct {
		name, extName, schema string
	}

	extensions := []ext{
		{composite + "-postgis", "postgis", ""},
		{composite + "-postgis-topology", "postgis_topology", "topology"},
		{composite + "-fuzzystrmatch", "fuzzystrmatch", ""},
		{composite + "-postgis-tiger-geocoder", "postgis_tiger_geocoder", "tiger"},
		{composite + "-uuid-ossp", "uuid-ossp", ""},
		{composite + "-btree-gist", "btree_gist", ""},
	}

	for _, e := range extensions {
		require.Equal(t, e.name, getField(t, pgNs, SqlExtensionKind, e.name, ".metadata.name"), "extension %s not found", e.name)
		require.Equal(t, e.extName, getField(t, pgNs, SqlExtensionKind, e.name, ".spec.forProvider.extension"))
		require.Equal(t, dbSpecName, getField(t, pgNs, SqlExtensionKind, e.name, ".spec.forProvider.database"))

		if e.schema != "" {
			require.Equal(t, e.schema, getField(t, pgNs, SqlExtensionKind, e.name, ".spec.forProvider.schema"))
		}
	}
}
