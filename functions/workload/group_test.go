package main

import (
	"maps"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/response"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
)

const (
	webAppJson = `{
		"apiVersion": "workload.entigo.com/v1alpha1",
		"kind": "WebApp",
		"metadata": {"name":"web-app","namespace":"test"},
		"spec": {
			"crossplane": {
			  "compositionRef": {
				"name": "webapps.workload.entigo.com"
			  }
			},
			"replicas": 1,
			"restartPolicy": "Always",
			"containers": [{
				"environment": [{"name":"NEW_ENV","secret":false,"value":"ENV_VALUE"},{"name":"NEW_SECRET","secret":true,"value":"SECRET_VALUE"}],
				"livenessProbe": {"failureThreshold":3,"initialDelaySeconds":0,"path":"/","periodSeconds":10,"port":"http-tcp-80","successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},
				"name": "nginx",
				"registry": "docker.io",
				"repository": "nginx",
				"tag": "latest",
				"resources": {"limits":{"cpu":0.25,"ram":128}},
				"services": [{"name":"http-tcp-80","port":80,"protocol":"TCP"},{"name":"http-tcp-443","port":443,"protocol":"TCP"}]
			}],
			"initContainers": [{
				"environment": [{"name":"NEW_ENV","secret":false,"value":"ENV_VALUE"},{"name":"NEW_SECRET","secret":true,"value":"SECRET_VALUE"}],
				"name": "init-busybox",
				"registry": "docker.io",
				"repository": "busybox",
				"tag": "latest",
				"resources": {"limits":{"cpu":0.25,"ram":128}}
			}]
		}
	}`
	cronJobJson = `{
		"apiVersion": "workload.entigo.com/v1alpha1",
		"kind": "CronJob",
		"metadata": {"name":"cron-job","namespace":"test"},
		"spec": {
			"crossplane": {
			  "compositionRef": {
				"name": "cronjobs.workload.entigo.com"
			  }
			},
			"restartPolicy": "OnFailure",
			"concurrencyPolicy": "Allow",
			"schedule": "* * */1 * *",
			"containers":[{
				"environment": [{"name":"NEW_ENV","secret":false,"value":"ENV_VALUE"},{"name":"NEW_SECRET","secret":true,"value":"SECRET_VALUE"}],
				"name":"busybox",
				"registry":"docker.io",
				"repository":"busybox",
				"tag":"latest",
				"resources":{"limits":{"cpu":0.25,"ram":128}},
				"services":[{"name":"http-tcp-80","port":80,"protocol":"TCP"},{"name":"http-tcp-443","port":443,"protocol":"TCP"}]
			}],
			"initContainers": [{
				"environment": [{"name":"NEW_ENV","secret":false,"value":"ENV_VALUE"},{"name":"NEW_SECRET","secret":true,"value":"SECRET_VALUE"}],
				"name": "init-busybox",
				"registry": "docker.io",
				"repository": "busybox",
				"tag": "latest",
				"resources": {"limits":{"cpu":0.25,"ram":128}}
			}]
		}
	}`
	webAppServiceJson              = `{"apiVersion":"v1","kind":"Service","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-app-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"web-app"}},"status":{"loadBalancer":{}}}`
	deploymentJson                 = `{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp","tenancy.entigo.com/zone":"zone-a"},"name":"web-app","namespace":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web-app"}},"strategy":{},"template":{"metadata":{"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp","tenancy.entigo.com/zone":"zone-a"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-nginx-secret"}}],"image":"docker.io/nginx:latest","livenessProbe":{"failureThreshold":3,"httpGet":{"path":"/","port":"http-tcp-80"},"periodSeconds":10,"successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},"name":"nginx","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}]}}},"status":{}}`
	webAppContainerSecretJson      = `{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-app-nginx-secret","namespace":"test"},"type":"Opaque"}`
	webAppInitContainerSecretJson  = `{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"web-app-init-busybox-secret","namespace":"test"},"type":"Opaque"}`
	cronJobServiceJson             = `{"apiVersion":"v1","kind":"Service","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"cron-job-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"cron-job"}},"status":{"loadBalancer":{}}}`
	cronJobContainerSecretJson     = `{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"cron-job-busybox-secret","namespace":"test"},"type":"Opaque"}`
	cronJobInitContainerSecretJson = `{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"labels":{"tenancy.entigo.com/zone":"zone-a"},"name":"cron-job-init-busybox-secret","namespace":"test"},"type":"Opaque"}`
)

