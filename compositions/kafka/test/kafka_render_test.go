package render_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane/v2/cmd/crank/render"
	xptest "github.com/entigolabs/platform-apis/test/common/crossplane"
	"github.com/spf13/afero"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func helmValues() string {
	return filepath.Join(xptest.WorkspaceRoot(), "helm", "values.yaml")
}

func TestMSKObserverStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/msk-observer.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	_ = unstructured.SetNestedField(xr.Object, "arn:aws:ecs:region:eu-north-1:01234567891:cluster/test-cluster", "spec", "clusterARN")
	_ = unstructured.SetNestedField(xr.Object, "aws-provider", "spec", "providerConfig")

	comp, err := render.LoadComposition(fs, "../apis/msk-observer-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DockerFunctionsFromHelm(t, helmValues(), "function-go-templating", "function-auto-ready")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	kmsKey, err := xptest.LoadUnstructured("../examples/required-resources.yaml")
	if err != nil {
		t.Fatalf("cannot load required resources: %v", err)
	}

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Cluster (observe-only phase)")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    []unstructured.Unstructured{envConfig, kmsKey},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Cluster", 1)

	cluster := xptest.FindResourceByKind(t, out1.ComposedResources, "Cluster")
	if cluster == nil {
		t.Fatal("cannot build observed: Cluster not found in TEST 1 output")
	}

	t.Log("Mocking Cluster as observed with broker status")
	observedCluster := xptest.CloneComposed(t, *cluster)
	_ = unstructured.SetNestedMap(observedCluster.Object, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Synced", "status": "True"},
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
		"atProvider": map[string]interface{}{
			"bootstrapBrokersSaslIam": "test-broker-saas-iam",
		},
	}, "status")

	t.Log("TEST 2: rendering ClusterProviderConfig and Secret when Cluster is ready")
	out2, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    []unstructured.Unstructured{envConfig, kmsKey},
		ObservedResources: []xptest.ComposedUnstructured{observedCluster},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out2, "ClusterProviderConfig", 1, "Secret", 1)

	t.Log("TEST 2: asserting ClusterProviderConfig fields")
	cpc := xptest.FindResource(t, out2.ComposedResources, "ClusterProviderConfig", "test-cluster-observed")
	if cpc != nil {
		xptest.AssertNestedString(t, cpc.Object, "Secret", "spec", "credentials", "source")
		xptest.AssertNestedString(t, cpc.Object, "test-cluster-observed-config", "spec", "credentials", "secretRef", "name")
		xptest.AssertNestedString(t, cpc.Object, "crossplane-kafka", "spec", "credentials", "secretRef", "namespace")
	}

	t.Log("TEST 2: asserting Secret fields")
	secret := xptest.FindResource(t, out2.ComposedResources, "Secret", "test-cluster-observed-config")
	if secret != nil {
		xptest.AssertNestedString(t, secret.Object, "crossplane-kafka", "metadata", "namespace")
	}
}

func TestTopicStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/topic-a.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}
	_ = unstructured.SetNestedField(xr.Object, "topic-claimRef", "spec", "claimRef", "name")

	comp, err := render.LoadComposition(fs, "../apis/kafka-topic-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DockerFunctionsFromHelm(t, helmValues(), "function-go-templating", "function-auto-ready")

	msk := mockedMSKResource(t, "test-crossplane-cluster-observed", "eu-north-1",
		"arn:aws:kafka:eu-north-1:012345678901:cluster/test-crossplane-cluster/abcdef",
		"broker1:9098,broker2:9098", "broker1:9096,broker2:9096")

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Topic")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    []unstructured.Unstructured{msk},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Topic", 2)

	t.Log("TEST 1: asserting Topic fields")
	topic := xptest.FindResource(t, out1.ComposedResources, "Topic", "topic-a")
	if topic != nil {
		assertTopicFields(t, topic.Object)
	}
}

func TestKafkaUserStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/user-a.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}
	_ = unstructured.SetNestedField(xr.Object, "user-claimRef", "spec", "claimRef", "name")
	_ = unstructured.SetNestedField(xr.Object, "default", "spec", "claimRef", "namespace")

	comp, err := render.LoadComposition(fs, "../apis/kafka-user-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	xptest.RemoveCompositionStep(comp, "sequence-creation")

	fns := xptest.DockerFunctionsFromHelm(t, helmValues(), "function-go-templating", "function-auto-ready")

	const (
		mskRegion = "eu-north-1"
		mskArn    = "arn:aws:kafka:eu-north-1:012345678901:cluster/test-crossplane-cluster/abcdef"
	)
	msk := mockedMSKResource(t, "test-crossplane-cluster-observed", mskRegion, mskArn,
		"broker1:9098,broker2:9098", "broker1:9096,broker2:9096")
	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	kmsKey, err := xptest.LoadUnstructured("../examples/required-resources.yaml")
	if err != nil {
		t.Fatalf("cannot load required resources: %v", err)
	}

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering KafkaUser resources")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    []unstructured.Unstructured{msk, envConfig, kmsKey},
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Secret", 1, "SecretVersion", 1, "SecretPolicy", 1,
		"SingleScramSecretAssociation", 1, "AccessControlList", 4)

	t.Log("TEST 1: asserting k8s Secret fields")
	secret := xptest.FindResource(t, out1.ComposedResources, "Secret", "test-crossplane-cluster-user-a-k8s")
	if secret != nil {
		xptest.AssertNestedString(t, secret.Object, "default", "metadata", "namespace")
		_, found, _ := unstructured.NestedString(secret.Object, "stringData", "secretString")
		if !found {
			t.Error("Secret: stringData.secretString not found")
		}
	}

	t.Log("TEST 1: asserting SecretVersion fields")
	sv := xptest.FindResource(t, out1.ComposedResources, "SecretVersion", "test-crossplane-cluster-user-a-version")
	if sv != nil {
		xptest.AssertNestedString(t, sv.Object, mskRegion, "spec", "forProvider", "region")
	}

	t.Log("TEST 1: asserting SecretPolicy fields")
	sp := xptest.FindResource(t, out1.ComposedResources, "SecretPolicy", "test-crossplane-cluster-user-a-policy")
	if sp != nil {
		xptest.AssertNestedString(t, sp.Object, mskRegion, "spec", "forProvider", "region")
	}

	t.Log("TEST 1: asserting SingleScramSecretAssociation fields")
	scram := xptest.FindResource(t, out1.ComposedResources, "SingleScramSecretAssociation", "test-crossplane-cluster-user-a-scram")
	if scram != nil {
		xptest.AssertNestedString(t, scram.Object, mskRegion, "spec", "forProvider", "region")
		xptest.AssertNestedString(t, scram.Object, mskArn, "spec", "forProvider", "clusterArn")
	}

	t.Log("TEST 1: asserting consumer group AccessControlList fields")
	assertConsumerGroupACL(t, out1.ComposedResources, "user-a-alpha-cg", "alpha")
	assertConsumerGroupACL(t, out1.ComposedResources, "user-a-gamma-cg", "gamma")

	t.Log("TEST 1: asserting topic AccessControlList fields")
	assertTopicACL(t, out1.ComposedResources, "topic-a-user-a-read", "topic-a", "Read")
	assertTopicACL(t, out1.ComposedResources, "topic-b-user-a-write", "topic-b", "Write")
}

func mockedMSKResource(t *testing.T, providerConfig, region, arn, brokers, brokersScram string) unstructured.Unstructured {
	t.Helper()
	msk, err := xptest.LoadUnstructured("../examples/msk-observer.yaml")
	if err != nil {
		t.Fatalf("cannot load MSK observer: %v", err)
	}
	_ = unstructured.SetNestedMap(msk.Object, map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Synced", "status": "True"},
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
		"providerConfig": providerConfig,
		"region":         region,
		"arn":            arn,
		"brokers":        brokers,
		"brokersscram":   brokersScram,
	}, "status")
	return msk
}

func assertTopicFields(t *testing.T, obj map[string]interface{}) {
	t.Helper()

	partitions, _, _ := unstructured.NestedFieldNoCopy(obj, "spec", "forProvider", "partitions")
	var partitionsNum int64
	switch v := partitions.(type) {
	case int64:
		partitionsNum = v
	case float64:
		partitionsNum = int64(v)
	}
	if partitionsNum != 6 {
		t.Errorf("Topic: expected partitions 6, got %v", partitions)
	}

	replicationFactor, _, _ := unstructured.NestedFieldNoCopy(obj, "spec", "forProvider", "replicationFactor")
	var rfNum int64
	switch v := replicationFactor.(type) {
	case int64:
		rfNum = v
	case float64:
		rfNum = int64(v)
	}
	if rfNum != 3 {
		t.Errorf("Topic: expected replicationFactor 3, got %v", replicationFactor)
	}

	config, _, _ := unstructured.NestedMap(obj, "spec", "forProvider", "config")
	if config["retention.ms"] != "604800000" {
		t.Errorf("Topic: expected config[retention.ms]=604800000, got %v", config["retention.ms"])
	}

	xptest.AssertNestedString(t, obj, "ClusterProviderConfig", "spec", "providerConfigRef", "kind")
}

func assertConsumerGroupACL(t *testing.T, resources []xptest.ComposedUnstructured, name, consumerGroup string) {
	t.Helper()
	acl := xptest.FindResource(t, resources, "AccessControlList", name)
	if acl == nil {
		return
	}
	xptest.AssertNestedString(t, acl.Object, "Group", "spec", "forProvider", "resourceType")
	xptest.AssertNestedString(t, acl.Object, consumerGroup, "spec", "forProvider", "resourceName")
	xptest.AssertNestedString(t, acl.Object, "User:user-a", "spec", "forProvider", "resourcePrincipal")
	xptest.AssertNestedString(t, acl.Object, "Read", "spec", "forProvider", "resourceOperation")
}

func assertTopicACL(t *testing.T, resources []xptest.ComposedUnstructured, name, topicName, operation string) {
	t.Helper()
	acl := xptest.FindResource(t, resources, "AccessControlList", name)
	if acl == nil {
		return
	}
	xptest.AssertNestedString(t, acl.Object, "Topic", "spec", "forProvider", "resourceType")
	xptest.AssertNestedString(t, acl.Object, topicName, "spec", "forProvider", "resourceName")
	xptest.AssertNestedString(t, acl.Object, "User:user-a", "spec", "forProvider", "resourcePrincipal")
	xptest.AssertNestedString(t, acl.Object, operation, "spec", "forProvider", "resourceOperation")
}
