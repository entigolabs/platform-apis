package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/crossplane-common"
)

const (
	composition     = "../apis/webaccess-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/networking"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	webaccess       = "../examples/webaccess.yaml"
)

func TestWebAccessCrossplaneRender(t *testing.T) {
	t.Logf("Starting networking function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, webaccess, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "WebAccess", 1)
	crossplane.AssertResourceCount(t, resources, "VirtualService", 1)
	crossplane.AssertResourceCount(t, resources, "ServiceEntry", 2)
	crossplane.AssertResourceCount(t, resources, "DestinationRule", 2)

	t.Log("Validating networking.entigo.com WebAccess fields")
	crossplane.AssertFieldValues(t, resources, "WebAccess", "networking.entigo.com/v1alpha1", map[string]string{
		"metadata.name":           "new-web-access",
		"metadata.namespace":      "default",
		"spec.aliases.0":          "alias1.com",
		"spec.aliases.1":          "alias2.com",
		"spec.domain":             "example.com",
		"spec.paths.0.host":       "service1",
		"spec.paths.0.namespace":  "test",
		"spec.paths.0.path":       "/api/v1",
		"spec.paths.0.pathType":   "Prefix",
		"spec.paths.0.port":       "80",
		"spec.paths.0.targetPath": "/v1",
		"spec.paths.1.host":       "service2",
		"spec.paths.1.path":       "/api/v2",
		"spec.paths.1.pathType":   "Exact",
		"spec.paths.1.port":       "443",
		"spec.paths.1.targetPath": "/v2",
	})

	t.Log("Validating networking.istio.io VirtualService fields")
	crossplane.AssertFieldValues(t, resources, "VirtualService", "networking.istio.io/v1", map[string]string{
		"metadata.name":                               "new-web-access",
		"metadata.namespace":                          "default",
		"metadata.ownerReferences.0.apiVersion":       "networking.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":             "WebAccess",
		"metadata.ownerReferences.0.name":             "new-web-access",
		"spec.gateways.0":                             "istio-gateway/istio-gateway",
		"spec.hosts.0":                                "service1",
		"spec.hosts.1":                                "service2",
		"spec.hosts.2":                                "example.com",
		"spec.hosts.3":                                "alias1.com",
		"spec.hosts.4":                                "alias2.com",
		"spec.http.0.match.0.uri.prefix":              "/api/v1",
		"spec.http.0.rewrite.authority":               "service1",
		"spec.http.0.rewrite.uri":                     "/v1",
		"spec.http.0.route.0.destination.host":        "service1",
		"spec.http.0.route.0.destination.port.number": "80",
		"spec.http.1.match.0.uri.exact":               "/api/v2",
		"spec.http.1.rewrite.authority":               "service2",
		"spec.http.1.rewrite.uri":                     "/v2",
		"spec.http.1.route.0.destination.host":        "service2",
		"spec.http.1.route.0.destination.port.number": "443",
	})

	t.Log("Validating networking.istio.io ServiceEntry fields")
	crossplane.AssertFieldValues(t, resources, "ServiceEntry", "networking.istio.io/v1", map[string]string{
		"metadata.name":                         "new-web-access-service1-test-svc-cluster-local",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "networking.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "WebAccess",
		"metadata.ownerReferences.0.name":       "new-web-access",
		"spec.hosts.0":                          "service1.test.svc.cluster.local",
		"spec.ports.0.name":                     "HTTPS-80",
		"spec.ports.0.number":                   "80",
		"spec.ports.0.protocol":                 "HTTPS",
		"spec.resolution":                       "DNS",
	})

	t.Log("Validating networking.istio.io ServiceEntry fields")
	crossplane.AssertFieldValues(t, resources, "ServiceEntry", "networking.istio.io/v1", map[string]string{
		"metadata.name":                         "new-web-access-service2-default-svc-cluster-local",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "networking.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "WebAccess",
		"metadata.ownerReferences.0.name":       "new-web-access",
		"spec.hosts.0":                          "service2.default.svc.cluster.local",
		"spec.ports.0.name":                     "HTTPS-443",
		"spec.ports.0.number":                   "443",
		"spec.ports.0.protocol":                 "HTTPS",
		"spec.resolution":                       "DNS",
	})

	t.Log("Validating networking.istio.io DestinationRule fields")
	crossplane.AssertFieldValues(t, resources, "DestinationRule", "networking.istio.io/v1", map[string]string{
		"metadata.name":                         "new-web-access-service1-test-svc-cluster-local-dr",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "networking.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "WebAccess",
		"metadata.ownerReferences.0.name":       "new-web-access",
		"spec.host":                             "service1.test.svc.cluster.local",
	})

	t.Log("Validating networking.istio.io DestinationRule fields")
	crossplane.AssertFieldValues(t, resources, "DestinationRule", "networking.istio.io/v1", map[string]string{
		"metadata.name":                         "new-web-access-service2-default-svc-cluster-local-dr",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "networking.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "WebAccess",
		"metadata.ownerReferences.0.name":       "new-web-access",
		"spec.host":                             "service2.default.svc.cluster.local",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetAPIVersion() == "networking.istio.io/v1" {
			crossplane.AppendToResources(t, observed, res)
		}
	}

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, webaccess, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting networking.entigo.com WebAccess Ready Status")
	crossplane.AssertResourceReady(t, resources, "WebAccess", "networking.entigo.com/v1alpha1")
}
