package pgcluster

import (
	"encoding/json"
	"strings"
	"testing"

	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/entigolabs/function-base/base"
	"github.com/entigolabs/platform-apis/apis/v1alpha2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	standInKind = "PostgreSQLStandInConfig.test.database.entigo.com"
	standInAPI  = "test.database.entigo.com/v1alpha1"
	otherAPI    = "other.database.entigo.com/v1alpha1"
)

// standIn is a backend with no cloud behind it: it composes one ConfigMap holding what it received,
// so the tests can see the effective spec and the entry a real backend would get.
type standIn struct{}

func (standIn) Kind() string { return standInKind }

func (standIn) Defaults() Defaults {
	return Defaults{HighAvailability: false, DeletionProtection: true, RetentionDays: 7}
}

func (standIn) Required(Input) map[string]*fnv1.ResourceSelector { return nil }

func (standIn) Compose(in Input) (map[string]client.Object, error) {
	spec, _ := json.Marshal(in.Spec)
	return map[string]client.Object{"cluster": &corev1.ConfigMap{
		TypeMeta:   metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{Name: in.Cluster.Name, Namespace: in.Cluster.Namespace},
		Data:       map[string]string{"spec": string(spec), "entry": string(in.Entry.Spec)},
	}}, nil
}

func (standIn) Status(observed *composed.Unstructured) (*Observed, error) {
	if observed.GetKind() != "ConfigMap" {
		return nil, nil
	}
	ha := true
	return &Observed{Version: "17.4", SizeGB: 25, RetentionDays: 7, HighAvailability: &ha, Host: "db.example", Port: 5432,
		Provider: map[string]interface{}{"state": "running"}}, nil
}

func init() { Register(standIn{}) }

func environment(backends ...string) map[string][]resource.Required {
	list := make([]interface{}, len(backends))
	for i, b := range backends {
		list[i] = b
	}
	return map[string][]resource.Required{base.EnvironmentKey: {{Resource: &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": base.EnvironmentApiVersion, "kind": base.EnvironmentKind,
		"metadata": map[string]interface{}{"name": EnvironmentName},
		"data":     map[string]interface{}{"postgresqlBackends": list},
	}}}}}
}

func ptr[T any](v T) *T { return &v }

func entry(api, kind, spec string) v1alpha2.ProviderConfigEntry {
	return v1alpha2.ProviderConfigEntry{APIVersion: api, Kind: kind, Spec: &runtime.RawExtension{Raw: []byte(spec)}}
}

func cluster(entries ...v1alpha2.ProviderConfigEntry) *v1alpha2.PostgreSQLCluster {
	return &v1alpha2.PostgreSQLCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "orders-db", Namespace: "shop"},
		Spec: v1alpha2.PostgreSQLClusterSpec{
			Version:         ptr("17"),
			Storage:         &v1alpha2.Storage{SizeGB: ptr(int64(20))},
			ProviderConfigs: entries,
		},
	}
}

func TestSelectEntry(t *testing.T) {
	standInEntry := entry(standInAPI, "PostgreSQLStandInConfig", `{"size":"small"}`)
	otherEntry := entry(otherAPI, "PostgreSQLOtherConfig", `{}`)
	cases := map[string]struct {
		cluster   *v1alpha2.PostgreSQLCluster
		installed []string
		wantErr   string
		ignored   []string
	}{
		"the installed backend's entry applies, the others are ignored": {
			cluster: cluster(otherEntry, standInEntry), installed: []string{standInKind},
			ignored: []string{otherAPI + ", PostgreSQLOtherConfig"},
		},
		"an entry of another version of the installed kind applies": {
			cluster:   cluster(entry("test.database.entigo.com/v1beta1", "PostgreSQLStandInConfig", `{}`)),
			installed: []string{standInKind},
		},
		"no entry for an installed backend is refused": {
			cluster: cluster(otherEntry), installed: []string{standInKind},
			wantErr: "no provider configuration entry for a backend this deployment installs: " + standInKind,
		},
		"entries for two installed backends are refused": {
			cluster: cluster(otherEntry, standInEntry), installed: []string{standInKind, "PostgreSQLOtherConfig.other.database.entigo.com"},
			wantErr: "entries for two backends this deployment installs",
		},
		"an installed backend this function lacks is refused": {
			cluster: cluster(otherEntry), installed: []string{"PostgreSQLOtherConfig.other.database.entigo.com"},
			wantErr: "does not implement it",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			sel, err := selectEntry(tc.cluster, Environment{PostgreSQLBackends: tc.installed})
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if sel.backend.Kind() != standInKind {
				t.Errorf("want backend %s, got %s", standInKind, sel.backend.Kind())
			}
			if strings.Join(sel.ignored, "|") != strings.Join(tc.ignored, "|") {
				t.Errorf("want ignored %v, got %v", tc.ignored, sel.ignored)
			}
		})
	}
}

