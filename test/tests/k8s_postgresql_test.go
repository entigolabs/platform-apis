package test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	// Application / namespace

	PostgresqlNamespaceName   = "test-postgresql"
	PostgresqlApplicationName = "test-postgresql"

	// Users
	PostgresqlAdminUserName     = "test-owner"
	PostgresqlUserKind          = "postgresqlusers.database.entigo.com"
	PostgresqlAdminUserSpecName = "test_owner"
	PostgresqlRegularUserName   = "test-user"
	SqlRoleKind                 = "role.postgresql.sql.m.crossplane.io"

	RegularUserExpectedGrantName  = "grant-" + PostgresqlRegularUserName + "-test-owner-" + PostgresqlInstanceName
	RegularUserExpectedUsageName  = "usage-" + RegularUserExpectedGrantName
	RegularUserExpectedSecretName = PostgresqlInstanceName + "-" + PostgresqlRegularUserName

	AdminUserInstanceProtectionName   = PostgresqlAdminUserName + "-instance-protection"
	RegularUserInstanceProtectionName = PostgresqlRegularUserName + "-instance-protection"

	// Databases
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

// ── Orchestrator ──────────────────────────────────────────────────────────────

func testPostgresql(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	pgNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, PostgresqlNamespaceName)
	defer cleanupPostgresql(t, cluster, argocd)

	applyFile(t, cluster, "./templates/postgresql_test_application.yaml")
	syncWithRetry(t, argocd, PostgresqlApplicationName)

	t.Run("Instance", func(t *testing.T) { testInstance(t, pgNs) })
	if t.Failed() {
		return
	}

	t.Run("AdminUser", func(t *testing.T) { testPostgresqlAdminUser(t, pgNs) })
	if t.Failed() {
		return
	}

	t.Run("UsersAndDatabases", func(t *testing.T) {
		t.Run("RegularUser", func(t *testing.T) { t.Parallel(); testPostgresqlRegularUser(t, pgNs) })
		t.Run("DatabaseOne", func(t *testing.T) { t.Parallel(); testPostgresqlDatabaseOne(t, pgNs) })
		t.Run("DatabaseTwo", func(t *testing.T) { t.Parallel(); testPostgresqlDatabaseTwo(t, pgNs) })
	})
	if t.Failed() {
		return
	}

	t.Run("MinimalDatabase", func(t *testing.T) { testPostgresqlMinimalDatabase(t, pgNs) })
}

// ── PostgreSQLInstance ────────────────────────────────────────────────────────

