package test

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

const (
	AppProjectName        = "zone"
	NamespaceZoneLabelKey = "app.kubernetes.io/name"
	ZoneApplicationName   = "app-of-zones"
	ZoneAName             = "a"
	ZoneBName             = "b"
	ZoneConfigurationName = "platform-apis-zone"
	ZoneKind              = "zone.tenancy.entigo.com"
	ZoneNamespaceName     = "test-zone"
	TenancyFunctionName   = "platform-apis-tenancy-fn"
)

func testPlatformApisZone(t *testing.T, argocdNamespace string, clusterOptions *terrak8s.KubectlOptions, argocdOptions *terrak8s.KubectlOptions, signalZonesReady func(bool)) {

	setupFailed := true
	defer func() {
		if setupFailed {
			signalZonesReady(false)
		}
	}()

	setupStart := time.Now()
	t.Run("setup", func(t *testing.T) {
		t.Parallel()
		t.Run("configuration", func(t *testing.T) {
			t.Parallel()
			waitForResourceHealthyAndInstalled(t, clusterOptions, ConfigurationKind, ZoneConfigurationName)
			fmt.Printf("[%s] Configuration '%s' is Healthy/Installed\n", argocdNamespace, ZoneConfigurationName)
		})
		t.Run("function", func(t *testing.T) {
			t.Parallel()
			waitForResourceHealthyAndInstalled(t, clusterOptions, FunctionKind, TenancyFunctionName)
			fmt.Printf("[%s] Function '%s' is Healthy/Installed\n", argocdNamespace, TenancyFunctionName)
		})
	})

	fmt.Printf("[%s] TIMING: Zone setup took %s\n", argocdNamespace, time.Since(setupStart))
	if t.Failed() {
		return
	}

	testAppProjectExists(t, argocdNamespace, argocdOptions)
	testZoneApplicationApplied(t, argocdNamespace, argocdOptions)
	testVerifyZoneApplicationName(t, argocdNamespace, argocdOptions)
	testZoneApplicationSynced(t, argocdNamespace, argocdOptions)

	setupFailed = false
	testZoneResourcesParallel(t, clusterOptions, signalZonesReady)
	testNamespaceCreated(t, argocdNamespace, clusterOptions)
	testNamespaceHasValidZoneLabel(t, argocdNamespace, clusterOptions)
}

func testAppProjectExists(t *testing.T, argocdNamespace string, argocdOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "appproject", AppProjectName, "-n", argocdNamespace)
	require.NoError(t, err, fmt.Sprintf("[%s] AppProject '%s' not found", argocdNamespace, AppProjectName))
}

func testZoneApplicationApplied(t *testing.T, argocdNamespace string, argocdOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "apply", "-f", "./templates/zone_test_application.yaml", "-n", argocdNamespace)
	require.NoError(t, err)
}

func testVerifyZoneApplicationName(t *testing.T, argocdNamespace string, argocdOptions *terrak8s.KubectlOptions) {
	appName, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "application", ZoneApplicationName, "-n", argocdNamespace, "-o", "jsonpath={.metadata.name}")
	require.NoError(t, err)
	require.Equal(t, ZoneApplicationName, appName)
}

func testZoneApplicationSynced(t *testing.T, argocdNamespace string, argocdOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "patch", "application", ZoneApplicationName, "-n", argocdNamespace, "--type", "merge", "-p", `{"operation":{"initiatedBy":{"username":"test"},"sync":{"revision":"HEAD"}}}`)
	require.NoError(t, err)

	_, err = retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for Application to sync", argocdNamespace), 30, 10*time.Second, func() (string, error) {
		output, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "application", ZoneApplicationName, "-n", argocdNamespace, "-o", "jsonpath={.status.sync.status}")
		if err != nil {
			return "", err
		}
		if output != "Synced" {
			return "", fmt.Errorf("status: %s", output)
		}
		return output, nil
	})
	require.NoError(t, err)
}

