package service

import (
	"fmt"
	"math"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type workloadGenerator[T v1alpha1.Workload] struct {
	labels   map[string]string
	workload T
}

func (g *workloadGenerator[T]) getSecrets() map[string]client.Object {
	spec := g.workload.GetWorkloadSpec()
	secrets := make(map[string]client.Object)
	for _, container := range spec.Containers {
		g.addPodContainerSecret(secrets, &container)
	}
	if spec.InitContainers == nil {
		return secrets
	}
	for _, container := range spec.InitContainers {
		g.addPodContainerSecret(secrets, &container)
	}
	return secrets
}

func (g *workloadGenerator[T]) addPodContainerSecret(secrets map[string]client.Object, container v1alpha1.PodContainer) {
	secretValues := make(map[string][]byte)
	for _, envVar := range container.GetEnvironment() {
		if !envVar.Secret {
			continue
		}
		secretValues[envVar.Name] = []byte(envVar.Value)
	}
	if len(secretValues) == 0 {
		return
	}
	name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s-secret", g.workload.GetName(),
		container.GetName()))
	secrets[name] = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: g.workload.GetNamespace(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretValues,
	}
}

func (g *workloadGenerator[T]) getImagePullSecrets() []corev1.LocalObjectReference {
	spec := g.workload.GetWorkloadSpec()
	if spec.ImagePullSecrets == nil {
		return nil
	}
	var imagePullSecretsRefs []corev1.LocalObjectReference
	for _, name := range spec.ImagePullSecrets {
		imagePullSecretsRefs = append(imagePullSecretsRefs, corev1.LocalObjectReference{Name: name})
	}
	return imagePullSecretsRefs
}

func (g *workloadGenerator[T]) getNodeSelector() map[string]string {
	spec := g.workload.GetWorkloadSpec()
	if spec.Architecture == "" {
		return nil
	}
	return map[string]string{
		"beta.kubernetes.io/arch": spec.Architecture,
	}
}

func (g *workloadGenerator[T]) getContainers() []corev1.Container {
	spec := g.workload.GetWorkloadSpec()
	var k8sContainers []corev1.Container
	for _, container := range spec.Containers {
		k8sContainers = append(k8sContainers, g.getContainer(&container))
	}
	return k8sContainers
}

func (g *workloadGenerator[T]) getInitContainers() []corev1.Container {
	spec := g.workload.GetWorkloadSpec()
	if spec.InitContainers == nil {
		return nil
	}
	var k8sContainers []corev1.Container
	for _, container := range spec.InitContainers {
		k8sContainers = append(k8sContainers, g.getContainer(&container))
	}
	return k8sContainers
}

func (g *workloadGenerator[T]) getContainer(container v1alpha1.PodContainer) corev1.Container {
	env, envFrom := getContainerEnv(g.workload.GetName(), container)
	return corev1.Container{
		Name:           container.GetName(),
		Image:          container.GetRegistry() + "/" + container.GetRepository() + ":" + container.GetTag(),
		Env:            env,
		EnvFrom:        envFrom,
		Command:        container.GetCommand(),
		Resources:      g.getContainerResources(container),
		LivenessProbe:  getProbe(container.GetLivenessProbe()),
		ReadinessProbe: getProbe(container.GetReadinessProbe()),
		StartupProbe:   getProbe(container.GetStartupProbe()),
	}
}

func getContainerEnv(workloadName string, container v1alpha1.PodContainer) ([]corev1.EnvVar, []corev1.EnvFromSource) {
	var env []corev1.EnvVar
	var envFrom []corev1.EnvFromSource
	for _, envVar := range container.GetEnvironment() {
		if !envVar.Secret {
			env = append(env, corev1.EnvVar{
				Name:  envVar.Name,
				Value: envVar.Value,
			})
			continue
		}
		if len(envFrom) > 0 {
			continue
		}
		envFrom = append(envFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s-secret",
						workloadName, container.GetName())),
				},
			},
		})
	}
	return env, envFrom
}

func (g *workloadGenerator[T]) getContainerResources(container v1alpha1.PodContainer) corev1.ResourceRequirements {
	spec := g.workload.GetWorkloadSpec()
	if container.GetResources().Limits.CPU == 0 && container.GetResources().Limits.RAM == 0 {
		return corev1.ResourceRequirements{}
	}
	resources := corev1.ResourceRequirements{
		Limits:   make(corev1.ResourceList),
		Requests: make(corev1.ResourceList),
	}
	cpuLimit := parseLimit(container.GetResources().Limits.CPU, "")
	if cpuLimit != nil {
		resources.Limits[corev1.ResourceCPU] = *cpuLimit
		cpuRequestLimit := RoundFloat32(container.GetResources().Limits.CPU*spec.CpuRequestMultiplier, 3)
		resources.Requests[corev1.ResourceCPU] = *parseLimit(cpuRequestLimit, "")
	}
	ramLimit := parseLimit(container.GetResources().Limits.RAM, "Mi")
	if ramLimit != nil {
		resources.Limits[corev1.ResourceMemory] = *ramLimit
		memoryRequestLimit := RoundFloat32(container.GetResources().Limits.RAM*spec.MemoryRequestMultiplier, 0)
		resources.Requests[corev1.ResourceMemory] = *parseLimit(memoryRequestLimit, "Mi")
	}
	return resources
}

func RoundFloat32(f float32, precision int) float32 {
	p := math.Pow(10, float64(precision))
	return float32(math.Round(float64(f)*p) / p)
}

func parseLimit(limit float32, unit string) *resource.Quantity {
	if limit <= 0 {
		return nil
	}
	quantity, err := resource.ParseQuantity(fmt.Sprintf("%f%s", limit, unit))
	if err != nil {
		fmt.Printf("Failed to parse limit %f, error %v", limit, err)
		return nil
	}
	return &quantity
}

func getProbe(probe v1alpha1.Probe) *corev1.Probe {
	if probe.Path == "" {
		return nil
	}
	var termGracePeriodSeconds *int64 = nil
	if probe.TerminationGracePeriodSeconds != 0 {
		seconds := int64(probe.TerminationGracePeriodSeconds)
		termGracePeriodSeconds = &seconds
	}
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: probe.Path,
				Port: intstr.FromString(probe.Port),
			},
		},
		InitialDelaySeconds:           probe.InitialDelaySeconds,
		PeriodSeconds:                 probe.PeriodSeconds,
		TimeoutSeconds:                probe.TimeoutSeconds,
		SuccessThreshold:              probe.SuccessThreshold,
		FailureThreshold:              probe.FailureThreshold,
		TerminationGracePeriodSeconds: termGracePeriodSeconds,
	}
}

func (g *workloadGenerator[T]) getServices() (string, client.Object) {
	var ports []corev1.ServicePort
	for _, container := range g.workload.GetWorkloadSpec().Containers {
		for _, service := range container.Services {
			ports = append(ports, corev1.ServicePort{
				Name:       service.Name,
				Protocol:   corev1.Protocol(service.Protocol),
				Port:       service.Port,
				TargetPort: intstr.FromInt32(service.Port),
			})
		}
	}
	if (len(ports)) == 0 {
		return "", nil
	}
	name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-service", g.workload.GetName()))
	return name, &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: g.workload.GetNamespace(),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": base.GenerateEligibleKubernetesLabelName(g.workload.GetName()),
			},
			Ports: ports,
		},
	}
}
