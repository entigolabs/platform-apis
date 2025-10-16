package service

import (
	"fmt"
	"maps"
	"math"

	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func GenerateWebAppObjects(webApp v1alpha1.WebApp) map[string]runtime.Object {
	objs := make(map[string]runtime.Object)
	secrets := getSecrets(&webApp)
	maps.Copy(objs, secrets)
	objs[webApp.Name] = getDeployment(webApp)
	name, service := getServices(&webApp)
	if service != nil {
		objs[name] = service
	}
	return objs
}

func getSecrets(workload v1alpha1.Workload) map[string]runtime.Object {
	spec := workload.GetWorkloadSpec()
	secrets := make(map[string]runtime.Object)
	for _, container := range spec.Containers {
		addPodContainerSecret(secrets, workload, &container)
	}
	if spec.InitContainers == nil {
		return secrets
	}
	for _, container := range spec.InitContainers {
		addPodContainerSecret(secrets, workload, &container)
	}
	return secrets
}

func addPodContainerSecret(secrets map[string]runtime.Object, workload v1alpha1.Workload, container v1alpha1.PodContainer) {
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
	name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-%s-secret", workload.GetName(),
		container.GetName()))
	secrets[name] = &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: workload.GetNamespace(),
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretValues,
	}
}

func getDeployment(webApp v1alpha1.WebApp) runtime.Object {
	var replicas int32 = 1
	if webApp.Spec.Replicas != 0 {
		replicas = webApp.Spec.Replicas
	}
	containers := getContainers(webApp.Name, webApp.Spec.WorkloadSpec)
	initContainers := getInitContainers(webApp.Name, webApp.Spec.WorkloadSpec)
	imagePullSecretsRefs := getImagePullSecrets(webApp.Spec.WorkloadSpec)
	nodeSelector := getNodeSelector(webApp.Spec.WorkloadSpec)

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      webApp.GetName(),
			Namespace: webApp.Namespace,
			Labels: map[string]string{
				base.AppLabel:          webApp.GetName(),
				base.ResourceLabel:     webApp.GetName(),
				base.ResourceKindLabel: apis.XRKindWebApp,
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": webApp.GetName(),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						base.AppLabel:          webApp.GetName(),
						base.ResourceLabel:     webApp.GetName(),
						base.ResourceKindLabel: apis.XRKindWebApp,
					},
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

func getImagePullSecrets(spec v1alpha1.WorkloadSpec) []corev1.LocalObjectReference {
	if spec.ImagePullSecrets == nil {
		return nil
	}
	var imagePullSecretsRefs []corev1.LocalObjectReference
	for _, name := range spec.ImagePullSecrets {
		imagePullSecretsRefs = append(imagePullSecretsRefs, corev1.LocalObjectReference{Name: name})
	}
	return imagePullSecretsRefs
}

func getNodeSelector(spec v1alpha1.WorkloadSpec) map[string]string {
	if spec.Architecture == "" {
		return nil
	}
	return map[string]string{
		"beta.kubernetes.io/arch": spec.Architecture,
	}
}

func getContainers(name string, spec v1alpha1.WorkloadSpec) []corev1.Container {
	var k8sContainers []corev1.Container
	for _, container := range spec.Containers {
		k8sContainers = append(k8sContainers, getContainer(name, spec, &container))
	}
	return k8sContainers
}

func getInitContainers(name string, spec v1alpha1.WorkloadSpec) []corev1.Container {
	if spec.InitContainers == nil {
		return nil
	}
	var k8sContainers []corev1.Container
	for _, container := range spec.InitContainers {
		k8sContainers = append(k8sContainers, getContainer(name, spec, &container))
	}
	return k8sContainers
}

func getContainer(name string, spec v1alpha1.WorkloadSpec, container v1alpha1.PodContainer) corev1.Container {
	env, envFrom := getContainerEnv(name, container)
	return corev1.Container{
		Name:           container.GetName(),
		Image:          container.GetRegistry() + "/" + container.GetRepository() + ":" + container.GetTag(),
		Env:            env,
		EnvFrom:        envFrom,
		Command:        container.GetCommand(),
		Resources:      getContainerResources(spec, container),
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

func getContainerResources(spec v1alpha1.WorkloadSpec, container v1alpha1.PodContainer) corev1.ResourceRequirements {
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

func getServices(workload v1alpha1.Workload) (string, runtime.Object) {
	var ports []corev1.ServicePort
	for _, container := range workload.GetWorkloadSpec().Containers {
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
	name := base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%s-service", workload.GetName()))
	return name, &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Service",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: workload.GetNamespace(),
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": base.GenerateEligibleKubernetesLabelName(workload.GetName()),
			},
			Ports: ports,
		},
	}
}
