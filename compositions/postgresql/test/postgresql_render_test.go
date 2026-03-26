package test

import (
	"path/filepath"
	"testing"

	"github.com/entigolabs/crossplane-common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	env             = "../examples/environment-config.yaml"
	function        = "../../../functions/database"
	functionsConfig = "../../../test/common/functions-dev.yaml"
	required        = "../examples/required-resources.yaml"

	instanceComposition = "../apis/instance-composition.yaml"
	instanceResource    = "../examples/instance.yaml"

	databaseComposition = "../apis/database-composition.yaml"
	databaseResource    = "../examples/database.yaml"

	userComposition       = "../apis/user-composition.yaml"
	userWithGrantResource = "../examples/user-with-role-grant.yaml"
)

func TestPostgreSQLCrossplaneRender(t *testing.T) {
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
	mockedInstance := crossplane.MockByKind(t, instanceUnstructured, "PostgreSQLInstance", "database.entigo.com/v1alpha1", false, map[string]interface{}{
		"metadata.uid":            "000000000000",
		"spec.snapshotIdentifier": "postgresql-instance-test-instance-snapshot",
	})
	crossplane.AppendToResources(t, tempInstanceResource, mockedInstance)

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, tempInstanceResource, instanceComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLInstance", 1)
	crossplane.AssertResourceCount(t, resources, "ProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)

	t.Log("Validating database.entigo.com PostgreSQLInstance fields")
	crossplane.AssertFieldValues(t, resources, "PostgreSQLInstance", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "postgresql-example",
		"spec.allocatedStorage": "20",
		"spec.engineVersion":    "17.2",
		"spec.instanceType":     "db.t3.micro",
	})

	t.Log("Validating postgresql.sql.m.crossplane.io ProviderConfig fields")
	crossplane.AssertFieldValues(t, resources, "ProviderConfig", "postgresql.sql.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                             "postgresql-example-providerconfig",
		"metadata.ownerReferences.0.apiVersion":     "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":           "PostgreSQLInstance",
		"metadata.ownerReferences.0.name":           "postgresql-example",
		"spec.credentials.connectionSecretRef.name": "postgresql-example-dbadmin",
		"spec.credentials.source":                   "PostgreSQLConnectionSecret",
		"spec.sslMode":                              "require",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroup fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                         "postgresql-example-sg-86d8a475",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLInstance",
		"metadata.ownerReferences.0.name":       "postgresql-example",
		"spec.forProvider.region":               "eu-north-1",
		"spec.forProvider.vpcIdRef.name":        "vpc",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroupRule fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroupRule", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                            "postgresql-example-sg-egress-86d8a475",
		"metadata.ownerReferences.0.apiVersion":    "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":          "PostgreSQLInstance",
		"metadata.ownerReferences.0.name":          "postgresql-example",
		"spec.forProvider.cidrBlocks.0":            "0.0.0.0/0",
		"spec.forProvider.region":                  "eu-north-1",
		"spec.forProvider.securityGroupIdRef.name": "postgresql-example-sg-86d8a475",
		"spec.forProvider.type":                    "egress",
	})

	t.Log("Validating ec2.aws.m.upbound.io SecurityGroupRule fields")
	crossplane.AssertFieldValues(t, resources, "SecurityGroupRule", "ec2.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                            "postgresql-example-sg-ingress-86d8a475",
		"metadata.ownerReferences.0.apiVersion":    "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":          "PostgreSQLInstance",
		"metadata.ownerReferences.0.name":          "postgresql-example",
		"spec.forProvider.cidrBlocks.0":            "0.0.0.0/0",
		"spec.forProvider.region":                  "eu-north-1",
		"spec.forProvider.securityGroupIdRef.name": "postgresql-example-sg-86d8a475",
		"spec.forProvider.type":                    "ingress",
	})

	t.Log("Mocking observed resources")
	mockedSecurityGroup := crossplane.MockByKind(t, resources, "SecurityGroup", "ec2.aws.m.upbound.io/v1beta1", true, nil)
	mockedProviderConfig := crossplane.MockByKind(t, resources, "ProviderConfig", "postgresql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedSecurityGroup, mockedProviderConfig)
	for _, res := range resources {
		if res.GetKind() == "SecurityGroupRule" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, tempInstanceResource, instanceComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLInstance", 1)
	crossplane.AssertResourceCount(t, resources, "ProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)
	crossplane.AssertResourceCount(t, resources, "Instance", 1)

	t.Log("Validating rds.aws.m.upbound.io Instance fields")
	crossplane.AssertFieldValues(t, resources, "Instance", "rds.aws.m.upbound.io/v1beta1", map[string]string{
		"metadata.name":                                     "postgresql-example-instance-86d8a475",
		"metadata.ownerReferences.0.apiVersion":             "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":                   "PostgreSQLInstance",
		"metadata.ownerReferences.0.name":                   "postgresql-example",
		"spec.forProvider.allocatedStorage":                 "20",
		"spec.forProvider.backupRetentionPeriod":            "14",
		"spec.forProvider.dbSubnetGroupNameRef.name":        "database",
		"spec.forProvider.dbSubnetGroupNameRef.namespace":   "crossplane-aws",
		"spec.forProvider.finalSnapshotIdentifier":          "postgresql-example-instance-snapshot-86d8a475",
		"spec.forProvider.kmsKeyIdRef.name":                 "data",
		"spec.forProvider.masterUserSecretKmsKeyIdRef.name": "config",
		"spec.forProvider.vpcSecurityGroupIdRefs.0.name":    "postgresql-example-sg-86d8a475",
		"spec.forProvider.snapshotIdentifier":               "postgresql-instance-test-instance-snapshot",
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
	crossplane.AssertResourceCount(t, resources, "PostgreSQLInstance", 1)
	crossplane.AssertResourceCount(t, resources, "ProviderConfig", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroup", 1)
	crossplane.AssertResourceCount(t, resources, "SecurityGroupRule", 2)
	crossplane.AssertResourceCount(t, resources, "Instance", 1)
	crossplane.AssertResourceCount(t, resources, "ExternalSecret", 1)

	t.Log("Validating external-secrets.io ExternalSecret fields")
	crossplane.AssertFieldValues(t, resources, "ExternalSecret", "external-secrets.io/v1", map[string]string{
		"metadata.name":                         "postgresql-example-es-86d8a475",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLInstance",
		"metadata.ownerReferences.0.name":       "postgresql-example",
		"spec.data.0.remoteRef.key":             "arn:aws:kms:eu-north-1:012345678901:key/mrk-1",
		"spec.data.0.remoteRef.property":        "password",
		"spec.data.0.remoteRef.version":         "AWSCURRENT",
		"spec.data.0.secretKey":                 "password",
		"spec.secretStoreRef.kind":              "ClusterSecretStore",
		"spec.secretStoreRef.name":              "external-secrets",
		"spec.target.name":                      "postgresql-example-dbadmin",
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

	t.Log("Asserting database.entigo.com PostgreSQLInstance Ready Status")
	crossplane.AssertResourceReady(t, resources, "PostgreSQLInstance", "database.entigo.com/v1alpha1")
}

func testDatabaseCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendToResources(t, extra, pgOwnerRoleExtraResource())

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)

	t.Log("Validating database.entigo.com PostgreSQLDatabase fields")
	crossplane.AssertFieldValues(t, resources, "PostgreSQLDatabase", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":                       "database-example",
		"spec.dbTemplate":                     "template0",
		"spec.encoding":                       "UTF8",
		"spec.deletionProtection":             "true",
		"spec.extensions.0":                   "postgis",
		"spec.extensionConfig.postgis.schema": "topology",
		"spec.instanceRef.name":               "postgresql-example",
		"spec.lcCType":                        "et_EE.UTF-8",
		"spec.lcCollate":                      "et_EE.UTF-8",
		"spec.owner":                          "owner",
	})

	t.Log("Validating postgresql.sql.m.crossplane.io Grant fields")
	crossplane.AssertFieldValues(t, resources, "Grant", "postgresql.sql.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "database-example-grant-owner-to-dbadmin",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLDatabase",
		"metadata.ownerReferences.0.name":       "database-example",
		"spec.forProvider.memberOf":             "owner",
		"spec.forProvider.role":                 "dbadmin",
	})

	t.Log("Mocking observed resources")
	mockedGrant := crossplane.MockByKind(t, resources, "Grant", "postgresql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedGrant)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)

	t.Log("Validating postgresql.sql.m.crossplane.io Database fields")
	crossplane.AssertFieldValues(t, resources, "Database", "postgresql.sql.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "database-example",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLDatabase",
		"metadata.ownerReferences.0.name":       "database-example",
		"spec.forProvider.template":             "template0",
		"spec.forProvider.encoding":             "UTF8",
		"spec.forProvider.lcCType":              "et_EE.UTF-8",
		"spec.forProvider.lcCollate":            "et_EE.UTF-8",
		"spec.forProvider.owner":                "owner",
	})

	t.Log("Mocking observed resources")
	mockedDatabase := crossplane.MockByKind(t, resources, "Database", "postgresql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedDatabase)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)
	crossplane.AssertResourceCount(t, resources, "Extension", 1)

	t.Log("Validating postgresql.sql.m.crossplane.io Extension fields")
	crossplane.AssertFieldValues(t, resources, "Extension", "postgresql.sql.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "database-example-postgis",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLDatabase",
		"metadata.ownerReferences.0.name":       "database-example",
		"spec.forProvider.database":             "database-example",
		"spec.forProvider.extension":            "postgis",
		"spec.forProvider.schema":               "topology",
	})

	t.Log("Mocking observed resources")
	mockedExtension := crossplane.MockByKind(t, resources, "Extension", "postgresql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedExtension)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)
	crossplane.AssertResourceCount(t, resources, "Extension", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 1)

	t.Log("Validating protection.crossplane.io Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "database-example-grant-usage",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLDatabase",
		"metadata.ownerReferences.0.name":       "database-example",
		"spec.by.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.by.kind":                          "Database",
		"spec.by.resourceRef.name":              "database-example",
		"spec.of.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.of.kind":                          "Grant",
		"spec.of.resourceRef.name":              "database-example-grant-owner-to-dbadmin",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	mockedUsage := crossplane.MockByKind(t, resources, "Usage", "protection.crossplane.io/v1beta1", true, nil)
	crossplane.AppendToResources(t, observed, mockedUsage)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)
	crossplane.AssertResourceCount(t, resources, "Extension", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 2)

	t.Log("Validating protection.crossplane.io Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "database-example-owner-protection",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLDatabase",
		"metadata.ownerReferences.0.name":       "database-example",
		"spec.by.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.by.kind":                          "Database",
		"spec.by.resourceRef.name":              "database-example",
		"spec.of.apiVersion":                    "database.entigo.com/v1alpha1",
		"spec.of.kind":                          "PostgreSQLUser",
		"spec.of.resourceRef.name":              "owner",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "database-example-owner-protection" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLDatabase", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Database", 1)
	crossplane.AssertResourceCount(t, resources, "Extension", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 3)

	t.Log("Validating protection.crossplane.io Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "database-example-instance-protection",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLDatabase",
		"metadata.ownerReferences.0.name":       "database-example",
		"spec.by.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.by.kind":                          "Database",
		"spec.by.resourceRef.name":              "database-example",
		"spec.of.apiVersion":                    "database.entigo.com/v1alpha1",
		"spec.of.kind":                          "PostgreSQLInstance",
		"spec.of.resourceRef.name":              "postgresql-example",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "database-example-instance-protection" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, databaseResource, databaseComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting database.entigo.com PostgreSQLDatabase Ready Status")
	crossplane.AssertResourceReady(t, resources, "PostgreSQLDatabase", "database.entigo.com/v1alpha1")
}

func testUserCrossplaneRender(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	observed := filepath.Join(tmpDir, "observed.yaml")

	crossplane.AppendToResources(t, extra, pgInstanceExtraResource())

	t.Log("Rendering...")
	resources := crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), nil)

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLUser", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)

	t.Log("Validating database.entigo.com PostgreSQLUser fields")
	crossplane.AssertFieldValues(t, resources, "PostgreSQLUser", "database.entigo.com/v1alpha1", map[string]string{
		"metadata.name":         "user-example",
		"spec.grant.roles.0":    "example-role",
		"spec.login":            "true",
		"spec.inherit":          "true",
		"spec.instanceRef.name": "postgresql-example",
		"spec.name":             "user_example",
	})

	t.Log("Validating postgresql.sql.m.crossplane.io Role fields")
	crossplane.AssertFieldValues(t, resources, "Role", "postgresql.sql.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                          "user-example",
		"metadata.ownerReferences.0.apiVersion":  "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":        "PostgreSQLUser",
		"metadata.ownerReferences.0.name":        "user-example",
		"spec.forProvider.privileges.createDb":   "false",
		"spec.forProvider.privileges.createRole": "false",
		"spec.forProvider.privileges.login":      "true",
		"spec.forProvider.privileges.inherit":    "true",
		"spec.writeConnectionSecretToRef.name":   "postgresql-example-user-example",
	})

	t.Log("Mocking observed resources")
	mockedRole := crossplane.MockByKind(t, resources, "Role", "postgresql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedRole)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLUser", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)

	t.Log("Validating postgresql.sql.m.crossplane.io Grant fields")
	crossplane.AssertFieldValues(t, resources, "Grant", "postgresql.sql.m.crossplane.io/v1alpha1", map[string]string{
		"metadata.name":                         "grant-user-example-example-role-postgresql-example",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.forProvider.memberOf":             "example-role",
		"spec.forProvider.role":                 "user_example",
	})

	t.Log("Mocking observed resources")
	mockedGrant := crossplane.MockByKind(t, resources, "Grant", "postgresql.sql.m.crossplane.io/v1alpha1", true, nil)
	crossplane.AppendToResources(t, observed, mockedGrant)

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLUser", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 1)

	t.Log("Validating protection.crossplane.io Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "usage-grant-user-example-example-role-postgresql-example",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.by.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.by.kind":                          "Grant",
		"spec.by.resourceRef.name":              "grant-user-example-example-role-postgresql-example",
		"spec.of.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.of.kind":                          "Role",
		"spec.of.resourceRef.name":              "user-example",
		"spec.replayDeletion":                   "true",
	})

	t.Log("Mocking observed resources")
	for _, res := range resources {
		if res.GetKind() == "Usage" && res.GetAPIVersion() == "protection.crossplane.io/v1beta1" && res.GetName() == "usage-grant-user-example-example-role-postgresql-example" {
			crossplane.AppendToResources(t, observed, crossplane.Mock(t, res, true, nil))
		}
	}

	t.Log("Rendering...")
	resources = crossplane.CrossplaneRender(t, userWithGrantResource, userComposition, functionsConfig, crossplane.Ptr(extra), crossplane.Ptr(observed))

	t.Log("Asserting rendered resources count")
	crossplane.AssertResourceCount(t, resources, "PostgreSQLUser", 1)
	crossplane.AssertResourceCount(t, resources, "Role", 1)
	crossplane.AssertResourceCount(t, resources, "Grant", 1)
	crossplane.AssertResourceCount(t, resources, "Usage", 2)

	t.Log("Validating protection.crossplane.io Usage fields")
	crossplane.AssertFieldValues(t, resources, "Usage", "protection.crossplane.io/v1beta1", map[string]string{
		"metadata.name":                         "user-example-instance-protection",
		"metadata.ownerReferences.0.apiVersion": "database.entigo.com/v1alpha1",
		"metadata.ownerReferences.0.kind":       "PostgreSQLUser",
		"metadata.ownerReferences.0.name":       "user-example",
		"spec.by.apiVersion":                    "postgresql.sql.m.crossplane.io/v1alpha1",
		"spec.by.kind":                          "Role",
		"spec.by.resourceRef.name":              "user-example",
		"spec.of.apiVersion":                    "database.entigo.com/v1alpha1",
		"spec.of.kind":                          "PostgreSQLInstance",
		"spec.of.resourceRef.name":              "postgresql-example",
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

	t.Log("Asserting database.entigo.com PostgreSQLUser Ready Status")
	crossplane.AssertResourceReady(t, resources, "PostgreSQLUser", "database.entigo.com/v1alpha1")
}

// pgOwnerRoleExtraResource creates a mock pg owner Role resource for use as an extra resource.
func pgOwnerRoleExtraResource() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.sql.m.crossplane.io/v1alpha1",
			"kind":       "Role",
			"metadata": map[string]interface{}{
				"name": "owner",
				"labels": map[string]interface{}{
					"database.entigo.com/role-name": "owner",
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			},
		},
	}
}

// pgInstanceExtraResource creates a mock PostgreSQLInstance resource for use as an extra resource.
func pgInstanceExtraResource() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "database.entigo.com/v1alpha1",
			"kind":       "PostgreSQLInstance",
			"metadata": map[string]interface{}{
				"name": "postgresql-example",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			},
		},
	}
}
