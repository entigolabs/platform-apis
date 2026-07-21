package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/mq"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"

	// Broker test files
	brokerComposition = "../apis/broker-composition.yaml"
	brokerResource    = "../examples/broker.yaml"
)

func TestRabbitMQCrossplaneRender(t *testing.T) {
	t.Logf("Starting mq function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	t.Run("Broker", testBrokerCrossplaneRender)
}

func testBrokerCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	tempBrokerResource := filepath.Join(tmpDir, "broker.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	brokerUnstructured := crossplane.ParseYamlFileToUnstructured(t, brokerResource)
	mockedBroker := crossplane.MockByKind(t, brokerUnstructured, "RabbitMQBroker", "mq.entigo.com/v1alpha1", false, map[string]interface{}{
		"metadata.uid": "000000000000",
	})
	crossplane.AppendToResources(t, tempBrokerResource, mockedBroker)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, tempBrokerResource, brokerComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "RabbitMQBroker", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)
	crossplane.AssertResourceCount(t, resources, "Broker", 0)

	t.Log("Validating mq.entigo.com RabbitMQBroker fields")
	crossplane.AssertFieldValues(t, resources, "RabbitMQBroker", "mq.entigo.com/v1alpha1", map[string]string{
		"metadata.name":      "rabbitmq-example",
		"spec.engineType":    "RabbitMQ",
		"spec.engineVersion": "4.2",
		"spec.instanceType":  "mq.m7g.medium",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroup fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.ownerReferences.0.apiVersion": "mq.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "RabbitMQBroker",
		"metadata.ownerReferences.0.name":       "rabbitmq-example",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.vpcIdRef.name":        "vpc",
	})

	t.Log("Mocking observed resources")
	mockedSG := crossplane.MockByKind(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", true, nil)
	mockedSecret := crossplane.MockByKind(t, resources, "Secret", "v1", true, nil)
	crossplane.AppendToResources(t, observed, mockedSG, mockedSecret)
	for _, res := range resources {
		if res.GetKind() == "SecurityGroupRule" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, tempBrokerResource, brokerComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "RabbitMQBroker", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)
	crossplane.AssertResourceCount(t, resources, "Broker", 1)

	t.Log("Validating mq.aws.m.upbound.io Broker fields")
	crossplane.AssertFieldValues(t, resources, "Broker", "mq.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.ownerReferences.0.apiVersion": "mq.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "RabbitMQBroker",
		"metadata.ownerReferences.0.name":       "rabbitmq-example",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.engineType":           "RabbitMQ",
		"spec.forProvider.hostInstanceType":     "mq.m7g.medium",
		"spec.forProvider.user.0.username":      "mqadmin",
		"spec.writeConnectionSecretToRef.name":  "rabbitmq-example-connection",
	})
}
