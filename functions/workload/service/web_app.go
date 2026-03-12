package service

import (
	"maps"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type webAppGenerator struct {
	workloadGenerator[*v1alpha1.WebApp]
}

func GenerateWebAppObjects(webApp v1alpha1.WebApp, labels map[string]string) map[string]client.Object {
	generator := webAppGenerator{
		workloadGenerator: workloadGenerator[*v1alpha1.WebApp]{
			labels:   labels,
			workload: &webApp,
		},
	}
	return generator.generate()
}

func (g *webAppGenerator) generate() map[string]client.Object {
	objs := make(map[string]client.Object)
	secrets := g.getSecrets()
	maps.Copy(objs, secrets)
	objs[g.workload.Name] = g.getDeployment()
	name, service := g.getServices()
	if service != nil {
		objs[name] = service
	}
	return objs
}

func (g *webAppGenerator) getDeployment() client.Object {
	var replicas int32 = 1
	if g.workload.Spec.Replicas != 0 {
		replicas = g.workload.Spec.Replicas
	}
	containers := g.getContainers()
	initContainers := g.getInitContainers()
	imagePullSecretsRefs := g.getImagePullSecrets()
	nodeSelector := g.getNodeSelector()
	labels := map[string]string{
		base.AppLabel:          g.workload.GetName(),
		base.ResourceLabel:     g.workload.GetName(),
		base.ResourceKindLabel: apis.XRKindWebApp,
	}
	maps.Copy(labels, g.labels)

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.workload.GetName(),
			Namespace: g.workload.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": g.workload.GetName(),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers:       containers,
					InitContainers:   initContainers,
					ImagePullSecrets: imagePullSecretsRefs,
					NodeSelector:     nodeSelector,
				},
			},
		},
	}
}
