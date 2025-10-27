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
	"k8s.io/apimachinery/pkg/runtime"
)

const apiVersion = "networking.istio.io/v1"

// TODO Ingress type from where? Can tenants have only 1 ingress per type?
// TODO use domain as a unique name suffix?
func GenerateIstioObjects(webAccess v1alpha1.WebAccess, required map[string][]resource.Required) (map[string]runtime.Object, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	objs := make(map[string]runtime.Object)
	entries := getServiceEntries(webAccess)
	maps.Copy(objs, entries)
	rules := getDestinationRules(webAccess)
	maps.Copy(objs, rules)
	objs[webAccess.Name] = getVirtualService(webAccess, env.IstioGateway)
	return objs, nil
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func getServiceEntries(webAccess v1alpha1.WebAccess) map[string]runtime.Object {
	entries := make(map[string]runtime.Object)
	for host, paths := range getHostPaths(webAccess) {
		name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s", webAccess.Name, host))
		entries[name] = &v1alpha3.ServiceEntry{
			TypeMeta: v1.TypeMeta{
				Kind:       "ServiceEntry",
				APIVersion: apiVersion,
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: webAccess.Namespace,
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

func getDestinationRules(webAccess v1alpha1.WebAccess) map[string]runtime.Object {
	rules := make(map[string]runtime.Object)
	for host := range getHostPaths(webAccess) {
		name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s-dr", webAccess.Name, host))
		rules[name] = &v1alpha3.DestinationRule{
			TypeMeta: v1.TypeMeta{
				Kind:       "DestinationRule",
				APIVersion: apiVersion,
			},
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: webAccess.Namespace,
			},
			Spec: networkingv1.DestinationRule{
				Host: host,
			},
		}
	}
	return rules
}

func getVirtualService(webAccess v1alpha1.WebAccess, gateway string) runtime.Object {
	return &v1alpha3.VirtualService{
		TypeMeta: v1.TypeMeta{
			Kind:       "VirtualService",
			APIVersion: apiVersion,
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      webAccess.Name,
			Namespace: webAccess.Namespace,
			Labels: map[string]string{
				"version": "master",
			},
		},
		Spec: getVirtualServiceSpec(webAccess.Spec, gateway),
	}
}

func getVirtualServiceSpec(spec v1alpha1.WebAccessSpec, gateway string) networkingv1.VirtualService {
	hosts := getHosts(spec)
	hosts = append(hosts, spec.Domain)
	hosts = append(hosts, spec.Aliases...)
	return networkingv1.VirtualService{
		Hosts:    hosts,
		Http:     getVirtualServiceHttp(spec),
		Gateways: []string{gateway}, // TODO Gateways by type?
	}
}

func getVirtualServiceHttp(spec v1alpha1.WebAccessSpec) []*networkingv1.HTTPRoute {
	routes := make([]*networkingv1.HTTPRoute, 0)
	for _, path := range spec.Paths {
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

func getHostPaths(webAccess v1alpha1.WebAccess) map[string][]v1alpha1.Path {
	paths := make(map[string][]v1alpha1.Path)
	for _, path := range webAccess.Spec.Paths {
		namespace := webAccess.Namespace
		if path.Namespace != "" {
			namespace = path.Namespace
		}
		host := fmt.Sprintf("%s.%s.svc.cluster.local", path.Host, namespace)
		paths[host] = append(paths[host], path)
	}
	return paths
}

func getHosts(spec v1alpha1.WebAccessSpec) []string {
	uniques := make(map[string]interface{})
	hosts := make([]string, 0)
	for _, path := range spec.Paths {
		if _, ok := uniques[path.Host]; !ok {
			uniques[path.Host] = nil
			hosts = append(hosts, path.Host)
		}
	}
	return hosts
}
