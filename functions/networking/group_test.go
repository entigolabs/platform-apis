package main

import (
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"google.golang.org/protobuf/types/known/durationpb"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

const (
	webAccessJson = `{
		"apiVersion": "networking.entigo.com/v1alpha1",
		"kind": "WebAccess",
		"metadata": {"name":"web-access","namespace":"test"},
		"spec": {
			"domain": "example.com",
			"aliases": ["alias1.com","alias2.com"],
			"paths": [
				{"path":"/api/v1","host":"service1","namespace":"test","port":80,"pathType":"Prefix","targetPath":"/v1"},
				{"path":"/api/v2","host":"service2","port":443,"pathType":"Exact","targetPath":"/v2"}]}
	}`
)

func TestWebAccessFunction(t *testing.T) {
	webAccessResource := resource.MustStructJSON(webAccessJson)
	environmentData := map[string]interface{}{
		"istioGateway": "generic-gw-int",
	}
	ns := "test"

	cases := map[string]test.Case{
		"CreateWebAccessObjects": {
			Reason: "The Function should create webaccess objects",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: webAccessResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
						base.NamespaceKey:   test.Namespace(ns, "zone-a"),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"web-access-service1-test-svc-cluster-local": {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"ServiceEntry","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-access-service1-test-svc-cluster-local","namespace":"test"},"spec":{"hosts":["service1.test.svc.cluster.local"],"ports":[{"name":"HTTPS-80","number":80,"protocol":"HTTPS"}],"resolution":"DNS"},"status":{}}
							`)},
							"web-access-service2-test-svc-cluster-local": {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"ServiceEntry","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-access-service2-test-svc-cluster-local","namespace":"test"},"spec":{"hosts":["service2.test.svc.cluster.local"],"ports":[{"name":"HTTPS-443","number":443,"protocol":"HTTPS"}],"resolution":"DNS"},"status":{}}
							`)},
							"web-access-service1-test-svc-cluster-local-dr": {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"DestinationRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-access-service1-test-svc-cluster-local-dr","namespace":"test"},"spec":{"host":"service1.test.svc.cluster.local"},"status":{}}
							`)},
							"web-access-service2-test-svc-cluster-local-dr": {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"DestinationRule","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-access-service2-test-svc-cluster-local-dr","namespace":"test"},"spec":{"host":"service2.test.svc.cluster.local"},"status":{}}
							`)},
							"web-access": {Resource: resource.MustStructJSON(`
{"apiVersion":"networking.istio.io/v1","kind":"VirtualService","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a","version":"master"},"name":"web-access","namespace":"test"},"spec":{"gateways":["generic-gw-int"],"hosts":["service1","service2","example.com","alias1.com","alias2.com"],"http":[{"match":[{"uri":{"prefix":"/api/v1"}}],"rewrite":{"authority":"service1","uri":"/v1"},"route":[{"destination":{"host":"service1","port":{"number":80}}}]},{"match":[{"uri":{"exact":"/api/v2"}}],"rewrite":{"authority":"service2","uri":"/v2"},"route":[{"destination":{"host":"service2","port":{"number":443}}}]}]},"status":{}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
							base.NamespaceKey:   base.RequiredNamespace(ns),
						},
					},
				},
			},
		},
	}

	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases)
}
