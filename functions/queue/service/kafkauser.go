package service

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	aclmv1alpha1 "github.com/crossplane-contrib/provider-kafka/apis/namespaced/acl/v1alpha1"
	kafkacommon "github.com/crossplane-contrib/provider-kafka/apis/v1alpha1"
	xpvcommon "github.com/crossplane/crossplane-runtime/v2/apis/common"
	xpv2v1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2v2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis"
	"github.com/entigolabs/platform-apis/apis/v1alpha1"
	kafkamv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/kafka/v1beta1"
	kmsmv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/kms/v1beta1"
	smv1beta1 "github.com/upbound/provider-aws/v2/apis/namespaced/secretsmanager/v1beta1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type kafkaUserGenerator struct {
	user         v1alpha1.KafkaUser
	env          apis.Environment
	msk          v1alpha1.MSKInstance
	kmsConfigKey kmsmv1beta1.Key
	password     string
	observed     map[resource.Name]resource.ObservedComposed
}

func GenerateKafkaUserObjects(
	user v1alpha1.KafkaUser,
	required map[string][]resource.Required,
	observed map[resource.Name]resource.ObservedComposed,
) (map[string]client.Object, error) {
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

	var kmsConfigKey kmsmv1beta1.Key
	if kmsResources, ok := required["KMSConfigKey"]; ok && len(kmsResources) > 0 {
		if err := base.ExtractRequiredResource(required, "KMSConfigKey", &kmsConfigKey); err != nil {
			return nil, err
		}
	}

	password, err := resolvePassword(user.Name, observed)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve password: %w", err)
	}

	return &kafkaUserGenerator{
		user:         user,
		env:          env,
		msk:          msk,
		kmsConfigKey: kmsConfigKey,
		password:     password,
		observed:     observed,
	}, nil
}

func resolvePassword(username string, observed map[resource.Name]resource.ObservedComposed) (string, error) {
	secretResourceName := resource.Name("k8s-secret-" + username)
	observedSecret, ok := observed[secretResourceName]
	if !ok {
		return generatePassword()
	}

	secretStringB64, found, _ := unstructured.NestedString(observedSecret.Resource.Object, "data", "secretString")
	if !found || secretStringB64 == "" {
		return generatePassword()
	}

	secretStringBytes, err := base64.StdEncoding.DecodeString(secretStringB64)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode existing secret for user %s: %w", username, err)
	}

	var secretData map[string]string
	if err := json.Unmarshal(secretStringBytes, &secretData); err != nil {
		return "", fmt.Errorf("failed to unmarshal existing secret for user %s: %w", username, err)
	}

	if pw := secretData["password"]; pw != "" {
		return pw, nil
	}

	return generatePassword()
}

func generatePassword() (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 32)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		b[i] = chars[n.Int64()]
	}
	return string(b), nil
}

func (g *kafkaUserGenerator) generate() (map[string]client.Object, error) {
	desired := make(map[string]client.Object)
	username := g.user.Name
	clusterName := g.user.Spec.ClusterName

	k8sSecretName := "k8s-secret-" + username
	k8sSecret, err := g.buildK8sSecret(clusterName, username)
	if err != nil {
		return nil, err
	}
	desired[k8sSecretName] = k8sSecret

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

func (g *kafkaUserGenerator) buildK8sSecret(clusterName, username string) (client.Object, error) {
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
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret data for user %s: %w", username, err)
	}

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
	}, nil
}

func (g *kafkaUserGenerator) buildAWSSecret(clusterName, username string) client.Object {
	tags := make(map[string]*string)
	for k, v := range g.env.Tags {
		tags[k] = v
	}
	tags["KafkaCluster"] = &clusterName
	tags["KafkaUser"] = &username

	secretName := fmt.Sprintf("AmazonMSK_%s-%s", clusterName, username)
	description := fmt.Sprintf("SCRAM credentials for Kafka user %s", username)
	recoveryWindow := float64(0)

	return &smv1beta1.Secret{
		TypeMeta: metav1.TypeMeta{Kind: "Secret", APIVersion: "secretsmanager.aws.m.upbound.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-" + username,
			Namespace: g.user.Namespace,
			Labels: map[string]string{
				"msk-cluster": clusterName,
				"kafka-user":  username,
			},
		},
		Spec: smv1beta1.SecretSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: smv1beta1.SecretParameters{
				Name:                 &secretName,
				Description:          &description,
				Region:               &g.msk.Status.Region,
				KMSKeyIDRef:          &xpv2v1.NamespacedReference{Name: g.kmsConfigKey.Name, Namespace: g.kmsConfigKey.Namespace},
				RecoveryWindowInDays: &recoveryWindow,
				Tags:                 tags,
			},
		},
	}
}

