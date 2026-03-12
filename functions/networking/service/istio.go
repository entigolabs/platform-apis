package service

import (
	"fmt"
	"maps"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	networkingv1 "istio.io/api/networking/v1"
	"istio.io/client-go/pkg/apis/networking/v1alpha3"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const apiVersion = "networking.istio.io/v1"

type istioGenerator struct {
	env       apis.Environment
	webAccess v1alpha1.WebAccess
}

// TODO Ingress type from where? Can tenants have only 1 ingress per type?
// TODO use domain as a unique name suffix?
func GenerateIstioObjects(webAccess v1alpha1.WebAccess, required map[string][]resource.Required) (map[string]client.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}
	generator := &istioGenerator{
		env:       env,
		webAccess: webAccess,
	}
	return generator.generate()
}

func (g *istioGenerator) generate() (map[string]client.Object, error) {
	objs := make(map[string]client.Object)
	entries := g.getServiceEntries()
	maps.Copy(objs, entries)
	rules := g.getDestinationRules()
	maps.Copy(objs, rules)
	objs[g.webAccess.Name] = g.getVirtualService()
	return objs, nil
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func (g *istioGenerator) getServiceEntries() map[string]client.Object {
	entries := make(map[string]client.Object)
	for host, paths := range g.getHostPaths() {
		name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", g.webAccess.Name, host))
		entries[name] = &v1alpha3.ServiceEntry{
			TypeMeta: v1.TypeMeta{
				Kind:       "ServiceEntry",
				APIVersion: apiVersion,
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: g.webAccess.Namespace,
			},
			Spec: getServiceEntrySpec(host, paths),
		}
	}
	return entries
}

func getServiceEntrySpec(host string, paths []v1alpha1.Path) networkingv1.ServiceEntry {
	ports := make([]*networkingv1.ServicePort, 0)
	for _, path := range paths {
		ports = append(ports, getServicePort(path))
	}
	return networkingv1.ServiceEntry{
		Hosts:      []string{host},
		Ports:      ports,
		Resolution: networkingv1.ServiceEntry_DNS,
		Location:   networkingv1.ServiceEntry_MESH_EXTERNAL,
	}
}

func getServicePort(path v1alpha1.Path) *networkingv1.ServicePort {
	return &networkingv1.ServicePort{
		Number:   path.Port,
		Name:     fmt.Sprintf("HTTPS-%d", path.Port),
		Protocol: "HTTPS",
	}
}

func (g *istioGenerator) getDestinationRules() map[string]client.Object {
	rules := make(map[string]client.Object)
	for host := range g.getHostPaths() {
		name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s-dr", g.webAccess.Name, host))
		rules[name] = &v1alpha3.DestinationRule{
			TypeMeta: v1.TypeMeta{
				Kind:       "DestinationRule",
				APIVersion: apiVersion,
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: g.webAccess.Namespace,
			},
			Spec: networkingv1.DestinationRule{
				Host: host,
			},
		}
	}
	return rules
}

func (g *istioGenerator) getVirtualService() client.Object {
	return &v1alpha3.VirtualService{
		TypeMeta: v1.TypeMeta{
			Kind:       "VirtualService",
			APIVersion: apiVersion,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      g.webAccess.Name,
			Namespace: g.webAccess.Namespace,
			Labels: map[string]string{
				"version": "master",
			},
		},
		Spec: g.getVirtualServiceSpec(),
	}
}

func (g *istioGenerator) getVirtualServiceSpec() networkingv1.VirtualService {
	hosts := g.getHosts()
	hosts = append(hosts, g.webAccess.Spec.Domain)
	hosts = append(hosts, g.webAccess.Spec.Aliases...)
	return networkingv1.VirtualService{
		Hosts:    hosts,
		Http:     g.getVirtualServiceHttp(),
		Gateways: []string{g.env.IstioGateway}, // TODO Gateways by type?
	}
}

func (g *istioGenerator) getVirtualServiceHttp() []*networkingv1.HTTPRoute {
	routes := make([]*networkingv1.HTTPRoute, 0)
	for _, path := range g.webAccess.Spec.Paths {
		routes = append(routes, &networkingv1.HTTPRoute{
			Match: []*networkingv1.HTTPMatchRequest{{Uri: getVirtualServiceHttpMatchUri(path)}},
			Route: []*networkingv1.HTTPRouteDestination{
				{
					Destination: &networkingv1.Destination{
						Host: path.Host,
						Port: &networkingv1.PortSelector{Number: path.Port},
					},
				},
			},
			Rewrite: getVirtualServiceRewrite(path),
		})
	}
	return routes
}

func getVirtualServiceHttpMatchUri(path v1alpha1.Path) *networkingv1.StringMatch {
	uri := &networkingv1.StringMatch{}
	if path.PathType == "Exact" {
		uri.MatchType = &networkingv1.StringMatch_Exact{
			Exact: path.Path,
		}
	} else {
		uri.MatchType = &networkingv1.StringMatch_Prefix{
			Prefix: path.Path,
		}
	}
	return uri
}

func getVirtualServiceRewrite(path v1alpha1.Path) *networkingv1.HTTPRewrite {
	if path.TargetPath == "" {
		return nil
	}
	return &networkingv1.HTTPRewrite{
		Uri:       path.TargetPath,
		Authority: path.Host,
	}
}

func (g *istioGenerator) getHostPaths() map[string][]v1alpha1.Path {
	paths := make(map[string][]v1alpha1.Path)
	for _, path := range g.webAccess.Spec.Paths {
		namespace := g.webAccess.Namespace
		if path.Namespace != "" {
			namespace = path.Namespace
		}
		host := fmt.Sprintf("%s.%s.svc.cluster.local", path.Host, namespace)
		paths[host] = append(paths[host], path)
	}
	return paths
}

func (g *istioGenerator) getHosts() []string {
	uniques := make(map[string]interface{})
	hosts := make([]string, 0)
	for _, path := range g.webAccess.Spec.Paths {
		if _, ok := uniques[path.Host]; !ok {
			uniques[path.Host] = nil
			hosts = append(hosts, path.Host)
		}
	}
	return hosts
}
