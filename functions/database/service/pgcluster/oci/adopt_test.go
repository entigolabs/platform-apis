package oci

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha2"
	"github.com/entigolabs/platform-apis/service/pgcluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const existingID = "ocid1.postgresqldbsystem.oc1..existing"

// inZone is what a deployment in workspace "ws" delivers for a cluster in Zone "shop".
func inZone() map[string][]resource.Required {
	base.NewFunction(logging.NewNopLogger(), nil, "ws")
	r := required()
	r[base.NamespaceKey] = []resource.Required{{Resource: &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Namespace",
		"metadata": map[string]interface{}{"name": "shop", "labels": map[string]interface{}{base.TenancyZoneLabel: "shop"}},
	}}}}
	return r
}

func observedCluster(entry string) *v1alpha2.PostgreSQLCluster {
	c := cluster(entry)
	c.Spec.Observe = ptr(true)
	c.Spec.Version = nil
	c.Spec.Storage = nil
	return c
}

// existingDB is a DB system someone created outside the platform, as the provider reports it.
func existingDB(tags map[string]interface{}, policies ...interface{}) *unstructured.Unstructured {
	db := obj(psqlAPIVersion, "PsqlDbSystem", "x", nil, map[string]interface{}{
		"id": existingID, "dbVersion": "16", "instanceCount": int64(1), "adminUsername": "owner",
		"compartmentId": "ocid1.compartment.oc1..team", "displayName": "orders", "shape": "PostgreSQL.VM.Standard.E4.Flex",
		"instanceOcpuCount": float64(4), "instanceMemorySizeInGbs": float64(64),
		"storageDetails": []interface{}{map[string]interface{}{"systemType": "OCI_OPTIMIZED_STORAGE", "isRegionallyDurable": true}},
		"networkDetails": []interface{}{map[string]interface{}{"subnetId": "ocid1.subnet.oc1..team",
			"nsgIds": []interface{}{"ocid1.nsg.oc1..team"}, "primaryDbEndpointPrivateIp": "10.0.0.5"}},
		"managementPolicy": []interface{}{map[string]interface{}{
			"backupPolicy": []interface{}{map[string]interface{}{"kind": "DAILY", "retentionDays": float64(30), "backupStart": "04:00"}},
			"pitrPolicy":   []interface{}{map[string]interface{}{"kind": "STANDARD", "restoreDays": float64(30)}},
		}},
		"freeformTags": tags,
	})
	if len(policies) == 0 {
		policies = []interface{}{"Observe"}
	}
	db.Object["spec"] = map[string]interface{}{"forProvider": map[string]interface{}{}, "managementPolicies": policies}
	return db
}

func tagged(zone bool) map[string]interface{} {
	tags := map[string]interface{}{pgcluster.WorkspaceTag: "ws", "team": "orders"}
	if zone {
		tags[pgcluster.ZoneTag] = "shop"
	}
	return tags
}

