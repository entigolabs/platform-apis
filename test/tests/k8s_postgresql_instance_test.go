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
	PostgresqlInstanceName = "postgresql-instance-test"
	PostgresqlInstanceKind = "pginstances.database.entigo.com"
	RdsInstanceKind        = "instance.rds.aws.m.upbound.io"
	SecurityGroupKind      = "securitygroup.ec2.aws.m.upbound.io"
	SecurityGroupRuleKind  = "securitygrouprule.ec2.aws.m.upbound.io"
	ExternalSecretKind     = "externalsecret.external-secrets.io"
	SqlProviderConfigKind  = "providerconfig.postgresql.sql.m.crossplane.io"
)

func testPostgresqlInstance(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	// Create: wait for instance composite and all provider-managed sub-resources
	t.Run("instance-create", func(t *testing.T) {
		verifyInstanceCreation(t, pgNs)
	})
	if t.Failed() {
		return
	}

	waitSyncedAndReady(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, 90, 10*time.Second)

	// Read: verify instance fields and endpoint
	t.Run("instance-read", func(t *testing.T) {
		t.Run("rds-fields", func(t *testing.T) { testRdsFields(t, pgNs) })
		t.Run("endpoint", func(t *testing.T) { testInstanceEndpointPopulated(t, pgNs) })
		t.Run("external-secret-username", func(t *testing.T) { testExternalSecretUsername(t, pgNs) })
	})

	// Update: patch deletionProtection on the composite and verify it propagates to RDS
	t.Run("instance-update-deletion-protection", func(t *testing.T) {
		testDeletionProtectionToggle(t, pgNs)
	})
}

func verifyInstanceCreation(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	t.Run("rds-instance", func(t *testing.T) {
		t.Parallel()
		waitSyncedAndReadyByLabel(t, pgNs, RdsInstanceKind, PostgresqlInstanceName, 60, 10*time.Second)
	})
	t.Run("security-group", func(t *testing.T) {
		t.Parallel()
		waitSyncedAndReadyByLabel(t, pgNs, SecurityGroupKind, PostgresqlInstanceName, 60, 10*time.Second)
	})
	t.Run("security-group-rules", func(t *testing.T) {
		t.Parallel()
		waitSecurityGroupRulesReady(t, pgNs)
	})
	t.Run("external-secret", func(t *testing.T) {
		t.Parallel()
		waitExternalSecretReady(t, pgNs)
	})
	t.Run("provider-config", func(t *testing.T) {
		t.Parallel()
		waitResourceExists(t, pgNs, SqlProviderConfigKind, PostgresqlInstanceName+"-providerconfig", 90, 10*time.Second)
	})
}

func waitSecurityGroupRulesReady(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("SecurityGroupRules for %s", PostgresqlInstanceName), 60, 10*time.Second,
		func() (string, error) {
			rules, err := getSecurityGroupRules(t, pgNs)
			if err != nil {
				return "", err
			}
			if err := validateIngressEgressExists(rules); err != nil {
				return "", err
			}
			return checkAllRulesReady(t, pgNs, rules)
		})
	require.NoError(t, err, "SecurityGroupRules for %s never became ready", PostgresqlInstanceName)
}

func getSecurityGroupRules(t *testing.T, pgNs *terrak8s.KubectlOptions) ([]string, error) {
	names, err := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", SecurityGroupRuleKind,
		"-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName),
		"-o", "jsonpath={.items[*].metadata.name}")
	if err != nil {
		return nil, err
	}
	rules := strings.Fields(names)
	if len(rules) < 2 {
		return nil, fmt.Errorf("expected ≥2 rules, got %d", len(rules))
	}
	return rules, nil
}

func validateIngressEgressExists(rules []string) error {
	var hasIngress, hasEgress bool
	for _, name := range rules {
		if strings.Contains(name, "-sg-ingress-") {
			hasIngress = true
		}
		if strings.Contains(name, "-sg-egress-") {
			hasEgress = true
		}
	}
	if !hasIngress || !hasEgress {
		return fmt.Errorf("missing ingress=%v or egress=%v rule", hasIngress, hasEgress)
	}
	return nil
}

func checkAllRulesReady(t *testing.T, pgNs *terrak8s.KubectlOptions, rules []string) (string, error) {
	for _, name := range rules {
		if _, err := checkConditions(t, pgNs, SecurityGroupRuleKind, name, "Synced", "Ready"); err != nil {
			return "", err
		}
	}
	return "ready", nil
}

func waitExternalSecretReady(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("ExternalSecret for %s", PostgresqlInstanceName), 90, 10*time.Second,
		func() (string, error) {
			name, err := getFirstByLabel(t, pgNs, ExternalSecretKind, PostgresqlInstanceName)
			if err != nil || name == "" {
				return "", fmt.Errorf("ExternalSecret not found yet")
			}

			if !strings.HasPrefix(name, PostgresqlInstanceName+"-es-") {
				return "", fmt.Errorf("unexpected ExternalSecret name: %s", name)
			}

			return checkConditions(t, pgNs, ExternalSecretKind, name, "Ready")
		})
	require.NoError(t, err)
}

func testRdsFields(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	rdsName, err := getFirstByLabel(t, pgNs, RdsInstanceKind, PostgresqlInstanceName)
	require.NoError(t, err)
	require.NotEmpty(t, rdsName)

	require.Equal(t, "20", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.allocatedStorage"))
	require.Equal(t, "17.2", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.engineVersion"))
	require.Equal(t, "db.t3.micro", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.instanceClass"))
	require.Equal(t, "false", getField(t, pgNs, RdsInstanceKind, rdsName, ".status.atProvider.deletionProtection"))
}

func testInstanceEndpointPopulated(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	require.NotEmpty(t, getField(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, ".status.endpoint.address"),
		"endpoint address should be populated")
	require.NotEmpty(t, getField(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, ".status.endpoint.port"),
		"endpoint port should be populated")
}

func testExternalSecretUsername(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	esName, err := getFirstByLabel(t, pgNs, ExternalSecretKind, PostgresqlInstanceName)
	require.NoError(t, err)
	require.Equal(t, "dbadmin", getField(t, pgNs, ExternalSecretKind, esName, ".spec.target.template.data.username"))
}

func testDeletionProtectionToggle(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	t.Helper()
	rdsName, err := getFirstByLabel(t, pgNs, RdsInstanceKind, PostgresqlInstanceName)
	require.NoError(t, err)
	require.NotEmpty(t, rdsName)

	// Enable on composite → propagates to RDS spec
	patchResource(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, `{"spec":{"deletionProtection":true}}`)
	waitFieldEquals(t, pgNs, RdsInstanceKind, rdsName, ".spec.forProvider.deletionProtection", "true", 30, 10*time.Second)

	// Restore to false (required for cleanup to succeed)
	patchResource(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, `{"spec":{"deletionProtection":false}}`)
	waitFieldEquals(t, pgNs, RdsInstanceKind, rdsName, ".spec.forProvider.deletionProtection", "false", 30, 10*time.Second)
}