func testInstance(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()

	t.Run("SubResources", func(t *testing.T) {
		t.Run("RdsInstance", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, pgNs, RdsInstanceKind, PostgresqlInstanceName, 60, 10*time.Second)
		})
		t.Run("SecurityGroup", func(t *testing.T) {
			t.Parallel()
			waitSyncedAndReadyByLabel(t, pgNs, SecurityGroupKind, PostgresqlInstanceName, 60, 10*time.Second)
		})
		t.Run("SecurityGroupRules", func(t *testing.T) {
			t.Parallel()
			waitSecurityGroupRulesReady(t, pgNs)
		})
		t.Run("ExternalSecret", func(t *testing.T) {
			t.Parallel()
			waitExternalSecretReady(t, pgNs)
		})
		t.Run("ProviderConfig", func(t *testing.T) {
			t.Parallel()
			waitResourceExists(t, pgNs, SqlProviderConfigKind, PostgresqlInstanceName+"-providerconfig", 90, 10*time.Second)
		})
	})
	if t.Failed() {
		return
	}

	waitSyncedAndReady(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, 90, 10*time.Second)
	if t.Failed() {
		return
	}

	rdsName, err := getFirstByLabel(t, pgNs, RdsInstanceKind, PostgresqlInstanceName)
	require.NoError(t, err)
	require.NotEmpty(t, rdsName)

	// RDS fields must reflect what was specified on the composite
	require.Equal(t, "20", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.allocatedStorage"))
	require.Equal(t, "17.2", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.engineVersion"))
	require.Equal(t, "db.t3.micro", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.instanceClass"))
	require.Equal(t, "false", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.deletionProtection"))

	// Composite status endpoint must be populated once instance is ready
	require.NotEmpty(t, getField(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, ".status.endpoint.address"),
		"endpoint address should be populated")
	require.NotEmpty(t, getField(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, ".status.endpoint.port"),
		"endpoint port should be populated")

	// ExternalSecret must carry the admin username
	esName, err := getFirstByLabel(t, pgNs, ExternalSecretKind, PostgresqlInstanceName)
	require.NoError(t, err)
	require.Equal(t, "dbadmin", getField(t, pgNs, ExternalSecretKind, esName, ".spec.target.template.data.username"))

	// deletionProtection on composite propagates to RDS spec
	patchResource(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, `{"spec":{"deletionProtection":true}}`)
	waitFieldEquals(t, pgNs, RdsInstanceKind, rdsName, ".spec.forProvider.deletionProtection", "true", 30, 10*time.Second)
	patchResource(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, `{"spec":{"deletionProtection":false}}`)
	waitFieldEquals(t, pgNs, RdsInstanceKind, rdsName, ".spec.forProvider.deletionProtection", "false", 30, 10*time.Second)
}

// ── PostgreSQLUser ────────────────────────────────────────────────────────────

func testPostgresqlAdminUser(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()

	waitSyncedAndReady(t, pgNs, PostgresqlUserKind, PostgresqlAdminUserName, 60, 10*time.Second)
	waitSyncedAndReadyByLabel(t, pgNs, SqlRoleKind, PostgresqlAdminUserName, 60, 10*time.Second)
	if t.Failed() {
		return
	}

	roleName, err := getFirstByLabel(t, pgNs, SqlRoleKind, PostgresqlAdminUserName)
	require.NoError(t, err)
	require.NotEmpty(t, roleName)

	// Role external name must match spec username (snake_case)
	require.Equal(t, PostgresqlAdminUserSpecName,
		getField(t, pgNs, SqlRoleKind, roleName, `.metadata.annotations.crossplane\.io/external-name`))

	// Instance is protected from deletion while this user's Role exists
	testUsage(t, pgNs, AdminUserInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Role", PostgresqlAdminUserName)
}

func testPostgresqlRegularUser(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()

	waitSyncedAndReady(t, pgNs, PostgresqlUserKind, PostgresqlRegularUserName, 60, 10*time.Second)
	if t.Failed() {
		return
	}

	roleName, err := getFirstByLabel(t, pgNs, SqlRoleKind, PostgresqlRegularUserName)
	require.NoError(t, err)
	require.NotEmpty(t, roleName)

	// Grant: regular user gets member-of admin role
	waitSyncedAndReady(t, pgNs, SqlGrantKind, RegularUserExpectedGrantName, 60, 10*time.Second)
	require.Equal(t, PostgresqlRegularUserName,
		getField(t, pgNs, SqlGrantKind, RegularUserExpectedGrantName, ".spec.forProvider.role"))
	require.Equal(t, PostgresqlAdminUserSpecName,
		getField(t, pgNs, SqlGrantKind, RegularUserExpectedGrantName, ".spec.forProvider.memberOf"))

	// Grant is protected by Role (cannot delete Role while Grant references it)
	testUsage(t, pgNs, RegularUserExpectedUsageName, "Role", PostgresqlRegularUserName, "Grant", RegularUserExpectedGrantName)

	// Instance is protected from deletion while this user's Role exists
	testUsage(t, pgNs, RegularUserInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Role", PostgresqlRegularUserName)

	// Role external name is same as spec username (no transform)
	require.Equal(t, PostgresqlRegularUserName,
		getField(t, pgNs, SqlRoleKind, roleName, `.metadata.annotations.crossplane\.io/external-name`))

	// Role privileges: login-only, no superuser capabilities
	require.Equal(t, "false", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.createDb"))
	require.Equal(t, "true", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.login"))
	require.Equal(t, "false", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.createRole"))
	require.Equal(t, "true", getField(t, pgNs, SqlRoleKind, roleName, ".spec.forProvider.privileges.inherit"))

	// Connection secret must be created
	waitResourceExists(t, pgNs, "secret", RegularUserExpectedSecretName, 60, 10*time.Second)

	// Role cannot be deleted while Grant exists
	testUsageBlocksDeletion(t, pgNs, SqlRoleKind, roleName)
}

// ── PostgreSQLDatabase ────────────────────────────────────────────────────────

func testPostgresqlDatabaseOne(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()

	waitSyncedAndReady(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName, 60, 10*time.Second)
	if t.Failed() {
		return
	}

	sqlDbName, err := getFirstByLabel(t, pgNs, SqlDatabaseKind, DatabaseOneName)
	require.NoError(t, err)
	require.NotEmpty(t, sqlDbName)

	// Database fields
	require.Equal(t, PostgresqlAdminUserSpecName, getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.owner"))
	require.Equal(t, "UTF8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.encoding"))
	require.Equal(t, "et_EE.UTF-8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.lcCType"))
	require.Equal(t, "et_EE.UTF-8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.lcCollate"))
	require.Equal(t, "template0", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.template"))
	require.Equal(t, DatabaseOneSpecName,
		getField(t, pgNs, SqlDatabaseKind, sqlDbName, `.metadata.annotations.crossplane\.io/external-name`))

	// Grant to dbadmin
	waitSyncedAndReady(t, pgNs, SqlGrantKind, DatabaseGrantExpectedName, 60, 10*time.Second)
	require.Equal(t, "dbadmin", getField(t, pgNs, SqlGrantKind, DatabaseGrantExpectedName, ".spec.forProvider.role"))
	require.Equal(t, PostgresqlAdminUserSpecName,
		getField(t, pgNs, SqlGrantKind, DatabaseGrantExpectedName, ".spec.forProvider.memberOf"))

	// Owner and instance protections
	testUsage(t, pgNs, DatabaseOneOwnerProtectionName, "PostgreSQLUser", PostgresqlAdminUserName, "Database", DatabaseOneName)
	testUsage(t, pgNs, DatabaseOneInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Database", DatabaseOneName)

	// Extensions
	testDatabaseExtensions(t, pgNs, DatabaseOneName, DatabaseOneSpecName)

	// deletionProtection=true by default; deletion and Grant deletion must both be blocked
	require.Equal(t, "true", getField(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName, ".spec.deletionProtection"))
	testDeletionRejected(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName)
	testUsageBlocksDeletion(t, pgNs, SqlGrantKind, DatabaseGrantExpectedName)
}

func testPostgresqlDatabaseTwo(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()

	waitSyncedAndReady(t, pgNs, PostgresqlDatabaseKind, DatabaseTwoName, 60, 10*time.Second)
	if t.Failed() {
		return
	}

	sqlDbName, err := getFirstByLabel(t, pgNs, SqlDatabaseKind, DatabaseTwoName)
	require.NoError(t, err)
	require.NotEmpty(t, sqlDbName)

	require.Equal(t, PostgresqlAdminUserSpecName, getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.owner"))
	require.Equal(t, "UTF8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.encoding"))
	require.Equal(t, "et_EE.UTF-8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.lcCType"))
	require.Equal(t, "et_EE.UTF-8", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.lcCollate"))
	require.Equal(t, "template0", getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.template"))

	waitSyncedAndReady(t, pgNs, SqlGrantKind, DatabaseTwoGrantExpectedName, 60, 10*time.Second)
	require.Equal(t, "dbadmin", getField(t, pgNs, SqlGrantKind, DatabaseTwoGrantExpectedName, ".spec.forProvider.role"))
	require.Equal(t, PostgresqlAdminUserSpecName,
		getField(t, pgNs, SqlGrantKind, DatabaseTwoGrantExpectedName, ".spec.forProvider.memberOf"))

	testUsage(t, pgNs, DatabaseTwoOwnerProtectionName, "PostgreSQLUser", PostgresqlAdminUserName, "Database", DatabaseTwoName)
	testUsage(t, pgNs, DatabaseTwoInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Database", DatabaseTwoName)
}

func testPostgresqlMinimalDatabase(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()

	waitSyncedAndReady(t, pgNs, PostgresqlDatabaseKind, MinimalDatabaseName, 60, 10*time.Second)
	if t.Failed() {
		return
	}

	sqlDbName, err := getFirstByLabel(t, pgNs, SqlDatabaseKind, MinimalDatabaseName)
	require.NoError(t, err)
	require.NotEmpty(t, sqlDbName)

	// Owner is the regular user (not admin) — minimal database uses non-admin owner
	require.Equal(t, PostgresqlRegularUserName, getField(t, pgNs, SqlDatabaseKind, sqlDbName, ".spec.forProvider.owner"))

	testUsage(t, pgNs, MinimalDatabaseOwnerProtectionName, "PostgreSQLUser", PostgresqlRegularUserName, "Database", MinimalDatabaseName)
	testUsage(t, pgNs, MinimalDatabaseInstanceProtectionName, "PostgreSQLInstance", PostgresqlInstanceName, "Database", MinimalDatabaseName)
}

// testDatabaseExtensions verifies all expected SQL extensions exist with correct settings.
// Kept as a helper because it iterates a fixed set of extensions — inline would be 30+ identical lines.
func testDatabaseExtensions(t *testing.T, pgNs *terrak8s.KubectlOptions, composite, dbSpecName string) {
	t.Helper()
	type ext struct{ name, extName, schema string }
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

// ── Cleanup ───────────────────────────────────────────────────────────────────

func cleanupPostgresql(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	if t.Failed() {
		return // leave resources in place for debugging
	}
	pgNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, PostgresqlNamespaceName)

	defer func() {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocd, "delete", "application", PostgresqlApplicationName, "--ignore-not-found")
	}()

	// Delete composites in dependency order; Crossplane cascade-deletes all sub-resources.
	cleanupDisableDeletionProtectionOnDatabases(t, pgNs)
	cleanupDeleteParallel(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName, DatabaseTwoName, MinimalDatabaseName)

	cleanupDeleteParallel(t, pgNs, PostgresqlUserKind, PostgresqlRegularUserName, PostgresqlAdminUserName)

	cleanupDisableDeletionProtectionOnInstance(t, pgNs)
	cleanupDeleteAndWait(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, 180)

	cleanupNamespace(t, pgNs, cluster)
}

func cleanupDeleteParallel(t *testing.T, opts *terrak8s.KubectlOptions, kind string, names ...string) {
	if len(names) == 0 {
		return
	}
	for _, name := range names {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, opts, "delete", kind, name,
			"--cascade=foreground", "--wait=false", "--ignore-not-found")
	}
	var wg sync.WaitGroup
	for _, name := range names {
		name := name
		wg.Add(1)
		go func() {
			defer wg.Done()
			cleanupWaitGone(t, opts, kind, name, 30)
		}()
	}
	wg.Wait()
}

func cleanupDeleteAndWait(t *testing.T, opts *terrak8s.KubectlOptions, kind, name string, maxRetries int) {
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, opts, "delete", kind, name,
		"--cascade=foreground", "--wait=false", "--ignore-not-found")
	cleanupWaitGone(t, opts, kind, name, maxRetries)
}

func cleanupWaitGone(t *testing.T, opts *terrak8s.KubectlOptions, kind, name string, maxRetries int) {
	_, _ = retry.DoWithRetryE(t, fmt.Sprintf("waiting for %s/%s deletion", kind, name), maxRetries, 10*time.Second,
		func() (string, error) {
			out, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, name, "--ignore-not-found", "-o", "jsonpath={.metadata.name}")
			if err != nil {
				return "", err
			}
			if out != "" {
				return "", fmt.Errorf("%s/%s still exists", kind, name)
			}
			return "deleted", nil
		})
}

func patchDeletionProtectionIfEnabled(t *testing.T, pgNs *terrak8s.KubectlOptions, kind, name string) {
	t.Helper()
	exists, _ := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", kind, name, "--ignore-not-found", "-o", "jsonpath={.metadata.name}")
	if exists == "" {
		return
	}
	if dp, _ := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", kind, name, "-o", "jsonpath={.spec.deletionProtection}"); dp == "true" {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, pgNs, "patch", kind, name, "--type", "merge", "-p", `{"spec":{"deletionProtection":false}}`)
	}
}

func cleanupDisableDeletionProtectionOnDatabases(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	for _, dbName := range []string{DatabaseOneName, DatabaseTwoName, MinimalDatabaseName} {
		patchDeletionProtectionIfEnabled(t, pgNs, PostgresqlDatabaseKind, dbName)
	}
}

func cleanupDisableDeletionProtectionOnInstance(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	patchDeletionProtectionIfEnabled(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName)

	_, _ = retry.DoWithRetryE(t, "waiting for RDS deletionProtection=false", 30, 10*time.Second,
		func() (string, error) {
			rdsName, err := getFirstByLabel(t, pgNs, RdsInstanceKind, PostgresqlInstanceName)
			if err != nil || rdsName == "" {
				return "no-rds", nil
			}
			dp, err := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.deletionProtection}")
			if err != nil {
				return "", err
			}
			if dp != "false" {
				return "", fmt.Errorf("deletionProtection=%q", dp)
			}
			return "propagated", nil
		})
}

func cleanupNamespace(t *testing.T, pgNs, cluster *terrak8s.KubectlOptions) {
	leftovers, _ := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", "all", "--ignore-not-found", "-o", "name")
	if leftovers != "" {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, pgNs, "delete", "all", "--all", "--cascade=foreground", "--wait=false", "--ignore-not-found")
		time.Sleep(10 * time.Second)
	}
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, cluster, "delete", "namespace", PostgresqlNamespaceName, "--ignore-not-found", "--wait=true")
}