func TestObserveAnExistingDBSystem(t *testing.T) {
	entry := `{"id":"` + existingID + `"}`

	t.Run("composes only the observe-only DB system", func(t *testing.T) {
		objs, err := pgcluster.Generate(observedCluster(entry), inZone(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(objs) != 1 || objs[nameDBSystem] == nil {
			t.Fatalf("want the DB system alone, got %v", objs)
		}
		db := objs[nameDBSystem]
		if got := field(t, db, "spec", "managementPolicies").([]interface{}); len(got) != 1 || got[0] != "Observe" {
			t.Errorf("want observe-only, got %v", got)
		}
		if db.GetAnnotations()[meta.AnnotationKeyExternalName] != existingID {
			t.Errorf("want the DB system its id names, got %v", db.GetAnnotations())
		}
		if len(field(t, db, "spec", "forProvider").(map[string]interface{})) != 0 || db.GetLabels()[pgcluster.DeletionProtectionLabel] != "" {
			t.Errorf("an observed DB system takes no settings and no deletion protection, got %v", db)
		}
	})

	t.Run("writes the admin Secret without a password", func(t *testing.T) {
		objs, err := pgcluster.Generate(observedCluster(entry), inZone(), observedComposed(nameDBSystem, existingDB(tagged(false))))
		if err != nil {
			t.Fatal(err)
		}
		data := field(t, objs[nameSecret+"orders-db-dbadmin"], "stringData").(map[string]interface{})
		if data["endpoint"] != "10.0.0.5" || data["username"] != "owner" || data["password"] != nil {
			t.Errorf("want the address and the owner's user, and no password, got %v", data)
		}
	})

	refused := map[string]struct {
		tags      map[string]interface{}
		workspace string
	}{
		"another workspace's DB system":    {map[string]interface{}{pgcluster.WorkspaceTag: "other"}, "ws"},
		"another Zone's DB system":         {map[string]interface{}{pgcluster.WorkspaceTag: "ws", pgcluster.ZoneTag: "other"}, "ws"},
		"an untagged DB system":            {map[string]interface{}{}, "ws"},
		"a deployment without a workspace": {tagged(true), ""},
	}
	for name, tc := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			r := inZone()
			base.NewFunction(logging.NewNopLogger(), nil, tc.workspace)
			if _, err := pgcluster.Generate(observedCluster(entry), r, observedComposed(nameDBSystem, existingDB(tc.tags))); err == nil {
				t.Error("want it refused")
			}
		})
	}

	t.Run("refuses settings in the entry", func(t *testing.T) {
		_, err := pgcluster.Generate(observedCluster(`{"id":"`+existingID+`","instanceType":"PostgreSQL.VM.Standard.E5.Flex","ocpus":2,"memoryGB":32}`), inZone(), nil)
		if err == nil || !strings.Contains(err.Error(), "only id") {
			t.Errorf("want it refused, got %v", err)
		}
	})

	t.Run("refuses observe without an id", func(t *testing.T) {
		if _, err := pgcluster.Generate(observedCluster(flexEntry), inZone(), nil); err == nil || !strings.Contains(err.Error(), "needs the id") {
			t.Errorf("want it refused, got %v", err)
		}
	})
}

func takenOver(entry string) *v1alpha2.PostgreSQLCluster {
	c := cluster(entry)
	c.Spec.Version = nil
	c.Spec.Storage = nil
	return c
}

