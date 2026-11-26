package test

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/entigolabs/static-common/crossplane"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	env             = "../examples/environment-config.yaml"
	required        = "../examples/required-resources.yaml"
	composition     = "../apis/cluster-composition.yaml"
	clusterResource = "../examples/cluster.yaml"
	function        = "../../../functions/database"
	functionsConfig = "../../../test/common/functions-dev.yaml"

	psqlAPIVersion       = "psql.oci.m.upbound.io/v1alpha1"
	networkingAPIVersion = "networking.oci.m.upbound.io/v1alpha1"
)

// TestPostgreSQLClusterCrossplaneRender renders a PostgreSQLCluster on OCI through its lifecycle:
// before the security group exists, once it does, and when high availability is switched on.
func TestPostgreSQLClusterCrossplaneRender(t *testing.T) {
	crossplane.StartCustomFunction(t, function, "9443")

	tmpDir := t.TempDir()
	extra := filepath.Join(tmpDir, "extra.yaml")
	crossplane.AppendYamlToResources(t, env, extra)
	crossplane.AppendYamlToResources(t, required, extra)

	xr := func(t *testing.T, changes map[string]interface{}) string {
		t.Helper()
		f := filepath.Join(t.TempDir(), "cluster.yaml")
		changes["metadata.uid"] = "000000000000"
		cluster := crossplane.ParseYamlFileToUnstructured(t, clusterResource)
		crossplane.AppendToResources(t, f, crossplane.MockByKind(t, cluster, "PostgreSQLCluster", "database.entigo.com/v1alpha2", false, changes))
		return f
	}
	render := func(t *testing.T, xrFile, extraFile string, observed ...*unstructured.Unstructured) []*unstructured.Unstructured {
		t.Helper()
		if len(observed) == 0 {
			return crossplane.CrossplaneRender(t, xrFile, composition, functionsConfig, crossplane.Ptr(extraFile), nil)
		}
		f := filepath.Join(t.TempDir(), "observed.yaml")
		crossplane.AppendToResources(t, f, observed...)
		return crossplane.CrossplaneRender(t, xrFile, composition, functionsConfig, crossplane.Ptr(extraFile), crossplane.Ptr(f))
	}

	created := render(t, xr(t, map[string]interface{}{}), extra)
	nsg := crossplane.MockByKind(t, created, "NetworkSecurityGroup", networkingAPIVersion, true,
		map[string]interface{}{"status.atProvider.id": "ocid1.networksecuritygroup.oc1..db"})

	t.Run("before the security group exists", func(t *testing.T) {
		crossplane.AssertResourceCount(t, created, "NetworkSecurityGroup", 1)
		crossplane.AssertResourceCount(t, created, "NetworkSecurityGroupSecurityRule", 2)
		crossplane.AssertResourceCount(t, created, "PsqlDbSystem", 0)
		crossplane.AssertFieldValues(t, created, "NetworkSecurityGroup", networkingAPIVersion, map[string]string{
			"spec.forProvider.compartmentId": "ocid1.compartment.oc1..example",
			"spec.forProvider.vcnId":         "ocid1.vcn.oc1..example",
		})
	})

	t.Run("the DB system once the security group exists", func(t *testing.T) {
		resources := render(t, xr(t, map[string]interface{}{}), extra, nsg)
		crossplane.AssertResourceCount(t, resources, "PsqlDbSystem", 1)
		crossplane.AssertFieldValues(t, resources, "PsqlDbSystem", psqlAPIVersion, map[string]string{
			"spec.forProvider.instanceCount":                                   "1",
			"spec.forProvider.dbVersion":                                       "17",
			"spec.forProvider.networkDetails.0.subnetId":                       "ocid1.subnet.oc1..database",
			"spec.forProvider.networkDetails.0.nsgIds.0":                       "ocid1.networksecuritygroup.oc1..db",
			"spec.forProvider.storageDetails.0.isRegionallyDurable":            "true",
			"spec.forProvider.managementPolicy.0.backupPolicy.0.kind":          "DAILY",
			"spec.forProvider.managementPolicy.0.backupPolicy.0.backupStart":   "02:00",
			"spec.forProvider.managementPolicy.0.backupPolicy.0.retentionDays": "7",
			"spec.initProvider.instancesDetails.#":                             "1",
			"spec.managementPolicies.#":                                        "4",
			"spec.providerConfigRef.name":                                      "crossplane-oracle",
		})
		for _, r := range resources {
			if r.GetKind() == "PsqlDbSystem" {
				policies, _, _ := unstructured.NestedStringSlice(r.Object, "spec", "managementPolicies")
				require.NotContains(t, policies, "LateInitialize", "the provider must not take over patchOperations")
				_, found, _ := unstructured.NestedFieldNoCopy(r.Object, "spec", "forProvider", "patchOperations")
				require.False(t, found, "a new DB system needs no patch operations")
			}
		}
	})

	t.Run("high availability on a running DB system inserts an instance", func(t *testing.T) {
		running := render(t, xr(t, map[string]interface{}{}), extra, nsg)
		db := crossplane.MockByKind(t, running, "PsqlDbSystem", psqlAPIVersion, true,
			map[string]interface{}{"status.atProvider.id": "ocid1.postgresqldbsystem.oc1..db", "status.atProvider.instanceCount": int64(1)})
		// A running cluster keeps its generated password in the password Secret.
		password := crossplane.MockByKind(t, running, "Secret", "v1", true,
			map[string]interface{}{"data.password": base64.StdEncoding.EncodeToString([]byte("Kept-password-1"))})
		resources := render(t, xr(t, map[string]interface{}{"spec.highAvailability": true}), extra, nsg, db, password)
		crossplane.AssertFieldValues(t, resources, "PsqlDbSystem", psqlAPIVersion, map[string]string{
			"spec.forProvider.instanceCount":               "2",
			"spec.forProvider.patchOperations.#":           "1",
			"spec.forProvider.patchOperations.0.operation": "INSERT",
			"spec.forProvider.patchOperations.0.selection": "instances",
		})
	})

	t.Run("storage stays in the only availability domain", func(t *testing.T) {
		single := filepath.Join(t.TempDir(), "extra.yaml")
		raw, err := os.ReadFile(extra)
		require.NoError(t, err)
		withAD := strings.Replace(string(raw), "    - pod\n", "    - pod\n    availabilityDomains:\n    - wAtM:eu-frankfurt-2-ad-1\n", 1)
		require.NotEqual(t, string(raw), withAD, "the environment config must list the client security groups")
		require.NoError(t, os.WriteFile(single, []byte(withAD), 0o600))
		resources := render(t, xr(t, map[string]interface{}{}), single, nsg)
		crossplane.AssertFieldValues(t, resources, "PsqlDbSystem", psqlAPIVersion, map[string]string{
			"spec.forProvider.storageDetails.0.isRegionallyDurable": "false",
			"spec.forProvider.storageDetails.0.availabilityDomain":  "wAtM:eu-frankfurt-2-ad-1",
		})
	})
}
