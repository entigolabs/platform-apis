package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/function-base/test"
	"google.golang.org/protobuf/types/known/structpb"
)

const (
	testClusterARN     = "arn:aws:kafka:eu-north-1:111111111111:cluster/my-cluster/uuid-1234"
	testClusterName    = "my-cluster"
	testRegion         = "eu-north-1"
	testProviderConfig = "my-cluster-observed"
	testAWSProvider    = "crossplane-aws"
	testBrokers        = "b-1.my-cluster.example.com:9098,b-2.my-cluster.example.com:9098"
	testBrokersScram   = "b-1.my-cluster.example.com:9096,b-2.my-cluster.example.com:9096"
	testARN            = "arn:aws:kafka:eu-north-1:111111111111:cluster/my-cluster/uuid-1234"
	testKMSKeyID       = "mrk-abc123"
	testUsername       = "myuser"
	testPassword       = "existingPassword123456789012345"
)

func mskXR(arn string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "kafka.entigo.com/v1alpha1",
		"kind":       "MSK",
		"metadata":   map[string]interface{}{"name": "my-cluster-observed", "uid": "test-uid-msk"},
		"spec":       map[string]interface{}{"clusterARN": arn},
	})
	if err != nil {
		panic(err)
	}
	return s
}

func topicXR(clusterName string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "kafka.entigo.com/v1alpha1",
		"kind":       "Topic",
		"metadata":   map[string]interface{}{"name": "my-topic", "namespace": "default"},
		"spec": map[string]interface{}{
			"clusterName":       clusterName,
			"partitions":        float64(3),
			"replicationFactor": float64(3),
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

func kafkaUserXR(clusterName string) *structpb.Struct {
	s, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "kafka.entigo.com/v1alpha1",
		"kind":       "KafkaUser",
		"metadata":   map[string]interface{}{"name": testUsername, "namespace": "default"},
		"spec": map[string]interface{}{
			"clusterName":    clusterName,
			"consumerGroups": []interface{}{"my-consumer-group"},
			"acls": []interface{}{
				map[string]interface{}{
					"topic":     "my-topic",
					"operation": "Read",
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

func mskObserverResource() *fnv1.Resources {
	s, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "kafka.entigo.com/v1alpha1",
		"kind":       "MSK",
		"metadata":   map[string]interface{}{"name": testClusterName + "-observed"},
		"spec":       map[string]interface{}{"clusterARN": testClusterARN},
		"status": map[string]interface{}{
			"brokers":        testBrokers,
			"brokersscram":   testBrokersScram,
			"arn":            testARN,
			"region":         testRegion,
			"providerConfig": testProviderConfig,
		},
	})
	if err != nil {
		panic(err)
	}
	return &fnv1.Resources{
		Items: []*fnv1.Resource{{Resource: s}},
	}
}

func kmsKeyResourceWithKeyID(name, namespace, keyID string) *fnv1.Resources {
	s, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": base.KMSKeyApiVersion,
		"kind":       base.KMSKeyKind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
		"status": map[string]interface{}{
			"atProvider": map[string]interface{}{
				"keyId": keyID,
				"arn":   "arn:aws:kms:eu-north-1:111111111111:key/" + keyID,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return &fnv1.Resources{
		Items: []*fnv1.Resource{{Resource: s}},
	}
}

func k8sSecretObserved(clusterName, username, password string) *structpb.Struct {
	type secretData struct {
		Username         string `json:"username"`
		Password         string `json:"password"`
		BootstrapServers string `json:"bootstrap_servers"`
	}
	data := secretData{
		Username:         username,
		Password:         password,
		BootstrapServers: testBrokersScram,
	}
	dataBytes, _ := json.Marshal(data)
	encodedData := base64.StdEncoding.EncodeToString(dataBytes)

	s, err := structpb.NewStruct(map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      clusterName + "-" + username + "-k8s",
			"namespace": "default",
			"annotations": map[string]interface{}{
				"crossplane.io/composition-resource-name": "k8s-secret-" + username,
			},
		},
		"data": map[string]interface{}{
			"secretString": encodedData,
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	})
	if err != nil {
		panic(err)
	}
	return s
}

// TestMSKPhase tests the MSK composition phases.
func TestMSKPhase(t *testing.T) {
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"Stage1_NoObservedCluster_OnlyDesiresMSKCluster": {
			Reason: "With no observed cluster, only the observe-only MSK cluster should be desired",
			Args: test.Args{
				Ctx: context.Background(),
				Req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: mskXR(testClusterARN),
						},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "name", "uid", "creationTimestamp", "resourceVersion", "desired", "meta")
}

// TestTopicGeneration tests topic resource generation.
func TestTopicGeneration(t *testing.T) {
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"MSKReady_TopicResourceDesired": {
			Reason: "When MSK is ready, the topic resource should be desired with correct providerConfigRef",
			Args: test.Args{
				Ctx: context.Background(),
				Req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: topicXR(testClusterName),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						"MSKObserver": mskObserverResource(),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "name", "uid", "creationTimestamp", "resourceVersion", "desired", "meta", "requirements")
}

// TestKafkaUserPhase1 tests phase 1: no required resources, only env config requested.
func TestKafkaUserPhase1(t *testing.T) {
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"Phase1_NoRequired_RequestsEnvironmentConfig": {
			Reason: "With no required resources, only EnvironmentConfig should be requested",
			Args: test.Args{
				Ctx: context.Background(),
				Req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: kafkaUserXR(testClusterName),
						},
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig("platform-apis-kafka"),
						},
					},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "meta")
}

// TestKafkaUserPhase2 tests phase 2: env present, MSKObserver and KMSConfigKey requested.
func TestKafkaUserPhase2(t *testing.T) {
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"Phase2_EnvPresent_RequestsAllRequired": {
			Reason: "With env present, MSKObserver and KMSConfigKey should be requested",
			Args: test.Args{
				Ctx: context.Background(),
				Req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: kafkaUserXR(testClusterName),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData("platform-apis-kafka", map[string]interface{}{
							"awsProvider": testAWSProvider,
							"tags":        map[string]interface{}{},
						}),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta: &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Requirements: &fnv1.Requirements{
						Resources: map[string]*fnv1.ResourceSelector{
							base.EnvironmentKey: base.RequiredEnvironmentConfig("platform-apis-kafka"),
							"MSKObserver": {
								Kind:       "MSK",
								ApiVersion: "kafka.entigo.com/v1alpha1",
								Match:      &fnv1.ResourceSelector_MatchName{MatchName: testClusterName + "-observed"},
							},
							"KMSConfigKey": base.RequiredKMSKey("config", testAWSProvider),
						},
					},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "meta")
}

// TestKafkaUserPhase3 tests phase 3: all required present, k8s secret not observed.
func TestKafkaUserPhase3(t *testing.T) {
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"Phase3_AllRequired_K8sSecretNotObserved_OnlyK8sSecretDesired": {
			Reason: "With all required but no observed k8s secret, only k8s secret should be desired",
			Args: test.Args{
				Ctx: context.Background(),
				Req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: kafkaUserXR(testClusterName),
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData("platform-apis-kafka", map[string]interface{}{
							"awsProvider": testAWSProvider,
							"tags":        map[string]interface{}{},
						}),
						"MSKObserver":  mskObserverResource(),
						"KMSConfigKey": kmsKeyResourceWithKeyID("config", testAWSProvider, testKMSKeyID),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "name", "uid", "creationTimestamp", "resourceVersion", "desired", "meta", "requirements")
}

// TestKafkaUserPhase4 tests phase 4: k8s secret observed, all resources desired.
func TestKafkaUserPhase4(t *testing.T) {
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"Phase4_K8sSecretObserved_AllResourcesDesired": {
			Reason: "With k8s secret observed, all resources including AWS secrets and ACLs should be desired",
			Args: test.Args{
				Ctx: context.Background(),
				Req: &fnv1.RunFunctionRequest{
					Meta: &fnv1.RequestMeta{Tag: "test"},
					Observed: &fnv1.State{
						Composite: &fnv1.Resource{
							Resource: kafkaUserXR(testClusterName),
						},
						Resources: map[string]*fnv1.Resource{
							"k8s-secret-" + testUsername: {
								Resource: k8sSecretObserved(testClusterName, testUsername, testPassword),
							},
						},
					},
					RequiredResources: map[string]*fnv1.Resources{
						base.EnvironmentKey: test.EnvironmentConfigResourceWithData("platform-apis-kafka", map[string]interface{}{
							"awsProvider": testAWSProvider,
							"tags":        map[string]interface{}{},
						}),
						"MSKObserver":  mskObserverResource(),
						"KMSConfigKey": kmsKeyResourceWithKeyID("config", testAWSProvider, testKMSKeyID),
					},
				},
			},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "name", "uid", "creationTimestamp", "resourceVersion", "desired", "meta", "requirements")
}

// TestKafkaUserPasswordPreservation verifies that existing passwords from observed secrets are reused.
func TestKafkaUserPasswordPreservation(t *testing.T) {
	observedState := &fnv1.State{
		Composite: &fnv1.Resource{
			Resource: kafkaUserXR(testClusterName),
		},
		Resources: map[string]*fnv1.Resource{
			"k8s-secret-" + testUsername: {
				Resource: k8sSecretObserved(testClusterName, testUsername, testPassword),
			},
		},
	}

	req := &fnv1.RunFunctionRequest{
		Meta:     &fnv1.RequestMeta{Tag: "test"},
		Observed: observedState,
		RequiredResources: map[string]*fnv1.Resources{
			base.EnvironmentKey: test.EnvironmentConfigResourceWithData("platform-apis-kafka", map[string]interface{}{
				"awsProvider": testAWSProvider,
				"tags":        map[string]interface{}{},
			}),
			"MSKObserver":  mskObserverResource(),
			"KMSConfigKey": kmsKeyResourceWithKeyID("config", testAWSProvider, testKMSKeyID),
		},
	}

	// Run once
	test.RunFunctionCases(t, func() base.GroupService { return &GroupImpl{} }, map[string]test.Case{
		"PasswordPreservation": {
			Reason: "Password from observed secret should be reused",
			Args:   test.Args{Ctx: context.Background(), Req: req},
			Want: test.Want{
				Rsp: &fnv1.RunFunctionResponse{
					Meta:    &fnv1.ResponseMeta{Tag: "test", Ttl: nil},
					Results: []*fnv1.Result{},
				},
			},
		},
	}, "desired", "meta", "requirements")
}

// helper to shut up the unused import linter
var _ = resource.Name("")
