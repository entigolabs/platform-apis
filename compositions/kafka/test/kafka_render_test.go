package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/queue"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"

	// User Test Files
	userComposition = "../apis/kafka-user-composition.yaml"
	userResource    = "../examples/user-a.yaml"

	// MSK Test Files
	mskObserverResource    = "../examples/msk-observer.yaml"
	mskObserverComposition = "../apis/msk-observer-composition.yaml"

	// Topic Test Files
	topicComposition = "../apis/kafka-topic-composition.yaml"
	topicResource    = "../examples/topic-a.yaml"

	// Hash-based names derived from MSK instance UID (empty UID in tests → hash 811c9dc5)
	mskClusterResourceName        = "test-crossplane-cluster-observed-msk-cluster-811c9dc5"
	mskConfigSecretResourceName   = "test-crossplane-cluster-observed-kafka-config-secret-811c9dc5"
	mskProviderConfigResourceName = "kafka-cluster-provider-config-811c9dc5"
)

func TestKafkaCrossplaneRender(t *testing.T) {
	t.Logf("Starting queue function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	t.Run("User", testKafkaUserCrossplaneRender)
	t.Run("MskObserver", testMskObserverCrossplaneRender)
	t.Run("Topic", testTopicCrossplaneRender)
}

func testKafkaUserCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")
	tempUserResource := filepath.Join(tmpDir, "user.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	mskObserverUnstructured := crossplane.ParseYamlFileToUnstructured(t, mskObserverResource)
	mockedMskObserver := crossplane.MockByKind(t, mskObserverUnstructured, "MSK", "queue.entigo.com/v1alpha1", true, nil)
	crossplane.AppendToResources(t, extra, mockedMskObserver)

	userResourceUnstructured := crossplane.ParseYamlFileToUnstructured(t, userResource)
	mockedUser := crossplane.MockByKind(t, userResourceUnstructured, "KafkaUser", "queue.entigo.com/v1alpha1", false, map[string]interface{}{
		"spec.claimRef.name":      "user-claimRef",
		"spec.claimRef.namespace": "default",
	})
	crossplane.AppendToResources(t, tempUserResource, mockedUser)

	t.Log("Phase 1: Rendering without observed resources")
	resources := crossplane.CrossplaneRender(t, tempUserResource, userComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting Phase 1 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Secret", 1)

	t.Log("Validating kafka.entigo.com KafkaUser fields")
	crossplane.AssertFieldValues(t, resources, "KafkaUser", "queue.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "user-a",
		"metadata.namespace":    "default",
		"spec.claimRef.name":    "user-claimRef",
		"spec.acls.0.operation": "Read",
		"spec.acls.0.topic":     "topic-a",
		"spec.acls.1.operation": "Write",
		"spec.acls.1.topic":     "topic-b",
	})

	t.Log("Validating v1 Secret (k8s secret) fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         "test-crossplane-cluster-user-a-k8s",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"stringData.secretString":               "*",
	})

	t.Log("Phase 2: Mocking k8s secret as observed")
	k8sSecret := crossplane.MockByKind(t, resources, "Secret", "v1", true, nil)
	crossplane.AppendToResources(t, observed, k8sSecret)
	resources = crossplane.CrossplaneRender(t, tempUserResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting Phase 2 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Secret", 2)

	t.Log("Phase 3: Mocking AWS secret as observed")
	awsSecret := crossplane.MockByKind(t, resources, "Secret", "secretsmanager.aws.m.upbound.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, awsSecret)
	resources = crossplane.CrossplaneRender(t, tempUserResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting Phase 3 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "SecretVersion", 1)

	t.Log("Phase 4: Mocking SecretVersion as observed")
	sv := crossplane.MockByKind(t, resources, "SecretVersion", "secretsmanager.aws.m.upbound.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, sv)
	resources = crossplane.CrossplaneRender(t, tempUserResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting Phase 4 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "SecretPolicy", 1)

	t.Log("Phase 5: Mocking SecretPolicy as observed")
	sp := crossplane.MockByKind(t, resources, "SecretPolicy", "secretsmanager.aws.m.upbound.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, sp)
	resources = crossplane.CrossplaneRender(t, tempUserResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting Phase 5 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "SingleScramSecretAssociation", 1)

	t.Log("Phase 6: Mocking ScramAssociation as observed")
	scram := crossplane.MockByKind(t, resources, "SingleScramSecretAssociation", "kafka.aws.m.upbound.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, scram)
	resources = crossplane.CrossplaneRender(t, tempUserResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting Phase 6 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Secret", 2)
	crossplane.AssertResourceCount(t, resources, "SecretVersion", 1)
	crossplane.AssertResourceCount(t, resources, "SecretPolicy", 1)
	crossplane.AssertResourceCount(t, resources, "SingleScramSecretAssociation", 1)
	crossplane.AssertResourceCount(t, resources, "AccessControlList", 4)

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "user-a-alpha-cg",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "alpha",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "user-a-gamma-cg",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "gamma",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "topic-a-user-a-read",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "topic-a",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating acl.kafka.m.crossplane.io AccessControlList fields")
	crossplane.AssertFieldValues(t, resources, "AccessControlList", "acl.kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "topic-b-user-a-write",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.resourceName":         "topic-b",
		"spec.forProvider.resourcePrincipal":    "User:user-a",
	})

	t.Log("Validating secretsmanager.aws.m.upbound.io SecretPolicy fields")
	crossplane.AssertFieldValues(t, resources, "SecretPolicy", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                              "test-crossplane-cluster-user-a-policy",
		"metadata.namespace":                                         "default",
		"metadata.ownerReferences.0.apiVersion":                      "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                            "KafkaUser",
		"metadata.ownerReferences.0.name":                            "user-a",
		"spec.forProvider.secretArnSelector.matchLabels.kafka-user":  "user-a",
		"spec.forProvider.secretArnSelector.matchLabels.msk-cluster": "test-crossplane-cluster",
	})

	t.Log("Validating kafka.aws.m.upbound.io SingleScramSecretAssociation fields")
	crossplane.AssertFieldValues(t, resources, "SingleScramSecretAssociation", "kafka.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "test-crossplane-cluster-user-a-scram",
		"metadata.namespace":                    "default",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "KafkaUser",
		"metadata.ownerReferences.0.name":       "user-a",
		"spec.forProvider.secretArnRef.name":    "test-crossplane-cluster-user-a",
	})

	t.Log("Validating secretsmanager.aws.m.upbound.io SecretVersion fields")
	crossplane.AssertFieldValues(t, resources, "SecretVersion", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                             "test-crossplane-cluster-user-a-version",
		"metadata.namespace":                                        "default",
		"metadata.ownerReferences.0.apiVersion":                     "queue.entigo.com/v1alpha1",
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
	observed3 := filepath.Join(tmpDir, "observed3.yaml")
	tempMskObserverResource := filepath.Join(tmpDir, "msk-observer.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	mskObserverResourceUnstructured := crossplane.ParseYamlFileToUnstructured(t, mskObserverResource)
	mockedMskObserver := crossplane.MockByKind(t, mskObserverResourceUnstructured, "MSK", "queue.entigo.com/v1alpha1", false, map[string]interface{}{
		"spec.clusterARN":     "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster",
		"spec.providerConfig": "aws-provider",
	})
	crossplane.AppendToResources(t, tempMskObserverResource, mockedMskObserver)

	t.Log("Phase 1: Rendering")
	resources := crossplane.CrossplaneRender(t, tempMskObserverResource, mskObserverComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting Phase 1 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MSK", 1)
	crossplane.AssertResourceCount(t, resources, "Cluster", 1)

	t.Log("Validating kafka.entigo.com MSK fields")
	crossplane.AssertFieldValues(t, resources, "MSK", "queue.entigo.com/v1alpha1", map[string]string{
		"metadata.name":       "test-crossplane-cluster-observed",
		"spec.clusterARN":     "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster",
		"spec.providerConfig": "aws-provider",
	})

	t.Log("Validating kafka.aws.upbound.io Cluster fields")
	crossplane.AssertFieldValues(t, resources, "Cluster", "kafka.aws.upbound.io/v1beta3", map[string]string{
		"metadata.name":                         mskClusterResourceName,
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MSK",
		"metadata.ownerReferences.0.name":       "test-crossplane-cluster-observed",
		"spec.forProvider.region":               "region",
		"spec.managementPolicies.0":             "Observe",
	})

	t.Log("Phase 2: Mocking observed Cluster with broker info")
	mockedCluster := crossplane.MockByKind(t, resources, "Cluster", "kafka.aws.upbound.io/v1beta3", true, map[string]interface{}{
		"status.atProvider.bootstrapBrokersSaslIam": "test-broker-saas-iam",
	})
	crossplane.AppendToResources(t, observed, mockedCluster)

	t.Log("Phase 2: Rerendering")
	resources = crossplane.CrossplaneRender(t, tempMskObserverResource, mskObserverComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting Phase 2 rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MSK", 1)
	crossplane.AssertResourceCount(t, resources, "Cluster", 1)
	crossplane.AssertResourceCount(t, resources, "ClusterProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)

	t.Log("Validating kafka.m.crossplane.io ClusterProviderConfig fields")
	crossplane.AssertFieldValues(t, resources, "ClusterProviderConfig", "kafka.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         mskProviderConfigResourceName,
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MSK",
		"metadata.ownerReferences.0.name":       "test-crossplane-cluster-observed",
		"spec.credentials.secretRef.key":        "credentials",
		"spec.credentials.secretRef.name":       mskConfigSecretResourceName,
		"spec.credentials.secretRef.namespace":  "crossplane-kafka",
		"spec.credentials.source":               "Secret",
	})

	t.Log("Validating v1 Secret (kafka config) fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "v1", map[string]string{
		"metadata.name":                         mskConfigSecretResourceName,
		"metadata.namespace":                    "crossplane-kafka",
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MSK",
		"metadata.ownerReferences.0.name":       "test-crossplane-cluster-observed",
		"data.credentials":                      "*",
	})

	t.Log("Phase 3: Mocking all resources as ready for MSK readiness check")
	mockedCluster3 := crossplane.MockByKind(t, resources, "Cluster", "kafka.aws.upbound.io/v1beta3", true, map[string]interface{}{
		"status.atProvider.bootstrapBrokersSaslIam": "test-broker-saas-iam",
	})
	mockedProviderConfig := crossplane.MockByKind(t, resources, "ClusterProviderConfig", "kafka.m.crossplane.io/v1alpha1", true, nil)
	mockedSecret := crossplane.MockByKind(t, resources, "Secret", "v1", true, nil)
	crossplane.AppendToResources(t, observed3, mockedCluster3, mockedProviderConfig, mockedSecret)

	t.Log("Phase 3: Rerendering")
	resources = crossplane.CrossplaneRender(t, tempMskObserverResource, mskObserverComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed3))

	t.Log("Asserting kafka.entigo.com MSK Ready Status")
	crossplane.AssertResourceReady(t, resources, "MSK", "queue.entigo.com/v1alpha1")
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
	mockedMskObserver := crossplane.MockByKind(t, mskObserverUnstructured, "MSK", "queue.entigo.com/v1alpha1", true, map[string]interface{}{
		"status.providerConfig": mskProviderConfigResourceName,
	})
	crossplane.AppendToResources(t, extra, mockedMskObserver)

	topicResourceUnstructured := crossplane.ParseYamlFileToUnstructured(t, topicResource)
	mockedTopic := crossplane.MockByKind(t, topicResourceUnstructured, "Topic", "queue.entigo.com/v1alpha1", false, map[string]interface{}{
		"spec.claimRef.name": "topic-claimRef",
	})
	crossplane.AppendToResources(t, tempTopicResource, mockedTopic)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, tempTopicResource, topicComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "Topic", 2)

	t.Log("Validating kafka.entigo.com Topic fields")
	crossplane.AssertFieldValues(t, resources, "Topic", "queue.entigo.com/v1alpha1", map[string]string{
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
		"metadata.ownerReferences.0.apiVersion": "queue.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "Topic",
		"metadata.ownerReferences.0.name":       "topic-a",
		"spec.forProvider.partitions":           "6",
		"spec.providerConfigRef.name":           mskProviderConfigResourceName,
	})

	t.Log("Mocking observed resources")
	mockedKafkaTopic := crossplane.MockByKind(t, resources, "Topic", "topic.kafka.m.crossplane.io/v1alpha1", true, map[string]interface{}{})
	crossplane.AppendToResources(t, observed, mockedKafkaTopic)

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, tempTopicResource, topicComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting kafka.entigo.com Topic Ready Status")
	crossplane.AssertResourceReady(t, resources, "Topic", "queue.entigo.com/v1alpha1")
}
