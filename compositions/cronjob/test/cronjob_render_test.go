package render_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane/v2/cmd/crank/render"
	xptest "github.com/entigolabs/platform-apis/test/common/crossplane"
	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCronJobStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "workload"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/cronjob.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/cronjob-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-workload-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering resources")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    []unstructured.Unstructured{envConfig},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "CronJob", 2, "Service", 1, "Secret", 2)

	t.Log("TEST 2: asserting CronJob fields")
	cronJob := xptest.FindResource(t, out1.ComposedResources, "CronJob", "new-cron-job")
	if cronJob != nil {
		assertCronJobFields(t, cronJob.Object)
	}

	t.Log("TEST 3: asserting Service fields")
	service := xptest.FindResource(t, out1.ComposedResources, "Service", "new-cron-job-service")
	if service != nil {
		assertServiceFields(t, service.Object)
	}

	t.Log("TEST 4: asserting Secret names")
	xptest.FindResource(t, out1.ComposedResources, "Secret", "new-cron-job-busybox-secret")
	xptest.FindResource(t, out1.ComposedResources, "Secret", "new-cron-job-init-busybox-secret")

	t.Log("Mocking observed resources")
	observed := xptest.BuildObservedResources(t, out1.ComposedResources, func(kind, apiVersion string) bool {
		return (kind == "CronJob" && apiVersion == "batch/v1") || kind == "Service"
	})

	t.Log("TEST 5: Checking CronJob Readiness")
	out5, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    []unstructured.Unstructured{envConfig},
		ObservedResources: observed,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertReady(t, out5.CompositeResource)
}

func assertCronJobFields(t *testing.T, obj map[string]interface{}) {
	t.Helper()

	assertNestedString(t, obj, "* * */1 * *", "spec", "schedule")
	assertNestedString(t, obj, "Allow", "spec", "concurrencyPolicy")
	assertNestedString(t, obj, "OnFailure", "spec", "jobTemplate", "spec", "template", "spec", "restartPolicy")
	assertNestedString(t, obj, "new-cron-job", "metadata", "labels", "app")
	assertNestedString(t, obj, "new-cron-job", "metadata", "labels", "entigo.com/resource")
	assertNestedString(t, obj, "CronJob", "metadata", "labels", "entigo.com/resource-kind")

	containers, _, _ := unstructured.NestedSlice(obj, "spec", "jobTemplate", "spec", "template", "spec", "containers")
	if len(containers) == 0 {
		t.Error("CronJob has no containers")
	} else {
		assertContainerFields(t, "container[busybox]", containers[0].(map[string]interface{}),
			"busybox", "docker.io/busybox:latest", "new-cron-job-busybox-secret")
	}

	initContainers, _, _ := unstructured.NestedSlice(obj, "spec", "jobTemplate", "spec", "template", "spec", "initContainers")
	if len(initContainers) == 0 {
		t.Error("CronJob has no initContainers")
	} else {
		assertContainerFields(t, "initContainer[init-busybox]", initContainers[0].(map[string]interface{}),
			"init-busybox", "docker.io/busybox:latest", "new-cron-job-init-busybox-secret")
	}
}

func assertContainerFields(t *testing.T, label string, container map[string]interface{}, expectedName, expectedImage, expectedSecretRef string) {
	t.Helper()

	name, _, _ := unstructured.NestedString(container, "name")
	if name != expectedName {
		t.Errorf("%s: expected name %q, got %q", label, expectedName, name)
	}

	image, _, _ := unstructured.NestedString(container, "image")
	if image != expectedImage {
		t.Errorf("%s: expected image %q, got %q", label, expectedImage, image)
	}

	envFound := false
	envSlice, _, _ := unstructured.NestedSlice(container, "env")
	for _, e := range envSlice {
		ev, _ := e.(map[string]interface{})
		if ev["name"] == "NEW_ENV" && ev["value"] == "ENV_VALUE" {
			envFound = true
		}
	}
	if !envFound {
		t.Errorf("%s: env var NEW_ENV=ENV_VALUE not found", label)
	}

	envFrom, _, _ := unstructured.NestedSlice(container, "envFrom")
	if len(envFrom) == 0 {
		t.Errorf("%s: expected envFrom to be non-empty", label)
	} else {
		secretRef, _, _ := unstructured.NestedString(envFrom[0].(map[string]interface{}), "secretRef", "name")
		if secretRef != expectedSecretRef {
			t.Errorf("%s: expected envFrom secretRef.name %q, got %q", label, expectedSecretRef, secretRef)
		}
	}

	assertNestedString(t, container, "250m", "resources", "limits", "cpu")
	assertNestedString(t, container, "128Mi", "resources", "limits", "memory")
	assertNestedString(t, container, "125m", "resources", "requests", "cpu")
	assertNestedString(t, container, "102Mi", "resources", "requests", "memory")
}

func assertServiceFields(t *testing.T, obj map[string]interface{}) {
	t.Helper()

	assertNestedString(t, obj, "new-cron-job", "spec", "selector", "app")

	ports, _, _ := unstructured.NestedSlice(obj, "spec", "ports")
	if len(ports) != 2 {
		t.Errorf("Service: expected 2 ports, got %d", len(ports))
		return
	}

	type expectedPort struct {
		name     string
		port     int64
		protocol string
	}
	expected := []expectedPort{
		{"http-tcp-80", 80, "TCP"},
		{"http-tcp-443", 443, "TCP"},
	}

	for i, ep := range expected {
		p, _ := ports[i].(map[string]interface{})
		if p["name"] != ep.name {
			t.Errorf("Service port[%d]: expected name %q, got %v", i, ep.name, p["name"])
		}
		var portNum int64
		switch v := p["port"].(type) {
		case int64:
			portNum = v
		case float64:
			portNum = int64(v)
		default:
			t.Errorf("Service port[%d]: unexpected port type %T", i, p["port"])
			continue
		}
		if portNum != ep.port {
			t.Errorf("Service port[%d]: expected port %d, got %d", i, ep.port, portNum)
		}
		if p["protocol"] != ep.protocol {
			t.Errorf("Service port[%d]: expected protocol %q, got %v", i, ep.protocol, p["protocol"])
		}
	}
}

func assertNestedString(t *testing.T, obj map[string]interface{}, expected string, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedString(obj, fields...)
	if err != nil {
		t.Errorf("field %v: error: %v", fields, err)
		return
	}
	if !found {
		t.Errorf("field %v: not found", fields)
		return
	}
	if got != expected {
		t.Errorf("field %v: expected %q, got %q", fields, expected, got)
	}
}
