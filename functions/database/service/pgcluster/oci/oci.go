// Package oci runs a PostgreSQLCluster on OCI Database with PostgreSQL, through the namespaced
// managed resources of crossplane-provider-oci v2. Its objects are unstructured, as the zone
// function builds its OCI objects, so the function needs no OCI Go module.
package oci

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/service/pgcluster"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// Kind names this backend's provider configuration kind.
	Kind = "PostgreSQLOCIConfig.oci.database.entigo.com"

	psqlAPIVersion       = "psql.oci.m.upbound.io/v1alpha1"
	networkingAPIVersion = "networking.oci.m.upbound.io/v1alpha1"
	// The infralib observers are cluster-scoped managed resources.
	observerNetworking = "networking.oci.upbound.io/v1alpha1"
	observerContainer  = "containerengine.oci.upbound.io/v1alpha1"

	keyCluster      = "OCICluster"
	keyVCN          = "OCIVCN"
	keySubnets      = "OCIDatabaseSubnets"
	keyClientNSG    = "OCIClientNSG-"
	subnetTypeLabel = "subnet-type"

	nameDBSystem  = "db-system"
	nameNSG       = "security-group"
	nameRule      = "security-rule-"
	namePassword  = "admin-password"
	nameSecret    = "admin-secret"
	adminUsername = "dbadmin"
	port          = int64(5432)
	passwordKey   = "password"
	passwordType  = "PLAIN_TEXT"
)

// Environment is the OCI part of the deployment's database settings: the provider configuration and
// the names of the infralib observers the cluster's network comes from.
type Environment struct {
	OCI struct {
		ProviderConfig       string   `json:"providerConfig"`
		Cluster              string   `json:"cluster"`
		VCN                  string   `json:"vcn"`
		DatabaseSubnetType   string   `json:"databaseSubnetType"`
		ClientSecurityGroups []string `json:"clientSecurityGroups"`
	} `json:"oci"`
}

// Validate refuses settings the backend cannot run without.
func (e *Environment) Validate() error {
	var missing []string
	for name, v := range map[string]string{"oci.providerConfig": e.OCI.ProviderConfig, "oci.cluster": e.OCI.Cluster,
		"oci.vcn": e.OCI.VCN, "oci.databaseSubnetType": e.OCI.DatabaseSubnetType} {
		if v == "" {
			missing = append(missing, name)
		}
	}
	if len(e.OCI.ClientSecurityGroups) == 0 {
		missing = append(missing, "oci.clientSecurityGroups")
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("the OCI PostgreSQL backend needs %s", strings.Join(missing, ", "))
	}
	return nil
}

// Config is the cluster's PostgreSQLOCIConfig entry.
type Config struct {
	ID                 string             `json:"id,omitempty"`
	InstanceType       string             `json:"instanceType,omitempty"`
	OCPUs              *float64           `json:"ocpus,omitempty"`
	MemoryGB           *int64             `json:"memoryGB,omitempty"`
	FromBackup         string             `json:"fromBackup,omitempty"`
	IOPS               *int64             `json:"iops,omitempty"`
	MaintenanceWindow  *MaintenanceWindow `json:"maintenanceWindow,omitempty"`
	PointInTimeDays    *int64             `json:"pointInTimeDays,omitempty"`
	BackupStartHourUTC *int64             `json:"backupStartHourUTC,omitempty"`
}

// MaintenanceWindow is the weekly hour, in UTC, in which OCI applies updates.
type MaintenanceWindow struct {
	Day     string `json:"day"`
	HourUTC int64  `json:"hourUTC"`
}

// Backend runs clusters on OCI Database with PostgreSQL.
type Backend struct{}

func init() { pgcluster.Register(Backend{}) }

// Kind names the backend's provider configuration kind.
func (Backend) Kind() string { return Kind }

// Defaults are the implementation's values for unset common fields.
func (Backend) Defaults() pgcluster.Defaults {
	return pgcluster.Defaults{HighAvailability: false, DeletionProtection: true, RetentionDays: 7}
}

