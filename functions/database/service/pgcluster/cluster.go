package pgcluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha2"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// EnvironmentName is the deployment's settings for the database kinds.
	EnvironmentName = "platform-apis-database"
	// refKeyPrefix keys the separate provider configuration object an entry names by ref.
	refKeyPrefix = "ProviderConfig"
	// secretKey keys the Secret the cluster's secretName names, if it exists.
	secretKey = "ConnectionSecret"

	SourceObject         = "object"
	SourceImplementation = "implementation"
	// SourceCloud marks a value an existing resource keeps, as its status shows it.
	SourceCloud = "cloud"
)

// cloudFields are the common fields an existing resource keeps its own value for when the cluster
// leaves them unset.
var cloudFields = []string{"version", "storage.sizeGB", "highAvailability", "backup.retentionDays"}

// Environment is the part of the deployment's settings a PostgreSQLCluster reads: the backends the
// deployment installs, each as "<Kind>.<group>" of its provider configuration kind.
type Environment struct {
	PostgreSQLBackends []string `json:"postgresqlBackends"`
}

// Validate refuses a deployment that installs no PostgreSQL backend.
func (e *Environment) Validate() error {
	if len(e.PostgreSQLBackends) == 0 {
		return errors.New("postgresqlBackends is required: the deployment installs no PostgreSQL backend")
	}
	return nil
}

func getEnvironment(required map[string][]resource.Required) (Environment, error) {
	var env Environment
	err := base.GetEnvironment(base.EnvironmentKey, required, &env)
	return env, err
}

// entryKind names an entry the way backends are named: "<Kind>.<group>", whatever its version.
func entryKind(e v1alpha2.ProviderConfigEntry) string {
	group, _, _ := strings.Cut(e.APIVersion, "/")
	return e.Kind + "." + group
}

func entryName(e v1alpha2.ProviderConfigEntry) string {
	return e.APIVersion + ", " + e.Kind
}

// selection is the entry a deployment applies, the backend it names, and the entries ignored.
type selection struct {
	backend Backend
	entry   v1alpha2.ProviderConfigEntry
	ignored []string
}

// selectEntry picks the one entry whose kind the deployment installs. A cluster with no such entry,
// or with entries for two installed backends, is refused with the kinds named.
func selectEntry(c *v1alpha2.PostgreSQLCluster, env Environment) (selection, error) {
	installed := map[string]bool{}
	for _, k := range env.PostgreSQLBackends {
		installed[k] = true
	}
	var matches []v1alpha2.ProviderConfigEntry
	var sel selection
	for _, e := range c.Spec.ProviderConfigs {
		if installed[entryKind(e)] {
			matches = append(matches, e)
		} else {
			sel.ignored = append(sel.ignored, entryName(e))
		}
	}
	switch len(matches) {
	case 0:
		return sel, fmt.Errorf("no provider configuration entry for a backend this deployment installs: %s",
			strings.Join(env.PostgreSQLBackends, ", "))
	case 1:
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, entryKind(m))
		}
		return sel, fmt.Errorf("entries for two backends this deployment installs: %s; a cluster with several needs a class to choose",
			strings.Join(names, ", "))
	}
	b, ok := lookup(entryKind(matches[0]))
	if !ok {
		return sel, fmt.Errorf("backend %s is installed but this function does not implement it", entryKind(matches[0]))
	}
	sel.backend = b
	sel.entry = matches[0]
	return sel, nil
}

// isObserved reports whether the cluster observes an existing resource.
func isObserved(c *v1alpha2.PostgreSQLCluster) bool {
	return c.Spec.Observe != nil && *c.Spec.Observe
}

