package pgcluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha2"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// EnvironmentName is the deployment's settings for the database kinds.
	EnvironmentName = "platform-apis-database"
	// refKeyPrefix keys the separate provider configuration object an entry names by ref.
	refKeyPrefix = "ProviderConfig"

	SourceObject         = "object"
	SourceImplementation = "implementation"
)

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

// effectiveSpec puts the implementation's defaults beneath the cluster's own values and records
// where each value came from. A value without a default that the cluster leaves unset is refused.
func effectiveSpec(c *v1alpha2.PostgreSQLCluster, d Defaults) (Spec, map[string]v1alpha2.Effective, error) {
	s := c.Spec
	var missing []string
	if s.Version == nil || *s.Version == "" {
		missing = append(missing, "version")
	}
	if s.Storage == nil || s.Storage.SizeGB == nil {
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
	boolOf := func(field string, v *bool, def bool) bool {
		if v != nil {
			put(field, *v, SourceObject)
			return *v
		}
		put(field, def, SourceImplementation)
		return def
	}

	out := Spec{
		Version:    *s.Version,
		SizeGB:     *s.Storage.SizeGB,
		Parameters: s.Parameters,
		SecretName: base.GenerateEligibleKubernetesFullName(c.Name + "-dbadmin"),
	}
	put("version", out.Version, SourceObject)
	put("storage.sizeGB", out.SizeGB, SourceObject)
	out.HighAvailability = boolOf("highAvailability", s.HighAvailability, d.HighAvailability)
	out.DeletionProtection = boolOf("deletionProtection", s.DeletionProtection, d.DeletionProtection)
	if s.Backup != nil && s.Backup.RetentionDays != nil {
		out.RetentionDays = *s.Backup.RetentionDays
		put("backup.retentionDays", out.RetentionDays, SourceObject)
	} else {
		out.RetentionDays = d.RetentionDays
		put("backup.retentionDays", out.RetentionDays, SourceImplementation)
	}
	if s.SecretName != nil && *s.SecretName != "" {
		out.SecretName = *s.SecretName
	}
	return out, sources, nil
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
	env, err := getEnvironment(required)
	if err != nil {
		return nil, err
	}
	sel, err := selectEntry(c, env)
	if err != nil {
		return nil, err
	}
	if sel.entry.Ref != nil {
		ns := c.Namespace
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
	spec, _, err := effectiveSpec(c, sel.backend.Defaults())
	if err != nil {
		return Input{}, err
	}
	entry, err := resolveEntry(sel.entry, required)
	if err != nil {
		return Input{}, err
	}
	return Input{Cluster: c, Spec: spec, Entry: entry, Required: required, Observed: observed}, nil
}

// Generate returns the resources that run the cluster on the deployment's backend.
func Generate(c *v1alpha2.PostgreSQLCluster, required map[string][]resource.Required, observed map[resource.Name]resource.ObservedComposed) (map[string]client.Object, error) {
	env, err := getEnvironment(required)
	if err != nil {
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
	spec, sources, err := effectiveSpec(c, sel.backend.Defaults())
	if err != nil {
		return nil, nil
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
		return out, nil
	}
	return nil, nil
}
