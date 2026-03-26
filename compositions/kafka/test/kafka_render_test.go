package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/crossplane-common"
)

const (
	env             = "../examples/environment-config.yaml"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	// TODO: Remove when Kafka transitioned to golang custom function
	helmValues = "../../../helm/values.yaml"
	required   = "../examples/required-resources.yaml"

	// User Test Files
	userComposition = "../apis/kafka-user-composition.yaml"
	userResource    = "../examples/user-a.yaml"

	// MSK Test Files
	mskObserverResource    = "../examples/msk-observer.yaml"
	mskObserverComposition = "../apis/msk-observer-composition.yaml"

	// Topic Test Files
	topicComposition = "../apis/kafka-topic-composition.yaml"
	topicResource    = "../examples/topic-a.yaml"
)

func TestKafkaCrossplaneRender(t *testing.T) {
	t.Run("User", testKafkaUserCrossplaneRender)
	t.Run("MskObserver", testMskObserverCrossplaneRender)
	t.Run("Topic", testTopicCrossplaneRender)
}

func testKafkaUserCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	tempUserResource := filepath.Join(tmpDir, "user.yaml")
	tempUserComposition := filepath.Join(tmpDir, "user-composition.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	mskObserverUnstructured := crossplane.ParseYamlFileToUnstructured(t, mskObserverResource)
	mockedMskObserver := crossplane.MockByKind(t, mskObserverUnstructured, "MSK", "kafka.entigo.com/v1alpha1", true, nil)
	crossplane.AppendToResources(t, extra, mockedMskObserver)

	userResourceUnstructured := crossplane.ParseYamlFileToUnstructured(t, userResource)
	mockedUser := crossplane.MockByKind(t, userResourceUnstructured, "KafkaUser", "kafka.entigo.com/v1alpha1", false, map[string]interface{}{
		"spec.claimRef.name":      "user-claimRef",
		"spec.claimRef.namespace": "default",
	})
	crossplane.AppendToResources(t, tempUserResource, mockedUser)

	//TODO: Remove when Kafka transitioned to golang custom function
	crossplane.RemovePipelineStep(t, userComposition, tempUserComposition, "sequence-creation")

	//TODO: Remove when Kafka transitioned to golang custom function
	updatedFunctionsConfig := crossplane.GenerateFunctionsConfig(t, helmValues, functionsConfig)

	t.Log("Rendering...")
	//TODO: Replace updatedFunctionsConfig with functionsConfig and tempUserComposition with userComposition when Kafka transitioned to golang custom function
	resources := crossplane.CrossplaneRender(t, tempUserResource, tempUserComposition, updatedFunctionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Secret", 1)
	crossplane.AssertResourceCount(t, resources, "SecretVersion", 1)
	crossplane.AssertResourceCount(t, resources, "SecretPolicy", 1)
	crossplane.AssertResourceCount(t, resources, "SingleScramSecretAssociation", 1)
	crossplane.AssertResourceCount(t, resources, "AccessControlList", 4)

	t.Log("Validating kafka.entigo.com KafkaUser fields")
	crossplane.AssertFieldValues(t, resources, "KafkaUser", "kafka.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "user-a",
		"metadata.namespace":    "default",
		"spec.claimRef.name":    "user-claimRef",
		"spec.acls.0.operation": "Read",
		"spec.acls.0.topic":     "topic-a",
		"spec.acls.1.operation": "Write",
		"spec.acls.1.topic":     "topic-b",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "user-a-alpha-cg",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "alpha",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "user-a-gamma-cg",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "gamma",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "topic-a-user-a-read",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "topic-a",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "topic-b-user-a-write",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "topic-b",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating v1 Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         "test-crossplane-cluster-user-a-k8s",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"stringData.secretString":               "*",
	})

	t.Log("Validating secretsmanager.aws.m.upbound.io SecretPolicy fields")
	crossplane.AssertFieldValues(t, resources, "SecretPolicy", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                              "test-crossplane-cluster-user-a-policy",
		"metadata.namespace":                                         "default",
		"metadata.ownerReferences.0.apiVersion":                      "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                            "KafkaUser",
		"metadata.ownerReferences.0.name":                            "user-a",
		"spec.forProvider.secretArnSelector.matchLabels.kafka-user":  "user-a",
		"spec.forProvider.secretArnSelector.matchLabels.msk-cluster": "test-crossplane-cluster",
	})

	t.Log("Validating kafka.aws.m.upbound.io SingleScramSecretAssociation fields")
	crossplane.AssertFieldValues(t, resources, "SingleScramSecretAssociation", "kafka.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "test-crossplane-cluster-user-a-scram",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.secretArnRef.name":    "test-crossplane-cluster-user-a",
	})

	t.Log("Validating secretsmanager.aws.m.upbound.io SecretVersion fields")
	crossplane.AssertFieldValues(t, resources, "SecretVersion", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                             "test-crossplane-cluster-user-a-version",
		"metadata.namespace":                                        "default",
		"metadata.ownerReferences.0.apiVersion":                     "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                           "KafkaUser",
		"metadata.ownerReferences.0.name":                           "user-a",
		"spec.forProvider.secretIdSelector.matchLabels.kafka-user":  "user-a",
		"spec.forProvider.secretIdSelector.matchLabels.msk-cluster": "test-crossplane-cluster",
		"spec.forProvider.secretStringSecretRef.key":                "secretString",
		"spec.forProvider.secretStringSecretRef.name":               "test-crossplane-cluster-user-a-k8s",
	})
}

func testMskObserverCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")
	tempMskObserverResource := filepath.Join(tmpDir, "msk-observer.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	mskObserverResourceUnstructured := crossplane.ParseYamlFileToUnstructured(t, mskObserverResource)
	mockedMskObserver := crossplane.MockByKind(t, mskObserverResourceUnstructured, "MSK", "kafka.entigo.com/v1alpha1", false, map[string]interface{}{
		"spec.clusterARN":     "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster",
		"spec.providerConfig": "aws-provider",
	})
	crossplane.AppendToResources(t, tempMskObserverResource, mockedMskObserver)

	//TODO: Remove when Kafka transitioned to golang custom function
	updatedFunctionsConfig := crossplane.GenerateFunctionsConfig(t, helmValues, functionsConfig)

	t.Log("Rendering...")
	//TODO: Replace updatedFunctionsConfig with functionsConfig when Kafka transitioned to golang custom function
	resources := crossplane.CrossplaneRender(t, tempMskObserverResource, mskObserverComposition, updatedFunctionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MSK", 1)
	crossplane.AssertResourceCount(t, resources, "Cluster", 1)

	t.Log("Validating kafka.entigo.com MSK fields")
	crossplane.AssertFieldValues(t, resources, "MSK", "kafka.entigo.com/v1alpha1", map[string]string{
		"metadata.name":       "test-crossplane-cluster-observed",
		"spec.clusterARN":     "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster",
		"spec.providerConfig": "aws-provider",
	})

	t.Log("Validating kafka.aws.upbound.io Cluster fields")
	crossplane.AssertFieldValues(t, resources, "Cluster", "kafka.aws.upbound.io/v1beta3", map[string]string{
		"metadata.generateName":                 "test-crossplane-cluster-observed-",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MSK",
		"metadata.ownerReferences.0.name":       "test-crossplane-cluster-observed",
		"spec.forProvider.region":               "region",
		"spec.managementPolicies.0":             "Observe",
	})

	t.Log("Mocking observed resources")
	mockedCluster := crossplane.MockByKind(t, resources, "Cluster", "kafka.aws.upbound.io/v1beta3", true, map[string]interface{}{
		"status.atProvider.bootstrapBrokersSaslIam": "test-broker-saas-iam",
	})
	crossplane.AppendToResources(t, observed, mockedCluster)

	t.Log("Rerendering...")
	//TODO: Replace updatedFunctionsConfig with functionsConfig when Kafka transitioned to golang custom function
	resources = crossplane.CrossplaneRender(t, tempMskObserverResource, mskObserverComposition, updatedFunctionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MSK", 1)
	crossplane.AssertResourceCount(t, resources, "Cluster", 1)
	crossplane.AssertResourceCount(t, resources, "ClusterProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)

	t.Log("Validating kafka.m.crossplane.io ClusterProviderConfig fields")
	crossplane.AssertFieldValues(t, resources, "ClusterProviderConfig", "kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "test-cluster-observed",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MSK",
		"metadata.ownerReferences.0.name":       "test-crossplane-cluster-observed",
		"spec.credentials.secretRef.key":        "credentials",
		"spec.credentials.secretRef.name":       "test-cluster-observed-config",
		"spec.credentials.secretRef.namespace":  "crossplane-kafka",
		"spec.credentials.source":               "Secret",
	})

	t.Log("Validating v1 Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         "test-cluster-observed-config",
		"metadata.namespace":                    "crossplane-kafka",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MSK",
		"metadata.ownerReferences.0.name":       "test-crossplane-cluster-observed",
		"stringData.credentials":                "*",
	})

	t.Log("Asserting kafka.entigo.com MSK Ready Status")
	crossplane.AssertResourceReady(t, resources, "MSK", "kafka.entigo.com/v1alpha1")
}

func testTopicCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")
	tempTopicResource := filepath.Join(tmpDir, "topic.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	mskObserverUnstructured := crossplane.ParseYamlFileToUnstructured(t, mskObserverResource)
	mockedMskObserver := crossplane.MockByKind(t, mskObserverUnstructured, "MSK", "kafka.entigo.com/v1alpha1", true, nil)
	crossplane.AppendToResources(t, extra, mockedMskObserver)

	topicResourceUnstructured := crossplane.ParseYamlFileToUnstructured(t, topicResource)
	mockedTopic := crossplane.MockByKind(t, topicResourceUnstructured, "Topic", "kafka.entigo.com/v1alpha1", false, map[string]interface{}{
		"spec.claimRef.name": "topic-claimRef",
	})
	crossplane.AppendToResources(t, tempTopicResource, mockedTopic)

	//TODO: Remove when Kafka transitioned to golang custom function
	updatedFunctionsConfig := crossplane.GenerateFunctionsConfig(t, helmValues, functionsConfig)

	t.Log("Rendering...")
	//TODO: Replace updatedFunctionsConfig with functionsConfig when Kafka transitioned to golang custom function
	resources := crossplane.CrossplaneRender(t, tempTopicResource, topicComposition, updatedFunctionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Topic", 2)

	t.Log("Validating kafka.entigo.com Topic fields")
	crossplane.AssertFieldValues(t, resources, "Topic", "kafka.entigo.com/v1alpha1", map[string]string{
		"metadata.name":      "topic-a",
		"metadata.namespace": "default",
		"spec.claimRef.name": "topic-claimRef",
		"spec.clusterName":   "test-crossplane-cluster",
		"spec.partitions":    "6",
	})

	t.Log("Validating topic.kafka.m.crossplane.io Topic fields")
	crossplane.AssertFieldValues(t, resources, "Topic", "topic.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "topic-a",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "kafka.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Topic",
		"metadata.ownerReferences.0.name":       "topic-a",
		"spec.forProvider.partitions":           "6",
		"spec.providerConfigRef.kind":           "ClusterProviderConfig",
	})

	t.Log("Mocking observed resources")
	mockedKafkaTopic := crossplane.MockByKind(t, resources, "Topic", "topic.kafka.m.crossplane.io/v1alpha1", true, map[string]interface{}{})
	crossplane.AppendToResources(t, observed, mockedKafkaTopic)

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, tempTopicResource, topicComposition, updatedFunctionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting kafka.entigo.com Topic Ready Status")
	crossplane.AssertResourceReady(t, resources, "Topic", "kafka.entigo.com/v1alpha1")
}