func environment(required map[string][]resource.Required) (Environment, error) {
	var env Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

// Required names the infralib observers the cluster's compartment and network come from.
func (Backend) Required(in pgcluster.Input) map[string]*fnv1.ResourceSelector {
	env, err := environment(in.Required)
	if err != nil {
		return nil
	}
	out := map[string]*fnv1.ResourceSelector{
		keyCluster: byName(observerContainer, "Cluster", env.OCI.Cluster),
		keyVCN:     byName(observerNetworking, "Vcn", env.OCI.VCN),
		keySubnets: {ApiVersion: observerNetworking, Kind: "Subnet", Match: &fnv1.ResourceSelector_MatchLabels{
			MatchLabels: &fnv1.MatchLabels{Labels: map[string]string{subnetTypeLabel: env.OCI.DatabaseSubnetType}}}},
	}
	for _, nsg := range env.OCI.ClientSecurityGroups {
		out[keyClientNSG+nsg] = byName(observerNetworking, "NetworkSecurityGroup", nsg)
	}
	return out
}

func byName(apiVersion, kind, name string) *fnv1.ResourceSelector {
	return &fnv1.ResourceSelector{ApiVersion: apiVersion, Kind: kind, Match: &fnv1.ResourceSelector_MatchName{MatchName: name}}
}

func parseConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &c); err != nil {
			return c, fmt.Errorf("cannot read the PostgreSQLOCIConfig entry: %w", err)
		}
	}
	if c.InstanceType == "" {
		return c, errors.New("the PostgreSQLOCIConfig entry sets instanceType, the DB system shape")
	}
	if strings.HasSuffix(c.InstanceType, ".Flex") && (c.OCPUs == nil || c.MemoryGB == nil) {
		return c, fmt.Errorf("the flexible shape %s needs ocpus and memoryGB in the PostgreSQLOCIConfig entry", c.InstanceType)
	}
	return c, nil
}

// network is what the composed resources need from the deployment's existing OCI resources.
type network struct {
	compartmentID string
	vcnID         string
	subnetID      string
	clientNSGIDs  []string
}

func observedID(r *unstructured.Unstructured) string {
	id, _, _ := unstructured.NestedString(r.Object, "status", "atProvider", "id")
	return id
}

func readNetwork(env Environment, required map[string][]resource.Required) (network, error) {
	var n network
	first := func(key string) *unstructured.Unstructured {
		if rs := required[key]; len(rs) > 0 {
			return rs[0].Resource
		}
		return nil
	}
	cluster := first(keyCluster)
	if cluster == nil {
		return n, fmt.Errorf("OKE cluster %s not found", env.OCI.Cluster)
	}
	n.compartmentID, _, _ = unstructured.NestedString(cluster.Object, "status", "atProvider", "compartmentId")
	if vcn := first(keyVCN); vcn != nil {
		n.vcnID = observedID(vcn)
	}
	// Several database subnets are taken in name order, so the choice is stable.
	subnets := required[keySubnets]
	sort.Slice(subnets, func(i, j int) bool { return subnets[i].Resource.GetName() < subnets[j].Resource.GetName() })
	for _, s := range subnets {
		if id := observedID(s.Resource); id != "" {
			n.subnetID = id
			break
		}
	}
	for _, name := range env.OCI.ClientSecurityGroups {
		if r := first(keyClientNSG + name); r != nil && observedID(r) != "" {
			n.clientNSGIDs = append(n.clientNSGIDs, observedID(r))
		}
	}
	switch {
	case n.compartmentID == "":
		return n, fmt.Errorf("OKE cluster %s reports no compartment yet", env.OCI.Cluster)
	case n.vcnID == "":
		return n, fmt.Errorf("VCN %s not found or not observed yet", env.OCI.VCN)
	case n.subnetID == "":
		return n, fmt.Errorf("no observed subnet labelled %s=%s", subnetTypeLabel, env.OCI.DatabaseSubnetType)
	case len(n.clientNSGIDs) == 0:
		return n, fmt.Errorf("none of the client security groups %s is observed yet", strings.Join(env.OCI.ClientSecurityGroups, ", "))
	}
	return n, nil
}

