package test

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

// testPostgreSQLClusterOCI creates a standalone PostgreSQLCluster on OCI Database with PostgreSQL
// and checks what a team relies on: the fields refused until they are implemented, the status, the
// admin Secret, a connection from a workload, and deletion protection.
func testPostgreSQLClusterOCI(t *testing.T, cluster *terrak8s.KubectlOptions) {
	t.Run("packages", func(t *testing.T) {
		checkPlatformApisHaveRequiredPackages(t, cluster, PostgreSQLClusterConfigurationName, DatabaseFunctionName)
	})
	if t.Failed() {
		return
	}
	ns := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, PostgreSQLClusterNamespace)
	t.Cleanup(func() { cleanupPostgreSQLClusterOCI(t, cluster, ns) })

	applyFile(t, cluster, "./templates/postgresqlcluster_oci.yaml")

	t.Run("refused-fields", func(t *testing.T) { testPostgreSQLClusterRefusals(t, ns) })
	t.Run("deletion-protected", func(t *testing.T) {
		// Unset, deletionProtection takes the implementation's default, true.
		testDeletionRejected(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName)
	})

	t.Run("ready", func(t *testing.T) {
		// Creating an OCI DB system takes 15 to 30 minutes.
		waitSyncedAndReady(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, 120, 30*time.Second)
	})
	if t.Failed() {
		return
	}

	t.Run("status", func(t *testing.T) {
		require.Equal(t, "oci.database.entigo.com/v1alpha1, PostgreSQLOCIConfig",
			getField(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, ".status.providerConfig.applied"))
		require.Equal(t, "implementation",
			getField(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, ".status.effective.deletionProtection.source"))
		require.Equal(t, PostgreSQLClusterSecret,
			getField(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, ".status.connectionSecret"))
		waitFieldNonEmpty(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, ".status.endpoint.host", 30, 10*time.Second)
		require.True(t, strings.HasPrefix(getField(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, ".status.version"), "17"))

		db, err := getFirstByLabel(t, ns, OCIDBSystemKind, PostgreSQLClusterName)
		require.NoError(t, err)
		require.Equal(t, "1", getField(t, ns, OCIDBSystemKind, db, ".spec.forProvider.instanceCount"),
			"highAvailability defaults to false: one instance")
	})

	t.Run("admin-secret", func(t *testing.T) {
		for _, key := range []string{"endpoint", "port", "username", "password"} {
			v := waitFieldNonEmpty(t, ns, "secret", PostgreSQLClusterSecret, ".data."+key, 30, 10*time.Second)
			raw, err := base64.StdEncoding.DecodeString(v)
			require.NoError(t, err)
			require.NotEmpty(t, raw, "key %s", key)
		}
	})
	if t.Failed() {
		return
	}

	t.Run("connect", func(t *testing.T) {
		applyFile(t, cluster, "./templates/postgresqlcluster_oci_client.yaml")
		_, err := retry.DoWithRetryE(t, "psql client succeeds", 30, 10*time.Second, func() (string, error) {
			phase, err := terrak8s.RunKubectlAndGetOutputE(t, ns, "get", "pod", PostgreSQLClusterClient, "-o", "jsonpath={.status.phase}")
			if err != nil {
				return "", err
			}
			if phase == "Failed" {
				logs, _ := terrak8s.RunKubectlAndGetOutputE(t, ns, "logs", PostgreSQLClusterClient)
				return "", retry.FatalError{Underlying: fmt.Errorf("client failed: %s", logs)}
			}
			if phase != "Succeeded" {
				return "", fmt.Errorf("phase=%q", phase)
			}
			return phase, nil
		})
		require.NoError(t, err)
	})
}

// testPostgreSQLClusterRefusals checks that the first iteration refuses fields it does not
// implement and requires the values a cluster without a class sets, each named in the message.
func testPostgreSQLClusterRefusals(t *testing.T, ns *terrak8s.KubectlOptions) {
	cases := map[string]struct{ spec, want string }{
		"className":      {`{"className":"standard","version":"17","storage":{"sizeGB":50}}`, "className is not available yet"},
		"parameters":     {`{"parameters":{"work_mem":"64MB"},"version":"17","storage":{"sizeGB":50}}`, "parameters is not available yet"},
		"version":        {`{"storage":{"sizeGB":50}}`, "version is required"},
		"storage.sizeGB": {`{"version":"17"}`, "storage.sizeGB is required"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			manifest := fmt.Sprintf(`{"apiVersion":"database.entigo.com/v1alpha2","kind":"PostgreSQLCluster",`+
				`"metadata":{"name":"pgc-refused","namespace":"%s"},"spec":%s}`, PostgreSQLClusterNamespace, tc.spec)
			out, err := terrak8s.RunKubectlAndGetOutputE(t, ns, "create", "--dry-run=server", "-f", writeTemp(t, manifest))
			require.Error(t, err, "the cluster must be refused")
			require.Contains(t, out, tc.want)
		})
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	f := t.TempDir() + "/manifest.json"
	require.NoError(t, os.WriteFile(f, []byte(content), 0o600))
	return f
}

func cleanupPostgreSQLClusterOCI(t *testing.T, cluster, ns *terrak8s.KubectlOptions) {
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, ns, "delete", "pod", PostgreSQLClusterClient, "--ignore-not-found")
	// Deletion protection is the default, so it is switched off before the cluster is deleted.
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, ns, "patch", PostgreSQLClusterKind, PostgreSQLClusterName,
		"--type=merge", "-p", `{"spec":{"deletionProtection":false}}`)
	cleanupDeleteAndWait(t, ns, PostgreSQLClusterKind, PostgreSQLClusterName, 90)
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, cluster, "delete", "namespace", PostgreSQLClusterNamespace, "--ignore-not-found")
}
