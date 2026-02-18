package test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	PostgresqlInstanceName     = "postgresql-instance-test"
	PostgresqlInstanceKind     = "postgresqlinstance.database.entigo.com"
	RdsInstanceKind            = "instance.rds.aws.m.upbound.io"
	SecurityGroupKind          = "securitygroup.ec2.aws.m.upbound.io"
	SecurityGroupRuleKind      = "securitygrouprule.ec2.aws.m.upbound.io"
	ExternalSecretKind         = "externalsecret.external-secrets.io"
	SqlProviderConfigKind      = "providerconfig.postgresql.sql.m.crossplane.io"
	ProviderConfigExpectedName = PostgresqlInstanceName + "-providerconfig"
)

func runPostgresqlInstanceTests(t *testing.T, argocdNamespace string, namespaceOptions *terrak8s.KubectlOptions) {
	testPostgresqlInstanceApplied(t, namespaceOptions)

	t.Run("sub-resources-ready", func(t *testing.T) {

		t.Run("sg-rules", func(t *testing.T) {
			t.Parallel()
			testSecurityGroupRulesSyncedAndReady(t, namespaceOptions)
		})
		t.Run("sg", func(t *testing.T) {
			t.Parallel()
			testSecurityGroupSyncedAndReady(t, namespaceOptions)
		})
		t.Run("ext-secret", func(t *testing.T) {
			t.Parallel()
			testExternalSecretReady(t, namespaceOptions)
		})
		t.Run("prov-config", func(t *testing.T) {
			t.Parallel()
			testProviderConfigExists(t, namespaceOptions)
		})
		t.Run("rds", func(t *testing.T) {
			t.Parallel()
			testRdsInstanceSyncedAndReady(t, namespaceOptions)
		})
	})
	if t.Failed() {
		return
	}

	testExternalSecretContentVerified(t, namespaceOptions)
	testPostgresqlInstanceSyncedAndReady(t, namespaceOptions)
	testRdsInstanceFieldsVerified(t, argocdNamespace, namespaceOptions)
	testDeletionProtectionToggle(t, argocdNamespace, namespaceOptions)
}

func testPostgresqlInstanceApplied(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "apply", "-f", "./templates/postgresql_test_instance.yaml", "-n", PostgresqlNamespaceName)
	require.NoError(t, err)
}

func testPostgresqlInstanceSyncedAndReady(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	waitForResourceSyncedAndReady(t, namespaceOptions, PostgresqlInstanceKind, PostgresqlInstanceName, 90)
}

func testSecurityGroupSyncedAndReady(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("Wait for SG %s", PostgresqlInstanceName), 90, 10*time.Second, func() (string, error) {
		sgName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SecurityGroupKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
		if err != nil || sgName == "" {
			return "", fmt.Errorf("SG missing")
		}
		waitForResourceSyncedAndReady(t, namespaceOptions, SecurityGroupKind, sgName, 90)
		return sgName, nil
	})
	require.NoError(t, err)
}

func testSecurityGroupRulesSyncedAndReady(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for SG Rules", 90, 10*time.Second, func() (string, error) {
		ruleNames, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SecurityGroupRuleKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[*].metadata.name}")
		if err != nil {
			return "", err
		}
		names := strings.Fields(ruleNames)
		if len(names) < 2 {
			return "", fmt.Errorf("found %d rules, expected >= 2", len(names))
		}
		for _, name := range names {
			waitForResourceSyncedAndReady(t, namespaceOptions, SecurityGroupRuleKind, name, 90)
		}
		return "Ready", nil
	})
	require.NoError(t, err)
}

func testExternalSecretReady(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for ExternalSecret", 90, 10*time.Second, func() (string, error) {
		esName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", ExternalSecretKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
		if err != nil || esName == "" {
			return "", fmt.Errorf("ES missing")
		}

		waitForResourceCondition(t, namespaceOptions, ExternalSecretKind, esName, "Ready", "True", 90)
		return esName, nil
	})
	require.NoError(t, err)
}

func testExternalSecretContentVerified(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	esName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", ExternalSecretKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
	require.NoError(t, err)
	require.NotEmpty(t, esName, "ExternalSecret name should not be empty")

	username, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", ExternalSecretKind, esName, "-o", "jsonpath={.spec.target.template.data.username}")
	require.NoError(t, err)
	require.Equal(t, "dbadmin", username, "ExternalSecret username mismatch")
}

