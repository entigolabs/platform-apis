package test

import (
	"fmt"
	"testing"
	"time"

	terrak8s "github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/gruntwork-io/terratest/modules/retry"
	"github.com/stretchr/testify/require"
)

func waitForResourceCondition(t *testing.T, options *terrak8s.KubectlOptions, kind, name, conditionType, expectedStatus string, optionalRetries ...int) {
	maxRetries := 60
	if len(optionalRetries) > 0 {
		maxRetries = optionalRetries[0]
	}

	msg := fmt.Sprintf("Waiting for %s '%s' to be %s=%s (limit: %d)", kind, name, conditionType, expectedStatus, maxRetries)

	_, err := retry.DoWithRetryE(t, msg, maxRetries, 6*time.Second, func() (string, error) {
		jsonPath := fmt.Sprintf(`jsonpath={.status.conditions[?(@.type=="%s")].status}`, conditionType)
		status, err := terrak8s.RunKubectlAndGetOutputE(t, options, "get", kind, name, "-o", jsonPath)
		if err != nil {
			return "", err
		}
		if status != expectedStatus {
			return "", fmt.Errorf("condition %s is '%s', expected '%s'", conditionType, status, expectedStatus)
		}
		return status, nil
	})
	require.NoError(t, err, fmt.Sprintf("%s '%s' failed to meet condition %s=%s", kind, name, conditionType, expectedStatus))
}

func waitForResourceSyncedAndReady(t *testing.T, options *terrak8s.KubectlOptions, kind, name string, optionalRetries ...int) {
	waitForResourceCondition(t, options, kind, name, "Synced", "True", optionalRetries...)
	waitForResourceCondition(t, options, kind, name, "Ready", "True", optionalRetries...)
}

func waitForResourceHealthyAndInstalled(t *testing.T, options *terrak8s.KubectlOptions, kind, name string, optionalRetries ...int) {
	waitForResourceCondition(t, options, kind, name, "Healthy", "True", optionalRetries...)
	waitForResourceCondition(t, options, kind, name, "Installed", "True", optionalRetries...)
}
