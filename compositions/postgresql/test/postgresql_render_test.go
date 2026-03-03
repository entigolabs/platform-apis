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

func TestInstanceStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	xptest.StartFunction(ctx, t, filepath.Join(xptest.WorkspaceRoot(), "functions", "database"), "9443")

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/instance.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}
	_ = unstructured.SetNestedField(xr.Object, "000000000000", "metadata", "uid")

	comp, err := render.LoadComposition(fs, "../apis/instance-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DevFunctions("platform-apis-database-fn")

	envConfig, err := xptest.LoadUnstructured("../examples/environment-config.yaml")
	if err != nil {
		t.Fatalf("cannot load environment config: %v", err)
	}
	required, err := xptest.LoadUnstructuredMulti("../examples/required-resources.yaml")
	if err != nil {
		t.Fatalf("cannot load required resources: %v", err)
	}
	extraResources := append([]unstructured.Unstructured{envConfig}, required...)

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering SecurityGroup and SecurityGroupRules")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "SecurityGroup", 1, "SecurityGroupRule", 2)

	t.Log("TEST 1: asserting SecurityGroup fields")
	sg := xptest.FindResourceByKind(t, out1.ComposedResources, "SecurityGroup")
	if sg != nil {
		xptest.AssertNestedString(t, sg.Object, "allow traffic from vpc", "spec", "forProvider", "description")
		xptest.AssertNestedString(t, sg.Object, "ClusterProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, sg.Object, "crossplane-aws", "spec", "providerConfigRef", "name")
	}

	t.Log("TEST 1: asserting SecurityGroupRule fields")
	ingress := findSGRuleByType(t, out1.ComposedResources, "ingress")
	if ingress != nil {
		xptest.AssertNestedString(t, ingress.Object, "tcp", "spec", "forProvider", "protocol")
		assertNestedNumber(t, ingress.Object, 5432, "spec", "forProvider", "fromPort")
		assertNestedNumber(t, ingress.Object, 5432, "spec", "forProvider", "toPort")
	}
	egress := findSGRuleByType(t, out1.ComposedResources, "egress")
	if egress != nil {
		xptest.AssertNestedString(t, egress.Object, "-1", "spec", "forProvider", "protocol")
	}

	sgObserved := mockSGAsObserved(t, out1.ComposedResources)

	t.Log("TEST 2: rendering RDS Instance")
	out2, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: sgObserved,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out2, "Instance", 1)

	t.Log("TEST 2: asserting Instance fields")
	inst := xptest.FindResourceByKind(t, out2.ComposedResources, "Instance")
	if inst != nil {
		xptest.AssertNestedString(t, inst.Object, "postgres", "spec", "forProvider", "engine")
		xptest.AssertNestedString(t, inst.Object, "17.2", "spec", "forProvider", "engineVersion")
		xptest.AssertNestedString(t, inst.Object, "db.t3.micro", "spec", "forProvider", "instanceClass")
		xptest.AssertNestedString(t, inst.Object, "dbadmin", "spec", "forProvider", "username")
		xptest.AssertNestedString(t, inst.Object, "gp3", "spec", "forProvider", "storageType")
		xptest.AssertNestedString(t, inst.Object, "eu-north-1", "spec", "forProvider", "region")
		xptest.AssertNestedBool(t, inst.Object, true, "spec", "forProvider", "storageEncrypted")
		xptest.AssertNestedBool(t, inst.Object, false, "spec", "forProvider", "publiclyAccessible")
		assertNestedNumber(t, inst.Object, 20, "spec", "forProvider", "allocatedStorage")
		assertNestedNumber(t, inst.Object, 14, "spec", "forProvider", "backupRetentionPeriod")
	}

	instanceObserved := mockInstanceAsObserved(t, inst)

	t.Log("TEST 3: rendering ExternalSecret and ProviderConfig")
	out3, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: append(sgObserved, instanceObserved),
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out3, "ExternalSecret", 1, "ProviderConfig", 1)

	t.Log("TEST 3: asserting ProviderConfig fields")
	pc := xptest.FindResource(t, out3.ComposedResources, "ProviderConfig", "postgresql-example-providerconfig")
	if pc != nil {
		xptest.AssertNestedString(t, pc.Object, "PostgreSQLConnectionSecret", "spec", "credentials", "source")
		xptest.AssertNestedString(t, pc.Object, "postgresql-example-dbadmin", "spec", "credentials", "connectionSecretRef", "name")
		xptest.AssertNestedString(t, pc.Object, "require", "spec", "sslMode")
	}

	t.Log("TEST 3: asserting ExternalSecret fields")
	es := xptest.FindResourceByKind(t, out3.ComposedResources, "ExternalSecret")
	if es != nil {
		xptest.AssertNestedString(t, es.Object, "ClusterSecretStore", "spec", "secretStoreRef", "kind")
		xptest.AssertNestedString(t, es.Object, "external-secrets", "spec", "secretStoreRef", "name")
		xptest.AssertNestedString(t, es.Object, "postgresql-example-dbadmin", "spec", "target", "name")
	}

	espcObserved := mockReadyAsObserved(t, out3.ComposedResources, "ExternalSecret", "ProviderConfig")

	t.Log("TEST 4: checking PostgreSQLInstance readiness")
	out4, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
		ExtraResources:    extraResources,
		ObservedResources: append(append(sgObserved, instanceObserved), espcObserved...),
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertReady(t, out4.CompositeResource)
}

func TestDatabaseStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/database.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/database-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DockerFunctionsFromHelm(t, xptest.HelmValues(), "function-go-templating", "function-auto-ready")

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Grant, Database, Extension and Usage")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Grant", 1, "Database", 1, "Extension", 1, "Usage", 1)

	t.Log("TEST 1: asserting Grant fields")
	grant := xptest.FindResource(t, out1.ComposedResources, "Grant", "database-example-grant-owner-to-dbadmin")
	if grant != nil {
		xptest.AssertNestedString(t, grant.Object, "dbadmin", "spec", "forProvider", "role")
		xptest.AssertNestedString(t, grant.Object, "owner", "spec", "forProvider", "memberOf")
		xptest.AssertNestedString(t, grant.Object, "ProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, grant.Object, "postgresql-example-providerconfig", "spec", "providerConfigRef", "name")
	}

	t.Log("TEST 1: asserting Database fields")
	db := xptest.FindResource(t, out1.ComposedResources, "Database", "database-example")
	if db != nil {
		xptest.AssertNestedString(t, db.Object, "owner", "spec", "forProvider", "owner")
		xptest.AssertNestedString(t, db.Object, "UTF8", "spec", "forProvider", "encoding")
		xptest.AssertNestedString(t, db.Object, "et_EE.UTF-8", "spec", "forProvider", "lcCType")
		xptest.AssertNestedString(t, db.Object, "et_EE.UTF-8", "spec", "forProvider", "lcCollate")
		xptest.AssertNestedString(t, db.Object, "template0", "spec", "forProvider", "template")
		xptest.AssertNestedString(t, db.Object, "ProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, db.Object, "postgresql-example-providerconfig", "spec", "providerConfigRef", "name")
	}

	t.Log("TEST 1: asserting Extension fields")
	ext := xptest.FindResource(t, out1.ComposedResources, "Extension", "database-example-postgis")
	if ext != nil {
		xptest.AssertNestedString(t, ext.Object, "postgis", "spec", "forProvider", "extension")
		xptest.AssertNestedString(t, ext.Object, "database-example", "spec", "forProvider", "database")
		xptest.AssertNestedString(t, ext.Object, "ProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, ext.Object, "postgresql-example-providerconfig", "spec", "providerConfigRef", "name")
	}

	t.Log("TEST 1: asserting Usage fields")
	usage := xptest.FindResource(t, out1.ComposedResources, "Usage", "database-example-grant-usage")
	if usage != nil {
		xptest.AssertNestedBool(t, usage.Object, true, "spec", "replayDeletion")
		xptest.AssertNestedString(t, usage.Object, "Grant", "spec", "of", "kind")
		xptest.AssertNestedString(t, usage.Object, "database-example-grant-owner-to-dbadmin", "spec", "of", "resourceRef", "name")
		xptest.AssertNestedString(t, usage.Object, "Database", "spec", "by", "kind")
		xptest.AssertNestedString(t, usage.Object, "database-example", "spec", "by", "resourceRef", "name")
	}
}

func TestDatabaseWithExtensionConfigStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/database-with-extension-config.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/database-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DockerFunctionsFromHelm(t, xptest.HelmValues(), "function-go-templating", "function-auto-ready")

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Extension with extensionConfig schema")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Grant", 1, "Database", 1, "Extension", 1, "Usage", 1)

	t.Log("TEST 1: asserting Extension with schema")
	ext := xptest.FindResource(t, out1.ComposedResources, "Extension", "database-extconfig-example-postgis")
	if ext != nil {
		xptest.AssertNestedString(t, ext.Object, "postgis", "spec", "forProvider", "extension")
		xptest.AssertNestedString(t, ext.Object, "database-extconfig-example", "spec", "forProvider", "database")
		xptest.AssertNestedString(t, ext.Object, "public", "spec", "forProvider", "schema")
		xptest.AssertNestedString(t, ext.Object, "ProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, ext.Object, "postgresql-example-providerconfig", "spec", "providerConfigRef", "name")
	}
}

func TestUserStatic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	fs := afero.NewOsFs()

	xr, err := render.LoadCompositeResource(fs, "../examples/user-with-role-grant.yaml")
	if err != nil {
		t.Fatalf("cannot load composite resource: %v", err)
	}

	comp, err := render.LoadComposition(fs, "../apis/user-composition.yaml")
	if err != nil {
		t.Fatalf("cannot load composition: %v", err)
	}

	fns := xptest.DockerFunctionsFromHelm(t, xptest.HelmValues(), "function-go-templating", "function-auto-ready")

	log := logging.NewNopLogger()

	t.Log("TEST 1: rendering Grant, Role and Usage")
	out1, err := render.Render(ctx, log, render.Inputs{
		CompositeResource: xr,
		Composition:       comp,
		Functions:         fns,
	})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	xptest.AssertCounts(t, out1, "Grant", 1, "Role", 1, "Usage", 1)

	t.Log("TEST 1: asserting Role fields")
	role := xptest.FindResource(t, out1.ComposedResources, "Role", "user-example")
	if role != nil {
		xptest.AssertNestedString(t, role.Object, "user_example", "metadata", "annotations", "crossplane.io/external-name")
		xptest.AssertNestedString(t, role.Object, "postgresql-example-user-example", "spec", "writeConnectionSecretToRef", "name")
		xptest.AssertNestedString(t, role.Object, "ProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, role.Object, "postgresql-example-providerconfig", "spec", "providerConfigRef", "name")
	}

	t.Log("TEST 1: asserting Grant fields")
	grant := xptest.FindResource(t, out1.ComposedResources, "Grant", "grant-user-example-example-role-postgresql-example")
	if grant != nil {
		xptest.AssertNestedString(t, grant.Object, "user_example", "spec", "forProvider", "role")
		xptest.AssertNestedString(t, grant.Object, "example-role", "spec", "forProvider", "memberOf")
		xptest.AssertNestedString(t, grant.Object, "ProviderConfig", "spec", "providerConfigRef", "kind")
		xptest.AssertNestedString(t, grant.Object, "postgresql-example-providerconfig", "spec", "providerConfigRef", "name")
	}

	t.Log("TEST 1: asserting Usage fields")
	usage := xptest.FindResource(t, out1.ComposedResources, "Usage", "usage-grant-user-example-example-role-postgresql-example")
	if usage != nil {
		xptest.AssertNestedBool(t, usage.Object, true, "spec", "replayDeletion")
		xptest.AssertNestedString(t, usage.Object, "Role", "spec", "of", "kind")
		xptest.AssertNestedString(t, usage.Object, "user-example", "spec", "of", "resourceRef", "name")
		xptest.AssertNestedString(t, usage.Object, "Grant", "spec", "by", "kind")
		xptest.AssertNestedString(t, usage.Object, "grant-user-example-example-role-postgresql-example", "spec", "by", "resourceRef", "name")
	}
}

func findSGRuleByType(t *testing.T, resources []xptest.ComposedUnstructured, ruleType string) *xptest.ComposedUnstructured {
	t.Helper()
	for i := range resources {
		if resources[i].GetKind() != "SecurityGroupRule" {
			continue
		}
		val, _, _ := unstructured.NestedString(resources[i].Object, "spec", "forProvider", "type")
		if val == ruleType {
			return &resources[i]
		}
	}
	t.Errorf("SecurityGroupRule type=%q not found", ruleType)
	return nil
}

func assertNestedNumber(t *testing.T, obj map[string]interface{}, expected float64, fields ...string) {
	t.Helper()
	val, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil {
		t.Errorf("field %v: error: %v", fields, err)
		return
	}
	if !found {
		t.Errorf("field %v: not found", fields)
		return
	}
	var got float64
	switch v := val.(type) {
	case float64:
		got = v
	case int64:
		got = float64(v)
	case int:
		got = float64(v)
	default:
		t.Errorf("field %v: expected number, got %T(%v)", fields, val, val)
		return
	}
	if got != expected {
		t.Errorf("field %v: expected %v, got %v", fields, expected, got)
	}
}

func mockSGAsObserved(t *testing.T, resources []xptest.ComposedUnstructured) []xptest.ComposedUnstructured {
	t.Helper()
	var observed []xptest.ComposedUnstructured
	for _, r := range resources {
		if r.GetKind() == "SecurityGroup" || r.GetKind() == "SecurityGroupRule" {
			clone := xptest.CloneComposed(t, r)
			_ = unstructured.SetNestedMap(clone.Object, map[string]interface{}{
				"atProvider": map[string]interface{}{
					"securityGroupId": "sg-mock-123",
				},
				"conditions": []interface{}{
					map[string]interface{}{"type": "Synced", "status": "True"},
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			}, "status")
			observed = append(observed, clone)
		}
	}
	return observed
}

func mockInstanceAsObserved(t *testing.T, inst *xptest.ComposedUnstructured) xptest.ComposedUnstructured {
	t.Helper()
	clone := xptest.CloneComposed(t, *inst)
	_ = unstructured.SetNestedMap(clone.Object, map[string]interface{}{
		"atProvider": map[string]interface{}{
			"status":       "Available",
			"address":      "mock-db.cluster-123.eu-north-1.rds.amazonaws.com",
			"port":         float64(5432),
			"hostedZoneId": "mock-zone",
			"masterUserSecret": []interface{}{
				map[string]interface{}{
					"secretArn":    "arn:aws:kms:eu-north-1:012345678901:key/mrk-1",
					"secretStatus": "active",
				},
			},
		},
		"conditions": []interface{}{
			map[string]interface{}{"type": "Synced", "status": "True"},
			map[string]interface{}{"type": "Ready", "status": "True"},
		},
	}, "status")
	return clone
}

func mockReadyAsObserved(t *testing.T, resources []xptest.ComposedUnstructured, kinds ...string) []xptest.ComposedUnstructured {
	t.Helper()
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}
	var observed []xptest.ComposedUnstructured
	for _, r := range resources {
		if kindSet[r.GetKind()] {
			clone := xptest.CloneComposed(t, r)
			_ = unstructured.SetNestedMap(clone.Object, map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Ready", "status": "True"},
				},
			}, "status")
			observed = append(observed, clone)
		}
	}
	return observed
}