func testProviderConfigExists(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for ProviderConfig", 90, 10*time.Second, func() (string, error) {
		name, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", SqlProviderConfigKind, ProviderConfigExpectedName, "-o", "jsonpath={.metadata.name}")
		if err != nil || name == "" {
			return "", fmt.Errorf("missing")
		}
		return name, nil
	})
	require.NoError(t, err)
}

func testRdsInstanceSyncedAndReady(t *testing.T, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, "Wait for RDS", 90, 10*time.Second, func() (string, error) {
		rdsName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
		if err != nil || rdsName == "" {
			return "", fmt.Errorf("RDS missing")
		}
		waitForResourceSyncedAndReady(t, namespaceOptions, RdsInstanceKind, rdsName, 90)
		return rdsName, nil
	})
	require.NoError(t, err)
}

func testRdsInstanceFieldsVerified(t *testing.T, argocdNamespace string, namespaceOptions *terrak8s.KubectlOptions) {
	rdsName, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to find RDS Instance", argocdNamespace))
	require.NotEmpty(t, rdsName, fmt.Sprintf("[%s] No RDS Instance found for composite '%s'", argocdNamespace, PostgresqlInstanceName))

	allocatedStorage, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.allocatedStorage}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get allocatedStorage", argocdNamespace))
	require.Equal(t, "20", allocatedStorage, fmt.Sprintf("[%s] RDS Instance '%s' allocatedStorage mismatch", argocdNamespace, rdsName))

	engineVersion, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.engineVersion}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get engineVersion", argocdNamespace))
	require.Equal(t, "17.2", engineVersion, fmt.Sprintf("[%s] RDS Instance '%s' engineVersion mismatch", argocdNamespace, rdsName))

	instanceClass, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.instanceClass}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get instanceClass", argocdNamespace))
	require.Equal(t, "db.t3.micro", instanceClass, fmt.Sprintf("[%s] RDS Instance '%s' instanceClass mismatch", argocdNamespace, rdsName))

	deletionProtection, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.deletionProtection}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get deletionProtection", argocdNamespace))
	require.Equal(t, "false", deletionProtection, fmt.Sprintf("[%s] RDS Instance '%s' deletionProtection should be false", argocdNamespace, rdsName))

	endpointAddress, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", PostgresqlInstanceKind, PostgresqlInstanceName, "-o", "jsonpath={.status.endpoint.address}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get endpoint address from PostgreSQLInstance status", argocdNamespace))
	require.NotEmpty(t, endpointAddress, fmt.Sprintf("[%s] PostgreSQLInstance '%s' status endpoint address is empty", argocdNamespace, PostgresqlInstanceName))

	endpointPort, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", PostgresqlInstanceKind, PostgresqlInstanceName, "-o", "jsonpath={.status.endpoint.port}")
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to get endpoint port from PostgreSQLInstance status", argocdNamespace))
	require.NotEmpty(t, endpointPort, fmt.Sprintf("[%s] PostgreSQLInstance '%s' status endpoint port is empty", argocdNamespace, PostgresqlInstanceName))
}

func testDeletionProtectionToggle(t *testing.T, argocdNamespace string, namespaceOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "patch", PostgresqlInstanceKind, PostgresqlInstanceName, "-n", PostgresqlNamespaceName, "--type", "merge", "-p", `{"spec":{"deletionProtection":true}}`)
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to patch deletionProtection to true", argocdNamespace))

	_, err = retry.DoWithRetryE(t, "Wait for True", 30, 10*time.Second, func() (string, error) {
		rdsName, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
		val, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.deletionProtection}")
		if val != "true" {
			return "", fmt.Errorf("not true yet")
		}
		return "ok", nil
	})

	_, err = terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "patch", PostgresqlInstanceKind, PostgresqlInstanceName, "-n", PostgresqlNamespaceName, "--type", "merge", "-p", `{"spec":{"deletionProtection":false}}`)
	require.NoError(t, err, fmt.Sprintf("[%s] Failed to patch deletionProtection to false", argocdNamespace))

	_, err = retry.DoWithRetryE(t, "Wait for False", 30, 10*time.Second, func() (string, error) {
		rdsName, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
		val, _ := terrak8s.RunKubectlAndGetOutputE(t, namespaceOptions, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.deletionProtection}")
		if val != "false" {
			return "", fmt.Errorf("not false yet")
		}
		return "ok", nil
	})
}
