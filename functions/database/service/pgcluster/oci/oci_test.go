package oci

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha2"
	"github.com/entigolabs/platform-apis/service/pgcluster"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func obj(apiVersion, kind, name string, labels map[string]interface{}, atProvider map[string]interface{}) *unstructured.Unstructured {
	meta := map[string]interface{}{"name": name}
	if labels != nil {
		meta["labels"] = labels
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion, "kind": kind, "metadata": meta,
		"status": map[string]interface{}{"atProvider": atProvider},
	}}
}

// required is what an OCI deployment delivers: its settings and the infralib observers.
func required() map[string][]resource.Required {
	return map[string][]resource.Required{
		base.EnvironmentKey: {{Resource: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": base.EnvironmentApiVersion, "kind": base.EnvironmentKind,
			"data": map[string]interface{}{
				"postgresqlBackends": []interface{}{Kind},
				"oci": map[string]interface{}{
					"providerConfig": "crossplane-oracle", "cluster": "oke", "vcn": "vcn",
					"databaseSubnetType": "database", "clientSecurityGroups": []interface{}{"node", "pod"},
				},
			},
		}}}},
		keyCluster: {{Resource: obj(observerContainer, "Cluster", "oke", nil, map[string]interface{}{"compartmentId": "ocid1.compartment.oc1..zone"})}},
		keyVCN:     {{Resource: obj(observerNetworking, "Vcn", "vcn", nil, map[string]interface{}{"id": "ocid1.vcn.oc1..vcn"})}},
		keySubnets: {
			{Resource: obj(observerNetworking, "Subnet", "database-1", map[string]interface{}{subnetTypeLabel: "database"}, map[string]interface{}{"id": "ocid1.subnet.oc1..b"})},
			{Resource: obj(observerNetworking, "Subnet", "database-0", map[string]interface{}{subnetTypeLabel: "database"}, map[string]interface{}{"id": "ocid1.subnet.oc1..a"})},
		},
		keyClientNSG + "node": {{Resource: obj(observerNetworking, "NetworkSecurityGroup", "node", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..node"})}},
		keyClientNSG + "pod":  {{Resource: obj(observerNetworking, "NetworkSecurityGroup", "pod", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..pod"})}},
	}
}

func ptr[T any](v T) *T { return &v }

func cluster(entry string) *v1alpha2.PostgreSQLCluster {
	return &v1alpha2.PostgreSQLCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-db", Namespace: "shop", UID: "uid-1"},
		Spec: v1alpha2.PostgreSQLClusterSpec{
			Version: ptr("17"),
			Storage: &v1alpha2.Storage{SizeGB: ptr(int64(20))},
			ProviderConfigs: []v1alpha2.ProviderConfigEntry{{
				APIVersion: "oci.database.entigo.com/v1alpha1", Kind: "PostgreSQLOCIConfig",
				Spec: &runtime.RawExtension{Raw: []byte(entry)},
			}},
		},
	}
}

const flexEntry = `{"instanceType":"PostgreSQL.VM.Standard.E5.Flex","ocpus":2,"memoryGB":32}`

func observedComposed(name string, u *unstructured.Unstructured) map[resource.Name]resource.ObservedComposed {
	return map[resource.Name]resource.ObservedComposed{resource.Name(name): {Resource: &composed.Unstructured{Unstructured: *u}}}
}

func field(t *testing.T, o client.Object, path ...string) interface{} {
	t.Helper()
	v, found, err := unstructured.NestedFieldNoCopy(o.(*unstructured.Unstructured).Object, path...)
	if err != nil || !found {
		t.Fatalf("%s %s has no %s", o.GetObjectKind().GroupVersionKind().Kind, o.GetName(), strings.Join(path, "."))
	}
	return v
}

func first(t *testing.T, o client.Object, path ...string) map[string]interface{} {
	t.Helper()
	return field(t, o, path...).([]interface{})[0].(map[string]interface{})
}

func TestComposeBeforeTheSecurityGroupExists(t *testing.T) {
	objs, err := pgcluster.Generate(cluster(flexEntry), required(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{nameNSG, nameRule + "0", nameRule + "1", namePassword} {
		if objs[name] == nil {
			t.Errorf("want %s composed", name)
		}
	}
	if objs[nameDBSystem] != nil || objs[nameSecret] != nil {
		t.Error("the DB system waits for its security group's OCID, and the admin Secret for its address")
	}
	if got := field(t, objs[nameNSG], "spec", "forProvider", "compartmentId"); got != "ocid1.compartment.oc1..zone" {
		t.Errorf("want the cluster's compartment, got %v", got)
	}
	rule := objs[nameRule+"1"]
	if field(t, rule, "spec", "forProvider", "source") != "ocid1.nsg.oc1..pod" ||
		first(t, rule, "spec", "forProvider", "tcpOptions")["destinationPortRange"].([]interface{})[0].(map[string]interface{})["min"] != port {
		t.Errorf("want port %d from the pod security group, got %v", port, rule)
	}
	if field(t, objs[nameNSG], "spec", "providerConfigRef", "kind") != "ClusterProviderConfig" {
		t.Error("want the cluster-wide provider config")
	}
}

func TestComposeTheDBSystem(t *testing.T) {
	nsg := obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"})

	t.Run("from the effective spec and the entry", func(t *testing.T) {
		objs, err := pgcluster.Generate(cluster(flexEntry), required(), observedComposed(nameNSG, nsg))
		if err != nil {
			t.Fatal(err)
		}
		db := objs[nameDBSystem]
		fp := field(t, db, "spec", "forProvider").(map[string]interface{})
		checks := map[string]interface{}{"dbVersion": "17", "shape": "PostgreSQL.VM.Standard.E5.Flex", "instanceOcpuCount": float64(2),
			"instanceMemorySizeInGbs": int64(32), "instanceCount": int64(1), "compartmentId": "ocid1.compartment.oc1..zone"}
		for k, want := range checks {
			if fp[k] != want {
				t.Errorf("%s: want %v, got %v", k, want, fp[k])
			}
		}
		net := first(t, db, "spec", "forProvider", "networkDetails")
		if net["subnetId"] != "ocid1.subnet.oc1..a" || net["nsgIds"].([]interface{})[0] != "ocid1.nsg.oc1..db" {
			t.Errorf("want the first database subnet by name and the cluster's security group, got %v", net)
		}
		backup := first(t, db, "spec", "forProvider", "managementPolicy")["backupPolicy"].([]interface{})[0].(map[string]interface{})
		if backup["kind"] != "DAILY" || backup["retentionDays"] != int64(7) || backup["backupStart"] != "02:00" {
			t.Errorf("want daily backups from 02:00 kept 7 days, the implementation's defaults, got %v", backup)
		}
		if v := field(t, db, "spec", "managementPolicies").([]interface{}); len(v) != 4 || v[3] != "Delete" {
			t.Errorf("want every management policy but LateInitialize, got %v", v)
		}
		if v, ok := field(t, db, "spec", "initProvider", "instancesDetails").([]interface{}); !ok || len(v) != 1 {
			t.Errorf("want one initProvider.instancesDetails entry per instance, got %v", v)
		}
		pw := first(t, db, "spec", "forProvider", "credentials")["passwordDetails"].([]interface{})[0].(map[string]interface{})
		if pw["passwordSecretRef"].(map[string]interface{})["name"] != objs[namePassword].GetName() {
			t.Errorf("want the password from the generated Secret, got %v", pw)
		}
	})

	t.Run("high availability, no backups and a maintenance window", func(t *testing.T) {
		c := cluster(`{"instanceType":"PostgreSQL.VM.Standard.E5.Flex","ocpus":2,"memoryGB":32,"maintenanceWindow":{"day":"Sun","hourUTC":3},"backupStartHourUTC":2}`)
		c.Spec.HighAvailability = ptr(true)
		c.Spec.Backup = &v1alpha2.Backup{RetentionDays: ptr(int64(0))}
		objs, err := pgcluster.Generate(c, required(), observedComposed(nameNSG, nsg))
		if err != nil {
			t.Fatal(err)
		}
		fp := field(t, objs[nameDBSystem], "spec", "forProvider").(map[string]interface{})
		policy := fp["managementPolicy"].([]interface{})[0].(map[string]interface{})
		if fp["instanceCount"] != int64(2) || policy["maintenanceWindowStart"] != "SUN 03:00" ||
			policy["backupPolicy"].([]interface{})[0].(map[string]interface{})["kind"] != "NONE" {
			t.Errorf("want 2 instances, a Sunday window and no backups, got %v", fp)
		}
	})

	t.Run("storage is regional unless the deployment names an availability domain", func(t *testing.T) {
		storage := func(req map[string][]resource.Required) map[string]interface{} {
			objs, err := pgcluster.Generate(cluster(flexEntry), req, observedComposed(nameNSG, nsg))
			if err != nil {
				t.Fatal(err)
			}
			return first(t, objs[nameDBSystem], "spec", "forProvider", "storageDetails")
		}
		if s := storage(required()); s["isRegionallyDurable"] != true || s["availabilityDomain"] != nil {
			t.Errorf("want regional storage, got %v", s)
		}
		req := required()
		env := req[base.EnvironmentKey][0].Resource.Object["data"].(map[string]interface{})["oci"].(map[string]interface{})
		env["availabilityDomain"] = "wAtM:eu-frankfurt-2-ad-1"
		if s := storage(req); s["isRegionallyDurable"] != false || s["availabilityDomain"] != "wAtM:eu-frankfurt-2-ad-1" {
			t.Errorf("want storage in the named availability domain, got %v", s)
		}
	})

	t.Run("a flexible shape without OCPUs and memory is refused", func(t *testing.T) {
		_, err := pgcluster.Generate(cluster(`{"instanceType":"PostgreSQL.VM.Standard.E5.Flex"}`), required(), nil)
		if err == nil || !strings.Contains(err.Error(), "needs ocpus and memoryGB") {
			t.Fatalf("want the shape refused, got %v", err)
		}
	})

	t.Run("an entry without a shape is refused", func(t *testing.T) {
		_, err := pgcluster.Generate(cluster(`{}`), required(), nil)
		if err == nil || !strings.Contains(err.Error(), "sets instanceType") {
			t.Fatalf("want the missing shape named, got %v", err)
		}
	})
}

func TestInstanceCountChanges(t *testing.T) {
	nsg := obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"})
	compose := func(ha bool, atProvider map[string]interface{}) []interface{} {
		t.Helper()
		c := cluster(flexEntry)
		c.Spec.HighAvailability = ptr(ha)
		observed := observedComposed(nameNSG, nsg)
		for k, v := range observedComposed(nameDBSystem, obj(psqlAPIVersion, "PsqlDbSystem", "x", nil, atProvider)) {
			observed[k] = v
		}
		objs, err := pgcluster.Generate(c, required(), observed)
		if err != nil {
			t.Fatal(err)
		}
		ops, _, _ := unstructured.NestedSlice(objs[nameDBSystem].(*unstructured.Unstructured).Object, "spec", "forProvider", "patchOperations")
		return ops
	}
	two := map[string]interface{}{"instanceCount": float64(2), "instances": []interface{}{
		map[string]interface{}{"id": "ocid1.first", "timeCreated": "2026-11-25T08:00:00Z"},
		map[string]interface{}{"id": "ocid1.replica", "timeCreated": "2026-11-25T09:00:00Z"},
	}}

	t.Run("adding an instance inserts one", func(t *testing.T) {
		ops := compose(true, map[string]interface{}{"instanceCount": float64(1)})
		if len(ops) != 1 || ops[0].(map[string]interface{})["operation"] != "INSERT" ||
			ops[0].(map[string]interface{})["selection"] != "instances" {
			t.Errorf("want one INSERT into instances, got %v", ops)
		}
	})
	t.Run("removing an instance removes the newest", func(t *testing.T) {
		ops := compose(false, two)
		if len(ops) != 1 || ops[0].(map[string]interface{})["selection"] != "instances[?id == 'ocid1.replica']" {
			t.Errorf("want the newest instance removed, got %v", ops)
		}
	})
	t.Run("no patch once OCI reports the wanted count", func(t *testing.T) {
		if ops := compose(true, two); len(ops) != 0 {
			t.Errorf("want no patch operations, got %v", ops)
		}
	})
}

func TestAdminSecret(t *testing.T) {
	password := "Kept-password-1"
	observed := observedComposed(nameNSG, obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"}))
	pwSecret := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Secret",
		"data": map[string]interface{}{passwordKey: base64.StdEncoding.EncodeToString([]byte(password))}}}
	observed[resource.Name(namePassword)] = resource.ObservedComposed{Resource: &composed.Unstructured{Unstructured: *pwSecret}}
	observed[resource.Name(nameDBSystem)] = resource.ObservedComposed{Resource: &composed.Unstructured{Unstructured: *obj(psqlAPIVersion, "PsqlDbSystem", "x", nil,
		map[string]interface{}{"networkDetails": []interface{}{map[string]interface{}{"primaryDbEndpointPrivateIp": "10.0.3.7"}}})}}

	objs, err := pgcluster.Generate(cluster(flexEntry), required(), observed)
	if err != nil {
		t.Fatal(err)
	}
	if got := field(t, objs[namePassword], "stringData", passwordKey); got != password {
		t.Errorf("want the generated password kept, got %v", got)
	}
	s := objs[nameSecret]
	if s.GetName() != "orders-db-dbadmin" {
		t.Errorf("want the admin Secret orders-db-dbadmin, got %s", s.GetName())
	}
	data := field(t, s, "stringData").(map[string]interface{})
	if data["endpoint"] != "10.0.3.7" || data["port"] != "5432" || data["username"] != adminUsername || data["password"] != password {
		t.Errorf("want endpoint, port, username and password, got %v", data)
	}
}

// The framework copies every composed object as JSON-compatible values, which panics on Go types
// such as int, so each object must survive a deep copy.
func TestComposedObjectsAreJSONCompatible(t *testing.T) {
	observed := observedComposed(nameNSG, obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"}))
	observed[resource.Name(nameDBSystem)] = resource.ObservedComposed{Resource: &composed.Unstructured{Unstructured: *obj(psqlAPIVersion, "PsqlDbSystem", "x", nil,
		map[string]interface{}{"networkDetails": []interface{}{map[string]interface{}{"primaryDbEndpointPrivateIp": "10.0.3.7"}}})}}
	c := cluster(`{"instanceType":"PostgreSQL.VM.Standard.E5.Flex","ocpus":2,"memoryGB":32,"iops":75000,"pointInTimeDays":7,"backupStartHourUTC":2,"maintenanceWindow":{"day":"Sun","hourUTC":3}}`)
	objs, err := pgcluster.Generate(c, required(), observed)
	if err != nil {
		t.Fatal(err)
	}
	for name, o := range objs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("%s is not JSON-compatible: %v", name, r)
				}
			}()
			if _, err := base.ToUnstructured(o); err != nil {
				t.Errorf("%s cannot be converted: %v", name, err)
			}
			o.(*unstructured.Unstructured).DeepCopy()
		}()
	}
}

func TestGeneratedPassword(t *testing.T) {
	p, err := generatePassword()
	if err != nil {
		t.Fatal(err)
	}
	classes := []string{"ABCDEFGHJKLMNPQRSTUVWXYZ", "abcdefghijkmnopqrstuvwxyz", "23456789", "-_#"}
	for _, set := range classes {
		if !strings.ContainsAny(p, set) {
			t.Errorf("password %q lacks one of %q", p, set)
		}
	}
	if len(p) != 24 {
		t.Errorf("want 24 characters, got %d", len(p))
	}
}

func TestEnvironmentAndNetwork(t *testing.T) {
	t.Run("missing OCI settings are named", func(t *testing.T) {
		env := Environment{}
		err := env.Validate()
		if err == nil || !strings.Contains(err.Error(), "oci.clientSecurityGroups") || !strings.Contains(err.Error(), "oci.providerConfig") {
			t.Fatalf("want the missing settings named, got %v", err)
		}
	})

	t.Run("a cluster waits until the observers report their OCIDs", func(t *testing.T) {
		r := required()
		r[keyVCN] = []resource.Required{{Resource: obj(observerNetworking, "Vcn", "vcn", nil, map[string]interface{}{})}}
		_, err := pgcluster.Generate(cluster(flexEntry), r, nil)
		if err == nil || !strings.Contains(err.Error(), "VCN vcn not found or not observed yet") {
			t.Fatalf("want the missing VCN named, got %v", err)
		}
	})

	t.Run("the backend asks for the observers by name and label", func(t *testing.T) {
		sel := Backend{}.Required(pgcluster.Input{Required: required()})
		if sel[keyCluster].GetMatchName() != "oke" || sel[keySubnets].GetMatchLabels().GetLabels()[subnetTypeLabel] != "database" ||
			sel[keyClientNSG+"pod"].GetMatchName() != "pod" {
			t.Errorf("unexpected selectors %v", sel)
		}
	})
}

func TestStatus(t *testing.T) {
	db := composed.New()
	db.SetUnstructuredContent(obj(psqlAPIVersion, "PsqlDbSystem", "x", nil, map[string]interface{}{
		"id": "ocid1.postgresqldbsystem.oc1..x", "state": "ACTIVE", "dbVersion": "17.4", "shape": "PostgreSQL.VM.Standard.E5.Flex",
		"networkDetails":   []interface{}{map[string]interface{}{"primaryDbEndpointPrivateIp": "10.0.3.7"}},
		"managementPolicy": []interface{}{map[string]interface{}{"backupPolicy": []interface{}{map[string]interface{}{"retentionDays": float64(7)}}}},
	}).Object)
	o, err := Backend{}.Status(db)
	if err != nil || o == nil {
		t.Fatalf("want status, got %v, %v", o, err)
	}
	if o.Version != "17.4" || o.Host != "10.0.3.7" || o.Port != port || o.RetentionDays != 7 ||
		o.Provider["state"] != "ACTIVE" || o.Provider["instanceType"] != "PostgreSQL.VM.Standard.E5.Flex" {
		t.Errorf("unexpected status %+v", o)
	}
	other := composed.New()
	other.SetAPIVersion("v1")
	other.SetKind("Secret")
	if o, _ := (Backend{}).Status(other); o != nil {
		t.Error("want no status from a resource that is not the DB system")
	}
}
