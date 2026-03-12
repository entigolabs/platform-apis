package service

import (
	"maps"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type cronJobGenerator struct {
	workloadGenerator[*v1alpha1.CronJob]
}

func GenerateCronJobObjects(cronJob v1alpha1.CronJob, labels map[string]string) map[string]client.Object {
	generator := cronJobGenerator{
		workloadGenerator: workloadGenerator[*v1alpha1.CronJob]{
			labels:   labels,
			workload: &cronJob,
		},
	}
	return generator.generate()
}

func (g *cronJobGenerator) generate() map[string]client.Object {
	objs := make(map[string]client.Object)
	secrets := g.getSecrets()
	maps.Copy(objs, secrets)
	objs[g.workload.Name] = g.getCronJob()
	name, service := g.getServices()
	if service != nil {
		objs[name] = service
	}
	return objs
}

func (g *cronJobGenerator) getCronJob() client.Object {
	containers := g.getContainers()
	initContainers := g.getInitContainers()
	imagePullSecretsRefs := g.getImagePullSecrets()
	labels := map[string]string{
		base.AppLabel:          g.workload.GetName(),
		base.ResourceLabel:     g.workload.GetName(),
		base.ResourceKindLabel: apis.XRKindCronJob,
	}
	maps.Copy(labels, g.labels)

	return &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      g.workload.GetName(),
			Namespace: g.workload.GetNamespace(),
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          g.workload.Spec.Schedule,
			ConcurrencyPolicy: batchv1.ConcurrencyPolicy(g.workload.Spec.ConcurrencyPolicy),
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: labels,
						},
						Spec: corev1.PodSpec{
							RestartPolicy:    corev1.RestartPolicy(g.workload.Spec.RestartPolicy),
							Containers:       containers,
							InitContainers:   initContainers,
							ImagePullSecrets: imagePullSecretsRefs,
						},
					},
				},
			},
		},
	}
}