// Compose returns the cluster's security group and rules, the generated admin password, the DB
// system once its security group has an OCID, and the admin Secret once the DB system has an address.
func (Backend) Compose(in pgcluster.Input) (map[string]client.Object, error) {
	env, err := environment(in.Required)
	if err != nil {
		return nil, err
	}
	cfg, err := parseConfig(in.Entry.Spec)
	if err != nil {
		return nil, err
	}
	net, err := readNetwork(env, in.Required)
	if err != nil {
		return nil, err
	}
	c := in.Cluster
	hash := base.GenerateFNVHash(c.UID)
	resourceName := base.GenerateEligibleKubernetesName(c.Name+"-"+hash, 63)
	tags := map[string]interface{}{"entigo-resource": c.Namespace + "-" + c.Name}

	out := map[string]client.Object{}
	out[nameNSG] = managed(networkingAPIVersion, "NetworkSecurityGroup", resourceName, c.Namespace, env, map[string]interface{}{
		"compartmentId": net.compartmentID,
		"vcnId":         net.vcnID,
		"displayName":   resourceName,
		"freeformTags":  tags,
	})
	for i, src := range net.clientNSGIDs {
		out[fmt.Sprintf("%s%d", nameRule, i)] = managed(networkingAPIVersion, "NetworkSecurityGroupSecurityRule",
			fmt.Sprintf("%s-%d", resourceName, i), c.Namespace, env, map[string]interface{}{
				"networkSecurityGroupIdRef": map[string]interface{}{"name": resourceName},
				"direction":                 "INGRESS",
				"protocol":                  "6",
				"source":                    src,
				"sourceType":                "NETWORK_SECURITY_GROUP",
				"tcpOptions": []interface{}{map[string]interface{}{"destinationPortRange": []interface{}{
					map[string]interface{}{"min": port, "max": port}}}},
				"description": "PostgreSQL from the cluster's workloads",
			})
	}

	passwordSecret := base.GenerateEligibleKubernetesFullName(c.Name + "-admin-password-" + hash)
	password, err := currentPassword(in.Observed)
	if err != nil {
		return nil, err
	}
	out[namePassword] = secret(passwordSecret, c.Namespace, map[string]string{passwordKey: password, "type": passwordType})

	nsgID := ""
	if o, ok := in.Observed[resource.Name(nameNSG)]; ok {
		nsgID = observedID(&o.Resource.Unstructured)
	}
	if nsgID == "" {
		// The DB system names its security group by OCID, which exists once OCI has created it.
		return out, nil
	}
	out[nameDBSystem] = dbSystem(in, cfg, net, env, resourceName, nsgID, passwordSecret, tags)

	if o, ok := in.Observed[resource.Name(nameDBSystem)]; ok {
		if host := endpoint(&o.Resource.Unstructured); host != "" {
			out[nameSecret] = secret(in.Spec.SecretName, c.Namespace, map[string]string{
				"endpoint": host, "port": fmt.Sprint(port), "username": adminUsername, "password": password,
			})
		}
	}
	return out, nil
}

func dbSystem(in pgcluster.Input, cfg Config, net network, env Environment, name, nsgID, passwordSecret string, tags map[string]interface{}) client.Object {
	instances := int64(1)
	if in.Spec.HighAvailability {
		instances = 2
	}
	storage := map[string]interface{}{
		"systemType": "OCI_OPTIMIZED_STORAGE",
		// Regional storage needs no availability domain, so a single instance keeps its data
		// across availability domains too.
		"isRegionallyDurable": true,
	}
	if cfg.IOPS != nil {
		storage["iops"] = fmt.Sprint(*cfg.IOPS)
	}
	backup := map[string]interface{}{"kind": "DAILY", "retentionDays": in.Spec.RetentionDays}
	if in.Spec.RetentionDays == 0 {
		backup = map[string]interface{}{"kind": "NONE"}
	} else if cfg.BackupStartHourUTC != nil {
		backup["backupStart"] = fmt.Sprintf("%02d:00", *cfg.BackupStartHourUTC)
	}
	policy := map[string]interface{}{"backupPolicy": []interface{}{backup}}
	if cfg.MaintenanceWindow != nil {
		policy["maintenanceWindowStart"] = fmt.Sprintf("%s %02d:00", strings.ToUpper(cfg.MaintenanceWindow.Day), cfg.MaintenanceWindow.HourUTC)
	}
	if cfg.PointInTimeDays != nil && in.Spec.RetentionDays > 0 {
		policy["pitrPolicy"] = []interface{}{map[string]interface{}{"kind": "STANDARD", "restoreDays": *cfg.PointInTimeDays}}
	}
	nsgIDs := []interface{}{nsgID}
	forProvider := map[string]interface{}{
		"compartmentId":    net.compartmentID,
		"displayName":      name,
		"dbVersion":        in.Spec.Version,
		"shape":            cfg.InstanceType,
		"instanceCount":    instances,
		"storageDetails":   []interface{}{storage},
		"managementPolicy": []interface{}{policy},
		"networkDetails":   []interface{}{map[string]interface{}{"subnetId": net.subnetID, "nsgIds": nsgIDs}},
		"credentials": []interface{}{map[string]interface{}{
			"username": adminUsername,
			"passwordDetails": []interface{}{map[string]interface{}{
				"passwordSecretRef":     map[string]interface{}{"name": passwordSecret, "key": passwordKey},
				"passwordTypeSecretRef": map[string]interface{}{"name": passwordSecret, "key": "type"},
			}},
		}},
		"freeformTags": tags,
	}
	if cfg.OCPUs != nil {
		forProvider["instanceOcpuCount"] = *cfg.OCPUs
	}
	if cfg.MemoryGB != nil {
		forProvider["instanceMemorySizeInGbs"] = *cfg.MemoryGB
	}
	if cfg.FromBackup != "" {
		forProvider["source"] = []interface{}{map[string]interface{}{"sourceType": "BACKUP", "backupId": cfg.FromBackup}}
	}
	return managed(psqlAPIVersion, "PsqlDbSystem", name, in.Cluster.Namespace, env, forProvider)
}

