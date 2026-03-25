package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/crossplane-common"
)

const (
	composition     = "../apis/valkey-composition.yaml"
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/database"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"
	instances       = "../examples/instance.yaml"
)

func TestValkeyCrossplaneRender(t *testing.T) {
	t.Logf("Starting database function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")
	tempInstance := filepath.Join(tmpDir, "instance.yaml")

	instancesUnstructured := crossplane.ParseYamlFileToUnstructured(t, instances)
	for _, unstructured := range instancesUnstructured {
		if unstructured.GetName() == "example-valkey-with-custom-settings" {
			crossplane.AppendToResources(t, tempInstance, unstructured)
		}
	}

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	t.Log("Rerendering...")
	resources := crossplane.CrossplaneRender(t, tempInstance, composition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "ValkeyInstance", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)

	t.Log("Validating database.entigo.com ValkeyInstance fields")
	crossplane.AssertFieldValues(t, resources, "ValkeyInstance", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":                "example-valkey-with-custom-settings",
		"spec.autoMinorVersionUpgrade": "true",
		"spec.deletionProtection":      "true",
		"spec.engineVersion":           "8.2",
		"spec.instanceType":            "cache.t4g.small",
		"spec.maintenanceWindow":       "mon:00:00-mon:03:00",
		"spec.numCacheClusters":        "3",
		"spec.parameterGroupName":      "default.valkey8",
		"spec.snapshotRetentionLimit":  "10",
		"spec.snapshotWindow":          "05:00-06:00",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroup fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-valkey-with-custom-settings",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "ValkeyInstance",
		"metadata.ownerReferences.0.name":       "example-valkey-with-custom-settings",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.tags.Name":            "example-valkey-with-custom-settings",
		"spec.forProvider.vpcIdRef.name":        "vpc",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed, crossplane.MockResource(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", true, nil))

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, tempInstance, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "ValkeyInstance", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "ReplicationGroup", 1)

	t.Log("Validating elasticache.aws.m.upbound.io ReplicationGroup fields")
	crossplane.AssertFieldValues(t, resources, "ReplicationGroup", "elasticache.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-valkey-with-custom-settings",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "ValkeyInstance",
		"metadata.ownerReferences.0.name":       "example-valkey-with-custom-settings",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.engine":               "valkey",
		"spec.forProvider.engineVersion":        "8.2",
		"spec.forProvider.nodeType":             "cache.t4g.small",
		"spec.forProvider.numCacheClusters":     "3",
		"spec.forProvider.kmsKeyId":             "arn:aws:kms:eu-north-1:123456789012:key/data-key-uuid",
		"spec.forProvider.subnetGroupName":      "my-elasticache-subnet-group",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed, crossplane.MockResource(t, resources, "ReplicationGroup", "elasticache.aws.m.upbound.io/v1beta1", true, map[string]interface{}{
		"status.atProvider.primaryEndpointAddress": "example.cache.amazonaws.com",
		"status.atProvider.port":                   float64(6379),
	}))

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, tempInstance, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "ValkeyInstance", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 1)

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroupRule fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroupRule", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                            "example-valkey-with-custom-settings-ingress-compute-subnet",
		"metadata.ownerReferences.0.apiVersion":    "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":          "ValkeyInstance",
		"metadata.ownerReferences.0.name":          "example-valkey-with-custom-settings",
		"spec.forProvider.fromPort":                "6379",
		"spec.forProvider.toPort":                  "6379",
		"spec.forProvider.securityGroupIdRef.name": "example-valkey-with-custom-settings",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed, crossplane.MockResource(t, resources, "SecurityGroupRule", "ec2.aws.m.upbound.io/v1beta1", true, nil))

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, tempInstance, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "ValkeyInstance", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 1)
	crossplane.AssertResourceCount(t, resources, "Secret", 1)

	t.Log("Validating secretsmanager.aws.m.upbound.io Secret fields")
	crossplane.AssertFieldValues(t, resources, "Secret", "secretsmanager.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "example-valkey-with-custom-settings-credentials",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "ValkeyInstance",
		"metadata.ownerReferences.0.name":       "example-valkey-with-custom-settings",
		"spec.forProvider.name":                 "example-valkey-with-custom-settings-credentials",
		"spec.forProvider.kmsKeyId":             "arn:aws:kms:eu-north-1:123456789012:key/config-key-uuid",
		"spec.forProvider.region":               "eu-north-1",
	})

	t.Log("Mocking observed resources")
	crossplane.AppendToResources(t, observed, crossplane.MockResource(t, resources, "Secret", "secretsmanager.aws.m.upbound.io/v1beta1", true, nil))

	t.Log("Rerendering...")
	resources = crossplane.CrossplaneRender(t, tempInstance, composition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting database.entigo.com ValkeyInstance Ready Status")
	crossplane.AssertResourceReady(t, resources, "ValkeyInstance", "database.entigo.com/v1alpha1")
}