func TestEffectiveSpec(t *testing.T) {
	d := standIn{}.Defaults()

	t.Run("unset fields take the implementation's defaults", func(t *testing.T) {
		spec, sources, err := effectiveSpec(cluster(), d)
		if err != nil {
			t.Fatal(err)
		}
		if spec.HighAvailability || !spec.DeletionProtection || spec.RetentionDays != 7 {
			t.Errorf("want defaults false, true, 7; got %+v", spec)
		}
		for _, f := range []string{"highAvailability", "deletionProtection", "backup.retentionDays"} {
			if sources[f].Source != SourceImplementation {
				t.Errorf("want %s from the implementation, got %q", f, sources[f].Source)
			}
		}
		if sources["version"].Source != SourceObject || spec.SecretName != "orders-db-dbadmin" {
			t.Errorf("want version from the object and the default secret name, got %q and %q", sources["version"].Source, spec.SecretName)
		}
	})

	t.Run("the cluster's own values win over the defaults", func(t *testing.T) {
		c := cluster()
		c.Spec.HighAvailability = ptr(true)
		c.Spec.DeletionProtection = ptr(false)
		c.Spec.Backup = &v1alpha2.Backup{RetentionDays: ptr(int64(0))}
		c.Spec.SecretName = ptr("orders-login")
		spec, sources, err := effectiveSpec(c, d)
		if err != nil {
			t.Fatal(err)
		}
		if !spec.HighAvailability || spec.DeletionProtection || spec.RetentionDays != 0 || spec.SecretName != "orders-login" {
			t.Errorf("want the cluster's values, got %+v", spec)
		}
		if sources["backup.retentionDays"].Source != SourceObject {
			t.Errorf("want backup.retentionDays from the object, got %q", sources["backup.retentionDays"].Source)
		}
	})

	t.Run("a cluster without its version and size is refused, naming both", func(t *testing.T) {
		c := cluster()
		c.Spec.Version = nil
		c.Spec.Storage = nil
		_, _, err := effectiveSpec(c, d)
		if err == nil || !strings.Contains(err.Error(), "version and storage.sizeGB") {
			t.Fatalf("want both values named, got %v", err)
		}
	})
}

func TestGenerate(t *testing.T) {
	t.Run("the backend composes from the effective spec and the inline entry", func(t *testing.T) {
		c := cluster(entry(standInAPI, "PostgreSQLStandInConfig", `{"size":"small"}`))
		objs, err := Generate(c, environment(standInKind), nil)
		if err != nil {
			t.Fatal(err)
		}
		cm := objs["cluster"].(*corev1.ConfigMap)
		if cm.Data["entry"] != `{"size":"small"}` || !strings.Contains(cm.Data["spec"], `"RetentionDays":7`) {
			t.Errorf("unexpected composition: %v", cm.Data)
		}
	})

	t.Run("an entry by ref takes the named object's spec", func(t *testing.T) {
		c := cluster(v1alpha2.ProviderConfigEntry{APIVersion: standInAPI, Kind: "PostgreSQLStandInConfig",
			Ref: &v1alpha2.LocalReference{Name: "orders-size"}})
		required := environment(standInKind)
		selectors, err := Required(c, required)
		if err != nil {
			t.Fatal(err)
		}
		if sel := selectors[refKeyPrefix]; sel == nil || sel.GetMatchName() != "orders-size" || sel.GetNamespace() != "shop" {
			t.Fatalf("want the referenced object required by name in the cluster's namespace, got %v", sel)
		}
		required[refKeyPrefix] = []resource.Required{{Resource: &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": standInAPI, "kind": "PostgreSQLStandInConfig",
			"metadata": map[string]interface{}{"name": "orders-size", "namespace": "shop"},
			"spec":     map[string]interface{}{"size": "large"},
		}}}}
		objs, err := Generate(c, required, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got := objs["cluster"].(*corev1.ConfigMap).Data["entry"]; got != `{"size":"large"}` {
			t.Errorf("want the referenced spec, got %s", got)
		}
	})

	t.Run("without the deployment's settings only they are required", func(t *testing.T) {
		selectors, err := Required(cluster(), map[string][]resource.Required{})
		if err != nil {
			t.Fatal(err)
		}
		if len(selectors) != 1 || selectors[base.EnvironmentKey] == nil {
			t.Errorf("want only the environment config, got %v", selectors)
		}
	})
}

func TestStatus(t *testing.T) {
	t.Run("the composite's status names the applied entry and each value's source", func(t *testing.T) {
		c := cluster(entry(otherAPI, "PostgreSQLOtherConfig", `{}`), entry(standInAPI, "PostgreSQLStandInConfig", `{}`))
		status, err := CompositeStatus(c, environment(standInKind))
		if err != nil {
			t.Fatal(err)
		}
		pc := status["providerConfig"].(map[string]interface{})
		if pc["applied"] != standInAPI+", PostgreSQLStandInConfig" {
			t.Errorf("unexpected applied entry %v", pc["applied"])
		}
		eff := status["effective"].(map[string]interface{})
		if eff["deletionProtection"].(map[string]interface{})["source"] != SourceImplementation {
			t.Errorf("want deletionProtection from the implementation, got %v", eff["deletionProtection"])
		}
		if status["connectionSecret"] != "orders-db-dbadmin" {
			t.Errorf("unexpected connection secret %v", status["connectionSecret"])
		}
	})

	t.Run("a refused cluster adds no status", func(t *testing.T) {
		status, err := CompositeStatus(cluster(), environment(standInKind))
		if err != nil || status != nil {
			t.Errorf("want no status, got %v, %v", status, err)
		}
	})

	t.Run("the backend's observed values become the actual status", func(t *testing.T) {
		cm := composed.New()
		cm.SetAPIVersion("v1")
		cm.SetKind("ConfigMap")
		status, err := ObservedStatus(cm)
		if err != nil {
			t.Fatal(err)
		}
		if status["version"] != "17.4" || status["highAvailability"] != true || status["endpoint"].(map[string]interface{})["host"] != "db.example" ||
			status["provider"].(map[string]interface{})["state"] != "running" {
			t.Errorf("unexpected status %v", status)
		}
	})

	t.Run("a resource no backend knows adds no status", func(t *testing.T) {
		other := composed.New()
		other.SetAPIVersion("v1")
		other.SetKind("Secret")
		if status, err := ObservedStatus(other); err != nil || status != nil {
			t.Errorf("want no status, got %v, %v", status, err)
		}
	})
}
