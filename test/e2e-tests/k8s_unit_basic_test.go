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

func TestK8sPlatformApisAWSBiz(t *testing.T) {
	testK8sPlatformApis(t, "aws", "biz")
}

// func TestK8sPlatformApisAWSPri(t *testing.T) {
// 	testK8sPlatformApis(t, "aws", "pri")
// }

func testK8sPlatformApis(t *testing.T, cloudName string, envName string) {
	t.Parallel()
	kubectlOptions, namespaceName := k8s.CheckKubectlConnection(t, cloudName, envName)

	err := terrak8s.WaitUntilDeploymentAvailableE(t, kubectlOptions, namespaceName, 20, 6*time.Second)
	require.NoError(t, err, fmt.Sprintf("platform-apis deployment %s not available", namespaceName))

	argocdNamespace := fmt.Sprintf("argocd-%s", envName)
	argocdOptions := terrak8s.NewKubectlOptions(kubectlOptions.ContextName, kubectlOptions.ConfigPath, argocdNamespace)

	clusterOptions := terrak8s.NewKubectlOptions(kubectlOptions.ContextName, kubectlOptions.ConfigPath, "")

	defer func() {
		fmt.Printf("[%s] Cleanup: deleting test resources\n", argocdNamespace)
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "delete", "application", "app-of-zone", "-n", argocdNamespace)
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "delete", "appproject", "zone", "-n", argocdNamespace)
		_, _ = terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "delete", "namespace", "test-namespace")
		fmt.Printf("[%s] Cleanup: done\n", argocdNamespace)
	}()

	fmt.Printf("[%s] Step 1: Applying AppProject 'zone'\n", argocdNamespace)
	_, err = terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "apply", "-f", "./templates/appproject.yaml", "-n", argocdNamespace)
	require.NoError(t, err, fmt.Sprintf("[%s] Applying AppProject error", argocdNamespace))
	fmt.Printf("[%s] Step 1: PASSED - AppProject applied\n", argocdNamespace)

	fmt.Printf("[%s] Step 2: Verifying AppProject 'zone'\n", argocdNamespace)
	projectName, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "appproject", "zone", "-n", argocdNamespace, "-o", "jsonpath={.metadata.name}")
	require.NoError(t, err, fmt.Sprintf("[%s] AppProject not found", argocdNamespace))
	require.Equal(t, "zone", projectName, fmt.Sprintf("[%s] AppProject name mismatch", argocdNamespace))

	projectDesc, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "appproject", "zone", "-n", argocdNamespace, "-o", "jsonpath={.spec.description}")
	require.NoError(t, err, fmt.Sprintf("[%s] AppProject description not found", argocdNamespace))
	require.Equal(t, "Zone project for platform-apis test", projectDesc, fmt.Sprintf("[%s] AppProject description mismatch", argocdNamespace))

	sourceRepos, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "appproject", "zone", "-n", argocdNamespace, "-o", "jsonpath={.spec.sourceRepos[0]}")
	require.NoError(t, err, fmt.Sprintf("[%s] AppProject sourceRepos not found", argocdNamespace))
	require.Equal(t, "*", sourceRepos, fmt.Sprintf("[%s] AppProject sourceRepos mismatch", argocdNamespace))
	fmt.Printf("[%s] Step 2: PASSED - AppProject verified (name, description, sourceRepos)\n", argocdNamespace)

	fmt.Printf("[%s] Step 3: Applying Application 'app-of-zone'\n", argocdNamespace)
	_, err = terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "apply", "-f", "./templates/application.yaml", "-n", argocdNamespace)
	require.NoError(t, err, fmt.Sprintf("[%s] Applying Application error", argocdNamespace))
	fmt.Printf("[%s] Step 3: PASSED - Application applied\n", argocdNamespace)

	fmt.Printf("[%s] Step 4: Verifying Application 'app-of-zone'\n", argocdNamespace)
	appName, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "application", "app-of-zone", "-n", argocdNamespace, "-o", "jsonpath={.metadata.name}")
	require.NoError(t, err, fmt.Sprintf("[%s] Application not found", argocdNamespace))
	require.Equal(t, "app-of-zone", appName, fmt.Sprintf("[%s] Application name mismatch", argocdNamespace))
	fmt.Printf("[%s] Step 4: PASSED - Application verified (name)\n", argocdNamespace)

	fmt.Printf("[%s] Step 5: Triggering sync for Application 'app-of-zone'\n", argocdNamespace)
	_, err = terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "patch", "application", "app-of-zone", "-n", argocdNamespace, "--type", "merge", "-p", `{"operation":{"initiatedBy":{"username":"test"},"sync":{"revision":"HEAD"}}}`)
	require.NoError(t, err, fmt.Sprintf("[%s] Force sync Application error", argocdNamespace))

	_, err = retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for Application to sync", argocdNamespace), 30, 10*time.Second, func() (string, error) {
		output, err := terrak8s.RunKubectlAndGetOutputE(t, argocdOptions, "get", "application", "app-of-zone", "-n", argocdNamespace, "-o", "jsonpath={.status.sync.status}")
		if err != nil {
			return "", err
		}
		if output != "Synced" {
			return "", fmt.Errorf("application not synced yet, status: %s", output)
		}
		return output, nil
	})
	require.NoError(t, err, fmt.Sprintf("[%s] Application 'app-of-zone' failed to sync", argocdNamespace))
	fmt.Printf("[%s] Step 5: PASSED - Application synced\n", argocdNamespace)

	for _, zone := range []string{"a", "b"} {

		fmt.Printf("[%s] Step 6-%s: Checking Zone '%s' exists\n", argocdNamespace, zone, zone)
		_, err = retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for zone '%s' to appear", argocdNamespace, zone), 30, 10*time.Second, func() (string, error) {
			name, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "zone.tenancy.entigo.com", zone, "-o", "jsonpath={.metadata.name}")
			if err != nil {
				return "", err
			}
			return name, nil
		})
		require.NoError(t, err, fmt.Sprintf("[%s] Zone '%s' not found", argocdNamespace, zone))
		fmt.Printf("[%s] Step 6-%s: PASSED - Zone '%s' exists\n", argocdNamespace, zone, zone)

		fmt.Printf("[%s] Step 7-%s: Waiting for Zone '%s' to be Synced and Ready\n", argocdNamespace, zone, zone)
		_, err = retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for zone '%s' to be Synced and Ready", argocdNamespace, zone), 30, 10*time.Second, func() (string, error) {
			syncStatus, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "zone.tenancy.entigo.com", zone, "-o", `jsonpath={.status.conditions[?(@.type=="Synced")].status}`)
			if err != nil {
				return "", err
			}
			if syncStatus != "True" {
				return "", fmt.Errorf("zone '%s' not synced yet, condition: %s", zone, syncStatus)
			}
			readyStatus, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "zone.tenancy.entigo.com", zone, "-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`)
			if err != nil {
				return "", err
			}
			if readyStatus != "True" {
				return "", fmt.Errorf("zone '%s' not ready yet, condition: %s", zone, readyStatus)
			}
			return "Synced+Ready", nil
		})
		require.NoError(t, err, fmt.Sprintf("[%s] Zone '%s' failed to become Synced and Ready", argocdNamespace, zone))
		fmt.Printf("[%s] Step 7-%s: PASSED - Zone '%s' is Synced and Ready\n", argocdNamespace, zone, zone)

		fmt.Printf("[%s] Step 8-%s: Checking Zone '%s' has working NodeGroup\n", argocdNamespace, zone, zone)
		_, err = retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for zone '%s' NodeGroup to be ready", argocdNamespace, zone), 30, 10*time.Second, func() (string, error) {
			count, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "nodegroup.eks.aws.upbound.io", "-l", fmt.Sprintf("crossplane.io/composite=%s", zone), "-o", "jsonpath={.items[*].metadata.name}")
			if err != nil {
				return "", err
			}
			if count == "" {
				return "", fmt.Errorf("zone '%s' has no NodeGroups", zone)
			}
			readyStatus, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "nodegroup.eks.aws.upbound.io", "-l", fmt.Sprintf("crossplane.io/composite=%s", zone), "-o", `jsonpath={.items[0].status.conditions[?(@.type=="Ready")].status}`)
			if err != nil {
				return "", err
			}
			if readyStatus != "True" {
				return "", fmt.Errorf("zone '%s' NodeGroup not ready yet, condition: %s", zone, readyStatus)
			}
			return readyStatus, nil
		})
		require.NoError(t, err, fmt.Sprintf("[%s] Zone '%s' NodeGroup not ready", argocdNamespace, zone))
		fmt.Printf("[%s] Step 8-%s: PASSED - Zone '%s' NodeGroup is Ready\n", argocdNamespace, zone, zone)
	}

	fmt.Printf("[%s] Step 9: Creating namespace 'test-namespace'\n", argocdNamespace)
	_, err = terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "create", "namespace", "test-namespace")
	if err != nil {
		_, err = terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "namespace", "test-namespace")
		require.NoError(t, err, fmt.Sprintf("[%s] Namespace 'test-namespace' not found", argocdNamespace))
	}
	fmt.Printf("[%s] Step 9: PASSED - Namespace created\n", argocdNamespace)

	fmt.Printf("[%s] Step 10: Verifying namespace 'test-namespace' has label tenancy.entigo.com/zone=a\n", argocdNamespace)
	_, err = retry.DoWithRetryE(t, fmt.Sprintf("[%s] Waiting for namespace label", argocdNamespace), 30, 10*time.Second, func() (string, error) {
		label, err := terrak8s.RunKubectlAndGetOutputE(t, clusterOptions, "get", "namespace", "test-namespace", "-o", "jsonpath={.metadata.labels.tenancy\\.entigo\\.com/zone}")
		if err != nil {
			return "", err
		}
		if label != "a" {
			return "", fmt.Errorf("namespace label tenancy.entigo.com/zone expected 'a', got '%s'", label)
		}
		return label, nil
	})
	require.NoError(t, err, fmt.Sprintf("[%s] Namespace 'test-namespace' label tenancy.entigo.com/zone != 'a'", argocdNamespace))
	fmt.Printf("[%s] Step 10: PASSED - Namespace label verified (tenancy.entigo.com/zone=a)\n", argocdNamespace)
}
