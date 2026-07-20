package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/database"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"

	// Instance test files
	instanceComposition = "../apis/instance-composition.yaml"
	instanceResource    = "../examples/instance.yaml"

	// User test files
	userComposition       = "../apis/user-composition.yaml"
	userWithGrantResource = "../examples/user-with-grant.yaml"

	// Database test files
	databaseComposition = "../apis/database-composition.yaml"
	databaseResource    = "../examples/database.yaml"

	mySqlApiVersion = "mysql.sql.m.crossplane.io/v1alpha1"
)

func TestMariaDBCrossplaneRender(t *testing.T) {
	t.Logf("Starting database function. Function path %s", function)
	crossplane.StartCustomFunction(t, function, "9443")

	t.Run("Instance", testInstanceCrossplaneRender)
	t.Run("Database", testDatabaseCrossplaneRender)
	t.Run("User", testUserCrossplaneRender)
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

func testUserCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendToResources(t, extra, mariaDBInstanceExtraResource())
	crossplane.AppendToResources(t, extra, mariaDBDatabaseExtraResource())

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBUser", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)

	t.Log("Validating database.entigo.com MariaDBUser fields")
	crossplane.AssertFieldValues(t, resources, "MariaDBUser", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "user-example",
		"spec.instanceRef.name": "mariadb-example",
		"spec.databaseRef.name": "example-db",
		"spec.name":             "user_example",
		"spec.grant.users.0":    "example-user",
		"spec.privileges.0":     "SELECT",
		"spec.privileges.1":     "INSERT",
	})

	t.Log("Validating mysql.sql.m.crossplane.io User fields")
	crossplane.AssertFieldValues(t, resources, "User", mySqlApiVersion, map[string]string{
		"metadata.name":                                      "user-example",
		"metadata.ownerReferences.0.apiVersion":              "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                    "MariaDBUser",
		"metadata.ownerReferences.0.name":                    "user-example",
		"metadata.annotations.crossplane\\.io/external-name": "user_example",
		"spec.providerConfigRef.name":                        "mariadb-example-providerconfig",
		"spec.writeConnectionSecretToRef.name":               "mariadb-example-user-example",
	})

	t.Log("Mocking observed resources")
	mockedUser := crossplane.MockByKind(t, resources, "User", mySqlApiVersion, true, nil)
	crossplane.AppendToResources(t, observed, mockedUser)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBUser", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)

	t.Log("Validating mysql.sql.m.crossplane.io Grant fields")
	crossplane.AssertFieldValues(t, resources, "Grant", mySqlApiVersion, map[string]string{
		"metadata.name":                         "grant-user-example-example-user-mariadb-example",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.providerConfigRef.name":           "mariadb-example-providerconfig",
		"spec.forProvider.user":                 "user_example",
		"spec.forProvider.databaseRef.name":     "example-db",
		"spec.forProvider.table":                "*",
		"spec.forProvider.privileges.0":         "SELECT",
		"spec.forProvider.privileges.1":         "INSERT",
	})

	t.Log("Mocking observed resources")
	mockedGrant := crossplane.MockByKind(t, resources, "Grant", mySqlApiVersion, true, nil)
	crossplane.AppendToResources(t, observed, mockedGrant)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBUser", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 1)

	t.Log("Validating protection.crossplane.io grant Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "usage-grant-user-example-example-user-mariadb-example",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.by.apiVersion":                    mySqlApiVersion,
		"spec.by.kind":                          "Grant",
		"spec.by.resourceRef.name":              "grant-user-example-example-user-mariadb-example",
		"spec.of.apiVersion":                    mySqlApiVersion,
		"spec.of.kind":                          "User",
		"spec.of.resourceRef.name":              "user-example",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "usage-grant-user-example-example-user-mariadb-example" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBUser", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 2)

	t.Log("Validating protection.crossplane.io db-protection Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "db-protection-user-example-example-user-mariadb-example",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.by.apiVersion":                    mySqlApiVersion,
		"spec.by.kind":                          "Grant",
		"spec.by.resourceRef.name":              "grant-user-example-example-user-mariadb-example",
		"spec.of.apiVersion":                    "database.entigo.com/v1alpha1",
		"spec.of.kind":                          "MariaDBDatabase",
		"spec.of.resourceRef.name":              "example-db",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "db-protection-user-example-example-user-mariadb-example" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBUser", 1)
	crossplane.AssertResourceCount(t, resources, "User", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 3)

	t.Log("Validating protection.crossplane.io instance-protection Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "user-example-instance-protection",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.by.apiVersion":                    mySqlApiVersion,
		"spec.by.kind":                          "User",
		"spec.by.resourceRef.name":              "user-example",
		"spec.of.apiVersion":                    "database.entigo.com/v1alpha1",
		"spec.of.kind":                          "MariaDBInstance",
		"spec.of.resourceRef.name":              "mariadb-example",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "user-example-instance-protection" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting database.entigo.com MariaDBUser Ready Status")
	crossplane.AssertResourceReady(t, resources, "MariaDBUser", "database.entigo.com/v1alpha1")
}

func testDatabaseCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendToResources(t, extra, mariaDBInstanceExtraResource())

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)

	t.Log("Validating database.entigo.com MariaDBDatabase fields")
	crossplane.AssertFieldValues(t, resources, "MariaDBDatabase", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "example-db",
		"spec.instanceRef.name": "mariadb-example",
	})

	t.Log("Validating mysql.sql.m.crossplane.io Database fields")
	crossplane.AssertFieldValues(t, resources, "Database", mySqlApiVersion, map[string]string{
		"metadata.name":                                      "example-db",
		"metadata.ownerReferences.0.apiVersion":              "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                    "MariaDBDatabase",
		"metadata.ownerReferences.0.name":                    "example-db",
		"metadata.annotations.crossplane\\.io/external-name": "example-db",
		"spec.providerConfigRef.name":                        "mariadb-example-providerconfig",
	})

	t.Log("Mocking observed resources")
	mockedDatabase := crossplane.MockByKind(t, resources, "Database", mySqlApiVersion, true, nil)
	crossplane.AppendToResources(t, observed, mockedDatabase)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "MariaDBDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 1)

	t.Log("Validating protection.crossplane.io instance-protection Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "example-db-instance-protection",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "MariaDBDatabase",
		"metadata.ownerReferences.0.name":       "example-db",
		"spec.by.apiVersion":                    mySqlApiVersion,
		"spec.by.kind":                          "Database",
		"spec.by.resourceRef.name":              "example-db",
		"spec.of.apiVersion":                    "database.entigo.com/v1alpha1",
		"spec.of.kind":                          "MariaDBInstance",
		"spec.of.resourceRef.name":              "mariadb-example",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "example-db-instance-protection" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting database.entigo.com MariaDBDatabase Ready Status")
	crossplane.AssertResourceReady(t, resources, "MariaDBDatabase", "database.entigo.com/v1alpha1")
}

// mariaDBInstanceExtraResource creates a mock MariaDBInstance resource for use as an extra resource.
func mariaDBInstanceExtraResource() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "database.entigo.com/v1alpha1",
			"kind":       "MariaDBInstance",
			"metadata": map[string]interface{}{
				"name": "mariadb-example",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			},
		},
	}
}

// mariaDBDatabaseExtraResource creates a mock MariaDBDatabase resource for use as an extra resource.
func mariaDBDatabaseExtraResource() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "database.entigo.com/v1alpha1",
			"kind":       "MariaDBDatabase",
			"metadata": map[string]interface{}{
				"name": "example-db",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			},
		},
	}
}