func TestTakeOverAnObservedDBSystem(t *testing.T) {
	entry := `{"id":"` + existingID + `"}`
	nsg := obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"})
	observed := func(db *unstructured.Unstructured) map[resource.Name]resource.ObservedComposed {
		o := observedComposed(nameDBSystem, db)
		o[resource.Name(nameNSG)] = resource.ObservedComposed{Resource: &composed.Unstructured{Unstructured: *nsg}}
		return o
	}

	t.Run("keeps the DB system observed until its security group exists", func(t *testing.T) {
		objs, err := pgcluster.Generate(takenOver(entry), inZone(), observedComposed(nameDBSystem, existingDB(tagged(true))))
		if err != nil {
			t.Fatal(err)
		}
		if !observeOnly(objs[nameDBSystem].(*unstructured.Unstructured)) || objs[nameNSG] == nil {
			t.Errorf("want the DB system still observed and its security group composed, got %v", objs)
		}
		if p := field(t, objs[namePassword], "stringData", passwordKey).(string); len(p) != 24 {
			t.Error("want a new password generated: OCI never returns the one the DB system has")
		}
	})

	t.Run("keeps what the DB system has", func(t *testing.T) {
		objs, err := pgcluster.Generate(takenOver(entry), inZone(), observed(existingDB(tagged(true))))
		if err != nil {
			t.Fatal(err)
		}
		db := objs[nameDBSystem]
		if observeOnly(db.(*unstructured.Unstructured)) || db.GetAnnotations()[meta.AnnotationKeyExternalName] != existingID {
			t.Errorf("want the DB system its id names, managed, got %v", db)
		}
		fp := field(t, db, "spec", "forProvider").(map[string]interface{})
		for k, want := range map[string]interface{}{"compartmentId": "ocid1.compartment.oc1..team", "displayName": "orders",
			"dbVersion": "16", "instanceCount": int64(1), "shape": "PostgreSQL.VM.Standard.E4.Flex", "instanceOcpuCount": float64(4)} {
			if fp[k] != want {
				t.Errorf("%s: want %v, got %v", k, want, fp[k])
			}
		}
		if _, ok := fp["source"]; ok {
			t.Error("want no source on a DB system taken over")
		}
		net := first(t, db, "spec", "forProvider", "networkDetails")
		if net["subnetId"] != "ocid1.subnet.oc1..team" || len(net["nsgIds"].([]interface{})) != 2 ||
			net["nsgIds"].([]interface{})[0] != "ocid1.nsg.oc1..db" || net["nsgIds"].([]interface{})[1] != "ocid1.nsg.oc1..team" {
			t.Errorf("want its subnet, and the cluster's security group beside its own, got %v", net)
		}
		backup := first(t, db, "spec", "forProvider", "managementPolicy")["backupPolicy"].([]interface{})[0].(map[string]interface{})
		if backup["retentionDays"] != float64(30) || backup["backupStart"] != "04:00" {
			t.Errorf("want its own backups, got %v", backup)
		}
		if first(t, db, "spec", "forProvider", "credentials")["username"] != "owner" {
			t.Error("want the user it was created with")
		}
		tags := field(t, db, "spec", "forProvider", "freeformTags").(map[string]interface{})
		if tags["team"] != "orders" || tags["entigo-resource"] != "shop-orders-db" || tags[pgcluster.ZoneTag] != "shop" {
			t.Errorf("want its owner's tags kept beside the platform's, got %v", tags)
		}
		if db.GetLabels()[pgcluster.DeletionProtectionLabel] != "true" {
			t.Error("want the default deletion protection once it holds the cluster's data")
		}
		data := field(t, objs[nameSecret+"orders-db-dbadmin"], "stringData").(map[string]interface{})
		if data["username"] != "owner" || data["password"] == nil {
			t.Errorf("want the owner's user with the new password, got %v", data)
		}
	})

	t.Run("applies the fields the cluster sets", func(t *testing.T) {
		c := takenOver(entry)
		c.Spec.HighAvailability = ptr(true)
		c.Spec.Backup = &v1alpha2.Backup{RetentionDays: ptr(int64(7))}
		objs, err := pgcluster.Generate(c, inZone(), observed(existingDB(tagged(true))))
		if err != nil {
			t.Fatal(err)
		}
		if got := field(t, objs[nameDBSystem], "spec", "forProvider", "instanceCount"); got != int64(2) {
			t.Errorf("want a standby added, got %v", got)
		}
		backup := first(t, objs[nameDBSystem], "spec", "forProvider", "managementPolicy")["backupPolicy"].([]interface{})[0].(map[string]interface{})
		if backup["retentionDays"] != int64(7) {
			t.Errorf("want 7 days of backups, got %v", backup)
		}
	})

	t.Run("refuses a DB system without this Zone's tag", func(t *testing.T) {
		if _, err := pgcluster.Generate(takenOver(entry), inZone(), observed(existingDB(tagged(false)))); err == nil {
			t.Error("want it refused")
		}
	})

	t.Run("refuses another major version", func(t *testing.T) {
		c := takenOver(entry)
		c.Spec.Version = ptr("17")
		if _, err := pgcluster.Generate(c, inZone(), observed(existingDB(tagged(true)))); err == nil || !strings.Contains(err.Error(), "differs") {
			t.Errorf("want it refused, got %v", err)
		}
	})

	t.Run("refuses an id the cluster did not observe first", func(t *testing.T) {
		if _, err := pgcluster.Generate(takenOver(entry), inZone(), nil); err == nil || !strings.Contains(err.Error(), "observes first") {
			t.Errorf("want it refused, got %v", err)
		}
	})

	t.Run("refuses fromBackup with id", func(t *testing.T) {
		_, err := pgcluster.Generate(takenOver(`{"id":"`+existingID+`","fromBackup":"ocid1.psqlbackup.oc1..b"}`), inZone(), observed(existingDB(tagged(true))))
		if err == nil || !strings.Contains(err.Error(), "fromBackup") {
			t.Errorf("want it refused, got %v", err)
		}
	})
}

