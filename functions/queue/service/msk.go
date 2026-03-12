package service

import (
	"encoding/json"
	"fmt"
	"strings"

	kafkanv1alpha1 "github.com/crossplane-contrib/provider-kafka/apis/namespaced/v1alpha1"
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type mskInstanceGenerator struct {
	mskInstance  v1alpha1.MSKInstance
	observed     map[resource.Name]resource.ObservedComposed
	hash         string
	names        resourceNames
	readinessMap map[resource.Name]bool
}

type resourceNames struct {
	mskCluster, configSecret, clusterProviderConfig resource.Name
}

const (
	mskApiVersion = "kafka.aws.upbound.io/v1beta3"
)

func GenerateMskInstanceObjects(
	mskInstance v1alpha1.MSKInstance,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	g, err := newMskInstanceGenerator(mskInstance, required, observed)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func GetEnvironment(required map[string][]resource.Required) (apis.Environment, error) {
	var env apis.Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

func newMskInstanceGenerator(
	mskInstance v1alpha1.MSKInstance,
	_ map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*mskInstanceGenerator, error) {
	g := &mskInstanceGenerator{
		mskInstance: mskInstance,
		observed:    observed,
		hash:        base.GenerateFNVHash(mskInstance.UID),
	}

	g.generateNames()

	g.readinessMap = make(map[resource.Name]bool)
	for name, obs := range observed {
		g.readinessMap[name] = isResourceReady(obs.Resource)
	}

	return g, nil
}

func (g *mskInstanceGenerator) generate() (map[string]runtime.Object, error) {
	desired := make(map[string]runtime.Object)

	cluster, err := g.buildMskCluster()
	if err != nil {
		return nil, err
	}
	desired[string(g.names.mskCluster)] = cluster

	observedCluster, ok := g.observed[g.names.mskCluster]
	if !ok || observedCluster.Resource == nil {
		return desired, nil
	}

	brokers, found, err := unstructured.NestedString(observedCluster.Resource.Object, "status", "atProvider", "bootstrapBrokersSaslIam")
	if err != nil || !found || brokers == "" {
		return desired, nil
	}

	secret, err := g.buildKafkaSecret(brokers)
	if err != nil {
		return nil, err
	}
	desired[string(g.names.configSecret)] = secret

	providerConfig, err := g.buildClusterProviderConfig()
	if err != nil {
		return nil, err
	}
	desired[string(g.names.clusterProviderConfig)] = providerConfig

	return desired, nil
}

func (g *mskInstanceGenerator) buildMskCluster() (runtime.Object, error) {
	arn := g.mskInstance.Spec.ClusterARN
	parts := strings.Split(arn, ":")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid cluster ARN: %s", arn)
	}
	region := parts[3]

	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": mskApiVersion,
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name": string(g.names.mskCluster),
				"annotations": map[string]interface{}{
					"crossplane.io/external-name": arn,
				},
			},
			"spec": map[string]interface{}{
				"managementPolicies": []interface{}{"Observe"},
				"forProvider": map[string]interface{}{
					"region": region,
				},
				"providerConfigRef": map[string]interface{}{
					"name": "crossplane-aws",
				},
			},
		},
	}
	return cluster, nil
}

func (g *mskInstanceGenerator) buildKafkaSecret(brokersStr string) (runtime.Object, error) {
	brokers := strings.Split(brokersStr, ",")

	type Sasl struct {
		Mechanism string `json:"mechanism"`
	}
	type Credentials struct {
		Brokers       []string `json:"brokers"`
		Sasl          Sasl     `json:"sasl"`
		TlsEnabled    bool     `json:"tlsEnabled"`
		SkipTlsVerify bool     `json:"skipTlsVerify"`
	}

	creds := Credentials{
		Brokers: brokers,
		Sasl: Sasl{
			Mechanism: "AWS-MSK-IAM",
		},
		TlsEnabled:    true,
		SkipTlsVerify: false,
	}

	credsBytes, err := json.Marshal(creds)
	if err != nil {
		return nil, err
	}

	secretName := string(g.names.configSecret)

	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: "crossplane-kafka",
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"credentials": credsBytes,
		},
	}
	return secret, nil
}

func (g *mskInstanceGenerator) buildClusterProviderConfig() (runtime.Object, error) {
	pcName := string(g.names.clusterProviderConfig)
	pc := &kafkanv1alpha1.ClusterProviderConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "kafka.m.crossplane.io/v1alpha1",
			Kind:       "ClusterProviderConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: pcName,
		},
		Spec: kafkanv1alpha1.ProviderConfigSpec{
			Credentials: kafkanv1alpha1.ProviderCredentials{
				Source: xpv1.CredentialsSourceSecret,
				CommonCredentialSelectors: xpv1.CommonCredentialSelectors{
					SecretRef: &xpv1.SecretKeySelector{
						SecretReference: xpv1.SecretReference{
							Name:      string(g.names.configSecret),
							Namespace: "crossplane-kafka",
						},
						Key: "credentials",
					},
				},
			},
		},
	}
	return pc, nil
}

func isResourceReady(observed *composed.Unstructured) bool {
	conditions, found, err := unstructured.NestedSlice(observed.Object, "status", "conditions")
	if err != nil || !found {
		return false
	}
	for _, condition := range conditions {
		conditionMap, ok := condition.(map[string]interface{})
		if !ok {
			continue
		}
		if conditionMap["type"] == "Ready" && conditionMap["status"] == "True" {
			return true
		}
	}
	return false
}

func (g *mskInstanceGenerator) generateNames() {
	g.names.mskCluster = resource.Name(GetMskClusterName(g.mskInstance.Name, g.hash))
	g.names.configSecret = resource.Name(GetConfigSecretName(g.mskInstance.Name, g.hash))
	g.names.clusterProviderConfig = resource.Name(GetClusterProviderConfigName(g.mskInstance.Name, g.hash))
}

func GetMskClusterName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%smsk-cluster%s", instanceName, hash))
}

func GetConfigSecretName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%skafka-config-secret%s", instanceName, hash))
}

func GetClusterProviderConfigName(instanceName string, hash string) string {
	return base.GenerateEligibleKubernetesFullName(fmt.Sprintf("%skafka-cluster-provider-config%s", instanceName, hash))
}
