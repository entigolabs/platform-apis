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

func cleanupPostgresqlResources(t *testing.T, argocdNamespace string, clusterOptions *terrak8s.KubectlOptions) {
	pgNsOptions := terrak8s.NewKubectlOptions(clusterOptions.ContextName, clusterOptions.ConfigPath, PostgresqlNamespaceName)
	nsTag := argocdNamespace

	fmt.Printf("[%s] Cleanup Phase 1: Deleting Databases\n", nsTag)
	deleteResourcesParallel(t, nsTag, pgNsOptions, []struct{ kind, name string }{
		{PostgresqlDatabaseKind, PostgresqlDatabaseName},
		{PostgresqlDatabaseKind, MinimalDatabaseName},
	})

	fmt.Printf("[%s] Cleanup Phase 2: Deleting Users\n", nsTag)
	deleteResourcesParallel(t, nsTag, pgNsOptions, []struct{ kind, name string }{
		{PostgresqlAdminUserKind, PostgresqlRegularUserName},
		{PostgresqlAdminUserKind, PostgresqlAdminUserName},
	})

	fmt.Printf("[%s] Cleanup Phase 3: Sweeping Leftovers (Usages, Grants, Roles)\n", nsTag)
	cleanupDeleteAllOfKind(t, nsTag, pgNsOptions, UsageKind)
	cleanupDeleteAllOfKind(t, nsTag, pgNsOptions, SqlGrantKind)
	cleanupDeleteAllOfKind(t, nsTag, pgNsOptions, SqlRoleKind)

	fmt.Printf("[%s] Cleanup Phase 4: Disabling Deletion Protection\n", nsTag)
	cleanupDisableDeletionProtection(t, pgNsOptions)

	fmt.Printf("[%s] Cleanup Phase 5: Deleting PostgreSQL Instance\n", nsTag)
	deleteResourcesParallel(t, nsTag, pgNsOptions, []struct{ kind, name string }{
		{PostgresqlInstanceKind, PostgresqlInstanceName},
	})

	fmt.Printf("[%s] Cleanup Phase 6: Verifying all generated resources are gone\n", nsTag)
	leftoversFound := cleanupCheckForLeftovers(t, nsTag, pgNsOptions)

	if leftoversFound {
		fmt.Printf("[%s] WARNING: Leftovers found. Initiating force sweep.\n", nsTag)
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, pgNsOptions, "delete", "all", "--all", "-n", PostgresqlNamespaceName, "--cascade=foreground", "--wait=false", "--ignore-not-found")
		time.Sleep(10 * time.Second)
	} else {
		fmt.Printf("[%s] Namespace appears clean.\n", nsTag)
	}

	fmt.Printf("[%s] Cleanup Phase 7: Deleting Namespace\n", nsTag)
	cleanupNamespace(t, clusterOptions)

	fmt.Printf("[%s] Cleanup Complete", nsTag)
}

func deleteResourcesParallel(t *testing.T, nsTag string, opts *terrak8s.KubectlOptions, targets []struct{ kind, name string }) {
	var wg sync.WaitGroup

	for _, target := range targets {
		fmt.Printf("[%s] Cleanup: Triggering delete %s/%s\n", nsTag, target.kind, target.name)
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, opts, "delete", target.kind, target.name, "-n", PostgresqlNamespaceName, "--cascade=foreground", "--wait=false", "--ignore-not-found")
	}

	for _, target := range targets {
		wg.Add(1)
		target := target
		go func() {
			defer wg.Done()
			cleanupWaitForDeletion(t, opts, target.kind, target.name, 60)
		}()
	}
	wg.Wait()
}

func cleanupDeleteAllOfKind(t *testing.T, nsTag string, opts *terrak8s.KubectlOptions, kind string) {
	out, _ := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, "-n", PostgresqlNamespaceName, "-o", "jsonpath={.items[*].metadata.name}", "--ignore-not-found")
	if out == "" {
		return
	}
	names := strings.Fields(out)

	targets := make([]struct{ kind, name string }, len(names))
	for i, name := range names {
		targets[i] = struct{ kind, name string }{kind, name}
	}
	deleteResourcesParallel(t, nsTag, opts, targets)
}

func cleanupWaitForDeletion(t *testing.T, opts *terrak8s.KubectlOptions, kind string, name string, maxRetries int) {
	_, _ = retry.DoWithRetryE(t, fmt.Sprintf("Wait delete %s/%s", kind, name), maxRetries, 5*time.Second, func() (string, error) {
		out, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, name, "-n", PostgresqlNamespaceName, "--ignore-not-found")
		if err != nil {
			return "", err
		}
		if out != "" {
			return "", fmt.Errorf("still terminating")
		}
		return "deleted", nil
	})
}

func cleanupDisableDeletionProtection(t *testing.T, opts *terrak8s.KubectlOptions) {
	exists, _ := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", PostgresqlInstanceKind, PostgresqlInstanceName, "-n", PostgresqlNamespaceName, "--ignore-not-found", "-o", "name")
	if exists == "" {
		return
	}

	_, _ = terrak8s.RunKubectlAndGetOutputE(t, opts, "patch", PostgresqlInstanceKind, PostgresqlInstanceName, "-n", PostgresqlNamespaceName, "--type", "merge", "-p", `{"spec":{"deletionProtection":false}}`)

	_, _ = retry.DoWithRetryE(t, "Wait RDS protection=false", 30, 5*time.Second, func() (string, error) {
		rdsName, _ := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", RdsInstanceKind, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "jsonpath={.items[0].metadata.name}")
		if rdsName == "" {
			return "no-rds", nil
		}
		dp, _ := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", RdsInstanceKind, rdsName, "-o", "jsonpath={.spec.forProvider.deletionProtection}")
		if dp != "false" {
			return "", fmt.Errorf("RDS deletionProtection is %s", dp)
		}
		return "ready-to-delete", nil
	})
}

func cleanupCheckForLeftovers(t *testing.T, nsTag string, opts *terrak8s.KubectlOptions) bool {
	kinds := []string{
		RdsInstanceKind,
		SecurityGroupRuleKind,
		SecurityGroupKind,
		ExternalSecretKind,
		SqlProviderConfigKind,
	}

	foundLeftovers := false
	var wg sync.WaitGroup
	var mutex sync.Mutex

	for _, k := range kinds {
		k := k
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, _ := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", k, "-l", fmt.Sprintf("crossplane.io/composite=%s", PostgresqlInstanceName), "-o", "name", "--ignore-not-found")
			if out != "" {
				fmt.Printf("[%s] Leftover found: %s\n", nsTag, out)
				mutex.Lock()
				foundLeftovers = true
				mutex.Unlock()
			}
		}()
	}
	wg.Wait()
	return foundLeftovers
}

func cleanupNamespace(t *testing.T, clusterOpts *terrak8s.KubectlOptions) {
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, clusterOpts, "delete", "namespace", PostgresqlNamespaceName, "--ignore-not-found", "--wait=true")
}
