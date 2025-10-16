package service

import (
	"maps"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func GenerateCronJobObjects(cronJob v1alpha1.CronJob) map[string]runtime.Object {
	objs := make(map[string]runtime.Object)
	secrets := getSecrets(&cronJob)
	maps.Copy(objs, secrets)
	objs[cronJob.Name] = getCronJob(cronJob)
	name, service := getServices(&cronJob)
	if service != nil {
		objs[name] = service
	}
	return objs
}

func getCronJob(cronJob v1alpha1.CronJob) runtime.Object {
	containers := getContainers(cronJob.Name, cronJob.Spec.WorkloadSpec)
	initContainers := getInitContainers(cronJob.Name, cronJob.Spec.WorkloadSpec)
	imagePullSecretsRefs := getImagePullSecrets(cronJob.Spec.WorkloadSpec)

	return &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cronJob.Name,
			Namespace: cronJob.Namespace,
			Labels: map[string]string{
				base.AppLabel:          cronJob.Name,
				base.ResourceLabel:     cronJob.Name,
				base.ResourceKindLabel: apis.XRKindCronJob,
			},
		},
		Spec: batchv1.CronJobSpec{
			Schedule:          cronJob.Spec.Schedule,
			ConcurrencyPolicy: batchv1.ConcurrencyPolicy(cronJob.Spec.ConcurrencyPolicy),
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								base.AppLabel:          cronJob.Name,
								base.ResourceLabel:     cronJob.Name,
								base.ResourceKindLabel: apis.XRKindCronJob,
							},
						},
						Spec: corev1.PodSpec{
							RestartPolicy:    corev1.RestartPolicy(cronJob.Spec.RestartPolicy),
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
