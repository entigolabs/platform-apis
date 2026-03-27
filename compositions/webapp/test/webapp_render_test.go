package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	composition     = "../apis/webapp-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/workload"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	webapp          = "../examples/webapp.yaml"
)

func TestWebAppCrossplaneRender(t *testing.T) {
	t.Logf("Starting workload function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, webapp, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "WebApp", 1)
	crossplane.AssertResourceCount(t, resources, "Deployment", 1)
	crossplane.AssertResourceCount(t, resources, "Service", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)

	t.Log("Validating workload.entigo.com WebApp fields")
	crossplane.AssertFieldValues(t, resources, "WebApp", "workload.entigo.com/v1alpha1", map[string]string{
		"metadata.name":                          "new-web-app",
		"metadata.namespace":                     "default",
		"spec.containers.0.environment.0.name":   "NEW_ENV",
		"spec.containers.0.environment.0.secret": "false",
		"spec.containers.0.environment.0.value":  "ENV_VALUE",
		"spec.containers.0.environment.1.name":   "NEW_SECRET",
		"spec.containers.0.environment.1.secret": "true",
		"spec.containers.0.environment.1.value":  "SECRET_VALUE",
		"spec.containers.0.name":                 "nginx",
		"spec.containers.0.registry":             "1234567890.dkr.ecr.eu-north-1.amazonaws.com",
		"spec.containers.0.repository":           "nginx",
		"spec.containers.0.resources.limits.cpu": "0.25",
		"spec.containers.0.resources.limits.ram": "128",
		"spec.containers.0.services.0.name":      "http-tcp-80",
		"spec.containers.0.services.0.port":      "80",
		"spec.containers.0.services.0.protocol":  "TCP",
		"spec.containers.0.services.1.name":      "http-tcp-443",
		"spec.containers.0.services.1.port":      "443",
		"spec.containers.0.services.1.protocol":  "TCP",
		"spec.containers.0.tag":                  "mainline-alpine",
		"spec.replicas":                          "1",
	})

	t.Log("Validating apps Deployment fields")
	crossplane.AssertFieldValues(t, resources, "Deployment", "apps/v1", map[string]string{
		"metadata.name":                                            "new-web-app",
		"metadata.namespace":                                       "default",
		"metadata.ownerReferences.0.apiVersion":                    "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                          "WebApp",
		"metadata.ownerReferences.0.name":                          "new-web-app",
		"spec.replicas":                                            "1",
		"spec.selector.matchLabels.app":                            "new-web-app",
		"spec.template.spec.containers.0.env.0.name":               "NEW_ENV",
		"spec.template.spec.containers.0.env.0.value":              "ENV_VALUE",
		"spec.template.spec.containers.0.name":                     "nginx",
		"spec.template.spec.containers.0.image":                    "1234567890.dkr.ecr.eu-north-1.amazonaws.com/nginx:mainline-alpine",
		"spec.template.spec.containers.0.envFrom.0.secretRef.name": "new-web-app-nginx-secret",
		"spec.template.spec.containers.0.resources.limits.cpu":     "250m",
		"spec.template.spec.containers.0.resources.limits.memory":  "128Mi",
	})

	t.Log("Validating v1 Service fields")
	crossplane.AssertFieldValues(t, resources, "Service", "v1", map[string]string{
		"metadata.name":                         "new-web-app-service",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "WebApp",
		"metadata.ownerReferences.0.name":       "new-web-app",
		"spec.ports.0.port":                     "80",
		"spec.ports.1.port":                     "443",
		"spec.selector.app":                     "new-web-app",
	})

	t.Log("Validating v1 Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         "new-web-app-nginx-secret",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "workload.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "WebApp",
		"metadata.ownerReferences.0.name":       "new-web-app",
		"data.NEW_SECRET":                       "*",
	})

	t.Log("Mocking observed resources")
	mockedDeploy := crossplane.MockByKind(t, resources, "Deployment", "apps/v1", false, map[string]interface{}{
		"status.readyReplicas":   float64(1),
		"status.replicas":        float64(1),
		"status.updatedReplicas": float64(1),
		"status.conditions": []interface{}{
			map[string]interface{}{"type": "Synced", "status": "True"},
			map[string]interface{}{"type": "Available", "status": "True"},
		},
	})
	mockedService := crossplane.MockByKind(t, resources, "Service", "v1", true, nil)
	mockedSecret := crossplane.MockByKind(t, resources, "Secret", "v1", true, nil)
	crossplane.AppendToResources(t, observed, mockedDeploy, mockedService, mockedSecret)

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, webapp, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting workload.entigo.com WebApp Ready Status")
	crossplane.AssertResourceReady(t, resources, "WebApp", "workload.entigo.com/v1alpha1")
}