func managed(apiVersion, kind, name, namespace string, env Environment, forProvider map[string]interface{}) client.Object {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec": map[string]interface{}{
			"forProvider":       forProvider,
			"providerConfigRef": map[string]interface{}{"kind": "ClusterProviderConfig", "name": env.OCI.ProviderConfig},
		},
	}}
}

func secret(name, namespace string, data map[string]string) client.Object {
	strData := map[string]interface{}{}
	for k, v := range data {
		strData[k] = v
	}
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"type":       "Opaque",
		"stringData": strData,
	}}
}

// currentPassword keeps the password the platform generated before, read from the observed Secret,
// and generates one for a new cluster.
func currentPassword(observed map[resource.Name]resource.ObservedComposed) (string, error) {
	if o, ok := observed[resource.Name(namePassword)]; ok {
		if v, found, _ := unstructured.NestedString(o.Resource.Object, "data", passwordKey); found && v != "" {
			raw, err := base64.StdEncoding.DecodeString(v)
			if err == nil && len(raw) > 0 {
				return string(raw), nil
			}
		}
	}
	return generatePassword()
}

// generatePassword returns 24 characters that meet OCI's rule for admin passwords: upper and lower
// case letters, a digit and a special character.
func generatePassword() (string, error) {
	const (
		upper   = "ABCDEFGHJKLMNPQRSTUVWXYZ"
		lower   = "abcdefghijkmnopqrstuvwxyz"
		digits  = "23456789"
		special = "-_#"
	)
	pick := func(set string) (byte, error) {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(set))))
		if err != nil {
			return 0, err
		}
		return set[n.Int64()], nil
	}
	out := make([]byte, 0, 24)
	for _, set := range []string{upper, lower, digits, special} {
		b, err := pick(set)
		if err != nil {
			return "", err
		}
		out = append(out, b)
	}
	all := upper + lower + digits
	for len(out) < 24 {
		b, err := pick(all)
		if err != nil {
			return "", err
		}
		out = append(out, b)
	}
	// Shuffle, so the required classes are not always first.
	for i := len(out) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return "", err
		}
		out[i], out[j.Int64()] = out[j.Int64()], out[i]
	}
	return string(out), nil
}

func endpoint(dbSystem *unstructured.Unstructured) string {
	details, _, _ := unstructured.NestedSlice(dbSystem.Object, "status", "atProvider", "networkDetails")
	if len(details) == 0 {
		return ""
	}
	m, _ := details[0].(map[string]interface{})
	ip, _ := m["primaryDbEndpointPrivateIp"].(string)
	return ip
}

// Status reads the cluster's actual values from its DB system.
func (Backend) Status(observed *composed.Unstructured) (*pgcluster.Observed, error) {
	if observed.GetKind() != "PsqlDbSystem" || observed.GetAPIVersion() != psqlAPIVersion {
		return nil, nil
	}
	at, _, _ := unstructured.NestedMap(observed.Object, "status", "atProvider")
	o := &pgcluster.Observed{Port: port, Provider: map[string]interface{}{}}
	o.Version, _ = at["dbVersion"].(string)
	o.Host = endpoint(&observed.Unstructured)
	if policies, ok := at["managementPolicy"].([]interface{}); ok && len(policies) > 0 {
		if p, ok := policies[0].(map[string]interface{}); ok {
			if backups, ok := p["backupPolicy"].([]interface{}); ok && len(backups) > 0 {
				if b, ok := backups[0].(map[string]interface{}); ok {
					if days, ok := b["retentionDays"].(float64); ok {
						o.RetentionDays = int64(days)
					} else if days, ok := b["retentionDays"].(int64); ok {
						o.RetentionDays = days
					}
				}
			}
		}
	}
	// status.provider, in the shape PostgreSQLOCIConfig's status defines, from the DB system's fields.
	for statusField, providerField := range map[string]string{"id": "id", "state": "state", "instanceType": "shape",
		"instanceOcpuCount": "instanceOcpuCount", "instanceMemorySizeInGbs": "instanceMemorySizeInGbs"} {
		if v, ok := at[providerField]; ok {
			o.Provider[statusField] = v
		}
	}
	// OCI reports no storage size, so status.storage stays empty.
	return o, nil
}
