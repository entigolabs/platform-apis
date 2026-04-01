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

	CronjobConfigurationName    = "platform-apis-cronjob"
	KafkaConfigurationName      = "platform-apis-kafka"
	PostgresqlConfigurationName = "platform-apis-postgresql"
	RepositoryConfigurationName = "platform-apis-repository"
	S3BucketConfigurationName   = "platform-apis-s3bucket"
	ValkeyConfigurationName     = "platform-apis-valkey"
	WebaccessConfigurationName  = "platform-apis-webaccess"
	WebappConfigurationName     = "platform-apis-webapp"
	ZoneConfigurationName       = "platform-apis-zone"

	ArtifactFunctionName   = "platform-apis-artifact-fn"
	DatabaseFunctionName   = "platform-apis-database-fn"
	NetwokringFunctionName = "platform-apis-networking-fn"
	StorageFunctionName    = "platform-apis-storage-fn"
	TenancyFunctionName    = "platform-apis-tenancy-fn"
	WorkloadFunctionName   = "platform-apis-workload-fn"
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
		if cfg.Has("cronjob") {
			t.Run("cronjob", func(t *testing.T) {
				t.Parallel()
				testCronjob(t, cluster, argocd)
			})
		}
		//TODO: Kafka tests placeholder. Currently disabled as kafka function work in progress
		/*if cfg.Has("kafka") {
			t.Run("kafka", func(t *testing.T) {
				t.Parallel()
				testKafka(t, cluster, argocd)
			})
		}*/
		if cfg.Has("postgresql") {
			t.Run("postgresql", func(t *testing.T) {
				t.Parallel()
				testPostgresql(t, cluster, argocd)
			})
		}
		if cfg.Has("zone") {
			t.Run("zone", func(t *testing.T) {
				t.Parallel()
				testZone(t, cluster)
			})
		}
	})
}

// waitPackagesReady waits for the Crossplane packages (Functions + Configurations) for active suites.
func waitPackagesReady(t *testing.T, cfg SuiteConfig, cluster *terrak8s.KubectlOptions) {
	t.Helper()
	t.Run("packages", func(t *testing.T) {
		if cfg.Has("cronjob") {
			checkPlatformApisHaveRequiredPackages(t, cluster, CronjobConfigurationName, WorkloadFunctionName)
		}
		//  TODO: Update when kafka in go ready
		if cfg.Has("kafka") {
			t.Run("kafka-configuration", func(t *testing.T) {
				t.Parallel()
				waitCrossplanePackageReady(t, cluster, ConfigurationKind, KafkaConfigurationName)
			})
		}
		if cfg.Has("postgresql") {
			checkPlatformApisHaveRequiredPackages(t, cluster, PostgresqlConfigurationName, DatabaseFunctionName)
		}
		if cfg.Has("repository") {
			checkPlatformApisHaveRequiredPackages(t, cluster, RepositoryConfigurationName, ArtifactFunctionName)
		}
		if cfg.Has("s3bucket") {
			checkPlatformApisHaveRequiredPackages(t, cluster, S3BucketConfigurationName, StorageFunctionName)
		}
		if cfg.Has("valkey") {
			checkPlatformApisHaveRequiredPackages(t, cluster, ValkeyConfigurationName, DatabaseFunctionName)
		}
		if cfg.Has("webaccess") {
			checkPlatformApisHaveRequiredPackages(t, cluster, WebaccessConfigurationName, NetwokringFunctionName)
		}
		if cfg.Has("webapp") {
			checkPlatformApisHaveRequiredPackages(t, cluster, WebappConfigurationName, WorkloadFunctionName)
		}
		if cfg.Has("zone") {
			checkPlatformApisHaveRequiredPackages(t, cluster, ZoneConfigurationName, TenancyFunctionName)
		}
	})
}

func checkPlatformApisHaveRequiredPackages(t *testing.T, cluster *terrak8s.KubectlOptions, configurationName, functionName string) {
	t.Run("configuration", func(t *testing.T) {
		t.Parallel()
		waitCrossplanePackageReady(t, cluster, ConfigurationKind, configurationName)
	})
	t.Run("function", func(t *testing.T) {
		t.Parallel()
		waitCrossplanePackageReady(t, cluster, FunctionKind, functionName)
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
