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
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"web-app-service": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Service","metadata":{"creationTimestamp":null,"name":"web-app-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"web-app"}},"status":{"loadBalancer":{}}}
							`)},
							"web-app-nginx-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"web-app-nginx-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app-init-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"web-app-init-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app": {Resource: resource.MustStructJSON(`
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"creationTimestamp":null,"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp"},"name":"web-app","namespace":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web-app"}},"strategy":{},"template":{"metadata":{"creationTimestamp":null,"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-nginx-secret"}}],"image":"docker.io/nginx:latest","livenessProbe":{"failureThreshold":3,"httpGet":{"path":"/","port":"http-tcp-80"},"periodSeconds":10,"successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},"name":"nginx","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}]}}},"status":{}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
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
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"web-app-nginx-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"web-app-nginx-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app-init-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"web-app-init-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app": {Resource: resource.MustStructJSON(`
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"creationTimestamp":null,"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp"},"name":"web-app","namespace":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web-app"}},"strategy":{},"template":{"metadata":{"creationTimestamp":null,"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-nginx-secret"}}],"image":"docker.io/nginx:latest","livenessProbe":{"failureThreshold":3,"httpGet":{"path":"/","port":"http-tcp-80"},"periodSeconds":10,"successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},"name":"nginx","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}]}}},"status":{}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
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
							"web-app-service": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Service","metadata":{"creationTimestamp":null,"name":"web-app-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"web-app"}},"status":{"loadBalancer":{}}}
							`)},
							"web-app-nginx-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"web-app-nginx-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app-init-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"web-app-init-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"web-app": {Resource: resource.MustStructJSON(`
{"apiVersion":"apps/v1","kind":"Deployment","metadata":{"creationTimestamp":null,"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp"},"name":"web-app","namespace":"test"},"spec":{"replicas":1,"selector":{"matchLabels":{"app":"web-app"}},"strategy":{},"template":{"metadata":{"creationTimestamp":null,"labels":{"app":"web-app","entigo.com/resource":"web-app","entigo.com/resource-kind":"WebApp"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-nginx-secret"}}],"image":"docker.io/nginx:latest","livenessProbe":{"failureThreshold":3,"httpGet":{"path":"/","port":"http-tcp-80"},"periodSeconds":10,"successThreshold":1,"terminationGracePeriodSeconds":30,"timeoutSeconds":1},"name":"nginx","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"imagePullSecrets":[{"name":"regcred"}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"web-app-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}]}}},"status":{}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
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
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData(environmentName, environmentData),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Ttl: durationpb.New(response.DefaultTTL)},
					Desired: &fnv1.State{
						Resources: map[string]*fnv1.Resource{
							"cron-job-service": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Service","metadata":{"creationTimestamp":null,"name":"cron-job-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"cron-job"}},"status":{"loadBalancer":{}}}
							`)},
							"cron-job-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"cron-job-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"cron-job-init-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"cron-job-init-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"cron-job": {Resource: resource.MustStructJSON(`
{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"creationTimestamp":null,"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob"},"name":"cron-job","namespace":"test"},"spec":{"concurrencyPolicy":"Allow","jobTemplate":{"metadata":{"creationTimestamp":null},"spec":{"template":{"metadata":{"creationTimestamp":null,"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"restartPolicy":"OnFailure"}}}},"schedule":"* * */1 * *"},"status":{}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
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
							"cron-job-service": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","kind":"Service","metadata":{"creationTimestamp":null,"name":"cron-job-service","namespace":"test"},"spec":{"ports":[{"name":"http-tcp-80","port":80,"protocol":"TCP","targetPort":80},{"name":"http-tcp-443","port":443,"protocol":"TCP","targetPort":443}],"selector":{"app":"cron-job"}},"status":{"loadBalancer":{}}}
							`)},
							"cron-job-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"cron-job-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"cron-job-init-busybox-secret": {Resource: resource.MustStructJSON(`
{"apiVersion":"v1","data":{"NEW_SECRET":"U0VDUkVUX1ZBTFVF"},"kind":"Secret","metadata":{"creationTimestamp":null,"name":"cron-job-init-busybox-secret","namespace":"test"},"type":"Opaque"}
							`)},
							"cron-job": {Resource: resource.MustStructJSON(`
{"apiVersion":"batch/v1","kind":"CronJob","metadata":{"creationTimestamp":null,"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob"},"name":"cron-job","namespace":"test"},"spec":{"concurrencyPolicy":"Allow","jobTemplate":{"metadata":{"creationTimestamp":null},"spec":{"template":{"metadata":{"creationTimestamp":null,"labels":{"app":"cron-job","entigo.com/resource":"cron-job","entigo.com/resource-kind":"CronJob"}},"spec":{"containers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"imagePullSecrets":[{"name":"regcred"}],"initContainers":[{"env":[{"name":"NEW_ENV","value":"ENV_VALUE"}],"envFrom":[{"secretRef":{"name":"cron-job-init-busybox-secret"}}],"image":"docker.io/busybox:latest","name":"init-busybox","resources":{"limits":{"cpu":"250m","memory":"128Mi"},"requests":{"cpu":"125m","memory":"102Mi"}}}],"restartPolicy":"OnFailure"}}}},"schedule":"* * */1 * *"},"status":{}}
							`)},
						},
					},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
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