func (g *kafkaUserGenerator) buildSecretVersion(clusterName, username string) client.Object {
	return &smv1beta1.SecretVersion{
		TypeMeta: metav1.TypeMeta{Kind: "SecretVersion", APIVersion: "secretsmanager.aws.m.upbound.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-" + username + "-version",
			Namespace: g.user.Namespace,
			Labels: map[string]string{
				"msk-cluster": clusterName,
				"kafka-user":  username,
			},
		},
		Spec: smv1beta1.SecretVersionSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
				ManagementPolicies:      xpv2v1.ManagementPolicies{"Observe", "Create", "Delete"},
			},
			ForProvider: smv1beta1.SecretVersionParameters{
				Region: &g.msk.Status.Region,
				SecretIDSelector: &xpv2v1.NamespacedSelector{
					MatchLabels: map[string]string{
						"msk-cluster": clusterName,
						"kafka-user":  username,
					},
				},
				SecretStringSecretRef: &xpv2v1.LocalSecretKeySelector{
					LocalSecretReference: xpv2v1.LocalSecretReference{Name: clusterName + "-" + username + "-k8s"},
					Key:                  "secretString",
				},
			},
		},
	}
}

func (g *kafkaUserGenerator) buildSecretPolicy(clusterName, username string) client.Object {
	policy := `{"Version":"2012-10-17","Statement":[{"Sid":"AWSKafkaResourcePolicy","Effect":"Allow","Principal":{"Service":"kafka.amazonaws.com"},"Action":"secretsmanager:GetSecretValue","Resource":"*"}]}`

	return &smv1beta1.SecretPolicy{
		TypeMeta: metav1.TypeMeta{Kind: "SecretPolicy", APIVersion: "secretsmanager.aws.m.upbound.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-" + username + "-policy",
			Namespace: g.user.Namespace,
			Labels: map[string]string{
				"msk-cluster": clusterName,
				"kafka-user":  username,
			},
		},
		Spec: smv1beta1.SecretPolicySpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: smv1beta1.SecretPolicyParameters{
				Region: &g.msk.Status.Region,
				SecretArnSelector: &xpv2v1.NamespacedSelector{
					MatchLabels: map[string]string{
						"msk-cluster": clusterName,
						"kafka-user":  username,
					},
				},
				Policy: &policy,
			},
		},
	}
}

func (g *kafkaUserGenerator) buildScramAssociation(clusterName, username string) client.Object {
	return &kafkamv1beta1.SingleScramSecretAssociation{
		TypeMeta: metav1.TypeMeta{Kind: "SingleScramSecretAssociation", APIVersion: "kafka.aws.m.upbound.io/v1beta1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      clusterName + "-" + username + "-scram",
			Namespace: g.user.Namespace,
		},
		Spec: kafkamv1beta1.SingleScramSecretAssociationSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.env.AWSProvider, Kind: "ClusterProviderConfig"},
			},
			ForProvider: kafkamv1beta1.SingleScramSecretAssociationParameters{
				Region:     &g.msk.Status.Region,
				ClusterArn: &g.msk.Status.ARN,
				SecretArnRef: &xpv2v1.NamespacedReference{
					Name:      clusterName + "-" + username,
					Namespace: g.user.Namespace,
				},
			},
		},
	}
}

func (g *kafkaUserGenerator) buildConsumerGroupACL(username, group string) client.Object {
	return &aclmv1alpha1.AccessControlList{
		TypeMeta: metav1.TypeMeta{Kind: "AccessControlList", APIVersion: "acl.kafka.m.crossplane.io/v1alpha1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      username + "-" + strings.ToLower(group) + "-cg",
			Namespace: g.user.Namespace,
		},
		Spec: aclmv1alpha1.AccessControlListSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.msk.Status.ProviderConfig, Kind: "ClusterProviderConfig"},
			},
			ForProvider: kafkacommon.AccessControlListParameters{
				ResourceType:              "Group",
				ResourceName:              group,
				ResourcePatternTypeFilter: "Literal",
				ResourcePrincipal:         "User:" + username,
				ResourceHost:              "*",
				ResourceOperation:         "Read",
				ResourcePermissionType:    "Allow",
			},
		},
	}
}

func (g *kafkaUserGenerator) buildTopicACL(username, topic, operation string) client.Object {
	return &aclmv1alpha1.AccessControlList{
		TypeMeta: metav1.TypeMeta{Kind: "AccessControlList", APIVersion: "acl.kafka.m.crossplane.io/v1alpha1"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      topic + "-" + username + "-" + strings.ToLower(operation),
			Namespace: g.user.Namespace,
		},
		Spec: aclmv1alpha1.AccessControlListSpec{
			ManagedResourceSpec: xpv2v2.ManagedResourceSpec{
				ProviderConfigReference: &xpvcommon.ProviderConfigReference{Name: g.msk.Status.ProviderConfig, Kind: "ClusterProviderConfig"},
			},
			ForProvider: kafkacommon.AccessControlListParameters{
				ResourceType:              "Topic",
				ResourceName:              topic,
				ResourcePatternTypeFilter: "Literal",
				ResourcePrincipal:         "User:" + username,
				ResourceHost:              "*",
				ResourceOperation:         operation,
				ResourcePermissionType:    "Allow",
			},
		},
	}
}
