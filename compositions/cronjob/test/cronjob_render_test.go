package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	composition     = "../apis/cronjob-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/workload"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	testResource    = "../examples/cronjob.yaml"
)

func TestCronJobCrossplaneRender(t *testing.T) {
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	t.Logf("Starting workload function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, testResource, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "CronJob", 2)
	crossplane.AssertResourceCount(t, resources, "Service", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 2)

	t.Log("Validating workload.entigo.com CronJob fields")
	crossplane.AssertFieldValues(t, resources, "CronJob", "workload.entigo.com/v1alpha1", map[string]string{
		"metadata.name":      "new-cron-job",
		"metadata.namespace": "default",
	})

	t.Log("Validating batch CronJob fields")
	crossplane.AssertFieldValues(t, resources, "CronJob", "batch/v1", map[string]string{
		"metadata.name":                                         "new-cron-job",
		"metadata.namespace":                                    "default",
		"metadata.ownerReferences.0.apiVersion":                 "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                       "CronJob",
		"metadata.ownerReferences.0.name":                       "new-cron-job",
		"spec.jobTemplate.spec.template.spec.containers.0.name": "busybox",
	})

	t.Log("Validating v1 Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         "new-cron-job-busybox-secret",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "CronJob",
		"metadata.ownerReferences.0.name":       "new-cron-job",
		"data.NEW_SECRET":                       "*",
	})

	t.Log("Validating v1 Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         "new-cron-job-init-busybox-secret",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "CronJob",
		"metadata.ownerReferences.0.name":       "new-cron-job",
		"data.NEW_SECRET":                       "*",
	})

	t.Log("Validating v1 Service fields")
	crossplane.AssertFieldValues(t, resources, "Service", "v1", map[string]string{
		"metadata.name":                         "new-cron-job-service",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "CronJob",
		"metadata.ownerReferences.0.name":       "new-cron-job",
		"spec.ports.0.port":                     "80",
		"spec.ports.1.port":                     "443",
		"spec.selector.app":                     "new-cron-job",
	})

	t.Log("Mocking observed resources")
	mockedBatchCronJob := crossplane.MockByKind(t, resources, "CronJob", "batch/v1", true, nil)
	mockedService := crossplane.MockByKind(t, resources, "Service", "v1", true, nil)
	crossplane.AppendToResources(t, observed, mockedBatchCronJob)
	crossplane.AppendToResources(t, observed, mockedService)
	for _, res := range resources {
		if res.GetKind() == "Secret" {
			crossplane.AppendToResources(t, observed, res)
		}
	}

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, testResource, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting workload.entigo.com CronJob Ready Status")
	crossplane.AssertResourceReady(t, resources, "CronJob", "workload.entigo.com/v1alpha1")
}
