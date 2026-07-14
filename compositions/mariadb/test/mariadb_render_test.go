package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
)

const (
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/database"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"

	// Instance test files
	instanceComposition = "../apis/instance-composition.yaml"
	instanceResource    = "../examples/instance.yaml"
)

func TestMariaDBCrossplaneRender(t *testing.T) {
	t.Logf("Starting database function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	t.Run("Instance", testInstanceCrossplaneRender)
}

func testInstanceCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	tempInstanceResource := filepath.Join(tmpDir, "instance.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	instanceUnstructured := crossplane.ParseYamlFileToUnstructured(t, instanceResource)
	mockedInstance := crossplane.MockByKind(t, instanceUnstructured, "MariaDBInstance", "database.entigo.com/v1alpha1", false, map[string]interface{}{
		"metadata.uid":            "000000000000",
		"spec.snapshotIdentifier": "mariadb-instance-test-instance-snapshot",
	})
	crossplane.AppendToResources(t, tempInstanceResource, mockedInstance)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, tempInstanceResource, instanceComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBInstance", 1)
	crossplane.AssertResourceCount(t, resources, "ProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)

	t.Log("Validating database.entigo.com MariaDBInstance fields")
	crossplane.AssertFieldValues(t, resources, "MariaDBInstance", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "mariadb-example",
		"spec.allocatedStorage": "20",
		"spec.engineVersion":    "11.4.10",
		"spec.instanceType":     "db.t3.micro",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroup fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "mariadb-example-sg-86d8a475",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBInstance",
		"metadata.ownerReferences.0.name":       "mariadb-example",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.vpcIdRef.name":        "vpc",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroupRule fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroupRule", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                            "mariadb-example-sg-egress-86d8a475",
		"metadata.ownerReferences.0.apiVersion":    "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":          "MariaDBInstance",
		"metadata.ownerReferences.0.name":          "mariadb-example",
		"spec.forProvider.cidrBlocks.0":            "0.0.0.0/0",
		"spec.forProvider.region":                  "eu-north-1",
		"spec.forProvider.securityGroupIdRef.name": "mariadb-example-sg-86d8a475",
		"spec.forProvider.type":                    "egress",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroupRule fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroupRule", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                            "mariadb-example-sg-ingress-86d8a475",
		"metadata.ownerReferences.0.apiVersion":    "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":          "MariaDBInstance",
		"metadata.ownerReferences.0.name":          "mariadb-example",
		"spec.forProvider.cidrBlocks.0":            "0.0.0.0/0",
		"spec.forProvider.region":                  "eu-north-1",
		"spec.forProvider.securityGroupIdRef.name": "mariadb-example-sg-86d8a475",
		"spec.forProvider.type":                    "ingress",
	})

	t.Log("Mocking observed resources")
	mockedSecurityGroup := crossplane.MockByKind(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", true, nil)
	mockedProviderConfig := crossplane.MockByKind(t, resources, "ProviderConfig", "mysql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedSecurityGroup, mockedProviderConfig)
	for _, res := range resources {
		if res.GetKind() == "SecurityGroupRule" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, tempInstanceResource, instanceComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBInstance", 1)
	crossplane.AssertResourceCount(t, resources, "ProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)
	crossplane.AssertResourceCount(t, resources, "Instance", 1)

	t.Log("Validating rds.aws.m.upbound.io Instance fields")
	crossplane.AssertFieldValues(t, resources, "Instance", "rds.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                     "mariadb-example-instance-86d8a475",
		"metadata.ownerReferences.0.apiVersion":             "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                   "MariaDBInstance",
		"metadata.ownerReferences.0.name":                   "mariadb-example",
		"spec.forProvider.allocatedStorage":                 "20",
		"spec.forProvider.backupRetentionPeriod":            "14",
		"spec.forProvider.dbSubnetGroupNameRef.name":        "database",
		"spec.forProvider.dbSubnetGroupNameRef.namespace":   "crossplane-aws",
		"spec.forProvider.finalSnapshotIdentifier":          "mariadb-example-instance-snapshot-86d8a475",
		"spec.forProvider.kmsKeyIdRef.name":                 "data",
		"spec.forProvider.masterUserSecretKmsKeyIdRef.name": "config",
		"spec.forProvider.vpcSecurityGroupIdRefs.0.name":    "mariadb-example-sg-86d8a475",
		"spec.forProvider.snapshotIdentifier":               "mariadb-instance-test-instance-snapshot",
	})

	t.Log("Mocking observed resources")
	mockedRdsInstance := crossplane.MockByKind(t, resources, "Instance", "rds.aws.m.upbound.io/v1beta1", true, map[string]interface{}{
		"status.atProvider.status":       "Available",
		"status.atProvider.address":      "mock-db.cluster-123.eu-north-1.rds.amazonaws.com",
		"status.atProvider.port":         float64(5432),
		"status.atProvider.hostedZoneId": "mock-zone",
		"status.atProvider.masterUserSecret": []interface{}{
			map[string]interface{}{
				"secretArn":    "arn:aws:kms:eu-north-1:012345678901:key/mrk-1",
				"secretStatus": "active",
			},
		},
	})
	crossplane.AppendToResources(t, observed, mockedRdsInstance)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, tempInstanceResource, instanceComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBInstance", 1)
	crossplane.AssertResourceCount(t, resources, "ProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)
	crossplane.AssertResourceCount(t, resources, "Instance", 1)
	crossplane.AssertResourceCount(t, resources, "ExternalSecret", 1)

	t.Log("Validating external-secrets.io ExternalSecret fields")
	crossplane.AssertFieldValues(t, resources, "ExternalSecret", "external-secrets.io/v1", map[string]string{
		"metadata.name":                         "mariadb-example-es-86d8a475",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBInstance",
		"metadata.ownerReferences.0.name":       "mariadb-example",
		"spec.data.0.remoteRef.key":             "arn:aws:kms:eu-north-1:012345678901:key/mrk-1",
		"spec.data.0.remoteRef.property":        "password",
		"spec.data.0.remoteRef.version":         "AWSCURRENT",
		"spec.data.0.secretKey":                 "password",
		"spec.secretStoreRef.kind":              "ClusterSecretStore",
		"spec.secretStoreRef.name":              "external-secrets",
		"spec.target.name":                      "mariadb-example-dbadmin",
		"spec.target.template.data.endpoint":    "mock-db.cluster-123.eu-north-1.rds.amazonaws.com",
		"spec.target.template.data.password":    "*",
		"spec.target.template.data.port":        "5432",
		"spec.target.template.data.username":    "dbadmin",
	})

	t.Log("Mocking observed resources")

	mockedExternalSecret := crossplane.MockByKind(t, resources, "ExternalSecret", "external-secrets.io/v1", true, nil)
	crossplane.AppendToResources(t, observed, mockedExternalSecret)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, tempInstanceResource, instanceComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting database.entigo.com MariaDBInstance Ready Status")
	crossplane.AssertResourceReady(t, resources, "MariaDBInstance", "database.entigo.com/v1alpha1")
}
