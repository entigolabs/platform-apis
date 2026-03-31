package test

import (
	"fmt"
	"testing"
	"time"

	"github.com/entigolabs/entigo-infralib-common/k8s"
	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	FunctionKind      = "function.pkg.crossplane.io"
	ConfigurationKind = "configuration.pkg.crossplane.io"
)

func TestK8sPlatformApisAWSBiz(t *testing.T) {
	testPlatformApis(t, "aws", "biz")
}

// func TestK8sPlatformApisAWSPri(t *testing.T) {
// 	testPlatformApis(t, "aws", "pri")
// }

func testPlatformApis(t *testing.T, cloudName, envName string) {
	t.Parallel()

	kubectlOptions, _ := k8s.CheckKubectlConnection(t, cloudName, envName)
	cfg := loadSuiteConfig()

	cluster, argocd := setupKubectlClients(kubectlOptions, envName)

	waitPackagesReady(t, cfg, cluster)
	if t.Failed() {
		return
	}

	t.Run("setup-zone", func(t *testing.T) {
		setupZoneSync(t, cluster, argocd)
	})
	if t.Failed() {
		t.Fatal("Zones deployment failed. Can not run tests.")
	}

	t.Run("parallel-tests", func(t *testing.T) {
		if cfg.Has("zone") {
			t.Run("zone", func(t *testing.T) {
				t.Parallel()
				testZone(t, cluster)
			})
		}

		if cfg.Has("postgresql") {
			t.Run("postgresql", func(t *testing.T) {
				t.Parallel()
				testPostgresql(t, cluster, argocd)
			})
		}
	})
}

// waitPackagesReady waits for the Crossplane packages (Functions + Configurations) for active suites.
func waitPackagesReady(t *testing.T, cfg SuiteConfig, cluster *terrak8s.KubectlOptions) {
	t.Helper()
	t.Run("packages", func(t *testing.T) {
		if cfg.Has("zone") {
			t.Run("zone-configuration", func(t *testing.T) {
				t.Parallel()
				waitCrossplanePackageReady(t, cluster, ConfigurationKind, ZoneConfigurationName)
			})
			t.Run("tenancy-function", func(t *testing.T) {
				t.Parallel()
				waitCrossplanePackageReady(t, cluster, FunctionKind, TenancyFunctionName)
			})
		}
		if cfg.Has("postgresql") {
			t.Run("postgresql-configuration", func(t *testing.T) {
				t.Parallel()
				waitCrossplanePackageReady(t, cluster, ConfigurationKind, PostgresqlConfigurationName)
			})
			t.Run("database-function", func(t *testing.T) {
				t.Parallel()
				waitCrossplanePackageReady(t, cluster, FunctionKind, DatabaseFunctionName)
			})
		}
	})
}

func setupKubectlClients(opts *terrak8s.KubectlOptions, envName string) (*terrak8s.KubectlOptions, *terrak8s.KubectlOptions) {
	argocdNamespace := fmt.Sprintf("argocd-%s", envName)
	cluster := terrak8s.NewKubectlOptions(opts.ContextName, opts.ConfigPath, "")
	argocd := terrak8s.NewKubectlOptions(opts.ContextName, opts.ConfigPath, argocdNamespace)
	return cluster, argocd
}

// setupZoneSync deploys zone
func setupZoneSync(t *testing.T, cluster, argocd *terrak8s.KubectlOptions) {
	t.Helper()

	applyFile(t, cluster, "./templates/zone_test_application.yaml")
	syncWithRetry(t, argocd, ZoneApplicationName)

	for _, zone := range []string{ZoneAName, ZoneBName} {
		waitSyncedAndReady(t, cluster, ZoneKind, zone, 30, 10*time.Second)
		waitZoneNodegroupReady(t, cluster, zone)
	}

	waitApplicationHealthy(t, argocd, ZoneApplicationName)
}

func waitZoneNodegroupReady(t *testing.T, cluster *terrak8s.KubectlOptions, zone string) {
	t.Helper()
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("zone %q NodeGroup Ready", zone), 30, 10*time.Second,
		func() (string, error) {
			status, err := terrak8s.RunKubectlAndGetOutputE(t, cluster, "get", NodeGroupKind,
				"-l", fmt.Sprintf("crossplane.io/composite=%s", zone),
				"-o", `jsonpath={.items[0].status.conditions[?(@.type=="Ready")].status}`)
			if err != nil {
				return "", err
			}
			if status != "True" {
				return "", fmt.Errorf("zone %q NodeGroup not Ready: %q", zone, status)
			}
			return status, nil
		})
	require.NoError(t, err, "zone %q NodeGroup never became Ready", zone)
}