// effectiveSpec puts the implementation's defaults beneath the cluster's own values and records
// where each value came from. A value without a default that the cluster leaves unset is refused.
// A cluster on an existing resource, named by id, takes no defaults for the fields the resource
// has: one it leaves unset keeps the resource's value, recorded as the cloud's. An observed cluster
// sets none of them.
func effectiveSpec(c *v1alpha2.PostgreSQLCluster, d Defaults, id string) (Spec, map[string]v1alpha2.Effective, error) {
	s := c.Spec
	existing := id != ""
	var missing []string
	if !existing && (s.Version == nil || *s.Version == "") {
		missing = append(missing, "version")
	}
	if !existing && (s.Storage == nil || s.Storage.SizeGB == nil) {
		missing = append(missing, "storage.sizeGB")
	}
	if len(missing) > 0 {
		return Spec{}, nil, fmt.Errorf("a cluster without a class sets %s", strings.Join(missing, " and "))
	}

	sources := map[string]v1alpha2.Effective{}
	put := func(field string, value interface{}, source string) {
		raw, _ := json.Marshal(value)
		sources[field] = v1alpha2.Effective{Value: runtime.RawExtension{Raw: raw}, Source: source}
	}
	out := Spec{
		Parameters: s.Parameters,
		SecretName: connectionSecretName(c),
		Cloud:      map[string]bool{},
	}
	if isObserved(c) {
		for _, f := range cloudFields {
			out.Cloud[f] = true
		}
		return out, sources, nil
	}
	boolOf := func(field string, v *bool, def bool) bool {
		switch {
		case v != nil:
			put(field, *v, SourceObject)
			return *v
		case existing && field != "deletionProtection":
			out.Cloud[field] = true
			return false
		}
		put(field, def, SourceImplementation)
		return def
	}

	if s.Version != nil && *s.Version != "" {
		out.Version = *s.Version
		put("version", out.Version, SourceObject)
	} else {
		out.Cloud["version"] = true
	}
	if s.Storage != nil && s.Storage.SizeGB != nil {
		out.SizeGB = *s.Storage.SizeGB
		put("storage.sizeGB", out.SizeGB, SourceObject)
	} else {
		out.Cloud["storage.sizeGB"] = true
	}
	out.HighAvailability = boolOf("highAvailability", s.HighAvailability, d.HighAvailability)
	out.DeletionProtection = boolOf("deletionProtection", s.DeletionProtection, d.DeletionProtection)
	switch {
	case s.Backup != nil && s.Backup.RetentionDays != nil:
		out.RetentionDays = *s.Backup.RetentionDays
		put("backup.retentionDays", out.RetentionDays, SourceObject)
	case existing:
		out.Cloud["backup.retentionDays"] = true
	default:
		out.RetentionDays = d.RetentionDays
		put("backup.retentionDays", out.RetentionDays, SourceImplementation)
	}
	return out, sources, nil
}

// connectionSecretName is the Secret the cluster's connection details go to.
func connectionSecretName(c *v1alpha2.PostgreSQLCluster) string {
	if c.Spec.SecretName != nil && *c.Spec.SecretName != "" {
		return *c.Spec.SecretName
	}
	return base.GenerateEligibleKubernetesFullName(c.Name + "-dbadmin")
}

// checkSecret refuses a secretName that names a Secret something else owns. Crossplane would take
// such a Secret over, write the connection details into it, and delete it with the cluster.
func checkSecret(c *v1alpha2.PostgreSQLCluster, required map[string][]resource.Required) error {
	for _, r := range required[secretKey] {
		if r.Resource == nil {
			continue
		}
		for _, o := range r.Resource.GetOwnerReferences() {
			if o.Controller != nil && *o.Controller && o.UID == c.UID {
				return nil
			}
		}
		return fmt.Errorf("Secret %s already exists and does not belong to this cluster: set another secretName", r.Resource.GetName())
	}
	return nil
}

// entryID returns the id of the existing resource an entry names, which every backend's provider
// configuration kind holds under the same key.
func entryID(e Entry) (string, error) {
	var v struct {
		ID string `json:"id"`
	}
	if len(e.Spec) == 0 {
		return "", nil
	}
	if err := json.Unmarshal(e.Spec, &v); err != nil {
		return "", fmt.Errorf("cannot read the %s entry: %w", e.Kind, err)
	}
	return v.ID, nil
}

// cloudSources records the values an existing resource keeps, as the cluster's status shows them.
func cloudSources(c *v1alpha2.PostgreSQLCluster, cloud map[string]bool, sources map[string]v1alpha2.Effective) {
	st := c.Status
	values := map[string]interface{}{}
	if st.Version != "" {
		values["version"] = st.Version
	}
	if st.Storage != nil && st.Storage.SizeGB > 0 {
		values["storage.sizeGB"] = st.Storage.SizeGB
	}
	if st.HighAvailability != nil {
		values["highAvailability"] = *st.HighAvailability
	}
	if st.Backup != nil {
		values["backup.retentionDays"] = st.Backup.RetentionDays
	}
	for _, f := range cloudFields {
		if !cloud[f] {
			continue
		}
		raw, _ := json.Marshal(values[f])
		sources[f] = v1alpha2.Effective{Value: runtime.RawExtension{Raw: raw}, Source: SourceCloud}
	}
}

