// Package pgcluster composes a PostgreSQLCluster on the backend its deployment installs. It holds
// what every backend shares: choosing the provider configuration entry, putting the
// implementation's defaults beneath the cluster's own values, and the status that follows from
// them. Each backend registers itself and composes its own resources.
package pgcluster

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha2"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Backend runs a PostgreSQLCluster on one implementation, such as OCI Database with PostgreSQL.
type Backend interface {
	// Kind names the backend's provider configuration kind as "<Kind>.<group>", such as
	// "PostgreSQLOCIConfig.oci.database.entigo.com".
	Kind() string
	// Defaults are the implementation's values for the common fields a cluster leaves unset.
	Defaults() Defaults
	// Required names the resources the backend needs to compose the cluster, keyed as the
	// function's required resources are.
	Required(in Input) map[string]*fnv1.ResourceSelector
	// Compose returns the resources that run the cluster, keyed by their composition name.
	Compose(in Input) (map[string]client.Object, error)
	// Status reads the cluster's actual values from one observed resource, or returns nil when
	// that resource holds none.
	Status(observed *composed.Unstructured) (*Observed, error)
}

// Defaults are an implementation's fixed values for the common fields a cluster may leave unset.
// Fields without a default, such as the version and the disk size, have no place here: a cluster
// that leaves them unset is refused.
type Defaults struct {
	HighAvailability   bool
	DeletionProtection bool
	RetentionDays      int64
}

// Spec is the cluster's effective spec: its own values over the implementation's defaults.
type Spec struct {
	Version            string
	SizeGB             int64
	HighAvailability   bool
	DeletionProtection bool
	RetentionDays      int64
	Parameters         map[string]string
	SecretName         string
	// Cloud names the common fields a cluster on an existing resource leaves unset, as
	// "version" or "backup.retentionDays": the resource keeps its own value for them.
	Cloud map[string]bool
}

// DeletionProtectionLabel marks a composed resource that holds the cluster's data while deletion
// protection is on. Admission refuses to delete it, Crossplane included when the function stops
// composing it; only the garbage collector may, removing a cluster deleted just after its
// protection was switched off.
const DeletionProtectionLabel = "database.entigo.com/deletion-protection"

// Entry is the provider configuration entry the deployment applies, with its settings resolved
// from the inline spec or the referenced object.
type Entry struct {
	APIVersion string
	Kind       string
	Spec       json.RawMessage
}

// Input is everything a backend composes from.
type Input struct {
	Cluster  *v1alpha2.PostgreSQLCluster
	Spec     Spec
	Entry    Entry
	Required map[string][]resource.Required
	Observed map[resource.Name]resource.ObservedComposed
	// ID is the existing resource the entry names, observed while Observe is set and owned once
	// observe is removed; "" for a resource the platform creates.
	ID      string
	Observe bool
	// Zone and Workspace are where the cluster runs, which an existing resource's tags must name.
	Zone      string
	Workspace string
}

// The tags that name the workspace and the Zone a cloud resource belongs to.
const (
	WorkspaceTag = base.TenancyWorkspaceAWSTag
	ZoneTag      = base.TenancyZoneAWSTag
)

// Tags returns the workspace and Zone tags the platform puts on every cloud resource it creates.
func (in Input) Tags() map[string]interface{} {
	tags := map[string]interface{}{}
	if in.Workspace != "" {
		tags[WorkspaceTag] = in.Workspace
	}
	if in.Zone != "" {
		tags[ZoneTag] = in.Zone
	}
	return tags
}

// CheckTags refuses an existing resource whose tags do not let this cluster use it. The workspace
// tag must name this workspace. A Zone tag must name the cluster's Zone; a resource without one may
// be observed from any Zone, but taking it over needs one.
func CheckTags(in Input, tags map[string]interface{}, takeover bool) error {
	if in.Workspace == "" {
		return errors.New("this deployment sets no workspace, so an existing resource's workspace tag cannot be checked")
	}
	if got, _ := tags[WorkspaceTag].(string); got != in.Workspace {
		return fmt.Errorf("resource %s has %s %q, not this workspace %q", in.ID, WorkspaceTag, got, in.Workspace)
	}
	zone, tagged := tags[ZoneTag].(string)
	switch {
	case tagged && zone != in.Zone:
		return fmt.Errorf("resource %s has %s %q, not this cluster's Zone %q", in.ID, ZoneTag, zone, in.Zone)
	case takeover && !tagged:
		return fmt.Errorf("taking over resource %s needs its %s tag to name Zone %q", in.ID, ZoneTag, in.Zone)
	}
	return nil
}

// Observed is the part of the status a backend reads from the resources it composed.
type Observed struct {
	Version       string
	SizeGB        int64
	RetentionDays int64
	// HighAvailability is whether the cluster runs with a standby; nil when the backend cannot tell.
	HighAvailability *bool
	Host             string
	Port             int64
	// Provider holds the values only this backend has, in the shape its provider configuration
	// kind's status defines.
	Provider map[string]interface{}
}

var (
	mu       sync.RWMutex
	backends = map[string]Backend{}
)

// Register makes a backend available to deployments that install it. A backend package calls it
// from its init function.
func Register(b Backend) {
	mu.Lock()
	defer mu.Unlock()
	if _, ok := backends[b.Kind()]; ok {
		panic(fmt.Sprintf("PostgreSQL backend %s registered twice", b.Kind()))
	}
	backends[b.Kind()] = b
}

func lookup(kind string) (Backend, bool) {
	mu.RLock()
	defer mu.RUnlock()
	b, ok := backends[kind]
	return b, ok
}

func registered() []Backend {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Backend, 0, len(backends))
	for _, b := range backends {
		out = append(out, b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind() < out[j].Kind() })
	return out
}