// composedDB is a DB system the function created, as it composed it.
func composedDB(t *testing.T, entry string) *unstructured.Unstructured {
	t.Helper()
	nsg := obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"})
	objs, err := pgcluster.Generate(cluster(entry), required(), observedComposed(nameNSG, nsg))
	if err != nil {
		t.Fatal(err)
	}
	db := objs[nameDBSystem].(*unstructured.Unstructured).DeepCopy()
	db.Object["status"] = map[string]interface{}{"atProvider": map[string]interface{}{"id": "ocid1.postgresqldbsystem.oc1..db", "instanceCount": int64(1)}}
	return db
}

func TestExistingDBSystemKeepsItsSourceAndPlacement(t *testing.T) {
	nsg := obj(networkingAPIVersion, "NetworkSecurityGroup", "x", nil, map[string]interface{}{"id": "ocid1.nsg.oc1..db"})
	password := &unstructured.Unstructured{Object: map[string]interface{}{"apiVersion": "v1", "kind": "Secret",
		"data": map[string]interface{}{passwordKey: base64.StdEncoding.EncodeToString([]byte("Kept-pass-1"))}}}
	observed := func(db *unstructured.Unstructured) map[resource.Name]resource.ObservedComposed {
		o := observedComposed(nameDBSystem, db)
		o[resource.Name(nameNSG)] = resource.ObservedComposed{Resource: &composed.Unstructured{Unstructured: *nsg}}
		o[resource.Name(namePassword)] = resource.ObservedComposed{Resource: &composed.Unstructured{Unstructured: *password}}
		return o
	}

	t.Run("a changed fromBackup", func(t *testing.T) {
		db := composedDB(t, flexEntry)
		entry := `{"instanceType":"PostgreSQL.VM.Standard.E5.Flex","ocpus":2,"memoryGB":32,"fromBackup":"ocid1.psqlbackup.oc1..b"}`
		objs, err := pgcluster.Generate(cluster(entry), required(), observed(db))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := field(t, objs[nameDBSystem], "spec", "forProvider").(map[string]interface{})["source"]; ok {
			t.Error("want the DB system kept without a source: it was created empty")
		}
	})

	t.Run("a new subnet and availability domains", func(t *testing.T) {
		db := composedDB(t, flexEntry)
		r := required()
		r[keySubnets] = append(r[keySubnets], resource.Required{Resource: obj(observerNetworking, "Subnet", "database-00",
			map[string]interface{}{subnetTypeLabel: "database"}, map[string]interface{}{"id": "ocid1.subnet.oc1..new"})})
		env := r[base.EnvironmentKey][0].Resource.Object["data"].(map[string]interface{})["oci"].(map[string]interface{})
		env["availabilityDomains"] = []interface{}{"AD-1"}
		objs, err := pgcluster.Generate(cluster(flexEntry), r, observed(db))
		if err != nil {
			t.Fatal(err)
		}
		if got := first(t, objs[nameDBSystem], "spec", "forProvider", "networkDetails")["subnetId"]; got != "ocid1.subnet.oc1..a" {
			t.Errorf("want the subnet it was created in, got %v", got)
		}
		storage := first(t, objs[nameDBSystem], "spec", "forProvider", "storageDetails")
		if storage["isRegionallyDurable"] != true || storage["availabilityDomain"] != nil {
			t.Errorf("want its storage where it was created, got %v", storage)
		}
	})
}