func TestWorkloadFunction(t *testing.T) {
	webAppResource := resource.MustStructJSON(webAppJson)
	webAppResourceNoService := getWebAppWithoutService(t)
	cronJobResource := resource.MustStructJSON(cronJobJson)
	environmentData := map[string]interface{}{
		"cpuRequestMultiplier":    float32(0.5),
		"memoryRequestMultiplier": float32(0.8),
	}
	optEnvironmentData := map[string]interface{}{
		"imagePullSecrets": []interface{}{"regcred"},
	}
	maps.Copy(optEnvironmentData, environmentData)
	ns := "test"

	cases := map[string]test.Case{
		"CreateWebAppObjects": {
			Reason: "The Function should create webapp objects",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: webAppResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.ZoneKey: test.ZoneWithMetadata("zone-a", map[string]interface{}{base.TagsPrefix + "foo": "bar"}, nil),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"web-app-service": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Service","metadata":{"labels":{"tags.entigo.com/foo":"bar","tenancy.entigo.com/zone":"zone-a"},"name":"web-app-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"web-app"}},"status":{"loadBalancer":{}}}
							`)},
							"web-app-nginx-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"labels":{"tags.entigo.com/foo":"bar","tenancy.entigo.com/zone":"zone-a"},"name":"web-app-nginx-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app-init-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"labels":{"tags.entigo.com/foo":"bar","tenancy.entigo.com/zone":"zone-a"},"name":"web-app-init-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app": {Resource: resource.MustStructJSON(`
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp","tags.entigo.com/foo":"bar","tenancy.entigo.com/zone":"zone-a"},"name":"web-app","namespace":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web-app"}},"strategy":{},"template":{"metadata":{"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp","tags.entigo.com/foo":"bar","tenancy.entigo.com/zone":"zone-a"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-nginx-secret"}}],"image":"docker.io/nginx:latest","livenessProbe":{"failureThreshold":3,"httpGet":{"path":"/","port":"http-tcp-80"},"periodSeconds":10,"successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},"name":"nginx","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}]}}},"status":{}}
							`)},
						},
					},
				},
			},
		},
		"CreateWebAppObjectsWithoutService": {
			Reason: "The Function should create webapp objects",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: webAppResourceNoService,
						},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"web-app-nginx-secret":        {Resource: resource.MustStructJSON(webAppContainerSecretJson)},
							"web-app-init-busybox-secret": {Resource: resource.MustStructJSON(webAppInitContainerSecretJson)},
							"web-app":                     {Resource: resource.MustStructJSON(deploymentJson)},
						},
					},
				},
			},
		},
		"CreateWebAppObjectsAllEnv": {
			Reason: "The Function should create webapp objects with all environment variables",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: webAppResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"web-app-service":             {Resource: resource.MustStructJSON(webAppServiceJson)},
							"web-app-nginx-secret":        {Resource: resource.MustStructJSON(webAppContainerSecretJson)},
							"web-app-init-busybox-secret": {Resource: resource.MustStructJSON(webAppInitContainerSecretJson)},
							"web-app": {Resource: resource.MustStructJSON(`
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp","tenancy.entigo.com/zone":"zone-a"},"name":"web-app","namespace":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web-app"}},"strategy":{},"template":{"metadata":{"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp","tenancy.entigo.com/zone":"zone-a"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-nginx-secret"}}],"image":"docker.io/nginx:latest","livenessProbe":{"failureThreshold":3,"httpGet":{"path":"/","port":"http-tcp-80"},"periodSeconds":10,"successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},"name":"nginx","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"imagePullSecrets":[{"name":"regcred"}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}]}}},"status":{}}
							`)},
						},
					},
				},
			},
		},
		"CreateCronJobObjects": {
			Reason: "The Function should create cronjob objects",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: cronJobResource,
						},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"cron-job-service":             {Resource: resource.MustStructJSON(cronJobServiceJson)},
							"cron-job-busybox-secret":      {Resource: resource.MustStructJSON(cronJobContainerSecretJson)},
							"cron-job-init-busybox-secret": {Resource: resource.MustStructJSON(cronJobInitContainerSecretJson)},
							"cron-job": {Resource: resource.MustStructJSON(`
{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob","tenancy.entigo.com/zone":"zone-a"},"name":"cron-job","namespace":"test"},"spec":{"concurrencyPolicy":"Allow","jobTemplate":{"metadata":{"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob","tenancy.entigo.com/zone":"zone-a"}},"spec":{"template":{"metadata":{"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob","tenancy.entigo.com/zone":"zone-a"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"restartPolicy":"OnFailure"}}}},"schedule":"* * */1 * *"},"status":{}}
							`)},
						},
					},
				},
			},
		},
		"CreateCronJobObjectsAllEnv": {
			Reason: "The Function should create cronjob objects with all environment variables",
			Args: test.Args{
				Req: &fnv1.RunFunctionRequest{
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: cronJobResource,
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, optEnvironmentData),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"cron-job-service":             {Resource: resource.MustStructJSON(cronJobServiceJson)},
							"cron-job-busybox-secret":      {Resource: resource.MustStructJSON(cronJobContainerSecretJson)},
							"cron-job-init-busybox-secret": {Resource: resource.MustStructJSON(cronJobInitContainerSecretJson)},
							"cron-job": {Resource: resource.MustStructJSON(`
{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob","tenancy.entigo.com/zone":"zone-a"},"name":"cron-job","namespace":"test"},"spec":{"concurrencyPolicy":"Allow","jobTemplate":{"metadata":{"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob","tenancy.entigo.com/zone":"zone-a"}},"spec":{"template":{"metadata":{"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob","tenancy.entigo.com/zone":"zone-a"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"imagePullSecrets":[{"name":"regcred"}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"restartPolicy":"OnFailure"}}}},"schedule":"* * */1 * *"},"status":{}}
							`)},
						},
					},
				},
			},
		},
	}

	test.AddEnvironmentConfig(cases, environmentName, environmentData)
	test.AddZoneResources(cases, ns, "zone-a")
	newService := func() base.GroupService {
		return &GroupImpl{}
	}
	test.RunFunctionCases(t, newService, cases)
}

func getWebAppWithoutService(t *testing.T) *structpb.Struct {
	webAppResource := resource.MustStructJSON(webAppJson)
	spec := webAppResource.Fields["spec"].GetStructValue()
	if spec == nil {
		t.Fatal("webApp json spec is nil")
	}
	containers := spec.Fields["containers"].GetListValue()
	if containers == nil {
		t.Fatal("webApp json spec containers is nil")
	}
	for _, c := range containers.Values {
		if containerStruct := c.GetStructValue(); containerStruct != nil {
			delete(containerStruct.Fields, "services")
		}
	}
	return webAppResource
}
