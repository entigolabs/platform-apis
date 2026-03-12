package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type kafkaUserGenerator struct {
	user     v1alpha1.KafkaUser
	env      apis.Environment
	msk      v1alpha1.MSKInstance
	kmsKeyID string
	password string
	observed map[resource.Name]resource.ObservedComposed
}

func GenerateKafkaUserObjects(
	user v1alpha1.KafkaUser,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]runtime.Object, error) {
	g, err := newKafkaUserGenerator(user, required, observed)
	if err != nil {
		return nil, err
	}
	return g.generate()
}

func newKafkaUserGenerator(
	user v1alpha1.KafkaUser,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (*kafkaUserGenerator, error) {
	env, err := GetEnvironment(required)
	if err != nil {
		return nil, err
	}

	var msk v1alpha1.MSKInstance
	if err := base.ExtractRequiredResource(required, "MSKObserver", &msk); err != nil {
		return nil, fmt.Errorf("cannot get MSKObserver: %w", err)
	}

	kmsKeyID := ""
	if kmsResources, ok := required["KMSConfigKey"]; ok && len(kmsResources) > 0 {
		keyID, _, _ := unstructured.NestedString(kmsResources[0].Resource.Object, "status", "atProvider", "keyId")
		kmsKeyID = keyID
	}

	password, err := resolvePassword(user.Name, observed)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve password: %w", err)
	}

	return &kafkaUserGenerator{
		user:     user,
		env:      env,
		msk:      msk,
		kmsKeyID: kmsKeyID,
		password: password,
		observed: observed,
	}, nil
}

func resolvePassword(username string, observed map[resource.Name]resource.ObservedComposed) (string, error) {
	secretResourceName := resource.Name("k8s-secret-" + username)
	if observedSecret, ok := observed[secretResourceName]; ok {
		secretStringB64, found, _ := unstructured.NestedString(observedSecret.Resource.Object, "data", "secretString")
		if found && secretStringB64 != "" {
			secretStringBytes, err := base64.StdEncoding.DecodeString(secretStringB64)
			if err == nil {
				var secretData map[string]string
				if err := json.Unmarshal(secretStringBytes, &secretData); err == nil {
					if pw := secretData["password"]; pw != "" {
						return pw, nil
					}
				}
			}
		}
	}
	return generatePassword()
}

func generatePassword() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i, v := range b {
		b[i] = chars[v%byte(len(chars))]
	}
	return string(b), nil
}

func (g *kafkaUserGenerator) generate() (map[string]runtime.Object, error) {
	desired := make(map[string]runtime.Object)
	username := g.user.Name
	clusterName := g.user.Spec.ClusterName

	k8sSecretName := "k8s-secret-" + username
	desired[k8sSecretName] = g.buildK8sSecret(clusterName, username)

	if _, ok := g.observed[resource.Name(k8sSecretName)]; !ok {
		return desired, nil
	}

	desired["aws-secret-"+username] = g.buildAWSSecret(clusterName, username)
	desired["secret-version-"+username] = g.buildSecretVersion(clusterName, username)
	desired["secret-policy-"+username] = g.buildSecretPolicy(clusterName, username)
	desired["scram-association-"+username] = g.buildScramAssociation(clusterName, username)

	for _, group := range g.user.Spec.ConsumerGroups {
		key := "acl-cg-" + username + "-" + group
		desired[key] = g.buildConsumerGroupACL(username, group)
	}

	for _, acl := range g.user.Spec.ACLs {
		key := "acl-topic-" + acl.Topic + "-" + username + "-" + strings.ToLower(acl.Operation)
		desired[key] = g.buildTopicACL(username, acl.Topic, acl.Operation)
	}

	return desired, nil
}

func (g *kafkaUserGenerator) buildK8sSecret(clusterName, username string) runtime.Object {
	type secretData struct {
		Username         string `json:"username"`
		Password         string `json:"password"`
		BootstrapServers string `json:"bootstrap_servers"`
	}
	data := secretData{
		Username:         username,
		Password:         g.password,
		BootstrapServers: g.msk.Status.BrokersScram,
	}
	dataBytes, _ := json.Marshal(data)

	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-" + username + "-k8s",
			Namespace: g.user.Namespace,
			Labels: map[string]string{
				"msk-cluster": clusterName,
				"kafka-user":  username,
			},
		},
		Type: corev1.SecretTypeOpaque,
		StringData: map[string]string{
			"secretString": string(dataBytes),
		},
	}
}

func (g *kafkaUserGenerator) buildAWSSecret(clusterName, username string) runtime.Object {
	tags := map[string]interface{}{
		"KafkaCluster": clusterName,
		"KafkaUser":    username,
	}
	for k, v := range g.env.Tags {
		if v != nil {
			tags[k] = *v
		}
	}

	spec := map[string]interface{}{
		"forProvider": map[string]interface{}{
			"name":                   fmt.Sprintf("AmazonMSK_%s-%s", clusterName, username),
			"description":            fmt.Sprintf("SCRAM credentials for Kafka user %s", username),
			"region":                 g.msk.Status.Region,
			"kmsKeyId":               g.kmsKeyID,
			"recoveryWindowInDays":   float64(0),
			"tags":                   tags,
		},
		"providerConfigRef": map[string]interface{}{
			"kind": "ClusterProviderConfig",
			"name": g.env.AWSProvider,
		},
	}

	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "secretsmanager.aws.m.upbound.io/v1beta1",
			"kind":       "Secret",
			"metadata": map[string]interface{}{
				"name": clusterName + "-" + username,
				"labels": map[string]interface{}{
					"msk-cluster": clusterName,
					"kafka-user":  username,
				},
			},
			"spec": spec,
		},
	}
}

