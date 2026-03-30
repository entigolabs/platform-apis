package test

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
)

var managedProviderKinds = []string{
	RdsInstanceKind,
	SecurityGroupRuleKind,
	SecurityGroupKind,
	ExternalSecretKind,
	SqlProviderConfigKind,
}

func cleanupPostgresql(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	if t.Failed() {
		return // leave resources in place for debugging
	}
	pgNs := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, PostgresqlNamespaceName)

	defer func() {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocd, "delete", "application", PostgresqlApplicationName, "--ignore-not-found")
	}()

	cleanupDisableDeletionProtectionOnDatabases(t, pgNs)
	cleanupDeleteParallel(t, pgNs, PostgresqlDatabaseKind, DatabaseOneName, DatabaseTwoName, MinimalDatabaseName)
	cleanupDeleteParallel(t, pgNs, PostgresqlAdminUserKind, PostgresqlRegularUserName, PostgresqlAdminUserName)

	cleanupDeleteAllOfKind(t, pgNs, UsageKind)
	cleanupDeleteAllOfKind(t, pgNs, SqlGrantKind)
	cleanupDeleteAllOfKind(t, pgNs, SqlRoleKind)

	cleanupDisableDeletionProtectionOnInstance(t, pgNs)
	cleanupDeleteAndWait(t, pgNs, PostgresqlInstanceKind, PostgresqlInstanceName, 180)

	cleanupWaitProviderResourcesGone(t, pgNs)

	if !cleanupCheckAllGone(t, pgNs) {
		t.Log("WARNING: some managed resources still exist; skipping namespace deletion to avoid finalizer deadlock")
		return
	}

	cleanupNamespace(t, pgNs, cluster)
}

// cleanupDeleteParallel deletes resources of a kind in parallel and waits for all to be gone.
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

func cleanupDeleteAllOfKind(t *testing.T, opts *terrak8s.KubectlOptions, kind string) {
	out, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, "--ignore-not-found", "-o", "jsonpath={.items[*].metadata.name}")
	if err != nil || out == "" {
		return
	}
	names := strings.Fields(out)

	cleanupDeleteParallel(t, opts, kind, names...)
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

func cleanupWaitProviderResourcesGone(t *testing.T, pgNs *terrak8s.KubectlOptions) {
	var wg sync.WaitGroup
	for _, kind := range managedProviderKinds {
		kind := kind
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = retry.DoWithRetryE(t, fmt.Sprintf("waiting for %s deletion", kind), 60, 10*time.Second,
				func() (string, error) {
					out, _ := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", kind,
						"-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName),
						"--ignore-not-found", "-o", "jsonpath={.items[*].metadata.name}")
					if out != "" {
						return "", fmt.Errorf("%s still exist: %s", kind, out)
					}
					return "gone", nil
				})
		}()
	}
	wg.Wait()
}

func cleanupCheckAllGone(t *testing.T, pgNs *terrak8s.KubectlOptions) bool {
	for _, kind := range managedProviderKinds {
		out, err := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", kind,
			"-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName),
			"--ignore-not-found", "-o", "jsonpath={.items[*].metadata.name}")
		if err != nil || out != "" {
			return false
		}
	}
	return true
}

func cleanupNamespace(t *testing.T, pgNs, cluster *terrak8s.KubectlOptions) {
	leftovers, _ := terrak8s.RunKubectlAndGetOutputE(t, pgNs, "get", "all", "--ignore-not-found", "-o", "name")
	if leftovers != "" {
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, pgNs, "delete", "all", "--all", "--cascade=foreground", "--wait=false", "--ignore-not-found")
		time.Sleep(10 * time.Second)
	}
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, cluster, "delete", "namespace", PostgresqlNamespaceName, "--ignore-not-found", "--wait=true")
}