// resolveEntry returns the entry's settings: its inline spec, or the spec of the separate object
// its ref names, delivered as a required resource.
func resolveEntry(e v1alpha2.ProviderConfigEntry, required map[string][]resource.Required) (Entry, error) {
	out := Entry{APIVersion: e.APIVersion, Kind: e.Kind}
	switch {
	case e.Spec != nil && e.Ref != nil:
		return out, fmt.Errorf("entry %s holds either spec or ref, not both", entryName(e))
	case e.Spec != nil:
		out.Spec = e.Spec.Raw
	case e.Ref != nil:
		objs := required[refKeyPrefix]
		if len(objs) == 0 {
			return out, fmt.Errorf("%s %s named by the entry's ref not found", e.Kind, e.Ref.Name)
		}
		spec, ok := objs[0].Resource.Object["spec"]
		if !ok {
			spec = map[string]interface{}{}
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return out, fmt.Errorf("cannot read %s %s: %w", e.Kind, e.Ref.Name, err)
		}
		out.Spec = raw
	default:
		out.Spec = json.RawMessage("{}")
	}
	return out, nil
}

// Required names the resources a cluster needs: the deployment's settings first, then the object
// its entry's ref names, then what its backend asks for.
func Required(xr client.Object, required map[string][]resource.Required) (map[string]*fnv1.ResourceSelector, error) {
	out := map[string]*fnv1.ResourceSelector{base.EnvironmentKey: base.RequiredEnvironmentConfig(EnvironmentName)}
	if _, ok := required[base.EnvironmentKey]; !ok {
		return out, nil
	}
	c, ok := xr.(*v1alpha2.PostgreSQLCluster)
	if !ok {
		return nil, fmt.Errorf("expected a PostgreSQLCluster, got %T", xr)
	}
	ns := c.Namespace
	out[secretKey] = &fnv1.ResourceSelector{
		ApiVersion: "v1",
		Kind:       "Secret",
		Match:      &fnv1.ResourceSelector_MatchName{MatchName: connectionSecretName(c)},
		Namespace:  &ns,
	}
	env, err := getEnvironment(required)
	if err != nil {
		return nil, err
	}
	sel, err := selectEntry(c, env)
	if err != nil {
		return nil, err
	}
	if sel.entry.Ref != nil {
		out[refKeyPrefix] = &fnv1.ResourceSelector{
			ApiVersion: sel.entry.APIVersion,
			Kind:       sel.entry.Kind,
			Match:      &fnv1.ResourceSelector_MatchName{MatchName: sel.entry.Ref.Name},
			Namespace:  &ns,
		}
	}
	in, err := input(c, sel, required, nil)
	if err != nil {
		// The entry's object or the backend's resources may not be delivered yet.
		return out, nil
	}
	for k, v := range sel.backend.Required(in) {
		out[k] = v
	}
	return out, nil
}

