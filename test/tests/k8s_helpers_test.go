package test

import (
	"fmt"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

func waitSyncedAndReady(t *testing.T, opts *terrak8s.KubectlOptions, kind, name string, retries int, interval time.Duration) {
	t.Helper()
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("waiting for %s/%s", kind, name), retries, interval, func() (string, error) {
		for _, condType := range []string{"Synced", "Ready"} {
			status, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, name, "-o",
				fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, condType))
			if err != nil {
				return "", err
			}
			if status != "True" {
				return "", fmt.Errorf("%s/%s: %s=%s", kind, name, condType, status)
			}
		}
		return "Synced+Ready", nil
	})
	require.NoError(t, err)
}

func waitSyncedAndReadyByLabel(t *testing.T, opts *terrak8s.KubectlOptions, kind, composite string, retries int, interval time.Duration) string {
	t.Helper()
	var name string
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("waiting for %s (composite=%s)", kind, composite), retries, interval, func() (string, error) {
		n, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, "-l",
			fmt.Sprintf("crossplane.io/composite=%s", composite), "-o", "jsonpath={.items[0].metadata.name}")
		if err != nil {
			return "", err
		}
		if n == "" {
			return "", fmt.Errorf("no %s found for composite=%s", kind, composite)
		}
		for _, condType := range []string{"Synced", "Ready"} {
			status, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, n, "-o",
				fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, condType))
			if err != nil {
				return "", err
			}
			if status != "True" {
				return "", fmt.Errorf("%s/%s: %s=%s", kind, n, condType, status)
			}
		}
		name = n
		return "Synced+Ready", nil
	})
	require.NoError(t, err)
	return name
}

func waitCrossplanePackageReady(t *testing.T, opts *terrak8s.KubectlOptions, kind, name string) {
	t.Helper()
	_, err := retry.DoWithRetryE(t, fmt.Sprintf("waiting for %s/%s", kind, name), 40, 6*time.Second, func() (string, error) {
		for _, condType := range []string{"Healthy", "Installed"} {
			status, err := terrak8s.RunKubectlAndGetOutputE(t, opts, "get", kind, name, "-o",
				fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, condType))
			if err != nil {
				return "", err
			}
			if status != "True" {
				return "", fmt.Errorf("%s/%s: %s=%s", kind, name, condType, status)
			}
		}
		return "Healthy+Installed", nil
	})
	require.NoError(t, err)
}