func (g *kafkaUserGenerator) buildSecretVersion(clusterName, username string) runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "secretsmanager.aws.m.upbound.io/v1beta1",
			"kind":       "SecretVersion",
			"metadata": map[string]interface{}{
				"name": clusterName + "-" + username + "-version",
				"labels": map[string]interface{}{
					"msk-cluster": clusterName,
					"kafka-user":  username,
				},
			},
			"spec": map[string]interface{}{
				"managementPolicies": []interface{}{"Observe", "Create", "Delete"},
				"forProvider": map[string]interface{}{
					"region": g.msk.Status.Region,
					"secretIdSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"msk-cluster": clusterName,
							"kafka-user":  username,
						},
					},
					"secretStringSecretRef": map[string]interface{}{
						"name":      clusterName + "-" + username + "-k8s",
						"namespace": g.user.Namespace,
						"key":       "secretString",
					},
				},
				"providerConfigRef": map[string]interface{}{
					"kind": "ClusterProviderConfig",
					"name": g.env.AWSProvider,
				},
			},
		},
	}
}

func (g *kafkaUserGenerator) buildSecretPolicy(clusterName, username string) runtime.Object {
	policy := `{"Version":"2012-10-17","Statement":[{"Sid":"AWSKafkaResourcePolicy","Effect":"Allow","Principal":{"Service":"kafka.amazonaws.com"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "secretsmanager.aws.m.upbound.io/v1beta1",
			"kind":       "SecretPolicy",
			"metadata": map[string]interface{}{
				"name": clusterName + "-" + username + "-policy",
				"labels": map[string]interface{}{
					"msk-cluster": clusterName,
					"kafka-user":  username,
				},
			},
			"spec": map[string]interface{}{
				"forProvider": map[string]interface{}{
					"region": g.msk.Status.Region,
					"secretArnSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"msk-cluster": clusterName,
							"kafka-user":  username,
						},
					},
					"policy": policy,
				},
				"providerConfigRef": map[string]interface{}{
					"kind": "ClusterProviderConfig",
					"name": g.env.AWSProvider,
				},
			},
		},
	}
}

func (g *kafkaUserGenerator) buildScramAssociation(clusterName, username string) runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.aws.m.upbound.io/v1beta1",
			"kind":       "SingleScramSecretAssociation",
			"metadata": map[string]interface{}{
				"name": clusterName + "-" + username + "-scram",
			},
			"spec": map[string]interface{}{
				"forProvider": map[string]interface{}{
					"region":     g.msk.Status.Region,
					"clusterArn": g.msk.Status.ARN,
					"secretArnRef": map[string]interface{}{
						"name": clusterName + "-" + username,
					},
				},
				"providerConfigRef": map[string]interface{}{
					"kind": "ClusterProviderConfig",
					"name": g.env.AWSProvider,
				},
			},
		},
	}
}

func (g *kafkaUserGenerator) buildConsumerGroupACL(username, group string) runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "acl.kafka.m.crossplane.io/v1alpha1",
			"kind":       "AccessControlList",
			"metadata": map[string]interface{}{
				"name":      username + "-" + strings.ToLower(group) + "-cg",
				"namespace": g.user.Namespace,
			},
			"spec": map[string]interface{}{
				"forProvider": map[string]interface{}{
					"resourceType":                "Group",
					"resourceName":                group,
					"resourcePatternTypeFilter":   "Literal",
					"resourcePrincipal":           "User:" + username,
					"resourceHost":                "*",
					"resourceOperation":           "Read",
					"resourcePermissionType":      "Allow",
				},
				"providerConfigRef": map[string]interface{}{
					"kind": "ClusterProviderConfig",
					"name": g.msk.Status.ProviderConfig,
				},
			},
		},
	}
}

func (g *kafkaUserGenerator) buildTopicACL(username, topic, operation string) runtime.Object {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "acl.kafka.m.crossplane.io/v1alpha1",
			"kind":       "AccessControlList",
			"metadata": map[string]interface{}{
				"name":      topic + "-" + username + "-" + strings.ToLower(operation),
				"namespace": g.user.Namespace,
			},
			"spec": map[string]interface{}{
				"forProvider": map[string]interface{}{
					"resourceType":              "Topic",
					"resourceName":              topic,
					"resourcePatternTypeFilter": "Literal",
					"resourcePrincipal":         "User:" + username,
					"resourceHost":              "*",
					"resourceOperation":         operation,
					"resourcePermissionType":    "Allow",
				},
				"providerConfigRef": map[string]interface{}{
					"kind": "ClusterProviderConfig",
					"name": g.msk.Status.ProviderConfig,
				},
			},
		},
	}
}
