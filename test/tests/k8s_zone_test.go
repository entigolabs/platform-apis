package test

import (
	"fmt"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	ZoneConfigurationName = "platform-apis-zone"
	TenancyFunctionName   = "platform-apis-tenancy-fn"
	ZoneApplicationName   = "app-of-zones"
	ZoneKind              = "zone.tenancy.entigo.com"
	NodeGroupKind         = "nodegroup.eks.aws.upbound.io"
	ZoneAName             = "a"
	ZoneBName             = "b"

	AAppsNamespace       = "a-apps"
	BAppsNamespace       = "b-apps"
	AAppsApplicationName = "a-apps"
	BAppsApplicationName = "b-apps"
)

func testZone(t *testing.T, cfg SuiteConfig, cluster *terrak8s.KubectlOptions) {
	testZoneApps(t, cfg, cluster)
}

func testZoneApps(t *testing.T, cfg SuiteConfig, cluster *terrak8s.KubectlOptions) {
	aApps := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, AAppsNamespace)
	bApps := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, BAppsNamespace)

	deployAndVerifyApp(t, cluster, aApps, "./templates/a_test_application.yaml", AAppsApplicationName)
	deployAndVerifyApp(t, cluster, bApps, "./templates/b_test_application.yaml", BAppsApplicationName)

	verifyAppsRunning(t, cfg, cluster)
}

func testPodsRunning(t *testing.T, cluster *terrak8s.KubectlOptions, namespace, podName string) {
	t.Helper()
	nsOpts := terrak8s.NewKubectlOptions(cluster.ContextName, cluster.ConfigPath, namespace)
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("pod %s/%s Running", namespace, podName), 10, 10*time.Second,
		func() (string, error) {
			phase, err := terrak8s.RunKubectlAndGetOutputE(t, nsOpts, "get", "pod", podName, "-o", "jsonpath={.status.phase}")
			if err != nil {
				return "", err
			}
			if phase != "Running" {
				return "", fmt.Errorf("phase=%q", phase)
			}
			return phase, nil
		})
	require.NoError(t, err, "pod %s/%s never reached Running", namespace, podName)
}

func deployAndVerifyApp(t *testing.T, cluster, appOpts *terrak8s.KubectlOptions, templatePath, appName string) {
	t.Helper()
	applyFile(t, cluster, templatePath)
	syncWithRetry(t, appOpts, appName)
	waitApplicationHealthy(t, appOpts, appName)
}

func verifyAppsRunning(t *testing.T, cfg SuiteConfig, cluster *terrak8s.KubectlOptions) {
	t.Run("apps-running", func(t *testing.T) {
		if cfg.Has("a-apps") {
			t.Run("a1", func(t *testing.T) {
				t.Parallel()
				testPodsRunning(t, cluster, "a1", "a1-curl")
			})
		}
		if cfg.Has("b-apps") {
			t.Run("b1", func(t *testing.T) {
				t.Parallel()
				testPodsRunning(t, cluster, "b1", "b1-curl")
			})
		}
	})
}
