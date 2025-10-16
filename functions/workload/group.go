package main

import (
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/resource/composite"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	"github.com/entigolabs/platform-apis/service"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	environmentName = "platform-apis-workload"
)

type GroupImpl struct {
}

var _ base.GroupService = &GroupImpl{}

func (g *GroupImpl) GetResourceHandlers() map[string]base.ResourceHandler {
	return map[string]base.ResourceHandler{
		apis.XRKindWebApp: {
			Instantiate: func() runtime.Object { return &v1alpha1.WebApp{} },
			Generate:    g.generateWebApp,
		},
		apis.XRKindCronJob: {
			Instantiate: func() runtime.Object { return &v1alpha1.CronJob{} },
			Generate:    g.generateCronJob,
		},
	}
}

func (g *GroupImpl) generateWebApp(obj runtime.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	webapp := obj.(*v1alpha1.WebApp)
	err := addWorkloadSpecValues(webapp, required)
	if err != nil {
		return nil, err
	}
	return service.GenerateWebAppObjects(*webapp), nil
}

func (g *GroupImpl) generateCronJob(obj runtime.Object, required map[string][]resource.Required, _ map[resource.Name]resource.ObservedComposed) (map[string]runtime.Object, error) {
	cronjob := obj.(*v1alpha1.CronJob)
	err := addWorkloadSpecValues(cronjob, required)
	if err != nil {
		return nil, err
	}
	return service.GenerateCronJobObjects(*cronjob), nil
}

func addWorkloadSpecValues(workload v1alpha1.Workload, required map[string][]resource.Required) error {
	spec := workload.GetWorkloadSpec()
	env, err := GetEnvironment(required)
	if err != nil {
		return err
	}
	spec.SetCpuRequestMultiplier(env.CPURequestMultiplier)
	spec.SetMemoryRequestMultiplier(env.MemoryRequestMultiplier)
	if len(env.ImagePullSecrets) != 0 {
		spec.SetImagePullSecrets(env.ImagePullSecrets)
	}
	return nil
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func (g *GroupImpl) GetReadyStatus(observed *composed.Unstructured) resource.Ready {
	switch observed.GetKind() {
	case "Deployment":
		return getDeploymentReadyStatus(observed)
	default:
		return ""
	}
}

func getDeploymentReadyStatus(observed *composed.Unstructured) resource.Ready {
	readyReplicas, foundReady, errReady := unstructured.NestedFloat64(observed.Object, "status", "readyReplicas")
	replicas, foundReplicas, errReplicas := unstructured.NestedFloat64(observed.Object, "status", "replicas")
	updatedReplicas, foundUpdated, errUpdated := unstructured.NestedFloat64(observed.Object, "status", "updatedReplicas")
	replicasNotReady := errReady != nil || errReplicas != nil || errUpdated != nil || !foundReady || !foundReplicas ||
		!foundUpdated || readyReplicas != replicas || updatedReplicas != replicas
	if replicasNotReady {
		return resource.ReadyFalse
	}
	conditions, foundCond, errCond := unstructured.NestedSlice(observed.Object, "status", "conditions")
	if errCond == nil && foundCond {
		for _, c := range conditions {
			cond, ok := c.(map[string]interface{})
			if !ok {
				continue
			}
			if cond["type"] == "Available" && cond["status"] == "True" {
				return resource.ReadyTrue
			}
		}
	}
	return resource.ReadyFalse
}

func (g *GroupImpl) GetRequiredResources(compositeResource *composite.Unstructured, _ map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	switch compositeResource.GetKind() {
	case apis.XRKindWebApp, apis.XRKindCronJob:
		return map[string]*fnv1.ResourceSelector{
			base.EnvironmentKey: base.RequiredEnvironmentConfig(environmentName),
		}, nil
	default:
		return nil, nil
	}
}

func (g *GroupImpl) GetObservedStatus(_ *composed.Unstructured) (map[string]interface{}, error) {
	return nil, nil
}

func (g *GroupImpl) AddStatusConditions(_ *composite.Unstructured, _ map[resource.Name]resource.ObservedComposed) {
	// No-op
}