func input(c *v1alpha2.PostgreSQLCluster, sel selection, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (Input, error) {
	entry, err := resolveEntry(sel.entry, required)
	if err != nil {
		return Input{}, err
	}
	id, err := entryID(entry)
	if err != nil {
		return Input{}, err
	}
	if isObserved(c) && id == "" {
		return Input{}, fmt.Errorf("observe needs the id of the resource to observe in the %s entry", entry.Kind)
	}
	spec, _, err := effectiveSpec(c, sel.backend.Defaults(), id)
	if err != nil {
		return Input{}, err
	}
	return Input{Cluster: c, Spec: spec, Entry: entry, Required: required, Observed: observed,
		ID: id, Observe: isObserved(c), Zone: base.GetTenancyZone(required, logging.NewNopLogger()),
		Workspace: base.Workspace()}, nil
}

// Generate returns the resources that run the cluster on the deployment's backend.
func Generate(c *v1alpha2.PostgreSQLCluster, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	env, err := getEnvironment(required)
	if err != nil {
		return nil, err
	}
	if err := checkSecret(c, required); err != nil {
		return nil, err
	}
	sel, err := selectEntry(c, env)
	if err != nil {
		return nil, err
	}
	in, err := input(c, sel, required, observed)
	if err != nil {
		return nil, err
	}
	return sel.backend.Compose(in)
}

// CompositeStatus is the status that follows from the cluster itself: which entry applied, where
// each effective value came from, and the connection secret's name.
func CompositeStatus(c *v1alpha2.PostgreSQLCluster, required map[string][]resource.Required) (map[string]interface{}, error) {
	env, err := getEnvironment(required)
	if err != nil {
		return nil, err
	}
	sel, err := selectEntry(c, env)
	if err != nil {
		// The cluster is refused; Generate reports why.
		return nil, nil
	}
	entry, err := resolveEntry(sel.entry, required)
	if err != nil {
		return nil, nil
	}
	id, err := entryID(entry)
	if err != nil {
		return nil, nil
	}
	spec, sources, err := effectiveSpec(c, sel.backend.Defaults(), id)
	if err != nil {
		return nil, nil
	}
	cloudSources(c, spec.Cloud, sources)
	for field, v := range sel.backend.Effective(entry, spec, c.Status) {
		source := v.Source
		if source == SourceObject {
			source = SourceObject + "/" + entry.Kind
		}
		raw, _ := json.Marshal(v.Value)
		sources[field] = v1alpha2.Effective{Value: runtime.RawExtension{Raw: raw}, Source: source}
	}
	status := v1alpha2.PostgreSQLClusterStatus{
		ConnectionSecret: spec.SecretName,
		ProviderConfig:   &v1alpha2.ProviderConfigStatus{Applied: entryName(sel.entry), Ignored: sel.ignored},
		Effective:        sources,
	}
	out, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ObservedStatus is the status a backend reads from one resource it composed: the cluster's actual
// values, its address and the backend's own values.
func ObservedStatus(observed *composed.Unstructured) (map[string]interface{}, error) {
	for _, b := range registered() {
		o, err := b.Status(observed)
		if err != nil {
			return nil, err
		}
		if o == nil {
			continue
		}
		status := v1alpha2.PostgreSQLClusterStatus{Version: o.Version}
		if o.SizeGB > 0 {
			status.Storage = &v1alpha2.StorageStatus{SizeGB: o.SizeGB}
		}
		if o.RetentionDays > 0 {
			status.Backup = &v1alpha2.BackupStatus{RetentionDays: o.RetentionDays}
		}
		status.HighAvailability = o.HighAvailability
		if o.Host != "" {
			status.Endpoint = &v1alpha2.Endpoint{Host: o.Host, Port: o.Port}
		}
		out, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
		if err != nil {
			return nil, err
		}
		if o.Provider != nil {
			out["provider"] = o.Provider
		}
		// A value the cluster no longer has, such as backups turned off, is removed from the status.
		// The endpoint is kept while the DB system reports none, as the admin Secret keeps it.
		for _, k := range []string{"storage", "backup"} {
			if _, ok := out[k]; !ok {
				out[k] = nil
			}
		}
		return out, nil
	}
	return nil, nil
}

// rejectedPrefixes begin the Synced message of a managed resource whose cloud call failed, as
// crossplane-runtime words it. A resource waiting for a reference is not rejected.
var rejectedPrefixes = []string{"create failed", "update failed", "delete failed", "observe failed"}

// Conditions reports CloudAccepted: false with the cloud's message while any managed resource the
// cluster composed fails its cloud call, and true otherwise.
func Conditions(observed map[resource.Name]resource.ObservedComposed) []base.Condition {
	var rejected []string
	for name, o := range observed {
		if _, managed, _ := unstructured.NestedMap(o.Resource.Object, "spec", "forProvider"); !managed {
			continue
		}
		conditions, _, _ := unstructured.NestedSlice(o.Resource.Object, "status", "conditions")
		for _, raw := range conditions {
			c, _ := raw.(map[string]interface{})
			msg, _ := c["message"].(string)
			if c["type"] != "Synced" || c["status"] != "False" {
				continue
			}
			for _, p := range rejectedPrefixes {
				if strings.HasPrefix(msg, p) {
					rejected = append(rejected, fmt.Sprintf("%s %s: %s", o.Resource.GetKind(), name, msg))
					break
				}
			}
		}
	}
	if len(rejected) == 0 {
		return []base.Condition{{Type: base.CloudAcceptedCondition, Status: true, Reason: "Accepted"}}
	}
	sort.Strings(rejected)
	return []base.Condition{{Type: base.CloudAcceptedCondition, Status: false, Reason: "Rejected",
		Message: strings.Join(rejected, "; ")}}
}