func testZoneResourcesParallel(t *testing.T, clusterOptions *terrak8s.KubectlOptions, signalZonesReady func(bool)) {
	var readyCount atomic.Int32

	for _, zone := range []string{ZoneAName, ZoneBName} {
		zone := zone
		t.Run(fmt.Sprintf("zone-%s", zone), func(t *testing.T) {
			t.Parallel()
			defer func() {
				if t.Failed() {
					signalZonesReady(false)
				}
			}()

			testZoneResourceExists(t, clusterOptions, zone)
			testZoneResourceSyncedAndReady(t, clusterOptions, zone)

			if readyCount.Add(1) == 2 {
				signalZonesReady(true)
			}

			testZoneHasNodegroupAndItIsReady(t, clusterOptions, zone)
		})
	}
}

func testZoneResourceExists(t *testing.T, clusterOptions *terrak8s.KubectlOptions, zone string) {
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("Wait for zone %s", zone), 30, 10*time.Second, func() (string, error) {
		return terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", ZoneKind, zone, "-o", "jsonpath={.metadata.name}")
	})
	require.NoError(t, err)
}

func testZoneResourceSyncedAndReady(t *testing.T, clusterOptions *terrak8s.KubectlOptions, zone string) {
	waitForResourceSyncedAndReady(t, clusterOptions, ZoneKind, zone)
}

func testZoneHasNodegroupAndItIsReady(t *testing.T, clusterOptions *terrak8s.KubectlOptions, zone string) {
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("Wait for nodegroup in zone %s", zone), 30, 10*time.Second, func() (string, error) {
		count, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "nodegroup.eks.aws.upbound.io", "-l", fmt.Sprintf("crossplane.io/composite=%s", zone), "-o", "jsonpath={.items[*].metadata.name}")
		if err != nil || count == "" {
			return "", fmt.Errorf("nodegroup missing")
		}

		ready, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "nodegroup.eks.aws.upbound.io", "-l", fmt.Sprintf("crossplane.io/composite=%s", zone), "-o", `jsonpath={.items[0].status.conditions[?(@.type=="Ready")].status}`)
		if ready != "True" {
			return "", fmt.Errorf("nodegroup not ready")
		}
		return ready, nil
	})
	require.NoError(t, err)
}

func testNamespaceCreated(t *testing.T, argocdNamespace string, clusterOptions *terrak8s.KubectlOptions) {
	_, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "create", "namespace", ZoneNamespaceName)
	if err != nil {
		_, err = terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "namespace", ZoneNamespaceName)
		require.NoError(t, err, fmt.Sprintf("[%s] Namespace '%s' not found", argocdNamespace, ZoneNamespaceName))
	}
}

func testNamespaceHasValidZoneLabel(t *testing.T, argocdNamespace string, clusterOptions *terrak8s.KubectlOptions) {
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for namespace label", argocdNamespace), 30, 10*time.Second, func() (string, error) {
		label, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "namespace", ZoneNamespaceName, "-o", "jsonpath={.metadata.labels.tenancy\\.entigo\\.com/zone}")
		if err != nil {
			return "", err
		}
		if label != ZoneAName {
			return "", fmt.Errorf("namespace label %s expected '%s', got '%s'", NamespaceZoneLabelKey, ZoneAName, label)
		}
		return label, nil
	})
	require.NoError(t, err, fmt.Sprintf("[%s] Namespace '%s' label %s != '%s'", argocdNamespace, ZoneNamespaceName, NamespaceZoneLabelKey, ZoneAName))
}

//---- CLEANUP FUNCTIONS ----

func cleanupZoneResources(t *testing.T, argocdNamespace string, argocdOptions *terrak8s.KubectlOptions, clusterOptions *terrak8s.KubectlOptions) {
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "delete", "namespace", ZoneNamespaceName, "--ignore-not-found")
	_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "delete", "application", ZoneApplicationName, "-n", argocdNamespace, "--ignore-not-found")
}

//apply apps in zones a and b
//check apps
//check pods
//check pods nodeselectors
//create users
//check user rights
//now cleanup all created resources... PostgresqlInstance and RdsInstance have deletionProtection... change it to false first...
